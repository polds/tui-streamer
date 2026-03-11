// Package server provides the HTTP and WebSocket handlers for tui-streamer.
package server

import (
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/polds/tui-streamer/internal/executor"
	"github.com/polds/tui-streamer/internal/session"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
	// Allow all origins for local/dev use. Restrict in production via Config.
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Config holds server-level defaults applied to every exec request.
type Config struct {
	// Stdout / Stderr control which streams are captured (default: both true).
	Stdout bool
	Stderr bool
	// Dir is the default working directory for executed commands.
	Dir string
	// AllowedCommands is an optional whitelist of binary names (first token of
	// the command slice). An empty slice means all commands are permitted.
	AllowedCommands []string
}

// Server wires together the session manager and HTTP mux.
type Server struct {
	manager *session.Manager
	cfg     Config
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
	s.mux.Handle("/", http.FileServer(http.FS(staticFS)))
	s.mux.HandleFunc("/ws/", s.handleWebSocket)
	s.mux.HandleFunc("/api/sessions", s.handleSessions)
	s.mux.HandleFunc("/api/sessions/", s.handleSession)
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
		sess := s.manager.Create(req.Name)
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

	// Whitelist check.
	if len(s.cfg.AllowedCommands) > 0 {
		allowed := false
		for _, a := range s.cfg.AllowedCommands {
			if a == command[0] {
				allowed = true
				break
			}
		}
		if !allowed {
			http.Error(w, `{"error":"command not allowed"}`, http.StatusForbidden)
			return
		}
	}

	opts := executor.Options{
		Command: command,
		Dir:     req.Dir,
		Env:     req.Env,
		Stdout:  s.cfg.Stdout,
		Stderr:  s.cfg.Stderr,
	}
	if opts.Dir == "" {
		opts.Dir = s.cfg.Dir
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
