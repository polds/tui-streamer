# CLAUDE.md — AI Assistant Guide for tui-streamer

## Project Overview

**tui-streamer** is a Go-based WebSocket server that executes OS commands and streams their output (stdout/stderr) line-by-line to a web-based terminal UI in real time. It supports multiple concurrent named "sessions", each of which can have multiple WebSocket subscriber clients.

Key capabilities:
- Execute arbitrary CLI commands on demand via REST API
- Stream output line-by-line via WebSocket in JSON format
- Serve a self-contained, theme-able web UI (no build step required)
- Optional command whitelisting for security
- Native macOS `.app`/`.dmg` packaging via WKWebView (darwin only)
- Pre-configure sessions from YAML bundle files (`-bundle` flag or UI import)

---

## Repository Structure

```
tui-streamer/
├── cmd/
│   ├── app/main.go          # macOS native WebView app entry point (darwin + CGO only)
│   └── server/main.go       # HTTP/WebSocket server entry point
├── internal/
│   ├── browser/open.go      # Cross-platform browser launcher
│   ├── bundle/bundle.go     # YAML bundle file parser (Bundle + BundleSet)
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
│   ├── make-icon.sh         # Generates AppIcon.icns from SVG source (macOS)
│   └── package-macos.sh     # macOS .app/.dmg packaging script
├── examples/
│   ├── README.md            # Example index and contribution guide
│   ├── lorem-ipsum/         # Fetch-and-stream article demo
│   └── network-bundle/      # Multi-bundle network diagnostics demo
├── .github/workflows/
│   ├── ci.yml               # Build + vet on push/PR
│   ├── release.yml          # Build and publish release binaries
│   └── release-please.yml   # Automated changelog and version bumps
├── release-please-config.json
├── go.mod                   # Go module (github.com/polds/tui-streamer)
├── go.sum                   # Dependency checksums
├── Makefile                 # Build, test, lint, packaging targets
└── README.md                # User-facing project documentation
```

---

## Architecture

### Backend (Go)

The backend uses a **session-based multiplexing** model:

1. **Bundle** (`internal/bundle/bundle.go`) — Parses YAML bundle files (multi-document `---` separated). Supports two document kinds:
   - `kind: Bundle` — named list of `Entry` objects (name, command, description, autorun flag).
   - `kind: BundleSet` — ordered references to `Bundle` documents within the same file.
   - Public types: `File` (parse result), `Bundle` (name + entries), `Entry` (single session config).
   - Entry point: `Load(path)` (reads file then calls `Parse`); `Parse(data []byte)` (parses raw bytes).

2. **Session** (`internal/session/session.go`) — Named execution context. Holds state (ID, name, creation time, running flag, cancel function), a map of subscribed WebSocket clients, and a replay buffer (`lineBuf`, capped at `maxLineBuf = 2,000` lines). Bundle-loaded sessions carry extra fields: `PendingCommand` (pre-populates the UI command bar), `BundleName` (group label), `Description` (Markdown, rendered in UI).

3. **Manager** (`internal/session/manager.go`) — Thread-safe registry (UUID → `*Session`). `Create(name, bundleName string)` allocates and registers a session. Provides `Get`, `List` (returns `[]Info`, sorted oldest-first), and `Delete` (also kills the running process).

4. **Executor** (`internal/executor/executor.go`) — Spawns a process via `exec.CommandContext`, reads stdout/stderr concurrently in separate goroutines, and emits `Line` structs over a buffered channel. Timestamps are Unix milliseconds (`int64`, `time.Now().UnixMilli()`). `WaitDelay = 5s` prevents goroutine leaks after cancellation. Line types: `start`, `stdout`, `stderr`, `exit`, `error`.

5. **Client** (`internal/session/client.go`) — Wraps a `gorilla/websocket` connection with read/write pumps, a 256-element buffered send channel, ping/pong keepalive (`pingPeriod = 54s` = `pongWait(60s) * 9/10`), and `sync.Once`-guarded cleanup. `Send()` is non-blocking: drops silently if the buffer is full or the client is done.

