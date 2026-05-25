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
└── README.md                # Minimal project description
```

---

## Architecture

### Backend (Go)

The backend uses a **session-based multiplexing** model:

1. **Session** (`internal/session/session.go`) — Named execution context. Holds state (ID, name, timestamps, running flag), a map of subscribed WebSocket clients, and a cancel function for the running process.
2. **Manager** (`internal/session/manager.go`) — Thread-safe registry (UUID → `*Session`). Provides Create/Get/List/Delete.
3. **Executor** (`internal/executor/executor.go`) — Spawns a process, reads stdout/stderr concurrently in separate goroutines, and emits `Line` structs (JSON) with timestamps and line type (`stdout`, `stderr`, `start`, `exit`, `error`). After context cancellation, scanner goroutines exit via a `select` on `ctx.Done()`, and `cmd.WaitDelay = 5s` forces pipe closure if the process ignores signals or produces excessive buffered output.
4. **Client** (`internal/session/client.go`) — Wraps a `gorilla/websocket` connection with read/write pumps, a 256-element buffered send channel, ping/pong keepalive (54s), and `sync.Once`-guarded cleanup. Messages dropped when buffer full rather than blocking the broadcaster.
5. **Server** (`internal/server/server.go`) — HTTP mux with:
 - `GET /` — serves embedded static files with server-side title and `window.STARTUP_BUNDLE` injection
 - `GET /ws/{id}` — upgrades to WebSocket, creates a Client, registers it to the session
 - `GET /api/sessions` — list all sessions
 - `POST /api/sessions` — create session
 - `GET /api/sessions/{id}` — get a single session
 - `DELETE /api/sessions/{id}` — delete session (also kills any running process)
 - `POST /api/sessions/{id}/exec` — execute command in session
 - `POST /api/sessions/{id}/kill` — kill running process
 - `POST /api/bundles` — parse a YAML bundle body (up to 4 MiB) and create declared sessions

### Frontend (Vanilla JS)

`web/static/app.js` is a single-file application with no build step:

- **`AnsiParser`** — Converts ANSI SGR escape sequences to safe HTML spans (supports bold, dim, italic, underline, blink, standard/256/true-color fg & bg).
- **`MarkdownRenderer`** — Renders a safe subset of Markdown: headings (h1–h3), paragraphs, unordered lists, fenced code blocks, and inline bold/italic/code/links. Text is HTML-escaped before inline patterns are applied, preventing injection from user-supplied content.
- **`api`** — Thin wrapper over `fetch()` for all REST endpoints.
- **`SessionSocket`** — WebSocket wrapper with 500 ms auto-reconnect on close. Reconnect is suppressed when `destroy()` is called (e.g. session deleted).
- **`Terminal`** — Renders output lines with auto-scroll. Caps the DOM at `MAX_DOM_LINES = 2000` stdout/stderr nodes, pruning oldest lines from the top while preserving event banners (start/exit/error).
- **`App`** — Main controller: session creation/deletion, command dispatch, theme persistence (`localStorage`), per-session in-memory replay buffer (capped at `MAX_BUFFER = 2000` messages), sidebar bundle grouping with collapse/expand, line selection, and copy/ray.so export.

#### Key App Features

**Interactive app detection** — Before running a command, `_runCommand` checks `INTERACTIVE_COMMANDS` (a `Set` of known curses-style binaries: `top`, `htop`, `vim`, `less`, `tmux`, etc.). If matched, the user sees a custom confirm dialog explaining that the app requires a real TTY and that output will likely appear garbled. The user can confirm to proceed anyway.

**Line selection and copy** — Click any stdout/stderr line to select it (highlights in place). `Cmd/Ctrl+click` toggles individual lines; `Shift+click` extends the selection range. A floating prompt bar shows the count and offers **Copy** (copies plain text to clipboard) and **Generate Image** (opens [ray.so](https://ray.so) with the selected text pre-filled, matching the current theme's dark/light mode). Keyboard shortcut `Cmd/Ctrl+Shift+C` copies all visible output.

**Copy all output** — The "Process started" banner contains a copy icon button that copies all stdout/stderr lines from that specific run (up to the next event banner), rather than the entire terminal history.

**Session descriptions** — When a session has a `description` field (from a bundle), it is rendered as Markdown above the terminal output using `MarkdownRenderer`. The panel is hidden if no description is set.

**Bundle grouping** — Sessions belonging to the same bundle are grouped in the sidebar under a collapsible header (the bundle name). Click the header to collapse/expand. A "ready" triangle badge appears on sessions that have a `pending_command` but haven't been run yet.

**`window.STARTUP_BUNDLE`** — The server injects `<script>window.STARTUP_BUNDLE = true/false;</script>` into the HTML response. When `true` (a bundle was loaded via `-bundle` flag), the Import button is hidden and the New Session button takes full width, since bundle-driven apps typically don't need additional sessions added interactively.

**Custom UI dialogs** — `_uiAlert` and `_uiConfirm` render modal dialogs instead of using native `alert()`/`confirm()`, which are blocked in WKWebView contexts (the macOS app mode).

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
make app            # .app bundle with native WKWebView window (CGO required)
make app-server     # .app bundle in headless mode, opens browser on launch
make dmg            # distributable .dmg (requires 'make app' first)
make app BUNDLE=./examples/network-bundle/bundle.yaml  # name app from bundle metadata

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
  -stdout         Capture stdout from spawned commands (default true)
  -stderr         Capture stderr from spawned commands (default true)
  -allow string   Whitelist a binary name; repeat flag for multiple binaries
                  (omit to allow all commands)
  -bundle string  Path to a YAML bundle file that pre-creates sessions on startup
  -open           Auto-launch browser on startup (default true inside a .app bundle)
```

