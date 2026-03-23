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
│   ├── bundle/bundle.go     # YAML bundle/BundleSet loader and parser
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
├── scripts/
│   └── package-macos.sh     # macOS .app/.dmg packaging script
├── examples/
│   ├── lorem-ipsum/         # Shell-script streaming example
│   └── network-bundle/      # BundleSet example for network diagnostics
├── go.mod                   # Go module (github.com/polds/tui-streamer)
├── go.sum                   # Dependency checksums
├── Makefile                 # Build, test, lint, packaging targets
└── README.md                # Project readme and usage guide
```

---

## Architecture

### Backend (Go)

The backend uses a **session-based multiplexing** model:

1. **Session** (`internal/session/session.go`) — Named execution context. Holds state (ID, name, timestamps, running flag), a map of subscribed WebSocket clients, and a cancel function for the running process. Buffers up to 2,000 output lines for replay to late-joining subscribers.
2. **Manager** (`internal/session/manager.go`) — Thread-safe registry (UUID → `*Session`). Provides Create/Get/List/Delete.
3. **Executor** (`internal/executor/executor.go`) — Spawns a process, reads stdout/stderr concurrently in separate goroutines, and emits `Line` structs (JSON) with Unix-millisecond timestamps and line type (`stdout`, `stderr`, `start`, `exit`, `error`). Applies a 5-second `WaitDelay` so pipe goroutines are never stranded after cancellation.
4. **Bundle** (`internal/bundle/bundle.go`) — Parses multi-document YAML bundle files. Supports `kind: Bundle` (a named set of sessions) and `kind: BundleSet` (an ordered list of Bundle references). Used by both the CLI `-bundle` flag and the `POST /api/bundles` endpoint.
5. **Client** (`internal/session/client.go`) — Wraps a `gorilla/websocket` connection with read/write pumps, a 256-element buffered send channel, ping/pong keepalive (54s), and `sync.Once`-guarded cleanup. Messages are dropped (non-blocking) when the buffer is full rather than blocking the broadcaster.
6. **Server** (`internal/server/server.go`) — HTTP mux with:
   - `GET /` — serves embedded static files (index templated with `-title` and `STARTUP_BUNDLE`)
   - `GET /ws/{id}` — upgrades to WebSocket, creates a Client, registers it to the session
   - `GET /api/sessions` — list all sessions
   - `POST /api/sessions` — create session
   - `GET /api/sessions/{id}` — get a single session
   - `DELETE /api/sessions/{id}` — delete session (also kills running process)
   - `POST /api/sessions/{id}/exec` — execute command in session
   - `POST /api/sessions/{id}/kill` — kill running process
   - `POST /api/bundles` — import a YAML bundle (4 MiB body limit)

### Frontend (Vanilla JS)

`web/static/app.js` is a single-file application with no build step:

- **`AnsiParser`** — Converts ANSI SGR escape sequences to safe HTML spans (supports bold, dim, italic, underline, standard/256/true-color fg & bg).
- **`api`** — Thin wrapper over `fetch()` for all REST endpoints.
- **`SessionSocket`** — WebSocket wrapper with 500ms auto-reconnect.
- **`Terminal`** — Renders output lines with auto-scroll.
- **`App`** — Main controller: session creation/deletion, command dispatch, theme persistence (localStorage), per-session output buffering for replay.

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
make build-darwin        # universal binary (arm64 + amd64 via lipo)
make app                 # create .app bundle with native WKWebView window (CGO required)
make app-server          # create headless .app bundle that opens the browser (no CGO)
make dmg                 # create distributable .dmg (requires make app first)

# Clean
make clean
```

### Running the Server

```bash
./dist/tui-streamer [flags]

Flags:
  -port string    TCP port to listen on (default: "8080")
  -title string   Window / browser-tab title (defaults to bundle name or "TUI Streamer")
  -dir string     Default working directory for executed commands (default: ".")
  -stdout         Capture stdout (default true)
  -stderr         Capture stderr (default true)
  -allow string   Whitelist a binary name; repeat for multiple
                  (e.g. -allow make -allow npm); omit to allow all commands
  -bundle string  Path to a YAML bundle file that pre-creates sessions at startup
  -open           Auto-launch browser on startup (default true inside .app bundle)
```

### Adding a New REST Endpoint

