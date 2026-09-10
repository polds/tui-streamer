# Splash Screen Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bundles can configure an animated splash screen (Arc/Dia full-window or JetBrains-style chromeless card) that renders identically in the native macOS `.app` and in browser mode and is dismissed only when its intro animation has finished **and** the UI is connected.

**Architecture:** A new `internal/splash` package renders one self-contained HTML document (built-in `html/template` styles or a custom `splash.html`, assets inlined, plus a small JS shim implementing the `animated / dismiss / dismissed` protocol). `cmd/app` shows it via `SetHtml` before the server binds and, in `card` mode, restyles the `NSWindow` borderless via cgo; `internal/server` injects the same document into `index.html` as a `#splash` overlay that `app.js` dismisses when connected.

**Tech Stack:** Go 1.22+ (`html/template`, `embed`), `webview/webview_go` + cgo/Cocoa (darwin only), vanilla JS/CSS, `gopkg.in/yaml.v3`, bash packaging script.

**Spec:** `docs/superpowers/specs/2026-09-09-splash-screen-design.md`

## Global Constraints

- No new Go module dependencies; no JS build step (vanilla JS only).
- Every Go file with cgo/Cocoa code carries `//go:build darwin` and lives in `cmd/app`.
- Go style: wrap errors `fmt.Errorf("context: %w", err)`, pointer receivers, `// ──────` section separators in long files.
- Schema enums: `style ∈ {minimal, arc, dia, jetbrains}`, `window ∈ {full, card}`; defaults `minimal`, `full`, `jetbrains → card`, `minDuration 1200ms`, `size 640×400`, `size` values in `[200, 4000]`.
- Colours must match `^[#a-zA-Z0-9(),.% -]+$`. `html` must resolve inside the bundle directory. Inlined custom assets ≤ 10 MiB total.
- Readiness protocol: dismiss only when `animated ∧ connected ∧ minDuration elapsed`. Fade-out is 400 ms. Auto-`animated` after 5 s if a page never calls it.
- Server-unreachable timeout in the native app: 10 s, then navigate anyway.
- Commit after every task; run `go vet ./... && go test ./...` before each commit. Commit trailers: `Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>` and `Claude-Session: https://claude.ai/code/session_016xrijz6yhTDJWMiSy7TXdH`.

---

## File map

| File | Responsibility |
|---|---|
| `internal/bundle/splash.go` | `SplashConfig`, defaults, validation, YAML decode, `html` path resolution |
| `internal/bundle/splash_test.go` | parse/validation tests |
| `internal/bundle/bundle.go` | wire `metadata.splash` into `File.Splash`; resolve `html` in `Load` |
| `internal/bundle/icon.go` | `LoadIconSVG(bundlePath, appIcon)` — packaged → bundle-relative → stock |
| `internal/splash/splash.go` | `Render`, `Inputs`, `Document`, `Phase` |
| `internal/splash/builtin.go` + `templates/*.html`, `assets/shim.js`, `assets/base.css` | built-in styles |
| `internal/splash/custom.go` | custom HTML: extract head/body, inject shim, inline assets, `Assets()` |
| `internal/splash/splash_test.go`, `custom_test.go` | render tests |
| `internal/server/server.go` | `Config.Splash/SplashIcon/Version`, `/api/config.splash`, overlay injection |
| `web/static/index.html`, `style.css`, `app.js` | `#splash` mount, overlay CSS, `Splash` controller |
| `cmd/server/main.go`, `cmd/app/main.go` | wiring; native handoff state machine |
| `cmd/app/window_darwin.go` | cgo `applyCard` / `restoreMain` |
| `cmd/bundlemeta/main.go`, `scripts/package-macos.sh` | `-splash`, `-splash-assets`, copy into `Resources/` |
| `examples/*/bundle.yaml`, `README.md`, `CLAUDE.md` | examples + docs |

---

### Task 1: `SplashConfig` — schema, defaults, validation

**Files:**
- Create: `internal/bundle/splash.go`
- Create: `internal/bundle/splash_test.go`
- Modify: `internal/bundle/bundle.go` (`metadata` struct line ~36, `File` struct line ~119, both branches of `Parse` that assign `file.AppIcon`, and `Load`)

**Interfaces:**
- Produces: `bundle.SplashConfig{Style, Window, HTML, Tagline, Accent, Background string; MinDuration time.Duration; Size [2]int}`, `bundle.DefaultSplash() SplashConfig`, `(SplashConfig).WithDefaults() SplashConfig`, `File.Splash SplashConfig` (always populated after `Parse`), constants `SplashStyleMinimal/Arc/Dia/JetBrains`, `SplashWindowFull/Card`.

- [ ] **Step 1: Write the failing tests**

`internal/bundle/splash_test.go`:

```go
package bundle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSplashDefaults(t *testing.T) {
	f, err := Parse([]byte(`
apiVersion: v1
kind: Bundle
metadata:
  name: Plain
spec:
  sessions: []
`))
	if err != nil {
		t.Fatal(err)
	}
	s := f.Splash
	if s.Style != SplashStyleMinimal || s.Window != SplashWindowFull {
		t.Errorf("defaults: style=%q window=%q", s.Style, s.Window)
	}
	if s.MinDuration != 1200*time.Millisecond {
		t.Errorf("minDuration = %v", s.MinDuration)
	}
	if s.Size != [2]int{640, 400} {
		t.Errorf("size = %v", s.Size)
	}
	if s.Accent == "" || s.Background == "" {
		t.Errorf("colours should have defaults: %+v", s)
	}
}

func TestSplashParseFields(t *testing.T) {
	f, err := Parse([]byte(`
apiVersion: v1
kind: BundleSet
metadata:
  name: Set
  splash:
    style: arc
    window: card
    tagline: Hello there
    accent: "#ff0000"
    background: "rgb(1, 2, 3)"
    minDuration: 2s
    size: [800, 500]
spec:
  bundles: []
`))
	if err != nil {
		t.Fatal(err)
	}
	s := f.Splash
	if s.Style != SplashStyleArc || s.Window != SplashWindowCard || s.Tagline != "Hello there" {
		t.Errorf("parsed = %+v", s)
	}
	if s.Accent != "#ff0000" || s.Background != "rgb(1, 2, 3)" {
		t.Errorf("colours = %q %q", s.Accent, s.Background)
	}
	if s.MinDuration != 2*time.Second || s.Size != [2]int{800, 500} {
		t.Errorf("duration/size = %v %v", s.MinDuration, s.Size)
	}
}

func TestSplashJetBrainsDefaultsToCard(t *testing.T) {
	f, err := Parse([]byte(`
apiVersion: v1
kind: Bundle
metadata:
  name: JB
  splash:
    style: jetbrains
spec:
  sessions: []
`))
	if err != nil {
		t.Fatal(err)
	}
	if f.Splash.Window != SplashWindowCard {
		t.Errorf("jetbrains window = %q, want card", f.Splash.Window)
	}
}

func TestSplashBundleSetWinsOverBundle(t *testing.T) {
	f, err := Parse([]byte(`
apiVersion: v1
kind: BundleSet
metadata:
  name: Set
  splash:
    style: dia
spec:
  bundles:
    - name: One
---
apiVersion: v1
kind: Bundle
metadata:
  name: One
  splash:
    style: arc
spec:
  sessions: []
`))
	if err != nil {
		t.Fatal(err)
	}
	if f.Splash.Style != SplashStyleDia {
		t.Errorf("style = %q, want dia (BundleSet wins)", f.Splash.Style)
	}
}

func TestSplashValidation(t *testing.T) {
	cases := map[string]string{
		"bad style":    "style: neon",
		"bad window":   "window: popup",
		"bad accent":   `accent: "red; } </style><script>"`,
		"bad duration": "minDuration: soon",
		"size small":   "size: [10, 400]",
		"size len":     "size: [640]",
	}
	for name, field := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte("apiVersion: v1\nkind: Bundle\nmetadata:\n  name: X\n  splash:\n    " + field + "\nspec:\n  sessions: []\n"))
			if err == nil {
				t.Fatalf("expected error for %s", field)
			}
		})
	}
}

func TestSplashHTMLResolvedByLoad(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "splash.html"), []byte("<html></html>"), 0o644)
	bundlePath := filepath.Join(dir, "bundle.yaml")
	os.WriteFile(bundlePath, []byte("apiVersion: v1\nkind: Bundle\nmetadata:\n  name: X\n  splash:\n    html: ./splash.html\nspec:\n  sessions: []\n"), 0o644)

	f, err := Load(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	if f.Splash.HTML != filepath.Join(dir, "splash.html") {
		t.Errorf("HTML = %q", f.Splash.HTML)
	}
}

func TestSplashHTMLMustStayInsideBundleDir(t *testing.T) {
	dir := t.TempDir()
	bundlePath := filepath.Join(dir, "bundle.yaml")
	os.WriteFile(bundlePath, []byte("apiVersion: v1\nkind: Bundle\nmetadata:\n  name: X\n  splash:\n    html: ../outside.html\nspec:\n  sessions: []\n"), 0o644)
	_, err := Load(bundlePath)
	if err == nil || !strings.Contains(err.Error(), "splash html") {
		t.Fatalf("expected splash html error, got %v", err)
	}
}

func TestSplashHTMLMissingFile(t *testing.T) {
	dir := t.TempDir()
	bundlePath := filepath.Join(dir, "bundle.yaml")
	os.WriteFile(bundlePath, []byte("apiVersion: v1\nkind: Bundle\nmetadata:\n  name: X\n  splash:\n    html: ./nope.html\nspec:\n  sessions: []\n"), 0o644)
	if _, err := Load(bundlePath); err == nil {
		t.Fatal("expected error for missing splash html")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/bundle/ -run 'TestSplash' 2>&1 | head`
Expected: build failure `undefined: SplashStyleMinimal` (and friends).

- [ ] **Step 3: Implement `internal/bundle/splash.go`**

```go
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
```

Add the unexported field to the struct (it lets `minDuration: 0` mean "dismiss immediately" rather than "use default"):

```go
	// minDurationSet records that minDuration was given explicitly (so 0 is honoured).
	minDurationSet bool
```

- [ ] **Step 4: Wire into `bundle.go`**

In the `metadata` struct:

```go
type metadata struct {
	Name    string      `yaml:"name"`
	AppIcon string      `yaml:"appIcon"`
	Splash  *splashYAML `yaml:"splash"`
}
```

