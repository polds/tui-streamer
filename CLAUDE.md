# CLAUDE.md — AI Assistant Guide for tui-streamer

## Project Overview

**tui-streamer** is a Go-based WebSocket server that executes OS commands and streams their output (stdout/stderr) line-by-line to a web-based terminal UI in real time. It supports multiple concurrent named "sessions", each of which can have multiple WebSocket subscriber clients.

Key capabilities:
- Execute arbitrary CLI commands on demand via REST API
- Stream output line-by-line via WebSocket in JSON format
- Serve a self-contained, theme-able web UI (no build step required)
- Optional command whitelisting for security
- Native macOS `.app`/`.dmg` packaging via WKWebView (darwin only)

---

## Repository Structure

```
tui-streamer/
├── cmd/
│   ├── app/main.go          # macOS native WebView app entry point (darwin + CGO only)
│   └── server/main.go       # HTTP/WebSocket server entry point
├── internal/
│   ├── browser/open.go      # Cross-platform browser launcher
│   ├── bundle/bundle.go     # YAML bundle parser (Bundle / BundleSet kinds)
│   ├── executor/executor.go # Command execution engine (streaming output)
│   ├── server/server.go     # HTTP routes and WebSocket upgrade handler
│   └── session/
│       ├── manager.go       # Thread-safe session registry
│       ├── session.go       # Session state + client broadcast
│       └── client.go        # WebSocket client read/write pumps
├── web/
│   ├── embed.go             # Go embed directive for static assets
│   └── static/
│       ├── index.html       # Web UI markup
│       ├── style.css        # Styling with 10 themes
│       └── app.js           # Vanilla JS frontend (no framework/build step)
├── build/darwin/
│   ├── Info.plist           # macOS app bundle metadata
│   └── entitlements.plist   # macOS code signing entitlements
├── examples/
│   ├── README.md            # Examples overview and contribution guide
│   ├── lorem-ipsum/         # API-fetch + streaming example
│   └── network-bundle/      # Multi-bundle network diagnostic example
├── scripts/
│   ├── package-macos.sh     # macOS .app/.dmg packaging script
│   └── make-icon.sh         # Generate AppIcon.icns from SVG source
├── .github/workflows/
│   ├── ci.yml               # Go build + lint CI
│   ├── release.yml          # Binary release workflow
│   └── release-please.yml   # Automated release PR management
├── go.mod                   # Go module (github.com/polds/tui-streamer)
├── go.sum                   # Dependency checksums
├── Makefile                 # Build, test, lint, packaging targets
└── README.md                # User-facing project documentation
```

---

## Architecture

### Backend (Go)

The backend uses a **session-based multiplexing** model:

1. **Bundle** (`internal/bundle/bundle.go`) — Parses YAML bundle files into an in-memory `File` struct. Supports two document kinds: `Bundle` (a named list of sessions with optional commands, descriptions, and autorun flags) and `BundleSet` (an ordered reference to multiple `Bundle` documents within the same file). Multi-document YAML is parsed with `gopkg.in/yaml.v3`'s streaming decoder.
2. **Session** (`internal/session/session.go`) — Named execution context. Holds state (ID, name, timestamps, running flag), a map of subscribed WebSocket clients, and a cancel function for the running process. Also stores optional bundle metadata (`BundleName`, `PendingCommand`, `Description`) and a `lineBuf` ring of up to 2,000 recent output lines replayed to clients that connect after execution starts.
3. **Manager** (`internal/session/manager.go`) — Thread-safe registry (UUID → `*Session`). Provides Create/Get/List/Delete. List returns `Info` snapshots sorted by creation time for stable ordering.
4. **Executor** (`internal/executor/executor.go`) — Spawns a process, reads stdout/stderr concurrently in separate goroutines, and emits `Line` structs (JSON) with Unix-millisecond timestamps and line type (`stdout`, `stderr`, `start`, `exit`, `error`). Uses `cmd.WaitDelay` (5s) to force-close pipes after context cancellation, preventing goroutine leaks.
5. **Client** (`internal/session/client.go`) — Wraps a `gorilla/websocket` connection with read/write pumps, a 256-element buffered send channel, ping/pong keepalive (54s interval, 60s read deadline), and `sync.Once`-guarded cleanup. Messages are dropped (non-blocking send) if the buffer is full rather than blocking the broadcaster.
6. **Server** (`internal/server/server.go`) — HTTP mux with:
 - `GET /` — serves embedded static files; injects `<title>` and `window.STARTUP_BUNDLE` into `index.html` at runtime
 - `GET /ws/{id}` — upgrades to WebSocket, creates a Client, registers it to the session
 - `GET /api/sessions` — list all sessions
 - `POST /api/sessions` — create session
 - `GET /api/sessions/{id}` — get single session info
 - `DELETE /api/sessions/{id}` — delete session
 - `POST /api/sessions/{id}/exec` — execute command in session
 - `POST /api/sessions/{id}/kill` — kill running process
 - `POST /api/bundles` — parse a YAML bundle body and create the declared sessions

