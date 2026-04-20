# CLAUDE.md — AI Assistant Guide for tui-streamer

## Project Overview

**tui-streamer** is a Go-based WebSocket server that executes OS commands and streams their output (stdout/stderr) line-by-line to a web-based terminal UI in real time. It supports multiple concurrent named "sessions", each of which can have multiple WebSocket subscriber clients.

Key capabilities:
- Execute arbitrary CLI commands on demand via REST API
- Stream output line-by-line via WebSocket in JSON format
- Serve a self-contained, theme-able web UI (no build step required)
- Optional command whitelisting for security
- Pre-configured sessions via YAML bundle files (loaded at startup or imported via UI)
- Native macOS `.app`/`.dmg` packaging via WKWebView (darwin only)

---

## Repository Structure

```
tui-streamer/
├── cmd/
│   ├── app/main.go              # macOS native WebView app entry point (darwin + CGO only)
│   └── server/main.go           # HTTP/WebSocket server entry point
├── internal/
│   ├── browser/open.go          # Cross-platform browser launcher
│   ├── bundle/bundle.go         # YAML bundle parser (Bundle + BundleSet kinds)
│   ├── executor/executor.go     # Command execution engine (streaming output)
│   ├── server/server.go         # HTTP routes and WebSocket upgrade handler
│   └── session/
│       ├── manager.go           # Thread-safe session registry
│       ├── session.go           # Session state + client broadcast
│       └── client.go            # WebSocket client read/write pumps
├── web/
│   ├── embed.go                 # Go embed directive for static assets
│   └── static/
│       ├── index.html           # Web UI markup
│       ├── style.css            # Styling with 10 themes
│       └── app.js               # Vanilla JS frontend (no framework/build step)
├── build/darwin/
│   ├── Info.plist               # macOS app bundle metadata
│   └── entitlements.plist       # macOS code signing entitlements
├── scripts/
│   └── package-macos.sh         # macOS .app/.dmg packaging script
├── examples/
│   ├── README.md                # Example index and authoring guide
│   ├── network-bundle/          # Network troubleshooting BundleSet example
│   └── lorem-ipsum/             # External API + ANSI color streaming example
├── .github/workflows/
│   ├── ci.yml                   # Lint + test matrix (Go 1.22/1.23, 3 OSes)
│   ├── release.yml              # Builds and attaches release binaries
│   └── release-please.yml       # Automated changelog + version bump PRs
├── release-please-config.json   # release-please configuration
├── go.mod                       # Go module (github.com/polds/tui-streamer)
├── go.sum                       # Dependency checksums
├── Makefile                     # Build, test, lint, packaging targets
└── README.md                    # Project overview, quickstart, and API reference
```

---

## Architecture

### Backend (Go)

The backend uses a **session-based multiplexing** model:

1. **Bundle** (`internal/bundle/bundle.go`) — YAML parser for `Bundle` and `BundleSet` document kinds. `Load(path)` reads a file; `Parse(data)` processes raw bytes. Returns a `*File` containing an ordered slice of `*Bundle` objects, each with `[]Entry` (name, description, command, autorun).
2. **Session** (`internal/session/session.go`) — Named execution context. Holds state (ID, name, BundleName, timestamps, running flag), a map of subscribed WebSocket clients, a cancel function for the running process, and a 2,000-line replay buffer (`maxLineBuf = 2_000`). Late-joining WebSocket clients receive the buffered output on subscribe.
3. **Manager** (`internal/session/manager.go`) — Thread-safe registry (UUID → `*Session`). Provides Create/Get/List/Delete. `List()` returns snapshots sorted by creation time (oldest first).
4. **Executor** (`internal/executor/executor.go`) — Spawns a process, reads stdout/stderr concurrently in separate goroutines, and emits `Line` structs (JSON) with Unix-millisecond timestamps and line type (`stdout`, `stderr`, `start`, `exit`, `error`). Sets `cmd.WaitDelay = 5s` so I/O pipes are forcibly closed 5 seconds after context cancellation — prevents goroutine leaks when a process ignores signals.
5. **Client** (`internal/session/client.go`) — Wraps a `gorilla/websocket` connection with read/write pumps, a 256-element buffered send channel, ping/pong keepalive (`pingPeriod = 54s = pongWait * 9 / 10`), and `sync.Once`-guarded cleanup. `Send()` is non-blocking: messages are dropped if the buffer is full or the client is already disconnected.
6. **Server** (`internal/server/server.go`) — HTTP mux wired in `routes()` with:
   - `GET /` — serves embedded static files; injects `<title>` and `window.STARTUP_BUNDLE` script tag
   - `GET /ws/{id}` — upgrades to WebSocket, creates a Client, calls `client.Run()` which subscribes and replays buffered output
   - `GET /api/sessions` — list all sessions
   - `POST /api/sessions` — create session
   - `GET /api/sessions/{id}` — get a single session
   - `DELETE /api/sessions/{id}` — delete session
   - `POST /api/sessions/{id}/exec` — execute command in session
   - `POST /api/sessions/{id}/kill` — kill running process
   - `POST /api/bundles` — import YAML bundle; body is raw YAML (max 4 MiB); returns 409 if any bundle name is already imported

