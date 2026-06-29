# CLAUDE.md — AI Assistant Guide for tui-streamer

## Project Overview

**tui-streamer** is a Go-based WebSocket server that executes OS commands and streams their output (stdout/stderr) line-by-line to a web-based terminal UI in real time. It supports multiple concurrent named "sessions", each of which can have multiple WebSocket subscriber clients.

Key capabilities:
- Execute arbitrary CLI commands on demand via REST API
- Stream output line-by-line via WebSocket in JSON format
- Serve a self-contained, theme-able web UI (no build step required)
- Optional command whitelisting for security
- YAML bundle files that pre-configure sessions with optional auto-execution
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
│   ├── bundle/bundle.go     # Bundle/BundleSet YAML loader and parser
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
├── examples/
│   ├── README.md            # Examples index and contribution guide
│   ├── lorem-ipsum/         # Fetches and streams an article from a public API
│   └── network-bundle/      # BundleSet demo with connectivity/DNS sessions
├── build/darwin/
│   ├── Info.plist           # macOS app bundle metadata
│   └── entitlements.plist   # macOS code signing entitlements
├── scripts/
│   ├── make-icon.sh         # Generates AppIcon.icns from SVG source (macOS)
│   └── package-macos.sh     # macOS .app/.dmg packaging script
├── .github/workflows/
│   ├── ci.yml               # Build and lint CI
│   ├── release.yml          # Binary release workflow
│   └── release-please.yml   # Automated release PR workflow
├── go.mod                   # Go module (github.com/polds/tui-streamer)
├── go.sum                   # Dependency checksums
├── Makefile                 # Build, test, lint, packaging targets
├── release-please-config.json
└── README.md                # User-facing project documentation
```

---

## Architecture

### Backend (Go)

The backend uses a **session-based multiplexing** model:

1. **Bundle** (`internal/bundle/bundle.go`) — Parses YAML bundle files. Entry points are `Load(path)` (reads file then parses) and `Parse(data)` (parses raw bytes). Supports two document kinds: `kind: Bundle` (a named list of sessions) and `kind: BundleSet` (orders multiple Bundle documents from the same file). Returns a `*File` with a `Name` and ordered `[]*Bundle` slice.

2. **Session** (`internal/session/session.go`) — Named execution context. Holds state (ID, name, timestamps, running flag), optional bundle metadata (`BundleName`, `PendingCommand`, `Description`), a map of subscribed WebSocket clients, a cancel function for the running process, and `lineBuf` — a replay buffer of up to `maxLineBuf = 2,000` output lines replayed to clients that connect after execution has started.

3. **Manager** (`internal/session/manager.go`) — Thread-safe registry (UUID → `*Session`). Provides Create/Get/List/Delete.

4. **Executor** (`internal/executor/executor.go`) — Spawns a process, reads stdout/stderr concurrently in separate goroutines, and emits `Line` structs (JSON) with Unix millisecond timestamps (`int64`) and a line type (`stdout`, `stderr`, `start`, `exit`, `error`). `ExitCode` is `*int` and is only set on `exit`-type lines. After context cancellation, `WaitDelay = 5s` prevents goroutine leaks.

5. **Client** (`internal/session/client.go`) — Wraps a `gorilla/websocket` connection with read/write pumps, a 256-element buffered send channel, ping/pong keepalive (`pingPeriod = 54s`, i.e. `pongWait(60s) × 9/10`), and `sync.Once`-guarded cleanup. `Send()` is non-blocking and drops messages if the buffer is full.

6. **Server** (`internal/server/server.go`) — Constructor is `New(manager, cfg, staticFS)`. Routes are registered in `routes()`. HTTP mux with:
   - `GET /` — serves embedded static files; injects `<title>` and `window.STARTUP_BUNDLE`
   - `GET /ws/{id}` — upgrades to WebSocket, creates a Client, registers it to the session
   - `GET /api/sessions` — list all sessions
   - `POST /api/sessions` — create session
   - `GET /api/sessions/{id}` — get a single session
   - `DELETE /api/sessions/{id}` — delete session
   - `POST /api/sessions/{id}/exec` — execute command in session
   - `POST /api/sessions/{id}/kill` — kill running process
   - `POST /api/bundles` — parse YAML bundle body (4 MiB limit) and create sessions; returns 409 if any bundle name is already imported

### Frontend (Vanilla JS)

`web/static/app.js` is a single-file application with no build step:

- **`AnsiParser`** — Converts ANSI SGR escape sequences to safe HTML spans (supports bold, dim, italic, underline, standard/256/true-color fg & bg).
- **`MarkdownRenderer`** — Renders Markdown `description` fields from bundle entries into styled HTML for display above terminal output.
- **`api`** — Thin wrapper over `fetch()` for all REST endpoints.
- **`SessionSocket`** — WebSocket wrapper with 500ms auto-reconnect.
- **`Terminal`** — Renders output lines with auto-scroll. Prunes the DOM to at most `MAX_DOM_LINES = 2,000` `.terminal-line` nodes to prevent memory bloat.
- **`App`** — Main controller: session creation/deletion, command dispatch, theme persistence (localStorage), per-session output buffering for replay. Warns before running commands in `INTERACTIVE_COMMANDS` (a set of TUI programs like `vim`, `top`, `tmux`) that require a real TTY and won't render correctly.

### Data Flow

```
Browser (REST) → POST /api/sessions/{id}/exec
                       ↓
               server.go: validate, call session.Exec()
                       ↓
               executor.go: spawn process, read stdout/stderr
                       ↓
               session.go: append to lineBuf, broadcast Line JSON to all clients
                       ↓
               client.go: write to WebSocket send channel
                       ↓
               Browser (WebSocket) receives Line JSON → Terminal renders