### Frontend (Vanilla JS)

`web/static/app.js` is a single-file application with no build step:

- **`AnsiParser`** — Converts ANSI SGR escape sequences to safe HTML spans (supports bold, dim, italic, underline, blink, standard/256/true-color fg & bg). All text is HTML-escaped before wrapping in spans.
- **`api`** — Thin wrapper over `fetch()` for all REST endpoints, including bundle import via `POST /api/bundles`.
- **`SessionSocket`** — WebSocket wrapper with 500ms auto-reconnect on disconnect.
- **`Terminal`** — Renders output lines with auto-scroll (pauses when user scrolls up). Prunes the oldest output lines once the DOM exceeds `MAX_DOM_LINES` to avoid unbounded memory growth.
- **`App`** — Main controller: session creation/deletion, command dispatch, theme persistence (localStorage), per-session output buffering for replay. Reads `window.STARTUP_BUNDLE` (injected server-side) to hide the Import button when a bundle was loaded at startup.

### Data Flow

```
Browser (REST) → POST /api/sessions/{id}/exec
                       ↓
               server.go: validate, call session.Exec()
                       ↓
               executor.go: spawn process, read stdout/stderr
                       ↓
               session.go: broadcast Line JSON to all clients
                       ↓
               client.go: write to WebSocket send channel
                       ↓
               Browser (WebSocket) receives Line JSON → Terminal renders
```

---

## Development Workflows

### Prerequisites

- Go 1.22+
- `make`
- macOS with Xcode (only for the native app target)

### Common Commands

```bash
# Build for current platform
make build          # outputs dist/tui-streamer

# Run tests
make test           # go test ./...

# Lint
make lint           # go vet ./...

# macOS-specific
make build-darwin   # universal binary (arm64 + amd64 via lipo)
make app            # create .app with native WKWebView window (CGO, macOS only)
make app-server     # create headless .app that opens the UI in the default browser
make dmg            # create distributable .dmg (requires 'make app' first)

# Package a bundle as a standalone app (app name taken from BundleSet/Bundle name)
make app BUNDLE=./examples/network-bundle/bundle.yaml

# Clean
make clean
```

### Running the Server

```bash
./dist/tui-streamer [flags]

Flags:
  -port string    TCP port to listen on (default: "8080")
  -dir string     Default working directory for executed commands (default: ".")
  -title string   Window / browser-tab title (defaults to tui-streamer or bundle name)
  -stdout         Capture stdout (default true)
  -stderr         Capture stderr (default true)
  -allow string   Whitelist a binary name; repeat for multiple (omit to allow all)
                  e.g. -allow make -allow npm
  -bundle string  Path to a YAML bundle file that pre-creates sessions on startup
  -open           Auto-launch browser on startup
```

The `-allow` flag uses a **repeatable** pattern: specify it multiple times, once per allowed binary. The whitelist check matches only the first token of the command (the binary name), not subcommands or arguments.

When `-bundle` is used, the bundle's `metadata.name` is used as the default window title, and the UI's Import button is hidden (controlled by `window.STARTUP_BUNDLE` injected into the HTML).

**macOS app bundle behaviour:** When the binary detects it is running inside a `.app` bundle (via path inspection for `.app/Contents/MacOS/`), `-open` defaults to `true`. If the port is already taken, the app opens the browser to the existing instance and exits cleanly instead of reporting an error.

### Adding a New REST Endpoint

1. Add the route in `internal/server/server.go` inside the `routes()` method.
2. Write the handler as a method on `*Server`.
3. Access `s.manager` for session operations and `s.cfg` for server-level config.
4. Respond with JSON using `json.NewEncoder(w).Encode(...)`.
5. Set `w.Header().Set("Content-Type", "application/json")` at the top of the handler.

### Adding a New Session Operation

1. Add method to `Session` in `internal/session/session.go`.
2. If it mutates shared state, protect with `s.mu` (RWMutex).
3. Wire it through a new REST endpoint in `server.go` if external access is needed.

### Exec Request Body Reference

`POST /api/sessions/{id}/exec` accepts JSON with the following fields:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `command` | string or `[]string` | yes | Command to run. A plain string is split on whitespace; a JSON array is used as-is. |
| `dir` | string | no | Working directory override. Falls back to the server's `-dir` flag. |
| `env` | `[]string` | no | Extra environment variables in `KEY=VALUE` format. Replaces the inherited environment entirely if provided. |
| `stdout` | bool | no | Per-request stdout capture override. Falls back to the server's `-stdout` flag. |
| `stderr` | bool | no | Per-request stderr capture override. Falls back to the server's `-stderr` flag. |