6. **Server** (`internal/server/server.go`) — Constructed via `New(manager, cfg, staticFS)`. Routes are registered in `routes()`:
   - `GET /` — serves embedded static files; injects `<title>` and `window.STARTUP_BUNDLE` into `index.html`
   - `GET /ws/{id}` — upgrades to WebSocket, creates a `Client`, calls `client.Run()` (registers to session + starts pumps)
   - `GET /api/sessions` — list all sessions
   - `POST /api/sessions` — create session
   - `GET /api/sessions/{id}` — get single session
   - `DELETE /api/sessions/{id}` — delete session (also kills any running process)
   - `POST /api/sessions/{id}/exec` — execute command in session
   - `POST /api/sessions/{id}/kill` — kill running process (204 No Content)
   - `POST /api/bundles` — parse YAML bundle body and create sessions; 409 Conflict if any bundle name already exists; 4 MiB body limit

### Frontend (Vanilla JS)

`web/static/app.js` is a single-file application with no build step:

- **`AnsiParser`** — Converts ANSI SGR escape sequences to safe HTML spans (bold, dim, italic, underline, blink, standard/bright/256/true-color fg & bg).
- **`SessionSocket`** — WebSocket wrapper with 500 ms auto-reconnect.
- **`Terminal`** — Renders output lines with auto-scroll. DOM pruned at `MAX_DOM_LINES = 2000` (`.terminal-line` nodes only).
- **`MarkdownRenderer`** — Renders bundle entry `description` fields as styled Markdown above terminal output.
- **`api`** — Thin wrapper over `fetch()` for all REST endpoints.
- **`App`** — Main controller: session creation/deletion, command dispatch, theme persistence (`localStorage`), per-session output buffering for replay, `INTERACTIVE_COMMANDS` detection (blocks tty-dependent tools with a warning).

Theme system: `data-theme` attribute is set on `document.documentElement` (`<html>`). The `dark` theme uses an empty-string attribute value. The theme selector element id is `theme-select`.

### Data Flow

```
Browser (REST) → POST /api/sessions/{id}/exec
                       ↓
               server.go: validate whitelist, call session.Exec()
                       ↓
               executor.go: spawn process, read stdout/stderr
                       ↓
               session.go: append to lineBuf (≤2000), broadcast Line JSON to all clients
                       ↓
               client.go: write to WebSocket send channel (non-blocking, drop if full)
                       ↓
               Browser (WebSocket) receives Line JSON → Terminal renders

Late-join replay: on subscribe(), buffered lineBuf lines are replayed to the
new client synchronously before any new output is delivered.
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
make build              # outputs dist/tui-streamer

# Run tests
make test               # go test ./...

# Lint
make lint               # go vet ./...

# macOS-specific
make build-darwin       # universal binary (arm64 + amd64 via lipo)
make build-darwin-webview  # WKWebView binary (CGO, macOS only)
make app                # create .app bundle with native WKWebView (CGO required)
make app-server         # create headless server .app (no CGO, cross-compilable)
make dmg                # create distributable .dmg (requires 'make app' first)
make icon               # generate AppIcon.icns from SVG (macOS + librsvg)

# Clean
make clean
```

The `BUNDLE=` make variable injects a bundle YAML into the .app at packaging time:

```bash
make app BUNDLE=./examples/network-bundle/bundle.yaml
```

### Running the Server

```bash
./dist/tui-streamer [flags]

Flags:
  -port string    Port to listen on (default: "8080")
  -title string   Window / browser-tab title (defaults to bundle name or "TUI Streamer")
  -dir string     Default working directory for commands (default: ".")
  -stdout         Capture stdout (default true)
  -stderr         Capture stderr (default true)
  -allow value    Whitelist a binary name; repeat for multiple
                  (omit to allow all commands)
  -bundle string  Path to a YAML bundle file that pre-creates sessions
  -open           Auto-launch browser on startup (default true inside .app bundle)
```

Note: `-allow` is a repeatable flag (e.g. `-allow make -allow npm`), not comma-separated. `-stdout` and `-stderr` are boolean flags, not string flags.

### Adding a New REST Endpoint

1. Add the route in `internal/server/server.go` inside `routes()`.
2. Write the handler as a method on `*Server` or inline.
3. Access `s.manager` for session operations and `s.cfg` for server-level config.
4. Respond with JSON using `json.NewEncoder(w).Encode(...)`.

### Adding a New Session Operation

1. Add a method to `Session` in `internal/session/session.go`.
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
- Theme system uses `data-theme` attribute on `document.documentElement`; add new themes in `style.css` using CSS custom properties under `[data-theme="<name>"]`.

