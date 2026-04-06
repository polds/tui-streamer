# CLAUDE.md — AI Assistant Guide for tui-streamer

## Project Overview

**tui-streamer** is a Go-based WebSocket server that executes OS commands and streams their output (stdout/stderr) line-by-line to a web-based terminal UI in real time. It supports multiple concurrent named "sessions", each of which can have multiple WebSocket subscriber clients.

Key capabilities:
- Execute arbitrary CLI commands on demand via REST API
- Stream output line-by-line via WebSocket in JSON format
- Serve a self-contained, theme-able web UI (no build step required)
- Optional command whitelisting for security
- Bundle files (YAML) to pre-configure sessions with optional auto-execution
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
│   ├── bundle/bundle.go     # YAML bundle/BundleSet parser
│   ├── executor/executor.go # Command execution engine (streaming output)
│   ├── server/server.go     # HTTP routes and WebSocket upgrade handler
│   └── session/
│       ├── manager.go       # Thread-safe session registry
│       ├── session.go       # Session state + client broadcast
│       └── client.go        # WebSocket client read/write pumps
├── examples/
│   ├── README.md            # Example index and usage guide
│   ├── lorem-ipsum/         # Lorem ipsum streaming demo
│   └── network-bundle/      # Network diagnostics BundleSet demo
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
├── go.mod                   # Go module (github.com/polds/tui-streamer)
├── go.sum                   # Dependency checksums
├── Makefile                 # Build, test, lint, packaging targets
└── README.md                # Project documentation
```

---

## Architecture

### Backend (Go)

The backend uses a **session-based multiplexing** model:

1. **Session** (`internal/session/session.go`) — Named execution context. Holds state (ID, name, timestamps, running flag), a map of subscribed WebSocket clients, a cancel function for the running process, and a 2,000-line replay buffer for late-joining clients. Also carries optional `BundleName`, `PendingCommand`, and `Description` fields from bundle entries.
2. **Manager** (`internal/session/manager.go`) — Thread-safe registry (UUID → `*Session`). Provides Create/Get/List/Delete.
3. **Executor** (`internal/executor/executor.go`) — Spawns a process, reads stdout/stderr concurrently in separate goroutines, and emits `Line` structs (JSON) with Unix millisecond timestamps and line type (`stdout`, `stderr`, `start`, `exit`, `error`). Uses `cmd.WaitDelay = 5s` to prevent goroutine leaks when a process ignores signals.
4. **Bundle** (`internal/bundle/bundle.go`) — Parses YAML bundle files (kind: `Bundle` or `BundleSet`) using multi-document YAML. A `Bundle` is a named list of session entries; a `BundleSet` references multiple `Bundle` documents from the same file. Each `Entry` has `name`, `command`, `description` (Markdown), and `autorun` fields.
5. **Client** (`internal/session/client.go`) — Wraps a `gorilla/websocket` connection with read/write pumps, a 256-element buffered send channel, ping/pong keepalive (54s), and `sync.Once`-guarded cleanup. Send is non-blocking: if the send buffer is full, the message is dropped rather than blocking the broadcaster.
6. **Server** (`internal/server/server.go`) — HTTP mux with:
   - `GET /` — serves embedded static files (title and `STARTUP_BUNDLE` flag injected at request time)
   - `GET /ws/{id}` — upgrades to WebSocket, creates a Client, registers it to the session
   - `GET /api/sessions` — list all sessions
   - `POST /api/sessions` — create session
   - `GET /api/sessions/{id}` — get single session info
   - `DELETE /api/sessions/{id}` — delete session
   - `POST /api/sessions/{id}/exec` — execute command in session
   - `POST /api/sessions/{id}/kill` — kill running process
   - `POST /api/bundles` — parse YAML bundle body and create all declared sessions

### Frontend (Vanilla JS)

`web/static/app.js` is a single-file application with no build step:

- **`AnsiParser`** — Converts ANSI SGR escape sequences to safe HTML spans (supports bold, dim, italic, underline, standard/256/true-color fg & bg).
- **`api`** — Thin wrapper over `fetch()` for all REST endpoints.
- **`SessionSocket`** — WebSocket wrapper with 500ms auto-reconnect.
- **`Terminal`** — Renders output lines with auto-scroll. Caps DOM to `MAX_DOM_LINES` (2,000) terminal lines by pruning oldest output; event banners (start/exit/error) are preserved.
- **`MarkdownRenderer`** — Inline Markdown-to-HTML renderer (headings h1–h3, paragraphs, unordered lists, fenced code blocks, bold, italic, inline code, links). Used to display bundle session `description` fields.
- **`App`** — Main controller: session creation/deletion, command dispatch, theme persistence (localStorage), per-session output buffering (2,000 lines) for replay, line selection, copy, and ray.so export. Warns before running known interactive/curses applications (`INTERACTIVE_COMMANDS` set).

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
- macOS with Xcode (only for the native WKWebView app target)

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
make app            # create .app bundle with WKWebView window (requires macOS + Xcode)
make app-server     # create headless server .app bundle (no CGO required)
make dmg            # create distributable .dmg (requires 'make app' first)

# Package a specific bundle YAML into the .app
make app BUNDLE=./examples/network-bundle/bundle.yaml

# Clean
make clean
```

