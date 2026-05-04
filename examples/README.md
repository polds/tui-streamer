# tui-streamer Examples

This directory contains practical examples demonstrating how to use tui-streamer for various use cases.

## Available Examples

### [Network Troubleshooting Bundle](network-bundle/)

A multi-document YAML bundle file with connectivity and DNS diagnostic sessions.

**Key concepts:**
- `BundleSet` grouping multiple `Bundle` documents in one file
- `autorun` sessions that execute on load
- `description` fields rendered as Markdown in the UI

**Quick start:**
```bash
tui-streamer -bundle network-bundle/bundle.yaml -open
```

---

### [Lorem Ipsum Streamer](lorem-ipsum/)

Demonstrates fetching content from an external API and streaming it line-by-line to the terminal UI.

**Key concepts:**
- External API integration
- JSON parsing
- ANSI color formatting
- Real-time output streaming
- Both manual (UI) and automated (API) workflows

**Quick start:**
```bash
cd lorem-ipsum
./run-automated.sh
```

---

## Running Examples

Each example directory contains:
- `README.md` - Detailed documentation for that example
- Shell scripts to run the example in different modes:
  - `run-direct.sh` - Run with the tui-streamer binary
  - `run-app.sh` - Run with the macOS app bundle (macOS only)
  - `run-automated.sh` - Fully automated demo via REST API

All examples assume you've built tui-streamer first:

```bash
# From the project root
make build

# For macOS app examples (native WKWebView window, macOS + Xcode required)
make app

# For headless macOS app examples (no CGO required)
make app-server
```

---

## Example Ideas

Here are some ideas for additional examples you could create:

### 1. Build Monitor
Monitor a build process (npm, go, cargo, make) and stream the output in real-time.

### 2. Log Tail Dashboard
Tail multiple log files simultaneously in different sessions.

### 3. System Monitor
Stream system metrics (CPU, memory, disk) using tools like `top`, `iostat`, or `vmstat`.

### 4. Git Operations
Watch git operations (clone, pull, large commits) stream their output.

### 5. Docker Container Logs
Stream logs from running Docker containers using `docker logs -f`.

### 6. Network Monitor
Use `ping`, `traceroute`, or `tcpdump` to monitor network activity.

### 7. File Watcher
Watch a directory for changes and stream notifications using `fswatch` or `inotifywait`.

### 8. SSH Remote Commands
Execute commands on remote servers via SSH and stream the output.

### 9. Database Operations
Stream output from database migrations, backups, or large queries.

### 10. CI/CD Pipeline
Integrate with CI/CD tools to stream build/deployment logs.

---

## Contributing Examples

We welcome new examples! To contribute:

1. Create a new directory under `examples/`
2. Include a descriptive `README.md`
3. Add executable scripts with clear comments
4. Follow the naming pattern: `run-direct.sh`, `run-app.sh`, `run-automated.sh`
5. Test on both macOS and Linux (if applicable)
6. Update this README with a link to your example

**Example structure:**
```
examples/your-example/
├── README.md
├── your-script.sh
├── run-direct.sh
├── run-app.sh
└── run-automated.sh
```

---

## Tips for Creating Examples

### 1. Use ANSI Colors
tui-streamer supports full ANSI escape sequences:

```bash
# Colors
echo -e "\033[0;32mGreen text\033[0m"
echo -e "\033[1;33mBold yellow\033[0m"

# 256-color
echo -e "\033[38;5;208mOrange text\033[0m"
```

### 2. Stream Output Incrementally
For visual effect, add small delays:

```bash
# Stream line by line
cat file.txt | while read line; do
    echo "$line"
    sleep 0.1
done
```

### 3. Add Progress Indicators
Use Unicode characters for visual interest:

```bash
echo "⏳ Processing..."
echo "✅ Complete!"
echo "❌ Failed!"
echo "📊 Stats: ..."
```

### 4. Handle Errors Gracefully
Always check for failures and provide helpful messages:

```bash
if ! command -v jq &> /dev/null; then
    echo "⚠️  Warning: jq not found, using fallback parser"
fi
```

### 5. Make Scripts Configurable
Accept arguments for flexibility:

```bash
DELAY="${1:-0.1}"
API_URL="${2:-https://api.example.com}"
```

---

## Testing Your Example

Before submitting, test all three run modes:

```bash
# Test direct binary
./run-direct.sh

# Test macOS app (macOS only)
./run-app.sh

# Test automated mode
./run-automated.sh
```

Ensure:
- ✅ Scripts are executable (`chmod +x`)
- ✅ Paths are relative (work from any directory)
- ✅ Error messages are clear and actionable
- ✅ Colors and formatting work in the web UI
- ✅ README is comprehensive and easy to follow

---

## Resources

- [Main README](../README.md) - Full project documentation
- [CLAUDE.md](../CLAUDE.md) - Architecture and API reference
- [REST API Documentation](../README.md#rest-api) - API endpoints
- [WebSocket Protocol](../README.md#websocket) - Message format

---

Happy streaming! 🚀
