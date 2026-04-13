# CLAUDE.md — AI Assistant Guide for tui-streamer

## Project Overview

**tui-streamer** is a Go-based WebSocket server that executes OS commands and streams their output (stdout/stderr) line-by-line to a web-based terminal UI in real time. It supports multiple concurrent named "sessions", each of which can have multiple WebSocket subscriber clients.

Key capabilities:
- Execute arbitrary CLI commands on demand via REST API
- Stream output line-by-line via WebSocket in JSON format
- Serve a self-contained, theme-able web UI (no build step required)
- Optional command whitelisting for security
- Pre-configure sessions via YAML bundle files
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
│   ├── bundle/bundle.go     # YAML bundle/BundleSet parser and loader
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
│   ├── README.md            # Overview of available examples
│   ├── lorem-ipsum/         # Lorem-ipsum streamer example
│   └── network-bundle/      # Network troubleshooting BundleSet example
├── go.mod                   # Go module (github.com/polds/tui-streamer)
├── go.sum                   # Dependency checksums
├── Makefile                 # Build, test, lint, packaging targets
└── README.md                # User-facing project documentation
```

---

## Architecture

### Backend (Go)

The backend uses a **session-based multiplexing** model:

1. **Bundle** (`internal/bundle/bundle.go`) — YAML multi-document parser. Supports `kind: Bundle` (a named group of sessions) and `kind: BundleSet` (an ordered reference to multiple Bundle documents in the same file). Exposes `Load(path)` and `Parse(data []byte)` returning a `*File` with the resolved `Bundles` slice and top-level `Name`.
2. **Session** (`internal/session/session.go`) — Named execution context. Holds state (ID, name, timestamps, running flag), optional bundle metadata (`BundleName`, `PendingCommand`, `Description`), a map of subscribed WebSocket clients, a cancel function for the running process, and a 2,000-line replay buffer (`maxLineBuf = 2_000`) for late-joining clients.
3. **Manager** (`internal/session/manager.go`) — Thread-safe registry (UUID → `*Session`). Provides Create/Get/List/Delete.
4. **Executor** (`internal/executor/executor.go`) — Spawns a process, reads stdout/stderr concurrently in separate goroutines, and emits `Line` structs (JSON) with Unix millisecond timestamps (`time.Now().UnixMilli()`) and line type (`stdout`, `stderr`, `start`, `exit`, `error`). Sets `cmd.WaitDelay = 5s` to prevent goroutine leaks when a process ignores signals or produces large buffered output.
5. **Client** (`internal/session/client.go`) — Wraps a `gorilla/websocket` connection with read/write pumps, a 256-element buffered send channel, ping/pong keepalive (`pingPeriod = 54s`, `pongWait = 60s`), and `sync.Once`-guarded cleanup. `Send()` is non-blocking: messages are dropped when the send buffer is full or the client has disconnected.
6. **Server** (`internal/server/server.go`) — HTTP mux registered in `routes()`:
   - `GET /` — serves embedded static files; injects `<title>` and `window.STARTUP_BUNDLE` into `index.html`
   - `GET /ws/{id}` — upgrades to WebSocket, creates a Client, registers it to the session
   - `GET /api/sessions` — list all sessions
   - `POST /api/sessions` — create session
   - `GET /api/sessions/{id}` — get a single session
   - `DELETE /api/sessions/{id}` — delete session
   - `POST /api/sessions/{id}/exec` — execute command in session
   - `POST /api/sessions/{id}/kill` — kill running process
   - `POST /api/bundles` — import a YAML bundle body, create sessions (4 MiB body cap)

### Frontend (Vanilla JS)

`web/static/app.js` is a single-file application with no build step:

- **`AnsiParser`** — Converts ANSI SGR escape sequences to safe HTML spans (supports bold, dim, italic, underline, blink, standard/256/true-color fg & bg).
- **`api`** — Thin wrapper over `fetch()` for all REST endpoints.
- **`SessionSocket`** — WebSocket wrapper with 500ms auto-reconnect on close.
- **`Terminal`** — Renders output lines with auto-scroll. Prunes oldest `.terminal-line` nodes when the DOM exceeds `MAX_DOM_LINES = 2000`; event banners (`start`/`exit`/`error`) are not pruned.
- **`MarkdownRenderer`** — Minimal safe Markdown renderer (headings h1–h3, paragraphs, unordered lists, fenced code blocks, inline bold/italic/code/links). Used to render bundle session `description` fields in the terminal panel.
- **`App`** — Main controller: session creation/deletion, command dispatch, theme persistence (`localStorage`), per-session output buffering for replay (`MAX_BUFFER = 2000`), line selection, copy, ray.so image export, and `INTERACTIVE_COMMANDS` guard (warns before running known curses/TUI programs).

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
- macOS with Xcode command-line tools (only for the native WKWebView app target)

### Common Commands

```bash
# Build for current platform
make build          # outputs dist/tui-streamer

# Run tests
make test           # go test ./...

# Lint
make lint           # go vet ./...