### Adding a New REST Endpoint

1. Add the route in `internal/server/server.go` inside `routes()`.
2. Write the handler as a method on `*Server` or a closure.
3. Access `s.manager` for session operations.
4. Respond with JSON using `json.NewEncoder(w).Encode(...)`.
5. Set `Content-Type: application/json` at the start of every handler.

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
- Always HTML-escape user/command output before inserting into the DOM. `AnsiParser._wrap` and `MarkdownRenderer._esc` both call `.replace(/&/g,'&amp;')` etc. before any HTML is injected.
- Do not use native `alert()` or `confirm()` — they are blocked inside WKWebView. Use `App._uiAlert()` and `App._uiConfirm()` instead.
- Theme values are applied as `data-theme` attributes on `<html>` (not body classes); the empty string maps to the default dark theme.

### Adding a New Theme

1. Add a `[data-theme="<name>"]` block in `web/static/style.css` defining all CSS custom properties (see existing themes for the full variable list, including ANSI colour variables).
2. Add the option to `<select id="theme-select">` in `web/static/index.html`.
3. No JS changes needed — `App._loadTheme()` reads the selector value and sets `document.documentElement.setAttribute('data-theme', ...)`.

### Adding a New Interactive-App Warning

Extend the `INTERACTIVE_COMMANDS` `Set` near the top of `app.js`. The set is checked in `App._runCommand()` against `command[0]` (the binary name). No other changes are required.

### WebSocket Protocol

Connect to `ws://localhost:<port>/ws/{session-id}` to receive real-time output.

Each message is a JSON object (one per WebSocket frame):
```json
{"type":"start","timestamp":1704067200000}
{"type":"stdout","timestamp":1704067200100,"data":"hello\n"}
{"type":"stderr","timestamp":1704067200200,"data":"error text\n"}
{"type":"exit","timestamp":1704067200300,"exit_code":0}
{"type":"error","timestamp":1704067200400,"data":"signal: killed"}
```

**Field notes:**
- `timestamp` is Unix milliseconds (integer), not an ISO string.
- `data` is omitted on `start` and `exit` messages.
- `exit_code` is only present on `exit` messages.
- After `sess.Kill()` is called, the broadcaster suppresses any remaining `stdout`/`stderr` lines mid-flight (context cancellation check in the goroutine), but still emits the `exit` event so the UI can update its state.