Example — run `ls -la` with a custom working directory:

```bash
curl -X POST http://localhost:8080/api/sessions/{id}/exec \
  -H "Content-Type: application/json" \
  -d '{"command": ["ls", "-la"], "dir": "/tmp"}'
```

Returns `202 Accepted` with `{"status":"started"}` on success, or `409 Conflict` if a command is already running in that session.

---

## Key Conventions

### Go Style

- **Error handling**: wrap errors with `fmt.Errorf("context: %w", err)`.
- **Concurrency**: use `sync.RWMutex` for shared maps/state; `sync.Once` for one-time teardown; `context.Context` for cancellation.
- **Defers**: use `defer` for cleanup (mutex unlock, channel close, process teardown).
- **No global state** in internal packages — all state is injected via structs.
- Section separators in longer files use `// ──────` style comment lines.
- Pointer receivers on all non-trivial structs.

### JavaScript Style

- Vanilla JS, no framework, no build step — keep it that way.
- Classes for stateful components (`App`, `Terminal`, `SessionSocket`, `AnsiParser`).
- Always HTML-escape user/command output before inserting into the DOM.
- Theme names are CSS class names applied to `<body>`; add new themes in `style.css` using CSS custom properties.

### Adding a New Theme

1. Add a `body.theme-<name>` block in `web/static/style.css` defining all CSS variables (see existing themes for the full variable list).
2. Add the option to the `<select id="themeSelect">` in `web/static/index.html`.
3. No JS changes needed — the `App` class reads the selector value and applies it as a body class.

### WebSocket Protocol

Messages are JSON objects delivered as WebSocket text frames. The `timestamp` field is a Unix epoch in **milliseconds** (int64), not an ISO-8601 string.

```json
{"type":"start","timestamp":1704067200000}
{"type":"stdout","timestamp":1704067200100,"data":"hello"}
{"type":"stderr","timestamp":1704067200200,"data":"error text"}
{"type":"exit","timestamp":1704067200300,"exit_code":0}
{"type":"error","timestamp":1704067200400,"data":"signal: killed"}
```

Field reference:

| Field | Type | Present on | Description |
|-------|------|------------|-------------|
| `type` | string | all | One of `start`, `stdout`, `stderr`, `exit`, `error` |
| `timestamp` | int64 | all | Unix time in milliseconds |
| `data` | string | stdout, stderr, error | Line text (no trailing newline) |
| `exit_code` | int | exit | Process exit code (0 = success) |

Notes:
- `start` is emitted immediately after the process is spawned.
- `exit` is always the last frame; `exit_code` is omitted from all other types.
- When a session is killed, stdout/stderr frames are suppressed after the kill signal, but the `exit` frame is still delivered so the UI can update its state.
- Late-joining WebSocket clients receive a replay of up to 2,000 buffered lines before live streaming begins.

---

## Dependencies

| Package | Purpose |
|---|---|
| `github.com/google/uuid` | Session ID generation |
| `github.com/gorilla/websocket` | WebSocket server implementation |
| `gopkg.in/yaml.v3` | YAML bundle file parsing (`internal/bundle`) |
| `github.com/webview/webview_go` | macOS native WebView (CGO, darwin only; build tag `darwin`) |

Go standard library is used for HTTP, JSON, process execution, embedding, and synchronization — no web framework.

---

## Security Notes

- **Command whitelisting**: use `-allow` flag in production to restrict which commands can be run.
- **No authentication**: the server assumes a trusted local network. Do not expose it publicly without adding auth.
- **WebSocket origin check** is permissive (`CheckOrigin` returns `true`) — appropriate for local dev, not for multi-tenant deployments.
- **HTML escaping**: the `AnsiParser` in `app.js` escapes all output before DOM insertion; do not bypass this.

---

## macOS Packaging

The `scripts/package-macos.sh` script:
1. Creates `.app` bundle structure under `dist/`.
2. Injects the version string (from git tags) into `Info.plist`.
3. Optionally code-signs with a provided identity or ad-hoc (`-`).
4. Optionally creates a `.dmg` with `hdiutil`.

Use `make app` for a windowed app with WKWebView (native macOS), or `make app-server` for a headless server app that opens the UI in the default browser.

---

## What Does Not Exist Yet (Contribution Opportunities)

- Unit tests (no `*_test.go` files currently exist)
- Session output **persistence** across server restarts (in-memory `lineBuf` is lost on restart; only 2,000 lines are replayed to late-joining clients within a single server run)
- Authentication / access control
- Windows packaging scripts
- Homebrew tap formula
