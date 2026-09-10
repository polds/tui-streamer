// Package server provides the HTTP and WebSocket handlers for tui-streamer.
package server

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/polds/tui-streamer/internal/bundle"
	"github.com/polds/tui-streamer/internal/session"
	"github.com/polds/tui-streamer/internal/splash"
)

// maxBundleBodyBytes caps the YAML body accepted by /api/bundles (4 MiB).
const maxBundleBodyBytes = 4 << 20

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
	// Allow all origins for local/dev use. Restrict in production via Config.
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Config holds server-level defaults applied to every exec request.
type Config struct {
	// Title is the dynamic text to display in the web UI header and browser tab title.
	Title string
	// Stdout / Stderr control which streams are captured (default: both true).
	Stdout bool
	Stderr bool
	// Dir is the default working directory for executed commands.
	Dir string
	// AllowedCommands is an optional whitelist of binary names (first token of
	// the command slice). An empty slice means all commands are permitted.
	AllowedCommands []string
	// HasStartupBundle indicates if a bundle was loaded on server startup.
	HasStartupBundle bool
	// Theme is the default UI theme name. Empty uses the built-in default.
	Theme string
	// Themes, when non-empty, is the allowlist of theme names shown in the UI.
	Themes []string
	// TUIPath is exported as TUI_PATH on executed commands. Empty means unset.
	TUIPath string
	// Splash configures the startup splash (zero value = built-in minimal).
	Splash bundle.SplashConfig
	// SplashIcon is the app icon SVG inlined into the splash, or nil.
	SplashIcon []byte
	// Version is shown on splash styles that display it.
	Version string
}

// Server wires together the session manager and HTTP mux.
type Server struct {
	manager *session.Manager
	cfg     Config
	mu      sync.RWMutex
	mux     *http.ServeMux
}

// New creates a Server. Call Handler() to obtain the http.Handler.
func New(manager *session.Manager, cfg Config, staticFS fs.FS) *Server {
	s := &Server{
		manager: manager,
		cfg:     cfg,
		mux:     http.NewServeMux(),
	}
	s.routes(staticFS)
	return s
}

// Handler returns the root HTTP handler.
func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) routes(staticFS fs.FS) {
	s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			s.handleIndex(w, r, staticFS)
			return
		}
		http.FileServer(http.FS(staticFS)).ServeHTTP(w, r)
	})
	s.mux.HandleFunc("/ws/", s.handleWebSocket)
	s.mux.HandleFunc("/api/sessions", s.handleSessions)
	s.mux.HandleFunc("/api/sessions/", s.handleSession)
	s.mux.HandleFunc("/api/bundles", s.handleBundles)
	s.mux.HandleFunc("/api/config", s.handleConfig)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request, staticFS fs.FS) {
	f, err := staticFS.Open("index.html")
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	defer f.Close()

	b, err := io.ReadAll(f)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	html := string(b)
	title := s.cfg.Title
	if title == "" {
		title = "tui-streamer"
	}

	html = strings.Replace(html, "<title>tui-streamer</title>", "<title>"+title+"</title>", 1)
	html = strings.Replace(html, "tui-streamer\n  </div>", title+"\n  </div>", 1)

	// Inject STARTUP_BUNDLE so the frontend knows whether to show the Import button.
	s.mu.RLock()
	hasStartup := s.cfg.HasStartupBundle
	s.mu.RUnlock()
	startupBundleScript := fmt.Sprintf("<script>window.STARTUP_BUNDLE = %v;</script>", hasStartup)
	html = strings.Replace(html, "</head>", "  "+startupBundleScript+"\n</head>", 1)

	// Inject THEME_CONFIG so the first paint honors the bundle theme allowlist.
	defaultTheme, themes := s.themeConfig()
	themePayload, err := json.Marshal(map[string]any{
		"default": defaultTheme,
		"themes":  themes,
	})
	if err != nil {
		themePayload = []byte(`{}`)
	}
	themeScript := fmt.Sprintf("<script>window.THEME_CONFIG = %s;</script>", themePayload)
	html = strings.Replace(html, "</head>", "  "+themeScript+"\n</head>", 1)

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
}

