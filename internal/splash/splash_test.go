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
