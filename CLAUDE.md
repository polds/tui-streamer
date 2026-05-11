# CLAUDE.md — AI Assistant Guide for tui-streamer

## Project Overview

**tui-streamer** is a Go-based WebSocket server that executes OS commands and streams their output (stdout/stderr) line-by-line to a web-based terminal UI in real time. It supports multiple concurrent named "sessions", each of which can have multiple WebSocket subscriber clients.

Key capabilities:
- Execute arbitrary CLI commands on demand via REST API
- Stream output line-by-line via WebSocket in JSON format
- Serve a self-contained, theme-able web UI (no build step required)
- Optional command whitelisting for security
- Native macOS `.app`/`.dmg` packaging via WKWebView (darwin only)
- Pre-configure sessions via YAML bundle files (`-bundle` flag or UI import)

---

## Repository Structure

```
tui-streamer/
├── cmd/
│   ├── app/main.go          # macOS native WebView app entry point (darwin + CGO only)
│   └── server/main.go       # HTTP/WebSocket server entry point
├── internal/
│   ├── browser/open.go      # Cross-platform browser launcher
│   ├── bundle/bundle.go     # YAML bundle loader/parser (Bundle + BundleSet kinds)
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
│   ├── make-icon.sh         # Generate AppIcon.icns from SVG (macOS, requires librsvg)
│   └── package-macos.sh     # macOS .app/.dmg packaging script
├── examples/
│   ├── README.md            # Examples index and contribution guide
│   ├── lorem-ipsum/         # External API streaming example
│   └── network-bundle/      # Multi-bundle network diagnostics example
├── .github/workflows/
│   ├── ci.yml               # Continuous integration (build + lint)
│   ├── release.yml          # Binary release workflow
│   └── release-please.yml   # Automated release PR generation
├── go.mod                   # Go module (github.com/polds/tui-streamer)
├── go.sum                   # Dependency checksums
├── Makefile                 # Build, test, lint, packaging targets
└── README.md                # Project description and usage guide
```

---

## Architecture

### Backend (Go)

The backend uses a **session-based multiplexing** model:

1. **Bundle** (`internal/bundle/bundle.go`) — Parses YAML bundle files. Supports two document kinds: `Bundle` (named collection of sessions with optional command/description/autorun) and `BundleSet` (ordered list of Bundle references within the same file). Entry points: `Load(path)` reads from disk, `Parse(data)` operates on raw bytes.

2. **Session** (`internal/session/session.go`) — Named execution context. Fields:
   - `ID`, `Name`, `CreatedAt` — identity
   - `PendingCommand` — pre-configured command string surfaced to the UI input bar
   - `BundleName` — which bundle this session belongs to (used for 409 duplicate check)
   - `Description` — optional Markdown string rendered in the terminal panel
   - `running`, `cancel`, `lineBuf` — private execution state
   - `lineBuf` caps at `maxLineBuf = 2_000` lines for replay to late-joining clients

3. **Manager** (`internal/session/manager.go`) — Thread-safe registry (UUID → `*Session`). Provides Create/Get/List/Delete.

4. **Executor** (`internal/executor/executor.go`) — Spawns a process, reads stdout/stderr concurrently in separate goroutines, and emits `Line` structs over a channel. Key details:
   - `Line.Timestamp` is a Unix millisecond integer (`int64`, `time.Now().UnixMilli()`)
   - `Line.ExitCode` is `*int` (pointer; only set on `exit` lines, omitted otherwise)
   - `waitDelay = 5s` — pipes are forcibly closed 5 seconds after context cancellation to prevent goroutine leaks

5. **Client** (`internal/session/client.go`) — Wraps a `gorilla/websocket` connection with read/write pumps:
   - 256-element buffered send channel
   - `Send()` is non-blocking; drops messages when the buffer is full (never blocks the broadcaster)
   - Ping/pong keepalive: `pongWait = 60s`, `pingPeriod = pongWait × 9/10 = 54s`
   - `sync.Once`-guarded cleanup via `close()`