// ── WebSocket ──────────────────────────────────────────────────────────────

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/ws/")
	sess, ok := s.manager.Get(id)
	if !ok {
		http.Error(w, `{"error":"session not found"}`, http.StatusNotFound)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws upgrade: %v", err)
		return
	}

	client := session.NewClient(conn, sess)
	go client.Run()
}

// ── REST: /api/sessions ────────────────────────────────────────────────────

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.Method {
	case http.MethodGet:
		json.NewEncoder(w).Encode(s.manager.List())

	case http.MethodPost:
		var req struct {
			Name string `json:"name"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if req.Name == "" {
			req.Name = "session"
		}
		sess := s.manager.Create(req.Name, "")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(sess.Info())

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// ── REST: /api/sessions/{id}[/exec|/kill] ─────────────────────────────────

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	path := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	parts := strings.SplitN(path, "/", 2)
	id := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	sess, ok := s.manager.Get(id)
	if !ok {
		http.Error(w, `{"error":"session not found"}`, http.StatusNotFound)
		return
	}

	switch action {
	case "":
		s.handleSessionRoot(w, r, sess, id)
	case "exec":
		s.handleExec(w, r, sess)
	case "kill":
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		sess.Kill()
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, `{"error":"unknown action"}`, http.StatusNotFound)
	}
}

func (s *Server) handleSessionRoot(w http.ResponseWriter, r *http.Request, sess *session.Session, id string) {
	switch r.Method {
	case http.MethodGet:
		json.NewEncoder(w).Encode(sess.Info())
	case http.MethodDelete:
		if err := s.manager.Delete(id); err != nil {
			http.Error(w, `{"error":"session not found"}`, http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleExec(w http.ResponseWriter, r *http.Request, sess *session.Session) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Command json.RawMessage `json:"command"`
		Dir     string          `json:"dir"`
		Env     []string        `json:"env"`
		Stdout  *bool           `json:"stdout"`
		Stderr  *bool           `json:"stderr"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	// Accept command as either a JSON array (["cmd","arg"]) or a plain string
	// ("cmd arg") which is split on whitespace.
	var command []string
	if len(req.Command) > 0 {
		if err := json.Unmarshal(req.Command, &command); err != nil {
			var s string
			if err2 := json.Unmarshal(req.Command, &s); err2 != nil {
				http.Error(w, `{"error":"command must be a string or array of strings"}`, http.StatusBadRequest)
				return
			}
			command = strings.Fields(s)
		}
	}
	if len(command) == 0 {
		http.Error(w, `{"error":"command is required"}`, http.StatusBadRequest)
		return
	}

	opts := s.execOptions(command)
	if req.Dir != "" {
		opts.Dir = req.Dir
	}
	if len(req.Env) > 0 {
		opts.Env = req.Env
	}

	// Whitelist check (after TUI_PATH expansion so basename matching works).
	if !bundle.CommandAllowed(s.allowed(), opts.Command) {
		http.Error(w, `{"error":"command not allowed"}`, http.StatusForbidden)
		return
	}
	// Per-request overrides for stdout/stderr capture.
	if req.Stdout != nil {
		opts.Stdout = *req.Stdout
	}
	if req.Stderr != nil {
		opts.Stderr = *req.Stderr
	}

	if err := sess.Exec(opts); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte(`{"status":"started"}`))
}

// ── REST: /api/bundles ──────────────────────────────────────────────────────

// handleBundles accepts a YAML bundle file (kind: Bundle or kind: BundleSet
// with inline Bundle documents) and creates the declared sessions.
func (s *Server) handleBundles(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxBundleBodyBytes))
	if err != nil {
		http.Error(w, `{"error":"could not read request body"}`, http.StatusBadRequest)
		return
	}

	f, err := bundle.Parse(body)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Check for already-imported bundles (by name) before creating anything.
	existing := s.manager.List()
	for _, b := range f.Bundles {
		for _, sess := range existing {
			if sess.BundleName == b.Name {
				writeJSONError(w, http.StatusConflict, `bundle "`+b.Name+`" already imported`)
				return
			}
		}
	}

	s.ImportFile(f)

	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(`{"status":"imported"}`))
}

// ── REST: /api/config ──────────────────────────────────────────────────────

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	defaultTheme, themes := s.themeConfig()
	s.mu.RLock()
	title := s.cfg.Title
	hasStartup := s.cfg.HasStartupBundle
	s.mu.RUnlock()
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
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
