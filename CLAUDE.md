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
├── go.mod                   # Go module (github.com/polds/tui-streamer)
├── go.sum                   # Dependency checksums
├── Makefile                 # Build, test, lint, packaging targets
└── README.md                # Project documentation
```

---

## Architecture

### Backend (Go)

The backend uses a **session-based multiplexing** model:

1. **Session** (`internal/session/session.go`) — Named execution context. Holds state (ID, name, timestamps, running flag), a map of subscribed WebSocket clients, and a cancel function for the running process. Buffers the last 2,000 output lines so late-joining WebSocket clients receive prior output on connect.
2. **Manager** (`internal/session/manager.go`) — Thread-safe registry (UUID → `*Session`). Provides Create/Get/List/Delete.
3. **Executor** (`internal/executor/executor.go`) — Spawns a process, reads stdout/stderr concurrently in separate goroutines, and emits `Line` structs (JSON) with Unix-millisecond timestamps and line type (`stdout`, `stderr`, `start`, `exit`, `error`). A 5-second `WaitDelay` ensures pipes are forcibly closed after context cancellation.
4. **Bundle** (`internal/bundle/bundle.go`) — Parses multi-document YAML files into `Bundle` and `BundleSet` structs. Supports `kind: Bundle` (sessions list) and `kind: BundleSet` (ordered references to named Bundles in the same file). Used both by the CLI (`-bundle` flag) and the `/api/bundles` endpoint.
5. **Client** (`internal/session/client.go`) — Wraps a `gorilla/websocket` connection with read/write pumps, a 256-element buffered send channel, ping/pong keepalive (54s ping period, 60s pong wait), and `sync.Once`-guarded cleanup. Messages dropped (not buffered) when the send channel is full.
6. **Server** (`internal/server/server.go`) — HTTP mux with:
   - `GET /` — serves embedded static files, injects `<title>` and `window.STARTUP_BUNDLE`
   - `GET /ws/{id}` — upgrades to WebSocket, creates a Client, registers it to the session
   - `GET /api/sessions` — list all sessions (sorted by creation time)
   - `POST /api/sessions` — create session
   - `GET /api/sessions/{id}` — get a single session
   - `DELETE /api/sessions/{id}` — delete session (also kills any running process)
   - `POST /api/sessions/{id}/exec` — execute command in session
   - `POST /api/sessions/{id}/kill` — kill running process
   - `POST /api/bundles` — import a YAML bundle (creates sessions; rejects duplicate bundle names)

### Frontend (Vanilla JS)

`web/static/app.js` is a single-file application with no build step:

- **`AnsiParser`** — Converts ANSI SGR escape sequences to safe HTML spans (supports bold, dim, italic, underline, blink, standard/256/true-color fg & bg). All text content is HTML-escaped before attribute injection.
- **`MarkdownRenderer`** — Minimal safe Markdown renderer used for bundle session descriptions. Supports headings (h1–h3), paragraphs, unordered lists, fenced code blocks, and inline bold/italic/code/links.
- **`api`** — Thin wrapper over `fetch()` for all REST endpoints.
- **`SessionSocket`** — WebSocket wrapper with 500ms auto-reconnect on disconnect.
- **`Terminal`** — Renders output lines with auto-scroll. Caps the DOM at 2,000 `terminal-line` nodes (oldest pruned); event banners (`start`/`exit`/`error`) are not counted toward the cap.
- **`App`** — Main controller: session creation/deletion, command dispatch, theme persistence (localStorage), per-session in-memory output buffer (2,000 lines) for replay when switching sessions. Detects interactive/curses commands (`top`, `vim`, `less`, etc.) and shows a warning rather than running them.

Key global constants in `app.js`:
- `MAX_BUFFER = 2000` — lines kept per-session in the JS replay buffer
- `MAX_DOM_LINES = 2000` — maximum `terminal-line` nodes in the DOM at once
- `INTERACTIVE_COMMANDS` — set of command names that require a real TTY and are blocked with a warning

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
make build-darwin       # universal binary (arm64 + amd64 via lipo)
make app                # create .app bundle with native WKWebView window (requires macOS + Xcode/CGO)
make app-server         # create headless .app bundle that opens UI in the default browser (cross-compilable)
make dmg                # create distributable .dmg (requires make app first)

# Package a bundle YAML into a named .app (name comes from BundleSet/Bundle metadata)
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
  -title string   Window / browser-tab title (defaults to bundle name or "TUI Streamer")
  -stdout         Capture stdout (default true)
  -stderr         Capture stderr (default true)
  -allow string   Whitelist a binary name; repeat the flag for multiple binaries
                  (omit to allow all commands)
  -bundle string  Path to a YAML bundle file that pre-creates sessions on startup
  -open           Auto-launch browser on startup (always true inside a macOS .app)
```