**Late-joining clients** receive a replay of the last `maxLineBuf = 2000` lines buffered by the session. The buffer is cleared at the start of each new `Exec` call.

**Keepalive**: the server sends a WebSocket `ping` every ~54 seconds (54 = `pongWait * 9 / 10` = 60 s × 0.9). Clients that miss a `pong` within 60 s are disconnected. The browser `SessionSocket` reconnects automatically after 500 ms.

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

## macOS Packaging

Two `.app` bundle modes are available:

| Target | Binary | Mode | Opens |
|---|---|---|---|
| `make app` | `cmd/app` (CGO) | Native WKWebView window | Embedded browser in-process |
| `make app-server` | `cmd/server` (no CGO) | Headless HTTP server | System default browser |

Both modes call `scripts/package-macos.sh`, which:
1. Creates the standard `.app` bundle structure under `dist/`.
2. Injects the version string (from git tags) into `Info.plist`.
3. Embeds the binary and any bundle YAML into `Contents/MacOS/`.
4. Optionally code-signs (pass `--sign <identity>` or uses ad-hoc `-`).
5. Optionally creates a `.dmg` with `hdiutil` when `--dmg` is passed.

When launched as a `.app` and the port is already bound (e.g. a previous instance is running), `cmd/server` detects this via `insideAppBundle()`, opens the browser to the existing server, and exits cleanly so macOS does not report "not responding".

The `make app BUNDLE=path/to/file.yaml` variant reads the bundle's `metadata.name` to rename the `.app` (e.g. `dist/NetworkTroubleshooting.app`).

---

## Bundle Format Reference

Bundle files are multi-document YAML. Two document kinds are supported:

| Kind | Purpose |
|---|---|
| `Bundle` | A named group of sessions with optional commands and descriptions |
| `BundleSet` | An ordered list of `Bundle` references resolved within the same file |

**Parsing rules** (`internal/bundle/bundle.go`):
- Documents are separated by `---`.
- If a `BundleSet` is present, only the bundles it references (by `metadata.name`) are included; document order is preserved within that list.
- If no `BundleSet` exists, all `Bundle` documents are included in document order.
- Unknown `kind` values return a parse error (except blank documents, which are skipped).
- A file with no `Bundle` or `BundleSet` documents returns an error.

**Duplicate-import protection**: `POST /api/bundles` checks all existing sessions' `bundle_name` fields before creating anything. If any bundle in the uploaded file is already loaded (matched by name), the entire request is rejected with `409 Conflict`.

---

## Known Limitations

- **Makefile `BUNDLE_NAME` extraction**: `make app BUNDLE=...` attempts to derive the app name from the bundle's `metadata.name`. The extraction uses a Python regex against the raw YAML text and may not handle all valid YAML encodings (e.g., multi-line names, YAML anchors).
- **No shell expansion**: commands are split on whitespace by `strings.Fields` (server-side) or a basic `_shellSplit` (client-side). Glob patterns, pipes, redirections, and environment variable substitution are **not** supported. Wrap complex commands in a shell script or use `bash -c '...'`.
- **Line-by-line streaming only**: full-screen/curses apps (TUI programs that use cursor movement sequences) do not render correctly. The `INTERACTIVE_COMMANDS` set in the frontend warns about known offenders, but the list is not exhaustive.
- **No TTY**: spawned processes receive no pseudo-terminal. Programs that detect the absence of a TTY and disable colour output (e.g. `ls` on some Linux distros) will not emit ANSI codes. Use the `--color=always` flag or equivalent where available.

---

## What Does Not Exist Yet (Contribution Opportunities)

- Unit tests (no `*_test.go` files currently exist)
- CI/CD pipelines (no `.github/workflows/`)
- Session output persistence / history replay on page load
- Authentication / access control
- Windows packaging scripts
- Pre-built release binaries
