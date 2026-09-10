// Package splash renders the configurable startup splash screen as one
// self-contained HTML document, shared by the native WKWebView app (SetHtml
// before the server binds) and the browser UI (overlay injected into
// index.html). See docs/superpowers/specs/2026-09-09-splash-screen-design.md.
package splash

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"strings"

	"github.com/polds/tui-streamer/internal/bundle"
)

// Phase selects whether intro animations play ("intro") or the splash shows
// its resting frame ("final", used after a native handoff).
type Phase string

const (
	PhaseIntro Phase = "intro"
	PhaseFinal Phase = "final"
)

// Inputs are the per-render values that are not part of the bundle config.
type Inputs struct {
	Title   string
	Version string
	IconSVG []byte // app icon; nil → template glyph
	Phase   Phase  // "" → PhaseIntro
	// RemainingMS is how much of minDuration is still owed after a native
	// handoff. 0 for a fresh page.
	RemainingMS int
}

// Document is a rendered splash.
type Document struct {
	// HTML is the full standalone page.
	HTML string
	// Head holds <style> and <script> blocks to inject before </head>.
	Head string
	// Body is the <div id="splash" …>…</div> element.
	Body string
}

// Render produces the splash document for cfg. cfg may be the zero value.
func Render(cfg bundle.SplashConfig, in Inputs) (*Document, error) {
	cfg = cfg.WithDefaults()
	if in.Phase == "" {
		in.Phase = PhaseIntro
	}
	if in.RemainingMS < 0 {
		in.RemainingMS = 0
	}

	// Defence in depth: bundle.Parse already rejects an accent/background
	// that could break out of CSS, but a caller that builds a SplashConfig
	// by hand (e.g. server.Config assembled directly, not via a bundle
	// file) bypasses that check. Fall back to the default colours rather
	// than passing an unvalidated string into a <style> block or a style=""
	// attribute.
	defaults := bundle.DefaultSplash()
	if !bundle.ValidCSSColor(cfg.Accent) {
		cfg.Accent = defaults.Accent
	}
	if !bundle.ValidCSSColor(cfg.Background) {
		cfg.Background = defaults.Background
	}

	head, body, err := renderContent(cfg, in)
	if err != nil {
		return nil, err
	}

	config, err := json.Marshal(map[string]any{
		"style":         cfg.Style,
		"window":        cfg.Window,
		"phase":         in.Phase,
		"minDurationMs": cfg.MinDuration.Milliseconds(),
		"remainingMs":   in.RemainingMS,
		"background":    cfg.Background,
		"accent":        cfg.Accent,
	})
	if err != nil {
		return nil, fmt.Errorf("splash config: %w", err)
	}
	// "</" cannot appear inside a <script>; escape so a tagline cannot close it.
	configJS := strings.ReplaceAll(string(config), "</", "<\\/")

	var h strings.Builder
	h.WriteString("<style data-splash>")
	h.WriteString(baseCSS)
	h.WriteString("</style>\n")
	h.WriteString(head)
	h.WriteString("\n<script data-splash>window.SPLASH = ")
	h.WriteString(configJS)
	h.WriteString(";</script>\n<script data-splash>")
	h.WriteString(shimJS)
	h.WriteString("</script>")

	doc := &Document{Head: h.String(), Body: body}
	doc.HTML = "<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n" +
		"<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n" +
		"<title>" + html.EscapeString(in.Title) + "</title>\n" +
		"<style>html, body { margin: 0; height: 100%; background: " + cfg.Background + "; }</style>\n" +
		doc.Head + "\n</head>\n<body>\n" + doc.Body + "\n</body>\n</html>\n"
	return doc, nil
}

// renderContent dispatches to the custom page or a built-in template.
func renderContent(cfg bundle.SplashConfig, in Inputs) (head, body string, err error) {
	if cfg.HTML != "" {
		return renderCustom(cfg, in)
	}
	d := templateData{
		Title:      in.Title,
		Tagline:    cfg.Tagline,
		Version:    in.Version,
		Accent:     cssValue(cfg.Accent),
		Background: cssValue(cfg.Background),
		Phase:      in.Phase,
	}
	if len(in.IconSVG) > 0 {
		d.IconDataURI = iconDataURI(in.IconSVG)
		d.HasIcon = true
	}
	return renderBuiltin(cfg.Style, d)
}

func iconDataURI(svg []byte) template.URL {
	return template.URL("data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(svg))
}

// cssValue marks a bundle-validated colour as safe CSS. bundle.resolveSplash
// only admits [#a-zA-Z0-9(),.% -], which cannot terminate a declaration.
func cssValue(v string) template.CSS { return template.CSS(v) }
