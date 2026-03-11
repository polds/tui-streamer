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

### Web UI

- **6 Beautiful Themes** — Dracula, Monokai, Solarized Dark, Solarized Light, Gruvbox, Nord
- **Auto-scroll** — Smart scrolling that pauses when you scroll up to review output
- **Responsive Design** — Works on desktop, tablet, and mobile
- **Session Management** — Create, switch between, and delete sessions from the UI

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

```bash
./tui-streamer [flags]

Flags:
  -port int       Port to listen on (default: 8080)
  -dir string     Working directory for executed commands
  -stdout string  Override stdout for spawned commands
  -stderr string  Override stderr for spawned commands
  -allow string   Comma-separated command whitelist (e.g. "ls,cat,echo")
  -open           Auto-launch browser on startup
```

### Examples

#### Monitor a Build Process

```bash
# Start server with command whitelisting
./tui-streamer -allow "npm,go,make,cargo" -port 3000

# Execute a build
curl -X POST http://localhost:3000/api/sessions/{id}/exec \
  -H "Content-Type: application/json" \
  -d '{"command": "npm", "args": ["run", "build"]}'
```

#### Tail Logs in Real-time

```bash
# Start server
./tui-streamer

# Tail a log file
curl -X POST http://localhost:8080/api/sessions/{id}/exec \
  -H "Content-Type: application/json" \
  -d '{"command": "tail", "args": ["-f", "/var/log/app.log"]}'
```

#### Run Long-running Scripts

```bash
# Perfect for deployment scripts, migrations, or backups
curl -X POST http://localhost:8080/api/sessions/{id}/exec \
  -H "Content-Type: application/json" \
  -d '{"command": "./deploy.sh", "args": ["production"]}'
```

### REST API

#### Sessions

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/sessions` | List all sessions |
| `POST` | `/api/sessions` | Create a new session |
| `DELETE` | `/api/sessions/{id}` | Delete a session |
| `POST` | `/api/sessions/{id}/exec` | Execute a command in a session |
| `POST` | `/api/sessions/{id}/kill` | Kill the running process |

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

# You can also package a specific bundle JSON, which will dynamically name the application
make app BUNDLE=./examples/network-bundle/bundle.json
```

### Project Structure

```
tui-streamer/
├── cmd/
│   ├── app/          # macOS native WebView app entry point
│   └── server/       # HTTP/WebSocket server entry point
├── internal/
│   ├── browser/      # Cross-platform browser launcher
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

tui-streamer includes 6 carefully crafted color themes:

- **Dracula** — Dark purple theme with vibrant accents
- **Monokai** — Classic dark theme with warm colors
- **Solarized Dark** — Low-contrast dark theme
- **Solarized Light** — Low-contrast light theme
- **Gruvbox** — Retro warm dark theme
- **Nord** — Arctic-inspired cool dark theme

Switch themes via the dropdown in the web UI. Your preference is saved to localStorage.

---

## Security Considerations

- **Command Whitelisting:** Use the `-allow` flag in production to restrict executable commands
- **No Authentication:** The server assumes a trusted local network. **Do not expose publicly** without adding authentication
- **HTML Escaping:** All output is automatically escaped to prevent XSS attacks
- **Origin Checks:** WebSocket origin validation is permissive for local development

---

## Contributing

Contributions are welcome! Areas for improvement:

- [ ] Unit tests (no test coverage currently exists)
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