### Frontend (Vanilla JS)

`web/static/app.js` is a single-file application with no build step:

- **`AnsiParser`** — Converts ANSI SGR escape sequences to safe HTML spans (supports bold, dim, italic, underline, standard/256/true-color fg & bg).
- **`SessionSocket`** — WebSocket wrapper with 500 ms auto-reconnect.
- **`Terminal`** — Renders output lines with auto-scroll. Prunes `.terminal-line` DOM nodes when count exceeds `MAX_DOM_LINES = 2000` (event banners are not pruned).
- **`MarkdownRenderer`** — Lightweight Markdown-to-HTML renderer for bundle session descriptions (headings, bold, italic, inline code, code blocks, unordered lists).
- **`api`** — Thin wrapper over `fetch()` for all REST endpoints.
- **`App`** — Main controller: session creation/deletion, command dispatch, theme persistence (`localStorage`), per-session output buffering for replay.

**`INTERACTIVE_COMMANDS`** is a module-level `Set` of command names (e.g. `vim`, `htop`, `tmux`, `less`, `watch`) that cannot be driven via the streaming interface. The UI shows a warning banner when the user attempts to run one of these commands.

### Data Flow

```
Browser (REST) → POST /api/sessions/{id}/exec
                       ↓
               server.go: whitelist check, call session.Exec(opts)
                       ↓
               executor.go: spawn process, read stdout/stderr
                       ↓
               session.go: append to lineBuf (≤2000), broadcast Line JSON to all clients
                       ↓
               client.go: non-blocking enqueue to 256-element sendCh
                       ↓
               Browser (WebSocket) receives Line JSON → Terminal renders
```

When a new WebSocket client connects mid-execution, `session.subscribe()` replays the entire `lineBuf` before the client enters the live broadcast.

The `GET /` handler injects `<script>window.STARTUP_BUNDLE = true|false;</script>` into `index.html` so the frontend can decide whether to show the **Import** button.

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
make app                # .app bundle with native WKWebView window (CGO required)
make app-server         # headless .app bundle that opens UI in default browser
make dmg                # create distributable .dmg (requires 'make app' first)

# Clean
make clean
```

### Running the Server

```bash
./dist/tui-streamer [flags]

Flags:
  -port string    TCP port to listen on (default: "8080")
  -dir string     Default working directory for executed commands (default: ".")
  -title string   Window / browser-tab title (defaults to "TUI Streamer" or bundle name)
  -stdout         Capture stdout (default true)
  -stderr         Capture stderr (default true)
  -allow string   Whitelist a binary name; repeat for multiple
                  (omit to allow all commands)
  -bundle string  Path to a YAML bundle file that pre-creates sessions
  -open           Auto-launch browser on startup