### Running the Server

```bash
./dist/tui-streamer [flags]

Flags:
  -port string    Port to listen on (default: "8080")
  -dir string     Default working directory for executed commands (default: ".")
  -title string   Window / browser-tab title (defaults to bundle name or "TUI Streamer")
  -stdout         Capture stdout (default: true)
  -stderr         Capture stderr (default: true)
  -allow string   Whitelist a binary name; repeat flag for multiple binaries
                  (omit to allow all commands)
  -bundle string  Path to a YAML bundle file that pre-creates sessions
  -open           Auto-launch browser on startup
```

### Adding a New REST Endpoint

1. Add the route in `internal/server/server.go` inside the `routes()` method.
2. Write the handler as a method on `*Server` or a closure.
3. Access `s.manager` for session operations.
4. Respond with JSON using `json.NewEncoder(w).Encode(...)`.

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
- Classes for stateful components (`App`, `Terminal`, `SessionSocket`, `AnsiParser`, `MarkdownRenderer`).
- Always HTML-escape user/command output before inserting into the DOM.
- Theme names are CSS custom properties applied to `<html>` via `data-theme` attribute; add new themes in `style.css` using CSS custom properties.

### Adding a New Theme

1. Add a `[data-theme="<name>"]` block in `web/static/style.css` defining all CSS variables (see existing themes for the full variable list).
2. Add the option to the `<select id="theme-select">` in `web/static/index.html`.
3. No JS changes needed — the `App` class reads the selector value and applies it as `data-theme` on `<html>`.

### WebSocket Protocol

Messages are newline-delimited JSON objects. `timestamp` is a Unix millisecond integer (`time.Now().UnixMilli()`):
```json
{"type":"start","timestamp":1704067200000}
{"type":"stdout","timestamp":1704067200100,"data":"hello"}
{"type":"stderr","timestamp":1704067200200,"data":"error text"}
{"type":"exit","timestamp":1704067200300,"exit_code":0}
```

Line types: `start`, `stdout`, `stderr`, `exit`, `error`.

Fields:
- `type` (string) — always present
- `timestamp` (int64) — Unix milliseconds, always present
- `data` (string) — stdout/stderr text or error message; omitted on `start`/`exit`
- `exit_code` (int) — process exit code; present only on `exit`

### Exec Request Body

`POST /api/sessions/{id}/exec` accepts JSON:

```json
{
  "command": "ls -la",
  "dir": "/tmp",
  "env": ["FOO=bar"],
  "stdout": true,
  "stderr": true
}
```

- `command` — string (split on whitespace) **or** `["ls", "-la"]` array; required
- `dir` — working directory override; falls back to server `-dir` flag
- `env` — additional environment variables (`KEY=value` strings)
- `stdout` / `stderr` — per-request capture overrides; fall back to server defaults

---

## Dependencies

| Package | Purpose |
|---|---|
| `github.com/google/uuid` | Session ID generation |
| `github.com/gorilla/websocket` | WebSocket server implementation |
| `gopkg.in/yaml.v3` | Multi-document YAML parsing for bundle files |
| `webview/webview_go` | macOS native WebView (CGO, darwin only, not in go.mod) |

Go standard library is used for HTTP, JSON, process execution, embedding, and synchronization — no web framework.

---

## CI/CD

Three GitHub Actions workflows live in `.github/workflows/`:

| Workflow | File | Trigger |
|---|---|---|
| CI | `ci.yml` | Push / PR to `main` — runs `go vet`, `go build`, `go test` |
| Release | `release.yml` | On published GitHub release — builds cross-platform binaries and uploads them as release assets |
| Release Please | `release-please.yml` | Push to `main` — manages automated changelog and release PRs via `google-github-actions/release-please-action` |

---

## Security Notes

- **Command whitelisting**: use `-allow` flag in production to restrict which commands can be run. The flag is repeatable; e.g. `-allow make -allow npm`.
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

`make app` builds a **WKWebView windowed** app (requires macOS + Xcode, CGO enabled). `make app-server` builds a **headless server** app that opens the UI in the default browser (no CGO required).

To embed a bundle YAML file in the `.app`:

```bash
make app BUNDLE=./examples/network-bundle/bundle.yaml
# or
make app-server BUNDLE=./my-runbook.yaml
```

The app name is derived from the `BundleSet`/`Bundle` `metadata.name` field when `BUNDLE=` is set.

---

## What Does Not Exist Yet (Contribution Opportunities)

- Unit tests (no `*_test.go` files currently exist)
- Session output persistence / history replay on page load
- Authentication / access control
- Windows packaging scripts