6. **Server** (`internal/server/server.go`) — Constructed with `New(manager, cfg, staticFS)`. Routes registered by `routes()`:
   - `GET /` — serves embedded static files (with `STARTUP_BUNDLE` JS injection and title substitution)
   - `GET /ws/{id}` — upgrades to WebSocket, creates a Client, registers it to the session
   - `GET /api/sessions` — list all sessions
   - `POST /api/sessions` — create session
   - `GET /api/sessions/{id}` — get single session info
   - `DELETE /api/sessions/{id}` — delete session
   - `POST /api/sessions/{id}/exec` — execute command in session
   - `POST /api/sessions/{id}/kill` — kill running process
   - `POST /api/bundles` — import a YAML bundle (4 MiB body limit; returns 409 if any bundle name already exists)

### Frontend (Vanilla JS)

`web/static/app.js` is a single-file application with no build step:

- **`AnsiParser`** — Converts ANSI SGR escape sequences to safe HTML spans (supports bold, dim, italic, underline, standard/256/true-color fg & bg).
- **`SessionSocket`** — WebSocket wrapper with **500 ms** auto-reconnect.
- **`Terminal`** — Renders output lines with auto-scroll. Prunes DOM to `MAX_DOM_LINES = 2000` `.terminal-line` elements to prevent unbounded memory growth.
- **`MarkdownRenderer`** — Renders bundle session descriptions as styled Markdown HTML inside the terminal panel.
- **`App`** — Main controller: session creation/deletion, command dispatch, theme persistence (`localStorage`), per-session output buffering (`MAX_BUFFER = 2000`) for replay.
- **`api`** — Thin wrapper over `fetch()` for all REST endpoints.
- **`INTERACTIVE_COMMANDS`** — A `Set` of command names (e.g. `vim`, `top`, `tmux`) that are flagged in the UI as requiring a true TTY and therefore incompatible with this server.

### Data Flow

```
Browser (REST) → POST /api/sessions/{id}/exec
                       ↓
               server.go: validate, call session.Exec()
                       ↓
               executor.go: spawn process, read stdout/stderr
                       ↓
               session.go: broadcast Line JSON to all clients
                         + append to lineBuf (up to 2000 lines)
                       ↓
               client.go: write to WebSocket send channel
                       ↓
               Browser (WebSocket) receives Line JSON → Terminal renders

Late-joining client:
Browser (WS)  → GET /ws/{id}
               client.go: subscribe() replays lineBuf to new client immediately
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
make build              # outputs dist/tui-streamer

# Run tests
make test               # go test ./...

# Lint
make lint               # go vet ./...

# macOS-specific
make build-darwin       # universal binary (arm64 + amd64 via lipo)
make build-darwin-webview  # WKWebView binary (CGO, macOS + Xcode required)
make app                # create .app bundle with native WKWebView window (CGO required)
make app-server         # create headless server .app bundle (no CGO; cross-compilable)
make dmg                # create distributable .dmg (requires 'make app' first)

# Bundle-specific packaging
make app BUNDLE=./examples/network-bundle/bundle.yaml

# Generate app icon
make icon               # requires: brew install librsvg

# Clean
make clean
```

### Running the Server

```bash
./dist/tui-streamer [flags]

Flags:
  -port string    TCP port to listen on (default: "8080")
  -title string   Window/browser-tab title (defaults to bundle name or "TUI Streamer")
  -dir string     Default working directory for executed commands (default: ".")
  -stdout         Capture stdout (bool, default true)
  -stderr         Capture stderr (bool, default true)
  -allow string   Whitelist a binary name; repeat for multiple:
                    -allow make -allow npm
                  (omit to allow all commands)
  -bundle string  Path to a YAML bundle file that pre-creates sessions
  -open           Auto-launch browser on startup (always true inside a .app bundle)
```

### Adding a New REST Endpoint

1. Add the route in `internal/server/server.go` inside `routes()`.
2. Write the handler as a method on `*Server`.
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
- Themes use the `data-theme` attribute on `document.documentElement` (`<html>`). The default dark theme is represented as an empty-string value (`data-theme=""`). All other themes use their kebab-case name (e.g. `data-theme="dracula"`).

### Adding a New Theme

1. Add a `[data-theme="<name>"]` block in `web/static/style.css` defining all CSS custom properties (see existing themes for the full variable list).
2. Add the option to `<select id="theme-select">` in `web/static/index.html`.
3. No JS changes needed — the `App` class reads the selector value and sets `document.documentElement.setAttribute('data-theme', value)`.