Late-joining client → GET /ws/{id}
                       ↓
               session.subscribe(): replay lineBuf to new client
                       ↓
               client receives buffered lines, then live output
```

---

## Development Workflows

### Prerequisites

- Go 1.22+
- `make`
- macOS with Xcode command-line tools (only for the native app target)

### Common Commands

```bash
# Build for current platform
make build          # outputs dist/tui-streamer

# Run tests
make test           # go test ./...

# Lint
make lint           # go vet ./...

# macOS-specific
make build-darwin          # universal binary (arm64 + amd64 via lipo)
make build-darwin-webview  # WKWebView binary (CGO_ENABLED=1, macOS + Xcode)
make icon                  # generate AppIcon.icns from SVG (requires librsvg)
make app                   # .app bundle with native WKWebView window (CGO)
make app-server            # .app bundle as headless server (no CGO required)
make dmg                   # create distributable .dmg (requires 'make app' first)

# Clean
make clean
```

The `BUNDLE=` variable can be passed to `make app` / `make app-server` / `make dmg` to embed a bundle YAML file and name the app from the BundleSet/Bundle metadata:

```bash
make app BUNDLE=./examples/network-bundle/bundle.yaml
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
  -allow string   Whitelist a binary name; repeat for multiple (omit to allow all)
  -bundle string  Path to a YAML bundle file that pre-creates sessions
  -open           Auto-launch browser on startup
```

Note: `-port` is a `string` flag (not `int`). `-stdout` and `-stderr` are boolean flags. `-allow` is a repeatable flag (specify it multiple times for multiple whitelisted commands).

### Adding a New REST Endpoint

1. Add the route in `internal/server/server.go` inside the `routes()` method.
2. Write the handler as a method on `*Server` or a closure.
3. Access `s.manager` for session operations and `s.cfg` for server config.
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
- Themes are applied as a `data-theme` attribute on `document.documentElement` (`<html>`). The special value `dark` is stored as an empty string (`""`). All other theme names are the attribute value verbatim.

### Adding a New Theme

1. Add a `[data-theme="<name>"]` block in `web/static/style.css` defining all CSS custom properties (see existing themes for the full variable list). The `dark` theme uses `[data-theme=""]`.
2. Add the option to the `<select id="theme-select">` in `web/static/index.html`.
3. No JS changes needed — the `App` class reads the selector value and applies it as the `data-theme` attribute on `document.documentElement`.

### Bundle File Format

Bundle files are YAML and support two document kinds separated by `---`. All fields follow a Kubernetes-style `apiVersion / kind / metadata / spec` envelope.

**Single Bundle:**
```yaml
---
apiVersion: v1
kind: Bundle
metadata:
  name: Deploy