In `File`, after `Files`:

```go
	// Splash is the resolved metadata.splash block (defaults applied).
	Splash SplashConfig
```

In `parsedBundle`, add `splash *splashYAML` and set `splash: d.Metadata.Splash` where `pb` is built. In `Parse`, BundleSet branch, right after `file.AppIcon = bundleSetDoc.Metadata.AppIcon`:

```go
		sp, err := resolveSplash(bundleSetDoc.Metadata.Splash)
		if err != nil {
			return nil, fmt.Errorf("bundleset %q: %w", bundleSetDoc.Metadata.Name, err)
		}
		file.Splash = sp
```

In the no-BundleSet branch, inside `if i == 0 {` after `file.Files = pb.files`:

```go
				sp, err := resolveSplash(pb.splash)
				if err != nil {
					return nil, fmt.Errorf("bundle %q: %w", pb.bundle.Name, err)
				}
				file.Splash = sp
```

In `Load`, after `Parse` succeeds and before `return f, nil`:

```go
	if err := f.Splash.resolveSplashHTML(filepath.Dir(path)); err != nil {
		return nil, fmt.Errorf("bundle %q: %w", path, err)
	}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/bundle/ 2>&1 | tail -3`
Expected: `ok`.

- [ ] **Step 6: Commit**

```bash
git add internal/bundle/splash.go internal/bundle/splash_test.go internal/bundle/bundle.go
git commit -m "feat(bundle): parse and validate metadata.splash"
```

---

### Task 2: `bundle.LoadIconSVG`

**Files:**
- Create: `internal/bundle/icon.go`
- Test: `internal/bundle/icon_test.go`

**Interfaces:**
- Produces: `bundle.LoadIconSVG(bundlePath, appIcon string) []byte` — bytes of `Contents/Resources/AppIcon.svg` when packaged, else `appIcon` resolved next to `bundlePath`, else `nil`. Never errors; callers fall back to a glyph.

- [ ] **Step 1: Write the failing test**

```go
package bundle

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadIconSVGFromBundleDir(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "icon.svg"), []byte("<svg/>"), 0o644)
	got := LoadIconSVG(filepath.Join(dir, "bundle.yaml"), "./icon.svg")
	if string(got) != "<svg/>" {
		t.Fatalf("got %q", got)
	}
}

func TestLoadIconSVGMissingIsNil(t *testing.T) {
	if got := LoadIconSVG(filepath.Join(t.TempDir(), "bundle.yaml"), "./none.svg"); got != nil {
		t.Fatalf("expected nil, got %q", got)
	}
	if got := LoadIconSVG("", ""); got != nil {
		t.Fatalf("expected nil for empty inputs, got %q", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/bundle/ -run TestLoadIconSVG`
Expected: `undefined: LoadIconSVG`.

- [ ] **Step 3: Implement**

```go
package bundle

import (
	"os"
	"path/filepath"
)

// LoadIconSVG returns the app icon SVG for splash rendering: the packaged
// Contents/Resources/AppIcon.svg when running inside a .app, otherwise
// appIcon resolved relative to bundlePath. Returns nil when nothing is found.
func LoadIconSVG(bundlePath, appIcon string) []byte {
	if dir := PackagedResourcesDir(); dir != "" {
		if b, err := os.ReadFile(filepath.Join(dir, "AppIcon.svg")); err == nil {
			return b
		}
	}
	if appIcon == "" {
		return nil
	}
	p := appIcon
	if !filepath.IsAbs(p) {
		if bundlePath == "" {
			return nil
		}
		p = filepath.Join(filepath.Dir(bundlePath), p)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	return b
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/bundle/`
Expected: `ok`. (In `go test` the executable is a temp binary with no `../Resources`, so the packaged branch is skipped.)

- [ ] **Step 5: Commit**

```bash
git add internal/bundle/icon.go internal/bundle/icon_test.go
git commit -m "feat(bundle): LoadIconSVG for splash rendering"
```

---

### Task 3: `internal/splash` core — shim, base CSS, `minimal` style, `Render`

**Files:**
- Create: `internal/splash/splash.go`, `internal/splash/builtin.go`, `internal/splash/assets/shim.js`, `internal/splash/assets/base.css`, `internal/splash/templates/minimal.html`
- Test: `internal/splash/splash_test.go`

**Interfaces:**
- Produces:
  ```go
  type Phase string
  const PhaseIntro Phase = "intro"; const PhaseFinal Phase = "final"
  type Inputs struct { Title, Version string; IconSVG []byte; Phase Phase; RemainingMS int }
  type Document struct { HTML, Head, Body string }
  func Render(cfg bundle.SplashConfig, in Inputs) (*Document, error)
  ```
  `Head` = `<style>` + `<script>` blocks (for injection before `</head>`), `Body` = `<div id="splash" data-phase="…" data-style="…">…</div>`, `HTML` = full standalone document.
- The page-side API defined by the shim: `window.SPLASH` (config object), `window.splash.animated()`, `window.splash.dismiss()`, `window.splash.dismissed()`; transport: `window.__splashPost(name)` if defined, plus `CustomEvent('splash:'+name)` on `document`.

- [ ] **Step 1: Write the failing tests**

```go
package splash

import (
	"strings"
	"testing"
	"time"

	"github.com/polds/tui-streamer/internal/bundle"
)

func cfg(style string) bundle.SplashConfig {
	c := bundle.DefaultSplash()
	c.Style = style
	c.Tagline = "Tag <b>line</b>"
	c.Accent = "#ff8800"
	c.Background = "#101010"
	c.MinDuration = 900 * time.Millisecond
	return c
}

func TestRenderMinimalStandalone(t *testing.T) {
	doc, err := Render(cfg("minimal"), Inputs{Title: "My <App>", Version: "1.2.3", IconSVG: []byte("<svg/>"), Phase: PhaseIntro})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<!DOCTYPE html>`,
		`id="splash"`, `data-phase="intro"`, `data-style="minimal"`,
		`My &lt;App&gt;`, `Tag &lt;b&gt;line&lt;/b&gt;`,
		`--splash-accent: #ff8800`, `--splash-bg: #101010`,
		`window.SPLASH = {`, `"minDurationMs":900`, `"phase":"intro"`, `"background":"#101010"`,
		`window.splash =`, // shim present
		`data:image/svg+xml;base64,`,
	} {
		if !strings.Contains(doc.HTML, want) {
			t.Errorf("HTML missing %q", want)
		}
	}
	if !strings.HasPrefix(strings.TrimSpace(doc.Body), `<div id="splash"`) {
		t.Errorf("Body should start with the #splash div, got %.60q", doc.Body)
	}
	if !strings.Contains(doc.Head, "<style>") || !strings.Contains(doc.Head, "<script>") {
		t.Errorf("Head should contain style and script blocks")
	}
	if strings.Contains(doc.HTML, "<b>line</b>") {
		t.Errorf("tagline was not escaped")
	}
}

func TestRenderFinalPhaseAndRemaining(t *testing.T) {
	doc, err := Render(cfg("minimal"), Inputs{Title: "T", Phase: PhaseFinal, RemainingMS: 250})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc.Body, `data-phase="final"`) || !strings.Contains(doc.Head, `"remainingMs":250`) {
		t.Errorf("final phase not rendered: %s", doc.Head)
	}
}

func TestRenderNoIconUsesGlyph(t *testing.T) {
	doc, err := Render(cfg("minimal"), Inputs{Title: "T", Phase: PhaseIntro})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(doc.HTML, "data:image/svg+xml") {
		t.Errorf("no icon should mean no data URI")
	}
	if !strings.Contains(doc.HTML, `class="splash-glyph"`) {
		t.Errorf("fallback glyph missing")
	}
}

func TestRenderZeroConfigUsesDefaults(t *testing.T) {
	doc, err := Render(bundle.SplashConfig{}, Inputs{Title: "T"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc.Body, `data-style="minimal"`) || !strings.Contains(doc.Body, `data-phase="intro"`) {
		t.Errorf("zero config should render minimal/intro: %.80q", doc.Body)
	}
}

func TestRenderUnknownStyle(t *testing.T) {
	c := cfg("nope")
	if _, err := Render(c, Inputs{Title: "T"}); err == nil {
		t.Fatal("expected error for unknown style")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/splash/`
Expected: build failure (package does not exist).

- [ ] **Step 3: Write the shim — `internal/splash/assets/shim.js`**

```js
// Splash readiness shim. Injected into every splash document (standalone and
// overlay). Page templates only ever call window.splash.animated().
//
//   page → host : splash.animated()   intro finished (auto after all
//                                     [data-splash-intro] animations end, or 5s)
//   host → page : splash.dismiss()    host decided: animated ∧ connected ∧ minDuration
//   page → host : splash.dismissed()  fade-out done (auto 400ms after dismiss)
//
// Transport: window.__splashPost(name) when the native host bound it, and a
// document CustomEvent('splash:' + name) for the browser host.
(function () {
  if (window.splash) return;
  var cfg = window.SPLASH || {};
  var sent = {};

  function post(name) {
    if (sent[name]) return;
    sent[name] = true;
    if (typeof window.__splashPost === 'function') {
      try { window.__splashPost(name); } catch (e) { /* host gone */ }
    }
    try { document.dispatchEvent(new CustomEvent('splash:' + name)); } catch (e) { /* old engine */ }
  }

  function root() { return document.getElementById('splash'); }

  var splash = window.splash = {
    animated: function () { post('animated'); },
    dismiss: function () {
      var r = root();
      if (r) r.classList.add('splash-dismissing');
      setTimeout(splash.dismissed, 400);
    },
    dismissed: function () { post('dismissed'); },
    config: cfg,
  };

  function watchIntro() {
    var r = root();
    if (!r) return;
    var reduced = window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    if (cfg.phase === 'final' || reduced) { splash.animated(); return; }
    var els = r.querySelectorAll('[data-splash-intro]');
    var pending = els.length;
    var cap = setTimeout(splash.animated, 5000);
    if (!pending) return; // custom page: it calls splash.animated() itself (or the cap fires)
    for (var i = 0; i < els.length; i++) {
      els[i].addEventListener('animationend', function () {
        if (--pending <= 0) { clearTimeout(cap); splash.animated(); }
      }, { once: true });
    }
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', watchIntro);
  } else {
    watchIntro();
  }
})();
```

- [ ] **Step 4: Write the base CSS — `internal/splash/assets/base.css`**

```css
/* Shared by every splash document. Everything is scoped under #splash so the
   same markup works standalone and as an overlay inside index.html. */
#splash {
  --splash-accent: #7aa2f7;
  --splash-bg: #1a1b26;
  --splash-fg: #e6e6e6;
  --splash-muted: rgba(255, 255, 255, 0.55);
  position: fixed;
  inset: 0;
  z-index: 10000;
  background: var(--splash-bg);
  color: var(--splash-fg);
  font-family: -apple-system, BlinkMacSystemFont, "SF Pro Text", "Inter", "Segoe UI", sans-serif;
  -webkit-font-smoothing: antialiased;
  user-select: none;
  -webkit-user-select: none;
  overflow: hidden;
  opacity: 1;
  transition: opacity 400ms ease;
}
#splash.splash-dismissing { opacity: 0; pointer-events: none; }
#splash *, #splash *::before, #splash *::after { box-sizing: border-box; }
#splash .splash-icon { width: 96px; height: 96px; display: block; }
#splash .splash-glyph {
  width: 96px; height: 96px; border-radius: 22px;
  display: flex; align-items: center; justify-content: center;
  font: 700 44px/1 "SF Mono", "JetBrains Mono", "Fira Code", monospace;
  color: var(--splash-bg); background: var(--splash-accent);
}
#splash .splash-title { font-size: 28px; font-weight: 600; letter-spacing: -0.01em; }
#splash .splash-tagline { font-size: 15px; color: var(--splash-muted); }
#splash .splash-version { font-size: 12px; color: var(--splash-muted); font-variant-numeric: tabular-nums; }
#splash .splash-bar { position: relative; height: 3px; overflow: hidden; background: rgba(255, 255, 255, 0.08); border-radius: 2px; }
#splash .splash-bar::after {
  content: ""; position: absolute; top: 0; left: 0; height: 100%; width: 40%;
  background: linear-gradient(90deg, transparent, var(--splash-accent) 50%, transparent);
  animation: splash-slide 1.4s cubic-bezier(0.4, 0, 0.6, 1) infinite;
}
@keyframes splash-slide { from { transform: translateX(-100%); } to { transform: translateX(350%); } }
@media (prefers-reduced-motion: reduce) {
  #splash [data-splash-intro] { animation: none !important; }
}
```

- [ ] **Step 5: Write the `minimal` template — `internal/splash/templates/minimal.html`**

Only the `#splash` div plus a style block; the assembler wraps it. Everything user-facing is escaped by `html/template`.