1. Add the route in `internal/server/server.go` inside `routes()`.
2. Write the handler as a method on `*Server`.
3. Access `s.manager` for session operations.
4. Respond with JSON using `json.NewEncoder(w).Encode(...)`.
5. Set `Content-Type: application/json` at the top of the handler.

### Adding a New Session Operation

1. Add method to `Session` in `internal/session/session.go`.
2. If it mutates shared state, protect with `s.mu` (RWMutex).
3. Wire it through a new REST endpoint in `server.go` if external access is needed.

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

Messages are JSON objects sent as WebSocket text frames. `timestamp` is a Unix millisecond integer (`int64`), not an ISO string.

```json
{"type":"start","timestamp":1700000000000}
{"type":"stdout","timestamp":1700000000100,"data":"hello"}
{"type":"stderr","timestamp":1700000000200,"data":"error text"}
{"type":"exit","timestamp":1700000000300,"exit_code":0}
{"type":"error","timestamp":1700000000400,"data":"wait: signal: killed"}
```

Field notes:
- `data` is omitted (not `""`) when empty.
- `exit_code` is only present on `exit` lines; it is an integer pointer so it can be `0` without being omitted.
- `error` lines are sent for non-exit-error process failures (e.g. killed by signal without an `ExitError`); they may be dropped if the 256-slot channel is full at that moment.

### Exec Request Body

`POST /api/sessions/{id}/exec` accepts JSON:

```json
{
  "command": ["make", "build"],   // string array OR a plain string split on whitespace
  "dir": "/path/to/workdir",      // optional; falls back to server -dir flag
  "env": ["FOO=bar"],             // optional; added to the process environment
  "stdout": true,                 // optional; per-request override of server default
  "stderr": true                  // optional; per-request override of server default
}
```

The `command` field is required. Sending a string (`"make build"`) is equivalent to `["make", "build"]` — it is split on whitespace with no shell expansion.

Line types: `start`, `stdout`, `stderr`, `exit`, `error`.

---

## Dependencies

| Package | Purpose |
|---|---|
| `github.com/google/uuid` | Session ID generation |
| `github.com/gorilla/websocket` | WebSocket server implementation |
| `gopkg.in/yaml.v3` | YAML bundle file parsing |
| `webview/webview_go` | macOS native WebView (CGO, darwin only, not in go.mod) |

Go standard library is used for HTTP, JSON, process execution, embedding, and synchronization — no web framework.

---

## Security Notes

- **Command whitelisting**: use `-allow` flag in production to restrict which commands can be run.
- **No authentication**: the server assumes a trusted local network. Do not expose it publicly without adding auth.
- **WebSocket origin check** is permissive (`CheckOrigin` returns `true`) — appropriate for local dev, not for multi-tenant deployments.
- **HTML escaping**: the `AnsiParser` in `app.js` escapes all output before DOM insertion; do not bypass this.

---

## CI/CD

Three GitHub Actions workflows live under `.github/workflows/`:

| Workflow | File | Trigger | What it does |
|---|---|---|---|
| **CI** | `ci.yml` | Push/PR to `main` | `go vet`, `staticcheck`, `go test -race` on Go 1.22 & 1.23 × ubuntu/macOS/Windows, cross-platform build matrix |
| **Release** | `release.yml` | Push of `v*` tag | Builds binaries for all platforms, creates GitHub release with assets |
| **Release Please** | `release-please.yml` | Push to `main` | Automates changelog and release PR creation via [release-please](https://github.com/googleapis/release-please) |

CI skips runs when only `.md` or `docs/**` files change.

---

## macOS Packaging

The `scripts/package-macos.sh` script:
1. Creates `.app` bundle structure under `dist/`.
2. Injects the version string (from git tags) into `Info.plist`.
3. Optionally code-signs with a provided identity or ad-hoc (`-`).
4. Optionally creates a `.dmg` with `hdiutil`.

Use `make app` for a windowed app with native WKWebView (requires Xcode/CGO). Use `make app-server` for a headless server app that opens the UI in the default browser (cross-compilable, no CGO required).

To bundle a pre-configured session file into the `.app`:

```bash
make app BUNDLE=./my-runbook.yaml
```

The app name is derived from the `BundleSet`/`Bundle` `metadata.name`.

---

## What Does Not Exist Yet (Contribution Opportunities)

- Unit tests (no `*_test.go` files currently exist)
- Session output persistence / history replay across server restarts
- Authentication / access control
- Windows packaging scripts