The `-allow` flag can be repeated: `-allow make -allow npm`. Whitelisting is enforced on the first token of the command string. An empty allowlist permits everything.

### Exec Request Body

`POST /api/sessions/{id}/exec` accepts a JSON body with these fields:

| Field | Type | Description |
|---|---|---|
| `command` | string or `[]string` | **Required.** Command to run. A plain string is split on whitespace; an array is used as-is. |
| `dir` | string | Working directory override for this request (falls back to server `-dir`). |
| `env` | `[]string` | Additional environment variables in `"KEY=VALUE"` form (replaces the inherited environment entirely when set). |
| `stdout` | bool | Per-request override for stdout capture (defaults to server `-stdout`). |
| `stderr` | bool | Per-request override for stderr capture (defaults to server `-stderr`). |

### Adding a New REST Endpoint

1. Add the route in `internal/server/server.go` inside `routes()`.
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
- Classes for stateful components (`App`, `Terminal`, `SessionSocket`, `AnsiParser`).
- Always HTML-escape user/command output before inserting into the DOM.
- Theme names are CSS class names applied to `<body>`; add new themes in `style.css` using CSS custom properties.

### Adding a New Theme

1. Add a `body.theme-<name>` block in `web/static/style.css` defining all CSS variables (see existing themes for the full variable list).
2. Add the option to the `<select id="themeSelect">` in `web/static/index.html`.
3. No JS changes needed — the `App` class reads the selector value and applies it as a body class.

### WebSocket Protocol

Messages are JSON objects sent as WebSocket text frames, one per line of output:
```json
{"type":"start","timestamp":1719072000000}
{"type":"stdout","timestamp":1719072000123,"data":"hello"}
{"type":"stderr","timestamp":1719072000456,"data":"error text"}
{"type":"exit","timestamp":1719072001000,"exit_code":0}
{"type":"error","timestamp":1719072001001,"data":"signal: killed"}
```

Field details:
- `type` — one of `start`, `stdout`, `stderr`, `exit`, `error`
- `timestamp` — Unix epoch in **milliseconds** (integer, not ISO string)
- `data` — output text for `stdout`/`stderr`/`error`; omitted for `start`/`exit`
- `exit_code` — integer exit code, present only on `exit` messages

When a client connects to an active or completed session, it receives a replay of up to 2,000 buffered lines before live messages begin. The replay is delivered synchronously during `subscribe()` before the client enters its write loop.

---

## Dependencies

| Package | Purpose |
|---|---|
| `github.com/google/uuid` | Session ID generation |
| `github.com/gorilla/websocket` | WebSocket server implementation |
| `webview/webview_go` | macOS native WebView (CGO, darwin only, not in go.mod) |

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

Use `make app` for a windowed app with WKWebView (requires macOS + Xcode), or `make app-server` for a headless server app that opens the UI in the default browser (cross-compilable from Linux/Windows using `make build-darwin` first).

---

### Bundle Package

`internal/bundle/bundle.go` parses multi-document YAML files. Rules:

- A file may contain multiple `---`-separated YAML documents.
- Each document must have `apiVersion: v1` and `kind: Bundle` or `kind: BundleSet`.
- A `BundleSet` lists `Bundle` documents by `metadata.name`; all referenced Bundles must be in the same file.
- Without a `BundleSet`, all `Bundle` documents are loaded in document order.
- The `File.Name` is set from the `BundleSet` name when present, or the first `Bundle` name otherwise.
- Unknown `kind` values (including empty documents) return a parse error or are silently skipped (empty/blank documents only).

Bundle entries support: `name` (required), `command` (optional), `description` (optional Markdown), `autorun` (bool, default false).

When imported via `/api/bundles`, a bundle is rejected if any `Bundle` in the file shares a name with an already-imported bundle, preventing accidental double-import.

---

## What Does Not Exist Yet (Contribution Opportunities)

- Unit tests (no `*_test.go` files currently exist)
- CI/CD pipelines (no `.github/workflows/`)
- Authentication / access control
- Windows packaging scripts
- TTY emulation (interactive/curses apps such as `top`, `vim`, `less` are intentionally blocked with a warning)