# macOS-specific
make build-darwin       # universal binary (arm64 + amd64 via lipo)
make app                # create .app bundle with native WKWebView window (requires CGO/Xcode)
make app-server         # create headless server .app bundle (no CGO required)
make dmg                # create distributable .dmg (requires 'make app' first)

# Package a bundle YAML into the .app (name is derived from Bundle/BundleSet metadata)
make app BUNDLE=./examples/network-bundle/bundle.yaml

# Clean
make clean
```

### Running the Server

```bash
./dist/tui-streamer [flags]

Flags:
  -port string   TCP port to listen on (default: "8080")
  -title string  Window / browser-tab title (defaults to bundle name or "TUI Streamer")
  -dir string    Default working directory for executed commands (default: ".")
  -stdout        Capture stdout (default true)
  -stderr        Capture stderr (default true)
  -allow string  Whitelist a binary name; repeat for multiple
                 (omit to allow all commands)
  -bundle string Path to a YAML bundle file that pre-creates sessions
  -open          Auto-launch browser on startup
```

Note: `-port` is a **string** flag (not int). `-stdout` and `-stderr` are **bool** flags (not string). `-allow` is a **repeatable** flag — pass it multiple times to whitelist multiple binaries (e.g. `-allow make -allow npm`).

### Exec Request Body

`POST /api/sessions/{id}/exec` accepts JSON:

```json
{
  "command": "make build",
  "dir": "/path/to/project",
  "env": ["KEY=value"],
  "stdout": true,
  "stderr": true
}
```

- `command` — required; accepts a plain string (`"ls -la"`, split on whitespace) or a JSON array (`["ls", "-la"]`).
- `dir` — optional; overrides the server's default working directory for this invocation.
- `env` — optional; array of `"KEY=value"` strings. When provided, replaces the inherited environment entirely.
- `stdout` / `stderr` — optional booleans; override the server-level capture flags for this invocation.

### Adding a New REST Endpoint

1. Add the route in `internal/server/server.go` inside the `routes()` method.
2. Write the handler as a method on `*Server` or a closure.
3. Access `s.manager` for session operations and `s.cfg` for server-level config.
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
- Theme is applied as a `data-theme` attribute on `<html>` (`document.documentElement`). The dark theme is the default (empty attribute). The select element ID is `theme-select`.

### Adding a New Theme

1. Add a `[data-theme="<name>"]` block in `web/static/style.css` defining all CSS custom properties (see existing theme blocks for the full variable list).
2. Add the option to the `<select id="theme-select">` in `web/static/index.html`.
3. No JS changes needed — `App._loadTheme()` reads the selector value and applies it via `document.documentElement.setAttribute('data-theme', ...)`.

### WebSocket Protocol

Messages are JSON objects. The `timestamp` field is a **Unix millisecond integer** (`int64`, from `time.Now().UnixMilli()`):

```json
{"type":"start","timestamp":1700000000000,"data":"","exit_code":null}
{"type":"stdout","timestamp":1700000000100,"data":"hello","exit_code":null}
{"type":"stderr","timestamp":1700000000200,"data":"error text","exit_code":null}
{"type":"exit","timestamp":1700000000300,"data":"","exit_code":0}
```

Field presence rules:
- `data` is present on `stdout`/`stderr`/`error`; empty string on `start`/`exit`.
- `exit_code` is present (integer) only on `exit` lines; omitted (`null`) on all others.

Line types: `start`, `stdout`, `stderr`, `exit`, `error`.

---

## Dependencies

| Package | Purpose |
|---|---|
| `github.com/google/uuid` | Session ID generation |
| `github.com/gorilla/websocket` | WebSocket server implementation |
| `gopkg.in/yaml.v3` | Bundle YAML parsing |
| `webview/webview_go` | macOS native WebView (CGO, darwin only, not in go.mod) |

Go standard library is used for HTTP, JSON, process execution, embedding, and synchronization — no web framework.

---

## CI/CD

Three GitHub Actions workflows live in `.github/workflows/`:

| Workflow | File | Trigger | Purpose |
|---|---|---|---|
| CI | `ci.yml` | push/PR to `main` | Lint (`go vet` + staticcheck), test (Go 1.22/1.23 × linux/macOS/Windows), cross-platform build |
| Release Please | `release-please.yml` | push to `main` | Opens/updates a release PR; outputs `release_created` when merged |
| Release | `release.yml` | push of `v*.*.*` tag | Builds cross-platform binaries and publishes a GitHub Release with assets |

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

- `make app` — WKWebView `.app` with a native window; requires macOS + Xcode (`CGO_ENABLED=1`).
- `make app-server` — headless server `.app` that opens the UI in the default browser; cross-compilable (no CGO).
- `make app BUNDLE=<path>` — embeds a bundle YAML; the `.app` name is derived from the Bundle/BundleSet `metadata.name`.

---

## What Does Not Exist Yet (Contribution Opportunities)

- Unit tests (no `*_test.go` files currently exist)
- Session output persistence / history replay across server restarts
- Authentication / access control
- Windows packaging scripts