```

Note: `-stdout` and `-stderr` are boolean flags, not string flags. `-allow` is a repeatable flag — specify it once per command name rather than as a comma-separated list.

### Adding a New REST Endpoint

1. Add the route in `internal/server/server.go` inside `routes()` (not `NewServer()`).
2. Write the handler as a method on `*Server`.
3. Access `s.manager` for session operations and `s.cfg` for server-level defaults.
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
- Themes use the `data-theme` attribute on `<html>` (the document root element). The `dark` theme is stored as the empty string (`data-theme=""`).

### Adding a New Theme

1. Add a `[data-theme="<name>"]` block in `web/static/style.css` defining all CSS custom properties (see existing themes for the full variable list).
2. Add the option to the `<select id="theme-select">` in `web/static/index.html`.
3. No JS changes needed — the `App` class reads the selector value and sets `document.documentElement.setAttribute('data-theme', value)`.

### WebSocket Protocol

Messages are JSON objects. The `timestamp` field is a **Unix millisecond integer** (`int64`), not an ISO string:

```json
{"type":"start","timestamp":1704067200000,"data":"","exit_code":null}
{"type":"stdout","timestamp":1704067200100,"data":"hello","exit_code":null}
{"type":"stderr","timestamp":1704067200200,"data":"error text","exit_code":null}
{"type":"exit","timestamp":1704067200300,"data":"","exit_code":0}
```

Line types: `start`, `stdout`, `stderr`, `exit`, `error`.

The frontend formats timestamps with `new Date(ms)` — the numeric millisecond value is passed directly to the `Date` constructor.

### Exec Request Body

`POST /api/sessions/{id}/exec` accepts:

```json
{
  "command": "ls -la",
  "dir": "/optional/working/dir",
  "env": ["KEY=value"],
  "stdout": true,
  "stderr": true
}
```

`command` can be either a JSON string (`"ls -la"`, split on whitespace) or a JSON array (`["ls", "-la"]`). `dir`, `env`, `stdout`, and `stderr` are optional; omitted values fall back to server-level defaults.

---

## Dependencies

| Package | Purpose |
|---|---|
| `github.com/google/uuid` | Session ID generation |
| `github.com/gorilla/websocket` | WebSocket server implementation |
| `github.com/webview/webview_go` | macOS native WebView (CGO, darwin only) |
| `gopkg.in/yaml.v3` | YAML bundle file parsing (indirect) |

Go standard library is used for HTTP, JSON, process execution, embedding, and synchronization — no web framework.

---

## CI/CD

Three GitHub Actions workflows live in `.github/workflows/`:

| Workflow | Trigger | What it does |
|---|---|---|
| `ci.yml` | Push/PR to `main` | Runs `go vet`, `staticcheck`, and `go test -race` across Go 1.22/1.23 on ubuntu/macOS/windows |
| `release.yml` | Push of `v*` tag | Builds cross-platform binaries and attaches them to the GitHub Release |
| `release-please.yml` | Push to `main` | Opens/updates a release PR that bumps the version and generates the changelog |

CI skips on changes to `*.md` and `docs/**`.

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

Use `make app` for a WKWebView windowed app (CGO required), `make app-server` for a headless server app that opens the UI in the default browser (no CGO required).

To embed a bundle YAML inside the `.app`:

```bash
make app BUNDLE=./examples/network-bundle/bundle.yaml
```

The app name defaults to the `BundleSet` or `Bundle` `metadata.name` when `BUNDLE=` is set.

---

## Bundle File Format

Bundle files are multi-document YAML. Two `kind` values are supported:

```yaml
---
apiVersion: v1
kind: BundleSet          # optional — names an ordered group of Bundles
metadata:
  name: Network Troubleshooting
spec:
  bundles:
    - name: Connectivity
    - name: DNS

---
apiVersion: v1
kind: Bundle
metadata:
  name: Connectivity
spec:
  sessions:
    - name: Ping
      description: |
        Run a **ping** test against `example.com`.
      command: ping -c 4 example.com
      autorun: true     # execute immediately when the bundle is loaded

---
apiVersion: v1
kind: Bundle
metadata:
  name: DNS
spec:
  sessions:
    - name: Dig
      description: Query `example.com` using the system resolver.
      command: dig +short example.com
      autorun: false    # pre-populate command bar; user presses Run
```

- A file without a `BundleSet` may contain one or more `Bundle` documents; they are loaded in document order.
- `description` is rendered as Markdown in the terminal panel by the `MarkdownRenderer` class.
- `POST /api/bundles` returns HTTP 409 if any bundle in the file is already imported (matched by `BundleName`).

---

## What Does Not Exist Yet (Contribution Opportunities)

- Unit tests (no `*_test.go` files currently exist)
- Session output persistence / history replay across server restarts
- Authentication / access control
- Windows packaging scripts