### WebSocket Protocol

Messages are JSON objects (one per WebSocket frame):

```json
{"type":"start","timestamp":1704067200000}
{"type":"stdout","timestamp":1704067200100,"data":"hello"}
{"type":"stderr","timestamp":1704067200200,"data":"error text"}
{"type":"exit","timestamp":1704067200300,"exit_code":0}
```

- `timestamp` — Unix millisecond integer (`int64`)
- `data` — output text; omitted on `start` and `exit` lines
- `exit_code` — only present on `exit` lines (integer, not a string)

Line types: `start`, `stdout`, `stderr`, `exit`, `error`.

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

- `command` — string (split on whitespace) **or** array of strings (e.g. `["ls", "-la"]`)
- `dir` — working directory override (falls back to server `-dir` default)
- `env` — additional environment variables as `KEY=VALUE` strings
- `stdout` / `stderr` — per-request capture overrides (booleans; fall back to server defaults)

---

## Bundle File Format

Bundle files are multi-document YAML. Two document kinds are supported:

```yaml
---
# BundleSet: declares the ordered set of bundles in this file.
# When present, only bundles listed here are loaded (in order).
apiVersion: v1
kind: BundleSet
metadata:
  name: My Runbook        # Also used as the window title and .app name
spec:
  bundles:
    - name: Setup
    - name: Verify

---
apiVersion: v1
kind: Bundle
metadata:
  name: Setup
spec:
  sessions:
    - name: Install deps
      description: |
        Install all project dependencies.
        Run this before any other step.
      command: make install
      autorun: true          # executes immediately when bundle loads

    - name: Build
      command: make build    # pre-loaded in the command bar; user presses Run

---
apiVersion: v1
kind: Bundle
metadata:
  name: Verify
spec:
  sessions:
    - name: Tests
      command: make test
      autorun: false
```

Rules:
- A file with only `Bundle` documents (no `BundleSet`) loads all bundles in document order.
- `BundleSet.spec.bundles` references bundles **by name**; every referenced name must have a corresponding `Bundle` document in the same file.
- `description` is Markdown; rendered in the terminal panel by `MarkdownRenderer`.
- `autorun: true` splits `command` on whitespace to build the `exec.Options.Command` slice.

---

## CI/CD

| Workflow | Trigger | Purpose |
|---|---|---|
| `.github/workflows/ci.yml` | push / PR to `main` | `go build` + `go vet` on linux/macos |
| `.github/workflows/release.yml` | push of `v*` tag | Cross-compile binaries, attach to GitHub Release |
| `.github/workflows/release-please.yml` | push to `main` | Auto-generate release PRs via release-please |

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

## Security Notes

- **Command whitelisting**: use `-allow` flag in production to restrict which commands can be run.
- **No authentication**: the server assumes a trusted local network. Do not expose it publicly without adding auth.
- **WebSocket origin check** is permissive (`CheckOrigin` returns `true`) — appropriate for local dev, not for multi-tenant deployments.
- **HTML escaping**: the `AnsiParser` in `app.js` escapes all output before DOM insertion; do not bypass this.
- **INTERACTIVE_COMMANDS**: the frontend detects commands that require a real TTY (e.g. `vim`, `top`, `tmux`) and warns the user rather than attempting to run them, as they will not work correctly without a PTY.

---

## macOS Packaging

The `scripts/package-macos.sh` script:
1. Creates `.app` bundle structure under `dist/`.
2. Injects the version string (from git tags) into `Info.plist`.
3. Optionally code-signs with a provided identity or ad-hoc (`-`).
4. Optionally creates a `.dmg` with `hdiutil`.

**`make app`** — WKWebView windowed app (requires CGO + Xcode; uses `cmd/app`).  
**`make app-server`** — Headless server app (no CGO; uses `cmd/server`; opens UI in default browser).

When `BUNDLE=<path>` is passed to make, the app and DMG are named after the `BundleSet`/`Bundle` metadata name extracted from the YAML file.

---

## What Does Not Exist Yet (Contribution Opportunities)

- Unit tests (no `*_test.go` files currently exist)
- Session output persistence / history replay across server restarts
- Authentication / access control
- Windows packaging scripts