### Adding a New Theme

1. Add a `[data-theme="<name>"]` block in `web/static/style.css` defining all CSS variables (see existing themes for the full variable list). The `dark` theme uses `[data-theme=""]` (empty string).
2. Add the option to the `<select id="theme-select">` in `web/static/index.html`.
3. No JS changes needed — the `App` class reads the selector value and sets it as the `data-theme` attribute on `document.documentElement`.

### WebSocket Protocol

Messages are JSON objects. The `timestamp` field is a Unix millisecond integer (`int64`):
```json
{"type":"start","timestamp":1704067200000,"data":"","exit_code":null}
{"type":"stdout","timestamp":1704067200100,"data":"hello","exit_code":null}
{"type":"stderr","timestamp":1704067200200,"data":"error text","exit_code":null}
{"type":"exit","timestamp":1704067200300,"data":"","exit_code":0}
```

Line types: `start`, `stdout`, `stderr`, `exit`, `error`.

`exit_code` is present and non-null only on `exit` lines. `data` is omitted on `start`/`exit` when empty.

### Exec Request Body

`POST /api/sessions/{id}/exec` accepts:

```json
{
  "command": "ls -la",          // string (split on whitespace) OR array ["ls", "-la"]
  "dir": "/optional/path",      // overrides server -dir for this request
  "env": ["KEY=VALUE"],         // additional environment variables
  "stdout": true,               // override server -stdout for this request
  "stderr": true                // override server -stderr for this request
}
```

`command` is required; all other fields are optional. Returns `202 Accepted` `{"status":"started"}` or `409 Conflict` if a command is already running.

### Bundle File Format

Bundle files are multi-document YAML. Two document kinds are supported:

```yaml
---
# Optional: a BundleSet groups multiple Bundle documents.
# Without a BundleSet, all Bundle documents are included in document order.
apiVersion: v1
kind: BundleSet
metadata:
  name: My Runbook          # becomes the app title when loaded via -bundle
spec:
  bundles:
    - name: Diagnostics     # must match a Bundle metadata.name below

---
apiVersion: v1
kind: Bundle
metadata:
  name: Diagnostics
spec:
  sessions:
    - name: ping                               # session display name
      description: |                          # optional Markdown (rendered in UI)
        Run a connectivity check.
        **Expected:** ~0% packet loss.
      command: ping -c 4 example.com           # pre-populated in command bar
      autorun: true                            # execute immediately on load
    - name: dig
      command: dig +short example.com
      autorun: false                           # user must click Run
```

`POST /api/bundles` (UI import) returns `201 Created` `{"status":"imported"}` or `409 Conflict` if any bundle name already exists, with a 4 MiB body limit.

---

## CI/CD

| File | Trigger | Purpose |
|---|---|---|
| `.github/workflows/ci.yml` | push / PR | `go build` + `go vet` on Linux and macOS |
| `.github/workflows/release.yml` | push tag `v*` | Build binaries for all platforms, create GitHub Release |
| `.github/workflows/release-please.yml` | push to `main` | Automated changelog PRs and version bump via release-please |

---

## Dependencies

| Package | Purpose |
|---|---|
| `github.com/google/uuid` | Session ID generation |
| `github.com/gorilla/websocket` | WebSocket server implementation |
| `github.com/webview/webview_go` | macOS native WebView (CGO, darwin only) |
| `gopkg.in/yaml.v3` | Bundle YAML parsing |

Go standard library is used for HTTP, JSON, process execution, embedding, and synchronization — no web framework.

---

## Security Notes

- **Command whitelisting**: use `-allow` flag (repeatable) in production to restrict which commands can be run. Checked against the first token of the command array.
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

| Make target | Binary | CGO | Description |
|---|---|---|---|
| `make app` | `cmd/app` (WKWebView) | Required | Windowed native app |
| `make app-server` | `cmd/server` (universal) | Not required | Headless server, opens browser |
| `make dmg` | Same as `make app` | Required | Creates distributable `.dmg` |

The `BUNDLE=` variable can be passed to any of these targets to embed a bundle YAML file inside the `.app`.

---

## What Does Not Exist Yet (Contribution Opportunities)

- Unit tests (no `*_test.go` files currently exist)
- Session output persistence across server restarts
- Authentication / access control
- Windows packaging scripts
