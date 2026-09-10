package bundle

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// ── Splash configuration ───────────────────────────────────────────────────

// Splash style and window enums (metadata.splash.style / .window).
const (
	SplashStyleMinimal   = "minimal"
	SplashStyleArc       = "arc"
	SplashStyleDia       = "dia"
	SplashStyleJetBrains = "jetbrains"

	SplashWindowFull = "full"
	SplashWindowCard = "card"
)

// SplashConfig is the resolved metadata.splash block. Zero values are not
// meaningful; call WithDefaults (Parse already does) before use.
type SplashConfig struct {
	// Style selects a built-in template. Ignored when HTML is set.
	Style string
	// Window is "full" (splash inside the normal window) or "card"
	// (borderless centred card that grows into the main window).
	Window string
	// HTML is a custom splash page. Relative after Parse; Load resolves it to
	// an absolute path inside the bundle directory.
	HTML string
	// Tagline is shown under the title by built-in styles (HTML-escaped).
	Tagline string
	// Accent and Background are CSS colours passed to templates verbatim
	// (validated against colorPattern so they cannot break out of CSS).
	Accent     string
	Background string
	// MinDuration is how long the splash stays up at minimum.
	MinDuration time.Duration
	// Size is the card width/height in points (card window only).
	Size [2]int
	// minDurationSet records that minDuration was given explicitly (so 0 is honoured).
	minDurationSet bool
}

// splashYAML is the on-disk shape of metadata.splash.
type splashYAML struct {
	Style       string `yaml:"style"`
	Window      string `yaml:"window"`
	HTML        string `yaml:"html"`
	Tagline     string `yaml:"tagline"`
	Accent      string `yaml:"accent"`
	Background  string `yaml:"background"`
	MinDuration string `yaml:"minDuration"`
	Size        []int  `yaml:"size"`
}

var colorPattern = regexp.MustCompile(`^[#a-zA-Z0-9(),.% -]+$`)

// ValidCSSColor reports whether s is safe to substitute verbatim into a CSS
// declaration or a style="" attribute — i.e. it cannot close a <style> tag,
// a style attribute, or start a new declaration. Bundle parsing (Parse)
// already rejects an invalid accent/background at load time; this is
// exported so other callers that build a SplashConfig by hand (rather than
// through Parse), such as internal/splash.Render, can re-check it in depth.
func ValidCSSColor(s string) bool {
	return s != "" && colorPattern.MatchString(s)
}

// DefaultSplash is the configuration used when a bundle has no splash block.
func DefaultSplash() SplashConfig {
	return SplashConfig{
		Style:       SplashStyleMinimal,
		Window:      SplashWindowFull,
		Accent:      "#7aa2f7",
		Background:  "#1a1b26",
		MinDuration: 1200 * time.Millisecond,
		Size:        [2]int{640, 400},
	}
}

// WithDefaults fills empty fields from DefaultSplash and applies the
// "jetbrains implies card" rule when no window was chosen.
func (s SplashConfig) WithDefaults() SplashConfig {
	d := DefaultSplash()
	if s.Style == "" {
		s.Style = d.Style
	}
	if s.Window == "" {
		if s.Style == SplashStyleJetBrains {
			s.Window = SplashWindowCard
		} else {
			s.Window = d.Window
		}
	}
	if s.Accent == "" {
		s.Accent = d.Accent
	}
	if s.Background == "" {
		s.Background = d.Background
	}
	if s.MinDuration == 0 && !s.minDurationSet {
		s.MinDuration = d.MinDuration
	}
	if s.Size == [2]int{} {
		s.Size = d.Size
	}
	return s
}

// resolveSplash validates a decoded metadata.splash block and applies defaults.
// A nil block yields DefaultSplash().
func resolveSplash(y *splashYAML) (SplashConfig, error) {
	if y == nil {
		return DefaultSplash(), nil
	}
	s := SplashConfig{
		Style:      y.Style,
		Window:     y.Window,
		HTML:       y.HTML,
		Tagline:    y.Tagline,
		Accent:     y.Accent,
		Background: y.Background,
	}
	switch s.Style {
	case "", SplashStyleMinimal, SplashStyleArc, SplashStyleDia, SplashStyleJetBrains:
	default:
		return s, fmt.Errorf("splash style %q: want minimal, arc, dia or jetbrains", s.Style)
	}
	switch s.Window {
	case "", SplashWindowFull, SplashWindowCard:
	default:
		return s, fmt.Errorf("splash window %q: want full or card", s.Window)
	}
	for name, v := range map[string]string{"accent": s.Accent, "background": s.Background} {
		if v != "" && !colorPattern.MatchString(v) {
			return s, fmt.Errorf("splash %s %q: not a plain CSS colour", name, v)
		}
	}
	if y.MinDuration != "" {
		d, err := time.ParseDuration(y.MinDuration)
		if err != nil || d < 0 {
			return s, fmt.Errorf("splash minDuration %q: want a Go duration such as 1200ms", y.MinDuration)
		}
		s.MinDuration = d
		s.minDurationSet = true
	}
	if len(y.Size) > 0 {
		if len(y.Size) != 2 {
			return s, fmt.Errorf("splash size: want [width, height]")
		}
		for _, v := range y.Size {
			if v < 200 || v > 4000 {
				return s, fmt.Errorf("splash size %d: want 200..4000", v)
			}
		}
		s.Size = [2]int{y.Size[0], y.Size[1]}
	}
	return s.WithDefaults(), nil
}

// resolveSplashHTML turns a relative HTML path into an absolute one inside
// bundleDir and checks the file exists. No-op when HTML is empty.
func (s *SplashConfig) resolveSplashHTML(bundleDir string) error {
	if s.HTML == "" {
		return nil
	}
	p := s.HTML
	if !filepath.IsAbs(p) {
		p = filepath.Join(bundleDir, p)
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return fmt.Errorf("splash html %q: %w", s.HTML, err)
	}
	absDir, err := filepath.Abs(bundleDir)
	if err != nil {
		return fmt.Errorf("splash html: bundle dir: %w", err)
	}
	if !isWithin(absDir, abs) {
		return fmt.Errorf("splash html %q escapes the bundle directory", s.HTML)
	}
	if _, err := os.Stat(abs); err != nil {
		return fmt.Errorf("splash html %q: %w", s.HTML, err)
	}
	s.HTML = abs
	return nil
}