```html
<style>
#splash[data-style="minimal"] { display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 28px;
  font-family: "SF Mono", "Fira Code", "Cascadia Code", "JetBrains Mono", monospace; }
#splash[data-style="minimal"] .logo { font-size: 5rem; font-weight: 700; color: var(--splash-accent); line-height: 1; letter-spacing: -0.02em; }
#splash[data-style="minimal"] .logo .cursor { animation: splash-blink 1s step-end infinite; }
#splash[data-style="minimal"] .label { font-size: 0.8rem; letter-spacing: 0.18em; text-transform: uppercase; color: var(--splash-muted); }
#splash[data-style="minimal"] .splash-bar { width: 220px; height: 2px; }
#splash[data-style="minimal"][data-phase="intro"] [data-splash-intro] { animation: splash-fade-up 500ms ease-out both; }
@keyframes splash-blink { 0%, 100% { opacity: 1; } 50% { opacity: 0; } }
@keyframes splash-fade-up { from { opacity: 0; transform: translateY(8px); } to { opacity: 1; transform: none; } }
</style>
<div id="splash" data-style="minimal" data-phase="{{.Phase}}" style="--splash-accent: {{.Accent}}; --splash-bg: {{.Background}};">
  <div class="logo" data-splash-intro>&gt;<span class="cursor">_</span></div>
  <div class="label">{{.Title}}{{if .Tagline}} &mdash; {{.Tagline}}{{else}} &mdash; Starting&hellip;{{end}}</div>
  <div class="splash-bar"></div>
</div>
```

- [ ] **Step 6: Write `internal/splash/builtin.go`**

```go
package splash

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"strings"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed assets/shim.js
var shimJS string

//go:embed assets/base.css
var baseCSS string

// templateData is what every built-in template receives.
type templateData struct {
	Title, Tagline, Version string
	Accent, Background      template.CSS // validated by bundle; safe to pass through
	Phase                   Phase
	IconDataURI             template.URL // "" when no icon
	HasIcon                 bool
}

var builtins = template.Must(template.ParseFS(templateFS, "templates/*.html"))

// renderBuiltin executes templates/<style>.html and splits the result into a
// <style> head fragment and the #splash body fragment.
func renderBuiltin(style string, d templateData) (head, body string, err error) {
	t := builtins.Lookup(style + ".html")
	if t == nil {
		return "", "", fmt.Errorf("splash style %q: no template", style)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, d); err != nil {
		return "", "", fmt.Errorf("splash style %q: %w", style, err)
	}
	out := buf.String()
	// Templates are written as <style>…</style> followed by the #splash div.
	i := strings.Index(out, "</style>")
	if i < 0 {
		return "", strings.TrimSpace(out), nil
	}
	i += len("</style>")
	return strings.TrimSpace(out[:i]), strings.TrimSpace(out[i:]), nil
}
```

- [ ] **Step 7: Write `internal/splash/splash.go`**

```go
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
	h.WriteString("<style>")
	h.WriteString(baseCSS)
	h.WriteString("</style>\n")
	h.WriteString(head)
	h.WriteString("\n<script>window.SPLASH = ")
	h.WriteString(configJS)
	h.WriteString(";</script>\n<script>")
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
```

Add `"html/template"` to the imports. `renderCustom` is written in Task 5; for this task add a stub in `custom.go` so the package compiles:

```go
package splash

import (
	"fmt"

	"github.com/polds/tui-streamer/internal/bundle"
)

func renderCustom(cfg bundle.SplashConfig, in Inputs) (string, string, error) {
	return "", "", fmt.Errorf("custom splash html not implemented")
}
```

- [ ] **Step 8: Run tests to verify they pass**

