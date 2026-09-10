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
	wantFontCSS, err := filepath.EvalSymlinks(filepath.Join(filepath.Dir(c.HTML), "font.css"))
	if err != nil {
		t.Fatal(err)
	}
	wantLogo, err := filepath.EvalSymlinks(filepath.Join(filepath.Dir(c.HTML), "img", "logo.svg"))
	if err != nil {
		t.Fatal(err)
	}
	// Expected paths are symlink-resolved because resolve() now evaluates
	// symlinks for containment checking and returns the real path (e.g. on
	// macOS, t.TempDir() lives under /var, itself a symlink to /private/var).
	want := map[string]bool{wantFontCSS: true, wantLogo: true}
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

func TestCustomPageKeepsAbsoluteStylesheetLink(t *testing.T) {
	c := writeCustom(t, `<html><head><link rel="stylesheet" href="https://fonts.example/x.css"></head><body>hi</body></html>`)
	doc, err := Render(c, Inputs{Title: "T"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc.Head, `<link rel="stylesheet" href="https://fonts.example/x.css">`) {
		t.Errorf("absolute stylesheet link should be preserved, got head: %s", doc.Head)
	}
}

func TestCustomPageRejectsSymlinkEscape(t *testing.T) {
	c := writeCustom(t, `<html><body><img src="img/leak.svg"></body></html>`)
	outsideDir := t.TempDir()
	outside := filepath.Join(outsideDir, "outside.txt")
	if err := os.WriteFile(outside, []byte("leaked"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(filepath.Dir(c.HTML), "img", "leak.svg")
	if err := os.Symlink(outside, link); err != nil {
		if os.IsPermission(err) {
			t.Skip("symlink not permitted in this environment")
		}
		t.Fatal(err)
	}
	if _, err := Render(c, Inputs{Title: "T"}); err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("expected escape error, got %v", err)
	}
}
