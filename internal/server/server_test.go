package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/polds/tui-streamer/internal/bundle"
	"github.com/polds/tui-streamer/internal/session"
)

func testStaticFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html": &fstest.MapFile{
			Data: []byte("<html><head><title>tui-streamer</title></head><body><div id=\"splash\" hidden></div><div class=\"header-logo\">\n    tui-streamer\n  </div></body></html>"),
		},
	}
}

func TestHandleConfigAndThemeInjection(t *testing.T) {
	mgr := session.NewManager()
	s := New(mgr, Config{
		Title:  "Network Troubleshooting",
		Theme:  "nord",
		Themes: []string{"nord", "not-a-theme"},
	}, testStaticFS())

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("config status %d", rec.Code)
	}
	var cfg map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&cfg); err != nil {
		t.Fatal(err)
	}
	if cfg["theme"] != "nord" {
		t.Errorf("theme = %#v", cfg["theme"])
	}
	themes, _ := cfg["themes"].([]any)
	if len(themes) != 1 || themes[0] != "nord" {
		t.Errorf("themes = %#v (unknown names should be dropped)", cfg["themes"])
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	body, _ := io.ReadAll(rec.Body)
	html := string(body)
	if !strings.Contains(html, "window.THEME_CONFIG") {
		t.Fatal("index.html missing THEME_CONFIG")
	}
	if !strings.Contains(html, `"default":"nord"`) {
		t.Fatalf("THEME_CONFIG default missing: %s", html)
	}
}

func TestExecAllowlist(t *testing.T) {
	mgr := session.NewManager()
	sess := mgr.Create("t", "")
	s := New(mgr, Config{AllowedCommands: []string{"echo"}}, testStaticFS())

	body := `{"command":"false"}`
	req := httptest.NewRequest(http.MethodPost, "/api/sessions/"+sess.ID+"/exec", strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

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