Run: `go test ./internal/splash/`
Expected: `ok`. If `TestRenderMinimalStandalone` fails on `--splash-accent: #ff8800`, check that `html/template` did not escape the `template.CSS` value inside the `style=` attribute (it shouldn't for a `CSS`-typed value); the assertion string must match the template byte-for-byte including the single space after the colon.

- [ ] **Step 9: Commit**

```bash
git add internal/splash
git commit -m "feat(splash): shared splash renderer with shim and minimal style"
```

---

### Task 4: Built-in `arc`, `dia`, `jetbrains` templates

**Files:**
- Create: `internal/splash/templates/arc.html`, `dia.html`, `jetbrains.html`
- Modify: `internal/splash/splash_test.go`

**Interfaces:**
- Consumes: `templateData` from Task 3.
- Every intro-animated element carries `data-splash-intro`; intro keyframes are gated on `[data-phase="intro"]`.

- [ ] **Step 1: Write the failing test**

Append to `splash_test.go`:

```go
func TestRenderAllBuiltinStyles(t *testing.T) {
	for _, style := range []string{"minimal", "arc", "dia", "jetbrains"} {
		t.Run(style, func(t *testing.T) {
			intro, err := Render(cfg(style), Inputs{Title: "App", Version: "v9", IconSVG: []byte("<svg/>"), Phase: PhaseIntro})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(intro.Body, `data-style="`+style+`"`) {
				t.Errorf("missing data-style")
			}
			if !strings.Contains(intro.Body, "data-splash-intro") {
				t.Errorf("intro phase must mark animated elements with data-splash-intro")
			}
			if !strings.Contains(intro.Body, "App") || !strings.Contains(intro.Body, "Tag &lt;b&gt;line&lt;/b&gt;") {
				t.Errorf("title/tagline missing or unescaped")
			}
			if style == "jetbrains" && !strings.Contains(intro.Body, "v9") {
				t.Errorf("jetbrains should show the version")
			}
			final, err := Render(cfg(style), Inputs{Title: "App", Phase: PhaseFinal})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(final.Body, `data-phase="final"`) {
				t.Errorf("final phase attribute missing")
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/splash/ -run TestRenderAllBuiltinStyles`
Expected: FAIL for `arc`, `dia`, `jetbrains` with `no template`.

- [ ] **Step 3: Write `templates/arc.html`** (full-bleed animated gradient mesh, icon rises and settles, tagline fades in)

```html
<style>
#splash[data-style="arc"] { display: grid; place-items: center; isolation: isolate; }
#splash[data-style="arc"] .mesh { position: absolute; inset: -20%; z-index: -1; filter: blur(60px) saturate(1.3); opacity: 0.9;
  background:
    radial-gradient(40% 40% at 20% 30%, var(--splash-accent), transparent 70%),
    radial-gradient(35% 35% at 80% 25%, color-mix(in srgb, var(--splash-accent) 60%, #ff6bcb), transparent 70%),
    radial-gradient(45% 45% at 60% 80%, color-mix(in srgb, var(--splash-accent) 50%, #34d399), transparent 70%);
  animation: splash-arc-drift 14s ease-in-out infinite alternate; }
#splash[data-style="arc"] .stack { display: flex; flex-direction: column; align-items: center; gap: 18px; text-align: center; }
#splash[data-style="arc"] .splash-icon, #splash[data-style="arc"] .splash-glyph { width: 128px; height: 128px; border-radius: 30px;
  box-shadow: 0 30px 60px rgba(0, 0, 0, 0.35); }
#splash[data-style="arc"] .splash-title { font-size: 34px; font-weight: 700; letter-spacing: -0.02em; }
#splash[data-style="arc"] .splash-tagline { font-size: 16px; max-width: 42ch; }
#splash[data-style="arc"] .splash-bar { width: 160px; margin-top: 10px; }
#splash[data-style="arc"][data-phase="intro"] .rise { animation: splash-arc-rise 900ms cubic-bezier(0.2, 0.8, 0.2, 1) both; }
#splash[data-style="arc"][data-phase="intro"] .fade { animation: splash-arc-fade 700ms ease-out 500ms both; }
@keyframes splash-arc-drift { from { transform: translate3d(-3%, -2%, 0) rotate(0deg); } to { transform: translate3d(3%, 4%, 0) rotate(8deg); } }
@keyframes splash-arc-rise { from { opacity: 0; transform: translateY(40px) scale(0.92); } to { opacity: 1; transform: none; } }
@keyframes splash-arc-fade { from { opacity: 0; transform: translateY(6px); } to { opacity: 1; transform: none; } }
</style>
<div id="splash" data-style="arc" data-phase="{{.Phase}}" style="--splash-accent: {{.Accent}}; --splash-bg: {{.Background}};">
  <div class="mesh"></div>
  <div class="stack">
    {{if .HasIcon}}<img class="splash-icon rise" data-splash-intro alt="" src="{{.IconDataURI}}">{{else}}<div class="splash-glyph rise" data-splash-intro>&gt;_</div>{{end}}
    <div class="splash-title fade" data-splash-intro>{{.Title}}</div>
    {{if .Tagline}}<div class="splash-tagline fade" data-splash-intro>{{.Tagline}}</div>{{end}}
    <div class="splash-bar"></div>
  </div>
</div>
```

- [ ] **Step 4: Write `templates/dia.html`** (soft light gradient, breathing orb behind the icon, title tracks in)

```html
<style>
#splash[data-style="dia"] { --splash-fg: #1c1c1e; --splash-muted: rgba(0, 0, 0, 0.5); display: grid; place-items: center;
  background: linear-gradient(160deg, var(--splash-bg), color-mix(in srgb, var(--splash-bg) 85%, var(--splash-accent))); }
#splash[data-style="dia"] .orb { position: absolute; width: 420px; height: 420px; border-radius: 50%; z-index: 0;
  background: radial-gradient(circle at 40% 40%, color-mix(in srgb, var(--splash-accent) 45%, white), transparent 65%);
  filter: blur(30px); animation: splash-dia-breathe 3.2s ease-in-out infinite; }
#splash[data-style="dia"] .stack { position: relative; z-index: 1; display: flex; flex-direction: column; align-items: center; gap: 16px; text-align: center; }
#splash[data-style="dia"] .splash-icon, #splash[data-style="dia"] .splash-glyph { width: 112px; height: 112px; border-radius: 28px;
  box-shadow: 0 20px 50px rgba(0, 0, 0, 0.18); }
#splash[data-style="dia"] .splash-title { font-size: 32px; font-weight: 600; letter-spacing: 0.02em; }
#splash[data-style="dia"] .splash-tagline { font-size: 15px; }
#splash[data-style="dia"] .splash-bar { width: 140px; background: rgba(0, 0, 0, 0.08); }
#splash[data-style="dia"][data-phase="intro"] .pop { animation: splash-dia-pop 700ms cubic-bezier(0.34, 1.4, 0.64, 1) both; }
#splash[data-style="dia"][data-phase="intro"] .track { animation: splash-dia-track 800ms ease-out 250ms both; }
@keyframes splash-dia-breathe { 0%, 100% { transform: scale(1); opacity: 0.8; } 50% { transform: scale(1.12); opacity: 1; } }
@keyframes splash-dia-pop { from { opacity: 0; transform: scale(0.7); } to { opacity: 1; transform: none; } }
@keyframes splash-dia-track { from { opacity: 0; letter-spacing: 0.3em; } to { opacity: 1; letter-spacing: 0.02em; } }
</style>
<div id="splash" data-style="dia" data-phase="{{.Phase}}" style="--splash-accent: {{.Accent}}; --splash-bg: {{.Background}};">
  <div class="orb"></div>
  <div class="stack">
    {{if .HasIcon}}<img class="splash-icon pop" data-splash-intro alt="" src="{{.IconDataURI}}">{{else}}<div class="splash-glyph pop" data-splash-intro>&gt;_</div>{{end}}
    <div class="splash-title track" data-splash-intro>{{.Title}}</div>
    {{if .Tagline}}<div class="splash-tagline">{{.Tagline}}</div>{{end}}
    <div class="splash-bar"></div>
  </div>
</div>
```

- [ ] **Step 5: Write `templates/jetbrains.html`** (card: artwork panel left, text right, bar along the bottom)

```html
<style>
#splash[data-style="jetbrains"] { display: grid; grid-template-columns: 42% 1fr; grid-template-rows: 1fr auto; }
#splash[data-style="jetbrains"] .art { grid-row: 1 / span 2; position: relative; overflow: hidden;
  background: linear-gradient(135deg, var(--splash-accent), color-mix(in srgb, var(--splash-accent) 40%, #000) 70%, #000); display: grid; place-items: center; }
#splash[data-style="jetbrains"] .art::before { content: ""; position: absolute; inset: -40%;
  background: conic-gradient(from 0deg, transparent, rgba(255, 255, 255, 0.18), transparent 30%);
  animation: splash-jb-spin 9s linear infinite; }
#splash[data-style="jetbrains"] .splash-icon, #splash[data-style="jetbrains"] .splash-glyph { position: relative; width: 120px; height: 120px; border-radius: 26px;
  box-shadow: 0 24px 48px rgba(0, 0, 0, 0.45); }
#splash[data-style="jetbrains"] .text { padding: 36px 40px; display: flex; flex-direction: column; justify-content: center; gap: 10px; }
#splash[data-style="jetbrains"] .splash-title { font-size: 30px; font-weight: 700; }
#splash[data-style="jetbrains"] .splash-tagline { font-size: 14px; }
#splash[data-style="jetbrains"] .splash-version { margin-top: 14px; }
#splash[data-style="jetbrains"] .splash-bar { grid-column: 2; height: 3px; border-radius: 0; }
#splash[data-style="jetbrains"][data-phase="intro"] .slide { animation: splash-jb-slide 600ms cubic-bezier(0.2, 0.8, 0.2, 1) both; }
#splash[data-style="jetbrains"][data-phase="intro"] .slide.d1 { animation-delay: 120ms; }
#splash[data-style="jetbrains"][data-phase="intro"] .slide.d2 { animation-delay: 240ms; }
@keyframes splash-jb-spin { to { transform: rotate(360deg); } }
@keyframes splash-jb-slide { from { opacity: 0; transform: translateX(-14px); } to { opacity: 1; transform: none; } }
</style>
<div id="splash" data-style="jetbrains" data-phase="{{.Phase}}" style="--splash-accent: {{.Accent}}; --splash-bg: {{.Background}};">
  <div class="art">
    {{if .HasIcon}}<img class="splash-icon" alt="" src="{{.IconDataURI}}">{{else}}<div class="splash-glyph">&gt;_</div>{{end}}
  </div>
  <div class="text">
    <div class="splash-title slide" data-splash-intro>{{.Title}}</div>
    {{if .Tagline}}<div class="splash-tagline slide d1" data-splash-intro>{{.Tagline}}</div>{{end}}
    <div class="splash-version slide d2" data-splash-intro>{{if .Version}}Version {{.Version}}{{else}}Starting&hellip;{{end}}</div>
  </div>
  <div class="splash-bar"></div>
</div>
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/splash/`
Expected: `ok`.

- [ ] **Step 7: Visual smoke check (no commit gate, but do it)**

```bash
cat > /tmp/splash-preview_test.go <<'EOF'
EOF
go run ./cmd/splashpreview 2>/dev/null || true
```

There is no preview command; instead write a throwaway test in `internal/splash` that renders each style to `$TMPDIR/splash-<style>.html` and `open` them in a browser to eyeball the animations. Delete the test before committing.

- [ ] **Step 8: Commit**

```bash
git add internal/splash/templates
git commit -m "feat(splash): arc, dia and jetbrains built-in styles"
```

---

### Task 5: Custom `html:` pages — shim injection and asset inlining

**Files:**
- Modify: `internal/splash/custom.go` (replace the stub)
- Test: `internal/splash/custom_test.go`

**Interfaces:**
- Produces: `splash.Assets(cfg bundle.SplashConfig) ([]string, error)` — absolute paths of relative assets a custom page references (for packaging). `renderCustom(cfg, in) (head, body string, err error)`.
- Rules: relative `src="…"`, `href="…"`, `url(…)` are inlined as data URIs; references must stay inside the bundle directory (`filepath.Dir(cfg.HTML)` and above up to the bundle dir are allowed — use the html file's directory as the root); total inlined bytes ≤ 10 MiB; body content is wrapped in `<div id="splash" data-style="custom" data-phase="…">`; `<style>`/`<script>` blocks from `<head>` go to `Head`.

- [ ] **Step 1: Write the failing tests**

```go
package splash

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/polds/tui-streamer/internal/bundle"
)

func writeCustom(t *testing.T, html string) bundle.SplashConfig {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "img"), 0o755)
	os.WriteFile(filepath.Join(dir, "img", "logo.svg"), []byte("<svg/>"), 0o644)
	os.WriteFile(filepath.Join(dir, "font.css"), []byte("#splash h1 { color: red; }"), 0o644)
	p := filepath.Join(dir, "splash.html")
	os.WriteFile(p, []byte(html), 0o644)
	c := bundle.DefaultSplash()
	c.HTML = p
	return c
}

const customPage = `<!DOCTYPE html><html><head>
<link rel="stylesheet" href="font.css">
<style>#splash .hero { background: url(img/logo.svg); }</style>
<script>window.__custom = 1;</script>
</head><body class="x">
<h1 class="hero">Custom</h1><img src="./img/logo.svg" alt="">
<script>setTimeout(function(){ splash.animated(); }, 10);</script>
</body></html>`

func TestCustomPageIsWrappedAndInlined(t *testing.T) {
	doc, err := Render(writeCustom(t, customPage), Inputs{Title: "T", Phase: PhaseIntro})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(strings.TrimSpace(doc.Body), `<div id="splash" data-style="custom" data-phase="intro"`) {
		t.Errorf("body not wrapped: %.80q", doc.Body)
	}
	if !strings.Contains(doc.Body, `<h1 class="hero">Custom</h1>`) {
		t.Errorf("body content missing")
	}
	if strings.Count(doc.HTML, "data:image/svg+xml;base64,") != 2 {
		t.Errorf("expected img src and css url() inlined, got %d", strings.Count(doc.HTML, "data:image/svg+xml;base64,"))
	}
	if !strings.Contains(doc.Head, "#splash h1 { color: red; }") {
		t.Errorf("linked stylesheet should be inlined into Head")
	}
	if !strings.Contains(doc.Head, "window.__custom = 1;") || !strings.Contains(doc.Body, "splash.animated()") {
		t.Errorf("scripts should be preserved")
	}
	if !strings.Contains(doc.Head, "window.splash =") {
		t.Errorf("shim missing")
	}
}

func TestCustomPageAssetsListed(t *testing.T) {
	c := writeCustom(t, customPage)
	got, err := Assets(c)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{filepath.Join(filepath.Dir(c.HTML), "font.css"): true, filepath.Join(filepath.Dir(c.HTML), "img", "logo.svg"): true}
	if len(got) != len(want) {
		t.Fatalf("assets = %v", got)
	}
	for _, p := range got {
		if !want[p] {
			t.Errorf("unexpected asset %q", p)
		}
	}
}

func TestCustomPageRejectsEscapingAsset(t *testing.T) {
	c := writeCustom(t, `<html><body><img src="../../etc/hosts"></body></html>`)
	if _, err := Render(c, Inputs{Title: "T"}); err == nil {
		t.Fatal("expected error for asset outside the splash directory")
	}
}

func TestCustomPageSkipsAbsoluteAndDataRefs(t *testing.T) {
	c := writeCustom(t, `<html><body><img src="https://x/y.png"><img src="data:image/png;base64,AA=="><a href="#top">t</a></body></html>`)
	doc, err := Render(c, Inputs{Title: "T"})
	if err != nil {
		t.Fatal(err)
	}
	for _, keep := range []string{`src="https://x/y.png"`, `src="data:image/png;base64,AA=="`, `href="#top"`} {
		if !strings.Contains(doc.Body, keep) {
			t.Errorf("should leave %q untouched", keep)
		}
	}
}

func TestCustomPageSizeCap(t *testing.T) {
	c := writeCustom(t, `<html><body><img src="big.bin"></body></html>`)
	big := make([]byte, 11<<20)
	os.WriteFile(filepath.Join(filepath.Dir(c.HTML), "big.bin"), big, 0o644)
	if _, err := Render(c, Inputs{Title: "T"}); err == nil || !strings.Contains(err.Error(), "10 MiB") {
		t.Fatalf("expected size cap error, got %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/splash/ -run TestCustom`
Expected: FAIL with `custom splash html not implemented` / `undefined: Assets`.

- [ ] **Step 3: Implement `internal/splash/custom.go`**

```go
package splash

import (
	"encoding/base64"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/polds/tui-streamer/internal/bundle"
)

// maxInlineBytes caps the total size of assets inlined into a custom page.
const maxInlineBytes = 10 << 20

var (
	reHead   = regexp.MustCompile(`(?is)<head[^>]*>(.*?)</head>`)
	reBody   = regexp.MustCompile(`(?is)<body[^>]*>(.*?)</body>`)
	reStyle  = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	reScript = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	reLink   = regexp.MustCompile(`(?is)<link[^>]+rel=["']?stylesheet["']?[^>]*>`)
	reHref   = regexp.MustCompile(`(?i)href=["']([^"']+)["']`)
	// Attribute and CSS references that may point at a relative file.
	reAttrRef = regexp.MustCompile(`(?i)\b(src|href)=["']([^"']+)["']`)
	reCSSURL  = regexp.MustCompile(`(?i)url\(\s*["']?([^"')]+)["']?\s*\)`)
)

// isRelativeRef reports whether ref is a bundle-relative file reference (as
// opposed to a URL, data URI, fragment or absolute path).
func isRelativeRef(ref string) bool {
	r := strings.TrimSpace(ref)
	if r == "" || strings.HasPrefix(r, "#") || strings.HasPrefix(r, "/") || strings.HasPrefix(r, "data:") {
		return false
	}
	if strings.Contains(r, "://") || strings.HasPrefix(r, "//") || strings.HasPrefix(r, "javascript:") {
		return false
	}
	return true
}

// inliner resolves relative references against root, enforcing containment
// and the size cap, and remembers every file it touched.
type inliner struct {
	root  string
	total int
	seen  map[string]bool
	files []string
}

func newInliner(htmlPath string) *inliner {
	return &inliner{root: filepath.Dir(htmlPath), seen: map[string]bool{}}
}

func (in *inliner) resolve(ref string) (string, error) {
	p := filepath.Join(in.root, filepath.FromSlash(strings.SplitN(ref, "?", 2)[0]))
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("splash asset %q: %w", ref, err)
	}
	rootAbs, _ := filepath.Abs(in.root)
	rel, err := filepath.Rel(rootAbs, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("splash asset %q escapes the splash directory", ref)
	}
	if !in.seen[abs] {
		in.seen[abs] = true
		in.files = append(in.files, abs)
	}
	return abs, nil
}

func (in *inliner) dataURI(ref string) (string, error) {
	abs, err := in.resolve(ref)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return "", fmt.Errorf("splash asset %q: %w", ref, err)
	}
	in.total += len(b)
	if in.total > maxInlineBytes {
		return "", fmt.Errorf("splash assets exceed 10 MiB")
	}
	mt := mime.TypeByExtension(filepath.Ext(abs))
	if mt == "" {
		mt = "application/octet-stream"
	}
	return "data:" + mt + ";base64," + base64.StdEncoding.EncodeToString(b), nil
}