spec:
  sessions:
    - name: Build
      description: Compile the project and run tests.
      command: make build test
      autorun: true

    - name: Deploy
      description: |
        Push artifacts to production.
        **Only run after Build succeeds.**
      command: make deploy
      autorun: false
```

**BundleSet + multiple Bundles in one file:**
```yaml
---
apiVersion: v1
kind: BundleSet
metadata:
  name: Network Troubleshooting   # becomes the app/window title
spec:
  bundles:
    - name: Connectivity          # must match Bundle metadata.name below
    - name: DNS
---
apiVersion: v1
kind: Bundle
metadata:
  name: Connectivity
spec:
  sessions:
    - name: Ping
      command: ping -c 4 example.com
      autorun: true
---
apiVersion: v1
kind: Bundle
metadata:
  name: DNS
spec:
  sessions:
    - name: Dig
      command: dig +short example.com
      autorun: true
```

`Entry` fields: `name` (string), `command` (string, whitespace-split to argv), `description` (Markdown string, optional), `autorun` (bool, default false).

### Exec Request Body

`POST /api/sessions/{id}/exec` accepts JSON with these fields:

```json
{
  "command": "ls -la",       // string (split on whitespace) OR array ["ls", "-la"]
  "dir":     "/tmp",         // optional working directory override
  "env":     ["FOO=bar"],    // optional environment variables
  "stdout":  true,           // optional bool override (server default applies if omitted)
  "stderr":  true            // optional bool override
}
```

There is no `args` field — arguments are part of the `command` value.

### WebSocket Protocol

Each WebSocket text frame carries a single JSON object. Frames are not newline-delimited.

```json
{"type":"start","timestamp":1704067200000,"data":""}
{"type":"stdout","timestamp":1704067200100,"data":"hello"}
{"type":"stderr","timestamp":1704067200200,"data":"error text"}
{"type":"exit","timestamp":1704067200300,"data":"","exit_code":0}
```

- `timestamp` is a Unix millisecond integer (`int64`), not an ISO string.
- `exit_code` is only present on `exit`-type lines (the field is omitted on all others).
- Line types: `start`, `stdout`, `stderr`, `exit`, `error`.

---

## Dependencies

| Package | Purpose |
|---|---|
| `github.com/google/uuid` | Session ID generation |
| `github.com/gorilla/websocket` | WebSocket server implementation |
| `github.com/webview/webview_go` | macOS native WebView (CGO, darwin only) |
| `gopkg.in/yaml.v3` | YAML bundle file parsing |

Go standard library is used for HTTP, JSON, process execution, embedding, and synchronization — no web framework.

---

## CI/CD

Three GitHub Actions workflows live in `.github/workflows/`:

| File | Trigger | Purpose |
|---|---|---|
| `ci.yml` | push / PR | `go build`, `go vet`, `go test ./...` |
| `release.yml` | push of `v*` tag | Cross-platform binary builds and GitHub Release upload |
| `release-please.yml` | push to `main` | Automates release PRs via `release-please` |

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

| Make target | Binary | Window |
|---|---|---|
| `make app` | `cmd/app` (CGO, WKWebView) | Native WKWebView window |
| `make app-server` | `cmd/server` (no CGO) | Opens default browser |
| `make dmg` | same as `make app` | Wraps the `.app` in a distributable `.dmg` |

Run `make icon` first to include the app icon (requires `librsvg` via Homebrew).

---

## What Does Not Exist Yet (Contribution Opportunities)

- Unit tests (no `*_test.go` files currently exist)
- Session output persistence across server restarts
- Authentication / access control
- Windows packaging scripts
