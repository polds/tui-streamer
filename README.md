# tui-streamer

<div align="center">

**Stream command-line output to a beautiful web terminal in real-time**

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go)](go.mod)
[![Platform](https://img.shields.io/badge/Platform-macOS%20%7C%20Linux%20%7C%20Windows-lightgrey)]()

[Features](#features) • [Installation](#installation) • [Quick Start](#quick-start) • [Examples](#examples) • [Usage](#usage) • [Development](#development)

</div>

---

## Overview

**tui-streamer** is a WebSocket-powered server that executes OS commands and streams their output (stdout/stderr) line-by-line to a modern web-based terminal UI in real time. Perfect for monitoring long-running tasks, creating dashboards for scripts, or building interactive command-line tools with a web interface.

### Use Cases

- **DevOps Dashboards** — Monitor deployment scripts, CI/CD pipelines, or server health checks from a browser
- **Build Monitoring** — Watch compilation output, test runs, or bundling processes in real-time
- **Remote Command Execution** — Run commands on a server and view output from anywhere on your network
- **Interactive Demos** — Showcase CLI tools with a polished web interface
- **Log Streaming** — Tail logs and system output with ANSI color support
- **macOS Native App** — Package as a standalone `.app` with native WKWebView integration

---

## Features

### Core Capabilities

- **Real-time Streaming** — Line-by-line output delivery via WebSocket with zero buffering delay
- **Multiple Sessions** — Run concurrent commands in isolated named sessions
- **ANSI Color Support** — Full support for ANSI escape sequences (bold, colors, 256-color, true-color)
- **Multi-client** — Multiple browser tabs can subscribe to the same session output
- **Process Control** — Start, stop, and kill running processes via REST API
- **No Build Step** — Vanilla JavaScript frontend, zero dependencies, runs anywhere

### Bundles & BundleSets

Pre-configure sessions in a YAML file and load them at startup or import them from the UI:

- **Bundle** — A named group of sessions, each with an optional command, description, and `autorun` flag
- **BundleSet** — Composes multiple `Bundle` documents from the same file into an ordered set
- **Markdown Descriptions** — Each session can include a `description` field rendered as styled Markdown in the terminal panel
- **Auto-execution** — Sessions with `autorun: true` start their commands immediately on load

### Web UI

- **10 Beautiful Themes** — Catppuccin (4 variants), Dark, Dracula, Matrix, Nord, Solarized, Light
- **Auto-scroll** — Smart scrolling that pauses when you scroll up to review output
- **Session Management** — Create, switch between, and delete sessions from the UI
- **Line Selection** — Click or shift-click terminal lines to copy or export to ray.so

### Security & Deployment

- **Command Whitelisting** — Restrict executable commands with `-allow` flag
- **Local-first** — Designed for trusted local networks (authentication not included)
- **macOS Packaging** — Build standalone `.app` bundles and `.dmg` installers
- **Cross-platform** — Runs on macOS, Linux, and Windows

---

## Screenshots

> **Note:** Add screenshots here showing:
> - Main terminal interface with output streaming
> - Theme selection (show 2-3 different themes)
> - Session management UI
> - macOS native app window

<!-- Example: -->
<!-- ![Main Interface](docs/screenshots/main-interface.png) -->
<!-- ![Theme Selection](docs/screenshots/themes.png) -->

---

## Installation

### Download Pre-built Binaries

Download the latest release for your platform from the [Releases](../../releases) page.

### macOS

```bash
# Install via Homebrew (coming soon)
brew install polds/tap/tui-streamer

# Or download the .dmg and drag to Applications
```

### Linux / macOS (from source)

```bash
# Clone the repository
git clone https://github.com/polds/tui-streamer.git
cd tui-streamer

# Build
make build

# The binary will be in dist/tui-streamer
./dist/tui-streamer
```

### Windows (from source)

```powershell
# Clone the repository
git clone https://github.com/polds/tui-streamer.git
cd tui-streamer

# Build
go build -o dist/tui-streamer.exe ./cmd/server

# Run
.\dist\tui-streamer.exe
```

---

## Quick Start

1. **Start the server:**
   ```bash
   ./tui-streamer -open
   ```
   This starts the server on port 8080 and opens your browser automatically.

2. **Create a session** via the web UI or API:
   ```bash
   curl -X POST http://localhost:8080/api/sessions \
     -H "Content-Type: application/json" \
     -d '{"name": "my-session"}'
   ```

3. **Execute a command:**
   ```bash
   curl -X POST http://localhost:8080/api/sessions/{session-id}/exec \
     -H "Content-Type: application/json" \
     -d '{"command": "ls", "args": ["-la"]}'
   ```

4. **Watch the output stream** in your browser at `http://localhost:8080`

---

## Examples

Ready-to-run examples are available in the [examples/](examples/) directory!

### [Network Troubleshooting Bundle](examples/network-bundle/)

A multi-bundle YAML file that pre-creates connectivity and DNS diagnostic sessions. Demonstrates `BundleSet`, multiple `Bundle` documents, `autorun`, and Markdown `description` fields.

```bash
tui-streamer -bundle examples/network-bundle/bundle.yaml -open
```

### [Lorem Ipsum Streamer](examples/lorem-ipsum/)

Fetch articles from an API and stream them in real-time. Demonstrates:
- External API integration
- JSON parsing
- ANSI color support
- Both manual (UI) and automated (API) workflows

**Quick run:**
```bash
cd examples/lorem-ipsum
./run-automated.sh
```

See the [examples README](examples/README.md) for more details and ideas for creating your own examples.

---

## Usage

### Command-Line Flags

```
./tui-streamer [flags]

Flags:
  -port string    Port to listen on (default: 8080)
  -dir string     Default working directory for executed commands (default: .)
  -title string   Window / browser-tab title
  -stdout         Capture stdout (default true)
  -stderr         Capture stderr (default true)
  -allow string   Whitelist a binary name; repeat for multiple
                  (omit to allow all commands)
  -bundle string  Path to a YAML bundle file that pre-creates sessions
  -open           Auto-launch browser on startup
```

### Bundles

A **bundle file** is a YAML document that declares sessions to be created on
startup. Sessions can have a pre-configured command, an optional Markdown
description, and an `autorun` flag.

#### Single Bundle

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
        Push the build artifacts to production.
        **Only run this after Build succeeds.**
      command: make deploy
      autorun: false
```

Load it at startup:

```bash
tui-streamer -bundle deploy.yaml -open
```

#### BundleSet — Multiple Bundles in One File

A `BundleSet` references multiple `Bundle` documents from the same file,
grouping them into an ordered set. Use `---` to separate YAML documents.

```yaml
---
apiVersion: v1
kind: BundleSet
metadata:
  name: Network Troubleshooting
  appIcon: ./icon.svg          # optional SVG used by `make app BUNDLE=...`
  splash:                      # optional startup splash (see "Splash" below)
    style: jetbrains           # minimal | arc | dia | jetbrains
    window: card               # full | card (jetbrains defaults to card)
    tagline: Connectivity & DNS diagnostics
    accent: "#bd93f9"
    background: "#282a36"
    minDuration: 1500ms
spec:
  theme: nord                  # default UI theme
  themes:                      # optional allowlist; omit for all built-in themes
    - nord
    - dracula
  allow:                       # optional command allowlist (unioned with -allow)
    - ping
    - dig
    - gum
  files:                       # extra files/dirs copied into $TUI_PATH
    - source: ./bin/gum
      dest: gum                # optional; defaults to the source basename
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
      description: Run a ping test against `example.com`
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
      description: Query `example.com` using the system resolver.
      command: dig +short example.com
      autorun: true
```

A standalone `Bundle` (no `BundleSet`) may use the same `metadata.appIcon`,
`metadata.splash`, `spec.theme`, `spec.themes`, `spec.allow`, and `spec.files`
fields. When a `BundleSet` is present it is the source of truth for those
file-level options.

#### Themes

`spec.theme` is the default. `spec.themes` limits the picker. If `themes` has a
single entry, the theme dropdown is hidden. A `localStorage` preference is
ignored when it is not in the allowlist. The server injects the config into
`index.html` (`window.THEME_CONFIG`) and also exposes `GET /api/config`.

Built-in theme names: `catppuccin-macchiato` (default), `catppuccin-latte`,
`catppuccin-frappe`, `catppuccin-mocha`, `dark`, `dracula`, `matrix`, `nord`,
`solarized`, `light`.

#### Splash

`metadata.splash` configures an animated splash shown while the app starts —
in the native `.app` (before the server is even up) and as an overlay in the
browser UI. Built-in styles: `minimal` (default), `arc` and `dia`
(full-window), `jetbrains` (borderless centred card that grows into the main
window). `html: ./splash.html` replaces the style with your own page: relative
images, fonts and stylesheets are inlined, and the page must call
`splash.animated()` when its intro is done (or it is assumed done after 5 s).

The splash is dismissed only when its intro animation has finished **and** the
UI is connected **and** `minDuration` (default `1200ms`) has elapsed, then it
fades out over 400 ms. `GET /api/config` exposes the non-HTML fields.

#### Command allowlists

`spec.allow` lists binary names (the first token of a command, or its
`filepath.Base`). **Merge semantics:** if either CLI `-allow` or `spec.allow` is
set, the effective allowlist is the **union** of both. If neither is set, every
command is allowed (historical default). Matching is by basename, so
`${TUI_PATH}/gum` is allowed when `gum` is listed.

`POST /api/bundles` merges `spec.allow` into the running server the same way.

#### Bundled files and `TUI_PATH`

`spec.files` copies extra files or directories into a known directory that is
exported as `TUI_PATH` on every executed command (and prepended to `PATH`).

Paths are resolved relative to the bundle YAML. `dest` is optional and must be
a relative path under `TUI_PATH` (no `..`). Execute bits are preserved.

```bash
"${TUI_PATH}/gum" spin --spinner dot --title "Buying Bubble Gum..." -- sleep 5
```

`TUI_PATH` is injected as an environment variable on every executed command.
Command tokens also expand `${TUI_PATH}`, `$TUI_PATH`, and `$(TUI_PATH)` so a
YAML `command:` field can reference the helper without wrapping in `sh -c`.

- **Runtime** (`tui-streamer -bundle ./file.yaml`): files next to the YAML are
  copied into a per-bundle cache directory.
- **Packaging** (`make app BUNDLE=...`): files are copied to
  `Contents/Resources/tui`, which becomes `TUI_PATH` inside the `.app`.

`POST /api/bundles` cannot stage files (the YAML arrives without a filesystem
tree). Use `-bundle` or a packaged app for `spec.files`.

#### Custom app icon

`metadata.appIcon` is an SVG path relative to the bundle file. Packaging uses
it as the macOS app icon (`make icon BUNDLE=...` / `make app BUNDLE=...`).
On macOS the SVG is converted to `AppIcon.icns`; the SVG is always copied into
`Contents/Resources/AppIcon.svg`.

#### Importing via the UI

Click **Import** in the sidebar and select a `.yaml` or `.yml` bundle file.
Bundles with a `BundleSet` or multiple `Bundle` documents are all imported in
one step.

#### Session Descriptions

When a bundle entry includes a `description` field, it is rendered as styled
Markdown in the terminal panel above the output — useful for documenting what
a command does and how to interpret its results.

```yaml
- name: Memory Usage
  description: |
    Show **resident set size** and **virtual memory** for all processes.
    Look for processes exceeding 1 GB RSS as potential memory leaks.
  command: ps aux --sort=-%mem
  autorun: true
```

### Examples

#### Monitor a Build Process

```bash
# Start server with command whitelisting
tui-streamer -allow make -allow npm -port 3000

# Execute a build
curl -X POST http://localhost:3000/api/sessions/{id}/exec \
  -H "Content-Type: application/json" \
  -d '{"command": "make build"}'
```

#### Tail Logs in Real-time

```bash
tui-streamer

curl -X POST http://localhost:8080/api/sessions/{id}/exec \
  -H "Content-Type: application/json" \
  -d '{"command": "tail -f /var/log/app.log"}'
```

### REST API

#### Sessions

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/sessions` | List all sessions |
| `POST` | `/api/sessions` | Create a new session |
| `GET` | `/api/sessions/{id}` | Get a single session |
| `DELETE` | `/api/sessions/{id}` | Delete a session |
| `POST` | `/api/sessions/{id}/exec` | Execute a command in a session |
| `POST` | `/api/sessions/{id}/kill` | Kill the running process |
| `POST` | `/api/bundles` | Import a YAML bundle (creates sessions) |
| `GET` | `/api/config` | Title, theme default/allowlist, startup-bundle flag |

#### WebSocket

Connect to `ws://localhost:8080/ws/{session-id}` to receive real-time output.

**Message Format:**
```json
{
  "type": "stdout",
  "timestamp": "2024-01-01T00:00:00Z",
  "data": "output line\n",
  "exit_code": 0
}
```

**Message Types:** `start`, `stdout`, `stderr`, `exit`, `error`

---

## Development

### Prerequisites

- Go 1.22 or later
- `make`
- macOS with Xcode (only for native app builds)

### Build from Source

```bash
# Clone the repository
git clone https://github.com/polds/tui-streamer.git
cd tui-streamer

# Build for current platform
make build

# Run tests
make test

# Run linter
make lint
```

### macOS-Specific Builds

```bash
# Build universal binary (arm64 + amd64)
make build-darwin

# Create a .app with native WKWebView window
make app

# Create a .app bundle (headless server)
make app-server

# Create a distributable .dmg
make dmg

# Package a specific bundle YAML — the app is named from the BundleSet/Bundle metadata.
# spec.files are copied to Contents/Resources/tui; metadata.appIcon becomes the app icon.
make app BUNDLE=./examples/network-bundle/bundle.yaml
make app BUNDLE=./examples/tui-path/bundle.yaml
```

### Project Structure

```
tui-streamer/
├── cmd/
│   ├── app/          # macOS native WebView app entry point
│   ├── server/       # HTTP/WebSocket server entry point
│   └── bundlemeta/   # Packaging helper (name, appIcon, files)
├── internal/
│   ├── browser/      # Cross-platform browser launcher
│   ├── bundle/       # YAML bundle parser, allowlists, TUI_PATH staging
│   ├── executor/     # Command execution engine
│   ├── server/       # HTTP routes and WebSocket handler
│   └── session/      # Session management and client connections
├── web/
│   └── static/       # Web UI (HTML, CSS, vanilla JS)
├── build/darwin/     # macOS packaging resources
└── scripts/          # Build and packaging scripts
```

### Adding New Features

See [CLAUDE.md](CLAUDE.md) for detailed architecture documentation and development guidelines.

---

## Themes

tui-streamer includes 10 carefully crafted color themes:

- **Catppuccin Macchiato** *(default)* — Warm mid-dark pastel
- **Catppuccin Latte** — Light pastel
- **Catppuccin Frappé** — Muted cool-dark pastel
- **Catppuccin Mocha** — Rich dark pastel
- **Dark** — GitHub-inspired dark
- **Dracula** — Dark purple with vibrant accents
- **Matrix** — Green-on-black with glow effect
- **Nord** — Arctic-inspired cool dark
- **Solarized** — Low-contrast warm dark
- **Light** — Clean light theme

Switch themes via the dropdown in the web UI. Your preference is saved to `localStorage`.
A bundle may set `spec.theme` / `spec.themes` to choose the default and limit
(or hide) the picker.

---

## Security Considerations

- **Command Whitelisting:** Use the `-allow` flag and/or bundle `spec.allow` in production to restrict executable commands. When either is set, the effective list is their union.
- **No Authentication:** The server assumes a trusted local network. **Do not expose publicly** without adding authentication
- **HTML Escaping:** All output is automatically escaped to prevent XSS attacks
- **Origin Checks:** WebSocket origin validation is permissive for local development

---

## Contributing

Contributions are welcome! Areas for improvement:

- [ ] Broader unit tests (bundle parser / allow / TUI_PATH helpers now have focused tests)
- [ ] CI/CD pipelines
- [ ] Session output persistence / history replay
- [ ] Authentication / access control
- [ ] Windows packaging scripts
- [ ] Pre-built binaries for releases

Please open an issue before starting work on major features.

---

## License

Apache License 2.0 — See [LICENSE](LICENSE) for details.

---

## Acknowledgments

Built with:
- [gorilla/websocket](https://github.com/gorilla/websocket) — WebSocket implementation
- [google/uuid](https://github.com/google/uuid) — UUID generation
- Vanilla JavaScript — No frameworks, no build step

---

<div align="center">

**Made with ⚡ by the tui-streamer team**

[Report Bug](../../issues) • [Request Feature](../../issues) • [Documentation](CLAUDE.md)

</div>