// inlineRefs rewrites relative src/href attributes and css url() values.
func (in *inliner) inlineRefs(s string) (string, error) {
	var firstErr error
	s = reAttrRef.ReplaceAllStringFunc(s, func(m string) string {
		parts := reAttrRef.FindStringSubmatch(m)
		if !isRelativeRef(parts[2]) {
			return m
		}
		uri, err := in.dataURI(parts[2])
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			return m
		}
		return parts[1] + `="` + uri + `"`
	})
	if firstErr != nil {
		return "", firstErr
	}
	s = reCSSURL.ReplaceAllStringFunc(s, func(m string) string {
		parts := reCSSURL.FindStringSubmatch(m)
		if !isRelativeRef(parts[1]) {
			return m
		}
		uri, err := in.dataURI(parts[1])
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			return m
		}
		return `url("` + uri + `")`
	})
	return s, firstErr
}

// renderCustom loads cfg.HTML and produces head/body fragments compatible
// with the built-in styles: styles and scripts from <head> (linked
// stylesheets inlined as <style>), body content wrapped in #splash.
func renderCustom(cfg bundle.SplashConfig, in Inputs) (string, string, error) {
	raw, err := os.ReadFile(cfg.HTML)
	if err != nil {
		return "", "", fmt.Errorf("splash html: %w", err)
	}
	src := string(raw)
	il := newInliner(cfg.HTML)

	headSrc := ""
	if m := reHead.FindStringSubmatch(src); m != nil {
		headSrc = m[1]
	}
	bodySrc := src
	if m := reBody.FindStringSubmatch(src); m != nil {
		bodySrc = m[1]
	}

	var head strings.Builder
	for _, link := range reLink.FindAllString(headSrc, -1) {
		hm := reHref.FindStringSubmatch(link)
		if hm == nil || !isRelativeRef(hm[1]) {
			continue
		}
		abs, err := il.resolve(hm[1])
		if err != nil {
			return "", "", err
		}
		css, err := os.ReadFile(abs)
		if err != nil {
			return "", "", fmt.Errorf("splash stylesheet %q: %w", hm[1], err)
		}
		il.total += len(css)
		if il.total > maxInlineBytes {
			return "", "", fmt.Errorf("splash assets exceed 10 MiB")
		}
		inlined, err := il.inlineRefs(string(css))
		if err != nil {
			return "", "", err
		}
		head.WriteString("<style>")
		head.WriteString(inlined)
		head.WriteString("</style>\n")
	}
	for _, block := range append(reStyle.FindAllString(headSrc, -1), reScript.FindAllString(headSrc, -1)...) {
		inlined, err := il.inlineRefs(block)
		if err != nil {
			return "", "", err
		}
		head.WriteString(inlined)
		head.WriteString("\n")
	}

	body, err := il.inlineRefs(bodySrc)
	if err != nil {
		return "", "", err
	}
	phase := in.Phase
	if phase == "" {
		phase = PhaseIntro
	}
	wrapped := `<div id="splash" data-style="custom" data-phase="` + string(phase) + `" style="--splash-accent: ` +
		cfg.Accent + `; --splash-bg: ` + cfg.Background + `;">` + "\n" + strings.TrimSpace(body) + "\n</div>"
	return strings.TrimSpace(head.String()), wrapped, nil
}

