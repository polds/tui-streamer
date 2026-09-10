package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gorilla/websocket"
	"github.com/polds/tui-streamer/internal/session"
)

func testStaticFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html": &fstest.MapFile{
			Data: []byte("<html><head><title>tui-streamer</title></head><body><div class=\"header-logo\">\n    tui-streamer\n  </div></body></html>"),
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

// replayLines runs cmd in a fresh session over a live test server, waits for
// it to finish, then returns the replayed output frames a late WebSocket
// subscriber receives.
func replayLines(t *testing.T, cmd string) []map[string]any {
	t.Helper()
	mgr := session.NewManager()
	sess := mgr.Create("t", "")
	srv := httptest.NewServer(New(mgr, Config{Stdout: true, Stderr: true}, testStaticFS()).Handler())
	t.Cleanup(srv.Close)

	res, err := http.Post(srv.URL+"/api/sessions/"+sess.ID+"/exec", "application/json",
		strings.NewReader(`{"command":`+strconv.Quote(cmd)+`}`))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("exec status = %d", res.StatusCode)
	}
	deadline := time.Now().Add(5 * time.Second)
	for sess.Info().Running && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}

	ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/ws/"+sess.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	var lines []map[string]any
	for {
		ws.SetReadDeadline(time.Now().Add(2 * time.Second))
		var m map[string]any
		if err := ws.ReadJSON(&m); err != nil {
			break
		}
		lines = append(lines, m)
		if m["type"] == "exit" {
			break
		}
	}
	return lines
}

func TestExecStringCommandHonoursQuotes(t *testing.T) {
	lines := replayLines(t, `sh -c 'echo one two'`)
	var sawOutput bool
	for _, l := range lines {
		if l["type"] == "stdout" && l["data"] == "one two" {
			sawOutput = true
		}
		if l["type"] == "exit" && l["exit_code"] != float64(0) {
			t.Fatalf("command exited %v — the quoted argument was split into separate words", l["exit_code"])
		}
	}
	if !sawOutput {
		t.Fatalf("expected stdout %q, got %v", "one two", lines)
	}
}

func TestExecStringCommandUnterminatedQuoteIs400(t *testing.T) {
	mgr := session.NewManager()
	sess := mgr.Create("t", "")
	s := New(mgr, Config{}, testStaticFS())

	body := `{"command":"sh -c 'echo oops"}`
	req := httptest.NewRequest(http.MethodPost, "/api/sessions/"+sess.ID+"/exec", strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
	}
}