// Assets returns the absolute paths of every relative file a custom splash
// page references (stylesheets, images, fonts). Empty for built-in styles.
func Assets(cfg bundle.SplashConfig) ([]string, error) {
	if cfg.HTML == "" {
		return nil, nil
	}
	raw, err := os.ReadFile(cfg.HTML)
	if err != nil {
		return nil, fmt.Errorf("splash html: %w", err)
	}
	il := newInliner(cfg.HTML)
	if _, err := il.inlineRefs(string(raw)); err != nil {
		return nil, err
	}
	// Stylesheets can reference further assets; walk them too.
	for _, link := range reLink.FindAllString(string(raw), -1) {
		if hm := reHref.FindStringSubmatch(link); hm != nil && isRelativeRef(hm[1]) {
			abs, err := il.resolve(hm[1])
			if err != nil {
				return nil, err
			}
			if css, err := os.ReadFile(abs); err == nil {
				if _, err := il.inlineRefs(string(css)); err != nil {
					return nil, err
				}
			}
		}
	}
	return il.files, nil
}
```

Note `cfg.Accent`/`cfg.Background` are bundle-validated (`^[#a-zA-Z0-9(),.% -]+$`), so string concatenation into the `style` attribute is safe.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/splash/`
Expected: `ok`. If `TestCustomPageAssetsListed` sees 3 files, the `<link href>` was inlined twice (once by `reAttrRef` in the raw walk and once via the stylesheet walk) — that is fine for the seen-set, but check `font.css` appears once.

- [ ] **Step 5: Commit**

```bash
git add internal/splash/custom.go internal/splash/custom_test.go
git commit -m "feat(splash): custom splash.html with shim injection and asset inlining"
```

---

### Task 6: Server — config exposure and overlay injection

**Files:**
- Modify: `internal/server/server.go` (`Config` struct ~line 40; `handleIndex` ~line 88; `handleConfig` ~line 337)
- Modify: `web/static/index.html` (after `<body>`)
- Test: `internal/server/server_test.go`

**Interfaces:**
- Consumes: `splash.Render`, `bundle.SplashConfig`.
- Produces: `server.Config{Splash bundle.SplashConfig; SplashIcon []byte; Version string}`; `GET /api/config` → `"splash": {style, window, tagline, accent, background, minDurationMs}`; `handleIndex` honours `?splash=final&remaining=<ms>`.

- [ ] **Step 1: Write the failing tests**

Append to `internal/server/server_test.go`:

```go
func TestIndexInjectsSplashOverlay(t *testing.T) {
	mgr := session.NewManager()
	cfg := bundle.DefaultSplash()
	cfg.Style = "arc"
	cfg.Tagline = "Sync <it>"
	s := New(mgr, Config{Title: "Sync App", Splash: cfg, SplashIcon: []byte("<svg/>")}, testStaticFS())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	html := rec.Body.String()
	for _, want := range []string{
		`<div id="splash" data-style="arc" data-phase="intro"`,
		`Sync &lt;it&gt;`,
		`window.SPLASH = {`,
		`"phase":"intro"`,
		`window.splash =`,
		`data:image/svg+xml;base64,`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("index missing %q", want)
		}
	}
	if strings.Contains(html, `<div id="splash" hidden></div>`) {
		t.Errorf("placeholder should have been replaced")
	}
}

func TestIndexSplashFinalPhase(t *testing.T) {
	mgr := session.NewManager()
	s := New(mgr, Config{Title: "T"}, testStaticFS())
	req := httptest.NewRequest(http.MethodGet, "/?splash=final&remaining=300", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	html := rec.Body.String()
	if !strings.Contains(html, `data-phase="final"`) || !strings.Contains(html, `"remainingMs":300`) {
		t.Errorf("final phase not honoured: %s", html)
	}
}

func TestConfigExposesSplash(t *testing.T) {
	mgr := session.NewManager()
	cfg := bundle.DefaultSplash()
	cfg.Style = "jetbrains"
	cfg.Window = "card"
	s := New(mgr, Config{Splash: cfg}, testStaticFS())
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var out map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	sp, _ := out["splash"].(map[string]any)
	if sp["style"] != "jetbrains" || sp["window"] != "card" || sp["minDurationMs"] != float64(1200) {
		t.Errorf("splash config = %#v", sp)
	}
	if _, has := sp["html"]; has {
		t.Errorf("html path must not be exposed")
	}
}
```

Update `testStaticFS()` so `index.html` contains the placeholder:

```go
			Data: []byte("<html><head><title>tui-streamer</title></head><body><div id=\"splash\" hidden></div><div class=\"header-logo\">\n    tui-streamer\n  </div></body></html>"),
```

Add `"github.com/polds/tui-streamer/internal/bundle"` to the test imports.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/server/ -run 'Splash|Index'`
Expected: build failure `unknown field Splash in struct literal`.

- [ ] **Step 3: Extend `Config`**

After `TUIPath string` in `Config`:

```go
	// Splash configures the startup splash (zero value = built-in minimal).
	Splash bundle.SplashConfig
	// SplashIcon is the app icon SVG inlined into the splash, or nil.
	SplashIcon []byte
	// Version is shown on splash styles that display it.
	Version string
```

- [ ] **Step 4: Inject the overlay in `handleIndex`**

Replace the final three lines of `handleIndex` (`w.Header().Set(...)`, `w.Write(...)`) with:

```go
	// Inject the splash overlay. ?splash=final is the native app handing off
	// after its own splash; the overlay then shows the resting frame only.
	phase := splash.PhaseIntro
	remaining := 0
	if r.URL.Query().Get("splash") == "final" {
		phase = splash.PhaseFinal
		remaining, _ = strconv.Atoi(r.URL.Query().Get("remaining"))
	}
	s.mu.RLock()
	splashCfg, icon, version := s.cfg.Splash, s.cfg.SplashIcon, s.cfg.Version
	s.mu.RUnlock()
	if doc, err := splash.Render(splashCfg, splash.Inputs{Title: title, Version: version, IconSVG: icon, Phase: phase, RemainingMS: remaining}); err == nil {
		html = strings.Replace(html, "</head>", "  "+doc.Head+"\n</head>", 1)
		html = strings.Replace(html, `<div id="splash" hidden></div>`, doc.Body, 1)
	} else {
		log.Printf("splash: %v", err)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
```

Add imports `"strconv"` and `"github.com/polds/tui-streamer/internal/splash"` (`log` is already imported).

- [ ] **Step 5: Expose in `handleConfig`**

Replace the `json.NewEncoder(w).Encode(map[string]any{...})` call with:

```go
	s.mu.RLock()
	sp := s.cfg.Splash.WithDefaults()
	s.mu.RUnlock()
	json.NewEncoder(w).Encode(map[string]any{
		"title":          title,
		"theme":          defaultTheme,
		"themes":         themes,
		"startup_bundle": hasStartup,
		"splash": map[string]any{
			"style":         sp.Style,
			"window":        sp.Window,
			"tagline":       sp.Tagline,
			"accent":        sp.Accent,
			"background":    sp.Background,
			"minDurationMs": sp.MinDuration.Milliseconds(),
		},
	})
```

- [ ] **Step 6: Add the mount point to `web/static/index.html`**

Directly after `<body>`:

```html
<!-- Splash overlay: filled in by the server (see internal/splash). -->
<div id="splash" hidden></div>
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `go vet ./... && go test ./internal/server/`
Expected: `ok`.

- [ ] **Step 8: Commit**

```bash
git add internal/server/server.go internal/server/server_test.go web/static/index.html
git commit -m "feat(server): inject splash overlay and expose splash config"
```

---

### Task 7: Web UI — `Splash` controller in `app.js`

**Files:**
- Modify: `web/static/app.js` (new class before `// ── Bootstrap`, hooks in `App` constructor, `_refresh`, `_subscribe`)
- Modify: `web/static/style.css` (nothing splash-specific is needed — base CSS ships inside the injected `<style>`; add only a guard so `#splash[hidden]` stays hidden if injection fails)

**Interfaces:**
- Consumes: `window.SPLASH` (`phase`, `minDurationMs`, `remainingMs`), `window.splash.dismiss()`, events `splash:animated` / `splash:dismissed`.
- Produces: `App.splash` (`Splash` instance) with `markConnected()`; on `splash:dismissed` the overlay element is removed.

- [ ] **Step 1: Write the controller** (no unit harness for the vanilla JS; verified in Task 12 via browser)

Add before `// ── Bootstrap`:

```js
// ── Splash overlay ──────────────────────────────────────────────────────────
//
// The server injects a #splash overlay (see internal/splash). It is dismissed
// only when all of these hold:
//   1. the page reported `splash:animated` (intro finished),
//   2. the app is connected (sessions listed + first WebSocket open, or none),
//   3. minDuration has elapsed (counted from page start, minus any
//      `remainingMs` a native handoff already spent).
// The shim inside the overlay owns the fade; we just call splash.dismiss().

class Splash {
  constructor() {
    this.$el = document.getElementById('splash');
    this.cfg = window.SPLASH || {};
    this.animated  = false;
    this.connected = false;
    this.done      = false;
    if (!this.$el || this.$el.hidden) { this.done = true; return; }

    const owed = this.cfg.phase === 'final'
      ? (this.cfg.remainingMs || 0)
      : (this.cfg.minDurationMs || 0);
    this._readyAt = performance.now() + Math.max(0, owed);

    document.addEventListener('splash:animated', () => { this.animated = true; this._maybeDismiss(); });
    document.addEventListener('splash:dismissed', () => {
      this.$el?.remove();
      this.$el = null;
    });
  }

  markConnected() {
    this.connected = true;
    this._maybeDismiss();
  }

  _maybeDismiss() {
    if (this.done || !this.animated || !this.connected) return;
    const wait = this._readyAt - performance.now();
    if (wait > 0) { setTimeout(() => this._maybeDismiss(), wait); return; }
    this.done = true;
    if (window.splash) window.splash.dismiss();
    else this.$el?.remove();
  }
}
```

- [ ] **Step 2: Hook it into `App`**

In the `App` constructor, right after `this.lastSelectedElement = null;`:

```js
    this.splash = new Splash();
```

In `_refresh()`, after the `for (const s of this.sessions)` loop that subscribes:

```js
    // No sessions → nothing will ever open a socket; we're as connected as we get.
    if (this.sessions.length === 0) this.splash.markConnected();
```

In `_subscribe(sessionId)`, inside the status callback `(status) => {` as the first statement:

```js
        if (status === 'connected') this.splash.markConnected();
```

- [ ] **Step 3: Guard in `style.css`**

Append:

```css
/* Splash overlay placeholder stays hidden if the server did not fill it. */
#splash[hidden] { display: none !important; }
```

- [ ] **Step 4: Manual check in a browser**

```bash
go build -o dist/tui-streamer ./cmd/server && ./dist/tui-streamer -bundle examples/network-bundle/bundle.yaml -port 18099 -open=false
```

Open `http://localhost:18099/`: the minimal splash shows, fades after ~1.2 s once sessions connect. Open `http://localhost:18099/?splash=final&remaining=0`: no intro animation, fades on connect. In DevTools: `document.getElementById('splash')` is `null` after the fade. Stop the server.

- [ ] **Step 5: Commit**

```bash
git add web/static/app.js web/static/style.css
git commit -m "feat(web): splash overlay controller with readiness protocol"
```

---

### Task 8: `cmd/server` wiring

**Files:**
- Modify: `cmd/server/main.go` (~lines 96–110 where `cfg` is built)

**Interfaces:**
- Consumes: `bundle.File.Splash`, `bundle.LoadIconSVG`, `version`.

- [ ] **Step 1: Wire config**

Replace the `if loaded != nil { cfg.Theme = ...; cfg.Themes = ... }` block with:

```go
	cfg.Version = version
	if loaded != nil {
		cfg.Theme = loaded.Theme
		cfg.Themes = bundle.FilterThemes(loaded.Themes)
		cfg.Splash = loaded.Splash
		cfg.SplashIcon = bundle.LoadIconSVG(*bundlePath, loaded.AppIcon)
	} else {
		cfg.Splash = bundle.DefaultSplash()
	}
```

- [ ] **Step 2: Build and smoke**

Run: `go vet ./... && go build -o dist/tui-streamer ./cmd/server && ./dist/tui-streamer -bundle examples/tui-path/bundle.yaml -port 18099 -open=false & sleep 1; curl -s localhost:18099/api/config; kill %1`
Expected: JSON includes `"splash":{"style":"minimal",…}`.

- [ ] **Step 3: Commit**

```bash
git add cmd/server/main.go
git commit -m "feat(server): pass bundle splash config and icon to the server"
```

---

### Task 9: Native card window — `cmd/app/window_darwin.go`

**Files:**
- Create: `cmd/app/window_darwin.go`

**Interfaces:**
- Produces: `applyCardWindow(win unsafe.Pointer, w, h int, background string)`, `restoreMainWindow(win unsafe.Pointer, w, h int, title string)`. Both must be called on the UI thread (`wv.Dispatch`).

- [ ] **Step 1: Write the file**

```go
//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

static NSRect splash_centered(double w, double h) {
	NSRect screen = [[NSScreen mainScreen] visibleFrame];
	return NSMakeRect(NSMidX(screen) - w / 2, NSMidY(screen) - h / 2, w, h);
}

// Borderless, centred, fixed-size card. The WKWebView content view is kept.
static void splash_apply_card(void *win, double w, double h, int hasColor, double r, double g, double b) {
	NSWindow *window = (NSWindow *)win;
	[window setStyleMask:NSWindowStyleMaskBorderless];
	if (hasColor) {
		[window setBackgroundColor:[NSColor colorWithSRGBRed:r green:g blue:b alpha:1.0]];
	}
	[window setOpaque:YES];
	[window setHasShadow:YES];
	[window setMovableByWindowBackground:YES];
	[window setFrame:splash_centered(w, h) display:YES];
	[window center];
	[window makeKeyAndOrderFront:nil];
}

// Back to a normal titled, resizable window, animating the frame change.
static void splash_restore_main(void *win, double w, double h, const char *title) {
	NSWindow *window = (NSWindow *)win;
	[window setStyleMask:(NSWindowStyleMaskTitled | NSWindowStyleMaskClosable |
	                      NSWindowStyleMaskMiniaturizable | NSWindowStyleMaskResizable)];
	[window setMovableByWindowBackground:NO];
	[window setTitle:[NSString stringWithUTF8String:title]];
	[window setFrame:splash_centered(w, h) display:YES animate:YES];
	[window makeKeyAndOrderFront:nil];
}
*/
import "C"

import (
	"strconv"
	"strings"
	"unsafe"
)

// parseHexColor accepts #rgb / #rrggbb; ok is false for anything else.
func parseHexColor(s string) (r, g, b float64, ok bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	if len(s) != 6 {
		return 0, 0, 0, false
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return 0, 0, 0, false
	}
	return float64(v>>16&0xff) / 255, float64(v>>8&0xff) / 255, float64(v&0xff) / 255, true
}

// applyCardWindow turns the webview's window into a borderless centred card.
func applyCardWindow(win unsafe.Pointer, w, h int, background string) {
	r, g, b, ok := parseHexColor(background)
	has := 0
	if ok {
		has = 1
	}
	C.splash_apply_card(win, C.double(w), C.double(h), C.int(has), C.double(r), C.double(g), C.double(b))
}

// restoreMainWindow returns the window to its normal chrome and size.
func restoreMainWindow(win unsafe.Pointer, w, h int, title string) {
	ct := C.CString(title)
	defer C.free(unsafe.Pointer(ct))
	C.splash_restore_main(win, C.double(w), C.double(h), ct)
}
```

Add `#include <stdlib.h>` to the preamble for `C.free`.

- [ ] **Step 2: Write a test for the pure-Go helper**

`cmd/app/window_darwin_test.go`:

```go
//go:build darwin

package main

import "testing"

func TestParseHexColor(t *testing.T) {
	if r, g, b, ok := parseHexColor("#ff8000"); !ok || r != 1 || g < 0.50 || g > 0.51 || b != 0 {
		t.Errorf("#ff8000 → %v %v %v %v", r, g, b, ok)
	}
	if _, _, _, ok := parseHexColor("#abc"); !ok {
		t.Errorf("#abc should parse")
	}
	if _, _, _, ok := parseHexColor("rgb(1,2,3)"); ok {
		t.Errorf("rgb() is not hex")
	}
}
```

- [ ] **Step 3: Build and test**

Run: `CGO_ENABLED=1 go vet ./cmd/app/ && CGO_ENABLED=1 go test ./cmd/app/`
Expected: `ok` (requires Xcode CLT; `cmd/app` already needs them).

- [ ] **Step 4: Commit**

```bash
git add cmd/app/window_darwin.go cmd/app/window_darwin_test.go
git commit -m "feat(app): cgo helpers for borderless splash card window"
```

---

### Task 10: Native handoff in `cmd/app/main.go`

**Files:**
- Modify: `cmd/app/main.go` (add `version` var; replace the `splashHTML` usage and function; add binds; card/full flow)

**Interfaces:**
- Consumes: `splash.Render`, `bundle.LoadIconSVG`, `applyCardWindow`, `restoreMainWindow`, `waitForServer`.
- Page contract: `window.__splashPost(name)` bound; the UI page's overlay posts `dismissed` through the same bind.

- [ ] **Step 1: Add the version variable** (near the top, after the imports)

```go
// version is set at build time via -ldflags "-X main.version=<tag>" (see Makefile).
var version = "dev"
```

- [ ] **Step 2: Build the splash document and config**

After `effectiveAllow := …` and the `cfg := server.Config{…}` literal, extend the `if b != nil {` block:

```go
	cfg.Version = version
	splashCfg := bundle.DefaultSplash()
	var iconSVG []byte
	if b != nil {
		cfg.Theme = b.Theme
		cfg.Themes = bundle.FilterThemes(b.Themes)
		splashCfg = b.Splash
		iconSVG = bundle.LoadIconSVG(bundle.PackagedPath(), b.AppIcon)
	} else {
		iconSVG = bundle.LoadIconSVG("", "")
	}
	cfg.Splash = splashCfg
	cfg.SplashIcon = iconSVG
```

(Remove the old `if b != nil { cfg.Theme…; cfg.Themes… }` block.)

- [ ] **Step 3: Replace the window/splash section**

Replace everything from `// ── create the native window` through `wv.SetHtml(splashHTML(*title))` and the `go func() { waitForServer…; wv.Dispatch(func(){ wv.Navigate(url) }) }()` block with:

```go
	// ── create the native window ─────────────────────────────────────────────
	wv := webview.New(*debug)
	defer wv.Destroy()

	const mainW, mainH = 1280, 800
	card := splashCfg.Window == bundle.SplashWindowCard
	wv.SetTitle(*title)
	if card {
		wv.SetSize(splashCfg.Size[0], splashCfg.Size[1], webview.HintNone)
		applyCardWindow(wv.Window(), splashCfg.Size[0], splashCfg.Size[1], splashCfg.Background)
	} else {
		wv.SetSize(mainW, mainH, webview.HintNone)
	}

	// Show the splash immediately so the user has feedback while the HTTP
	// server finishes binding its socket.
	doc, err := splash.Render(splashCfg, splash.Inputs{Title: *title, Version: version, IconSVG: iconSVG, Phase: splash.PhaseIntro})
	if err != nil {
		log.Fatalf("splash: %v", err)
	}
	splashShown := time.Now()
	wv.SetHtml(doc.HTML)

	// ── handoff state machine ────────────────────────────────────────────────
	// Navigate to the UI when the splash intro has finished and the server is
	// up (or after 10s regardless). In card mode, restore the main window once
	// the UI's overlay reports it has faded out.
	var (
		handoffMu sync.Mutex
		animated  bool
		serverUp  bool
		navigated bool
	)
	navigate := func() {
		handoffMu.Lock()
		defer handoffMu.Unlock()
		if navigated || !(animated && serverUp) {
			return
		}
		navigated = true
		remaining := splashCfg.MinDuration - time.Since(splashShown)
		if remaining < 0 {
			remaining = 0
		}
		target := url + "/?splash=final&remaining=" + strconv.FormatInt(remaining.Milliseconds(), 10)
		wv.Dispatch(func() { wv.Navigate(target) })
	}
	wv.Bind("__splashPost", func(name string) {
		switch name {
		case "animated":
			handoffMu.Lock()
			animated = true
			handoffMu.Unlock()
			navigate()
		case "dismissed":
			if card {
				wv.Dispatch(func() { restoreMainWindow(wv.Window(), mainW, mainH, *title) })
			}
		}
	})
	go func() {
		waitForServer(url, 10*time.Second)
		handoffMu.Lock()
		serverUp = true
		animated = animated || time.Since(splashShown) > 10*time.Second // give up waiting for the page
		handoffMu.Unlock()
		navigate()
	}()
	// Safety net: a custom page that never reports `animated` still hands off.
	time.AfterFunc(splashCfg.MinDuration+5*time.Second, func() {
		handoffMu.Lock()
		animated = true
		handoffMu.Unlock()
		navigate()
	})
```

Delete the old `splashHTML` function entirely. Add imports `"strconv"`, `"sync"`, `"github.com/polds/tui-streamer/internal/splash"`.

Note the shim's `__splashPost` only exists on pages where webview injected its bindings; `wv.Bind` registers the binding before any navigation so both the splash document and the UI page get it.

- [ ] **Step 4: Build**

Run: `make build-darwin-webview`
Expected: binary at `dist/tui-streamer-darwin-webview`. Run it directly with a bundle to sanity-check before packaging:

```bash
./dist/tui-streamer-darwin-webview -title "Splash Test"
```

Expected: minimal splash, then the UI; no white flash between them (the UI's overlay is the same background).

- [ ] **Step 5: Commit**

```bash
git add cmd/app/main.go
git commit -m "feat(app): render configurable splash and hand off to the UI"
```

---

### Task 11: Packaging — `bundlemeta -splash`, `-splash-assets`, script copy

**Files:**
- Modify: `cmd/bundlemeta/main.go`
- Modify: `scripts/package-macos.sh` (after the `spec.files` staging loop, ~line 140)

**Interfaces:**
- `bundlemeta -splash <bundle.yaml>` → absolute custom html path or empty; `-splash-assets` → one absolute asset path per line. Files are copied into `Contents/Resources/` **preserving their path relative to the bundle file**, so the packaged `bundle.yaml`'s relative `html:` keeps resolving.

- [ ] **Step 1: Extend `bundlemeta`**

Add flags:

```go
	splashHTML := flag.Bool("splash", false, "print the resolved custom splash html path (or empty)")
	splashAssets := flag.Bool("splash-assets", false, "print absolute paths of assets referenced by the custom splash html")
```

Update the usage string to `[-name|-icon|-files|-splash|-splash-assets]`, and add cases (the file is loaded with `bundle.Load`, so `f.Splash.HTML` is already absolute):

```go
	case *splashHTML:
		fmt.Print(f.Splash.HTML)
	case *splashAssets:
		assets, err := splash.Assets(f.Splash)
		if err != nil {
			fmt.Fprintf(os.Stderr, "bundlemeta: %v\n", err)
			os.Exit(1)
		}
		for _, a := range assets {
			fmt.Println(a)
		}
```

Import `"github.com/polds/tui-streamer/internal/splash"`.

- [ ] **Step 2: Copy in `package-macos.sh`**

After the `spec.files` loop (just before the closing `fi` of `if command -v go >/dev/null; then`):

```bash
    # Custom splash page + assets, keeping paths relative to the bundle so the
    # packaged bundle.yaml's `html:` still resolves under Resources/.
    bundle_dir="$(dirname "${BUNDLE_FILE}")"
    splash_html="$(cd "${REPO_ROOT}" && go run ./cmd/bundlemeta -splash "${BUNDLE_FILE}" 2>/dev/null || true)"
    if [[ -n "${splash_html}" ]]; then
      { echo "${splash_html}"; cd "${REPO_ROOT}" && go run ./cmd/bundlemeta -splash-assets "${BUNDLE_FILE}"; } | while IFS= read -r src; do
        [[ -z "${src}" ]] && continue
        rel="${src#"${bundle_dir}"/}"
        if [[ "${rel}" == "${src}" ]]; then
          echo "Error: splash asset ${src} is outside the bundle directory" >&2
          exit 1
        fi
        mkdir -p "${RESOURCES_DIR}/$(dirname "${rel}")"
        cp "${src}" "${RESOURCES_DIR}/${rel}"
        echo "  ✓ Bundled splash: ${rel} → Contents/Resources/${rel}"
      done
    fi
```

- [ ] **Step 3: Verify with a throwaway custom splash**

```bash
mkdir -p /tmp/splashpkg/art && cp examples/tui-path/bundle.yaml /tmp/splashpkg/ && cp examples/tui-path/icon.svg /tmp/splashpkg/ && cp -R examples/tui-path/bin /tmp/splashpkg/
cat > /tmp/splashpkg/splash.html <<'EOF'
<html><head><style>#splash{display:grid;place-items:center}</style></head>
<body><img src="art/logo.svg" width="120"><script>setTimeout(()=>splash.animated(),800)</script></body></html>
EOF
cp examples/tui-path/icon.svg /tmp/splashpkg/art/logo.svg
sed -i '' 's|  appIcon: ./icon.svg|  appIcon: ./icon.svg\n  splash:\n    html: ./splash.html\n    window: card|' /tmp/splashpkg/bundle.yaml
go run ./cmd/bundlemeta -splash /tmp/splashpkg/bundle.yaml; echo; go run ./cmd/bundlemeta -splash-assets /tmp/splashpkg/bundle.yaml
make app BUNDLE=/tmp/splashpkg/bundle.yaml
ls "dist/TUI Path Demo.app/Contents/Resources/" "dist/TUI Path Demo.app/Contents/Resources/art"
open "dist/TUI Path Demo.app"
```

Expected: `splash.html` and `art/logo.svg` under `Resources/`; the app opens as a borderless card showing the custom page, then grows into the main window.

- [ ] **Step 4: Commit**

```bash
git add cmd/bundlemeta/main.go scripts/package-macos.sh
git commit -m "feat(packaging): bundle custom splash html and assets into Resources"
```

---

### Task 12: Examples, docs, and manual verification

**Files:**
- Modify: `examples/tui-path/bundle.yaml`, `examples/tui-path/README.md`, `examples/network-bundle/bundle.yaml`
- Modify: `README.md` (BundleSet schema block ~line 246 and a new `#### Splash` subsection after `#### Themes`), `CLAUDE.md` (BundleSet schema ~line 268, `internal/splash` in the repo tree and architecture list, "Adding a new splash style" convention)

- [ ] **Step 1: Examples**

`examples/tui-path/bundle.yaml` metadata:

```yaml
metadata:
  name: TUI Path Demo
  appIcon: ./icon.svg
  splash:
    style: arc
    tagline: Bundled tools, zero setup
    accent: "#88c0d0"
    background: "#2e3440"
```

`examples/network-bundle/bundle.yaml` metadata:

```yaml
metadata:
  name: Network Troubleshooting
  splash:
    style: jetbrains          # implies window: card
    tagline: Connectivity & DNS diagnostics
    accent: "#bd93f9"
    background: "#282a36"
    minDuration: 1500ms
```

Add a `splash` row to the table in `examples/tui-path/README.md`:

```
| `metadata.splash` | Arc-style animated splash (`style: arc`) shared by the .app and the browser |
```

- [ ] **Step 2: README schema + subsection**

In the BundleSet YAML block add after `appIcon`:

```yaml
  splash:                      # optional startup splash (see "Splash" below)
    style: jetbrains           # minimal | arc | dia | jetbrains
    window: card               # full | card (jetbrains defaults to card)
    tagline: Connectivity & DNS diagnostics
    accent: "#bd93f9"
    background: "#282a36"
    minDuration: 1500ms
```

New subsection after `#### Themes`:

```markdown
#### Splash

`metadata.splash` configures an animated splash shown while the app starts —
in the native `.app` (before the server is even up) and as an overlay in the
browser UI. Built-in styles: `minimal` (default), `arc` and `dia`
(full-window), `jetbrains` (borderless centred card that grows into the main
window). `html: ./splash.html` replaces the style with your own page: relative
images, fonts and stylesheets are inlined, and the page must call
`splash.animated()` when its intro is done (or it is assumed done after 5 s).

The splash is dismissed only when its intro animation has finished **and** the
UI is connected **and** `minDuration` (default `1200ms`) has elapsed, then it
fades out over 400 ms. `GET /api/config` exposes the non-HTML fields.
```

- [ ] **Step 3: CLAUDE.md**

- Repo tree: add `│   ├── splash/              # Splash renderer: built-in styles, custom html, readiness shim` under `internal/`.
- Architecture list: add item **Splash** (`internal/splash/`) — one sentence per the README subsection, plus: native host = `cmd/app` (`SetHtml` + cgo card window), browser host = `handleIndex` overlay + `app.js` `Splash` controller; protocol `animated → dismiss → dismissed`.
- BundleSet schema block: same `splash:` snippet as README.
- Key Conventions: add "**Adding a new splash style**: 1. add `internal/splash/templates/<name>.html` (a `<style>` block then the `#splash` div; mark intro-animated elements with `data-splash-intro` and gate their keyframes on `[data-phase="intro"]`); 2. add the name to the `style` switch in `internal/bundle/splash.go`; 3. add it to `TestRenderAllBuiltinStyles`."
- Session-independent note in Security: custom `splash.html` scripts run inside the UI page in browser mode — trusted bundle content, like `command:`.

- [ ] **Step 4: Manual verification matrix**

```bash
make icon BUNDLE=./examples/tui-path/bundle.yaml && make app BUNDLE=./examples/tui-path/bundle.yaml && open "dist/TUI Path Demo.app"
```
Expected: normal-sized window, Arc mesh + icon rise, crossfade into the UI after ≥1.2 s with no white flash.

```bash
make app BUNDLE=./examples/network-bundle/bundle.yaml && open "dist/Network Troubleshooting.app"
```
Expected: 640×400 borderless card centred on screen with the JetBrains layout and version; after ≥1.5 s and connect, the card fades and the window animates to 1280×800 with a title bar.

```bash
./dist/tui-streamer -bundle examples/network-bundle/bundle.yaml -port 18099 -open=false
```
In Chrome at `http://localhost:18099/`: JetBrains splash as a full-page overlay, removed after connect. Check DevTools console for errors. Then with the WebSocket blocked (DevTools → Network → block `ws://localhost:18099/ws/*`) reload: the overlay must **stay** (animated but never connected). Unblock: it fades.

- [ ] **Step 5: Run everything and commit**

Run: `go vet ./... && go test ./... && CGO_ENABLED=1 go vet ./cmd/app/`
Expected: all `ok`.

```bash
git add examples README.md CLAUDE.md
git commit -m "docs: splash schema, examples and conventions"
```

---

## Self-review

**Spec coverage**
- §1 schema/validation/defaults/BundleSet precedence/`html` containment → Task 1. `/api/config.splash` → Task 6.
- §2 `Render`/`Inputs`/`Document`, built-in styles, final phase, custom HTML + inlining + 10 MiB, shim protocol, auto-`animated` 5 s, 400 ms fade, reduced motion → Tasks 3–5 (shim handles reduced motion and the caps).
- §3 native: `SetHtml` first, cgo card/restore, binds, handoff with `remaining`, 10 s timeout → Tasks 9–10. (Amended: single fade in the UI overlay; native does not fade the standalone document.)
- §4 overlay injection, `?splash=final`, `Splash` controller, `connected` definition, `#splash[hidden]` guard → Tasks 6–7.
- §5 `bundlemeta -splash/-splash-assets`, copy preserving relative paths, examples, docs → Tasks 11–12.
- §6 tests: bundle (T1–2), splash (T3–5), server (T6), cgo helper (T9), manual matrix incl. blocked-WebSocket check (T12).

**Placeholders:** Task 4 Step 7's preview mention is instructions for a throwaway file, not production code; everything else has concrete code.

**Type consistency:** `bundle.SplashConfig` fields, `splash.Inputs{Title, Version, IconSVG, Phase, RemainingMS}`, `Document{HTML, Head, Body}`, `Phase` constants, `applyCardWindow(win, w, h, background)`, `restoreMainWindow(win, w, h, title)`, `__splashPost(name)`, events `splash:animated` / `splash:dismissed` are used with the same names throughout.
