# Lorem Ipsum Streamer Example

This example demonstrates how to use **tui-streamer** to execute a script that fetches content from an external API and streams it line-by-line to the web terminal UI.

## What This Example Does

The `fetch-article.sh` script:
1. Fetches a lorem ipsum article from [lorem-api.com](https://lorem-api.com)
2. Parses the JSON response (title, author, content)
3. Streams the article content line-by-line with colorful ANSI formatting
4. Demonstrates real-time output streaming in tui-streamer

## Files

```
examples/lorem-ipsum/
├── README.md            # This file
├── fetch-article.sh     # Main script that fetches and streams article
├── run-direct.sh        # Helper: Run with tui-streamer binary
├── run-app.sh           # Helper: Run with macOS .app bundle
└── run-automated.sh     # Helper: Fully automated demo via REST API
```

---

## Running the Example

There are three ways to run this example:

### Option 1: Interactive (Manual) - Direct Binary

Run tui-streamer with the binary and execute the script through the web UI.

```bash
# From the examples/lorem-ipsum directory
./run-direct.sh
```

**What happens:**
1. Starts tui-streamer server on port 8080
2. Opens your browser automatically
3. You manually create a session and run the script via the web UI

**Steps in the browser:**
1. Click **"Create New Session"**
2. Enter a session name (e.g., `lorem-demo`)
3. In the command field, enter the full path shown in the terminal
4. Click **"Execute"** and watch the article stream!

---

### Option 2: Interactive (Manual) - macOS App

Run the packaged macOS app with native WKWebView.

**Prerequisites:**
- macOS only
- App bundle must be built: `make app` (WKWebView, CGO required) or `make app-server` (headless)

```bash
# From the examples/lorem-ipsum directory
./run-app.sh
```

**What happens:**
1. Launches the TuiStreamer.app
2. Opens a native macOS window with the terminal UI
3. You manually create a session and run the script

**Steps in the app:**
1. Click **"Create New Session"**
2. Enter a session name (e.g., `lorem-demo`)
3. In the command field, paste the script path shown in the terminal
4. Click **"Execute"** and watch the streaming output!

---

### Option 3: Fully Automated (API-driven)

This script automates everything using the REST API - no manual interaction needed!

```bash
# From the examples/lorem-ipsum directory
./run-automated.sh [port]

# Or with a custom port
./run-automated.sh 3000
```

**What happens:**
1. Starts tui-streamer server
2. Creates a session via `POST /api/sessions`
3. Executes the script via `POST /api/sessions/{id}/exec`
4. Opens browser to show the streaming output
5. You just watch the magic happen!

**Behind the scenes:**
```bash
# The script does this automatically:
POST http://localhost:8080/api/sessions
  → Creates session, gets session_id

POST http://localhost:8080/api/sessions/{session_id}/exec
  → Executes: ./fetch-article.sh

# Browser connects to:
ws://localhost:8080/ws/{session_id}
  → Receives streaming output
```

---

## Running the Script Standalone

You can also run the fetch script directly without tui-streamer to see the raw output:

```bash
# Default article (ID: "foo")
./fetch-article.sh

# Custom article ID
./fetch-article.sh bar
./fetch-article.sh 123
```

---

## Customization

### Change the Article ID

Edit any of the run scripts and modify the command to include an article ID:

```bash
# In run-automated.sh, change the command to:
"command": "$SCRIPT_DIR/fetch-article.sh custom-id"
```

### Adjust Streaming Speed

The `fetch-article.sh` script includes a 0.1-second delay between lines for visual effect. To make it faster or slower:

```bash
# Edit fetch-article.sh, line ~73
sleep 0.1  # Change to 0.05 for faster, 0.2 for slower, or remove for instant
```

### Use a Different API

You can modify `fetch-article.sh` to fetch from any API:

```bash
# Change the API_URL variable
API_URL="https://api.example.com/endpoint"

# Update the JSON parsing logic to match your API's response format
```

---

## Example Output

When you run this example, you'll see:

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
📰 Lorem Ipsum Article Streamer
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Fetching article: foo
URL: https://lorem-api.com/api/article/foo

⏳ Downloading article...

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Title: [Article Title Here]
Author: [Author Name]
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

📖 Article Content:

Lorem ipsum dolor sit amet, consectetur adipiscing elit...
[content streams line by line with colors]

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
✅ Article streaming complete!
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

All ANSI colors are preserved and rendered beautifully in the tui-streamer web UI!

---

## Troubleshooting

### "Binary not found"
```bash
# Build the binary first
cd ../..
make build
```

### "App bundle not found" (macOS)
```bash
# Build the app first (WKWebView, CGO required)
cd ../..
make app
```

### "Permission denied"
```bash
# Make scripts executable
chmod +x *.sh
```

### API Connection Failed
The lorem-api.com service may be down or rate-limited. The script will show an error message. Try:
- Waiting a few seconds and running again
- Using a different article ID
- Checking your internet connection

---

## Learning Points

This example demonstrates:

1. **External API Integration** — Fetching data from REST APIs
2. **JSON Parsing** — Using `jq` (or fallback to `grep`) to parse JSON
3. **Streaming Output** — Line-by-line output with controlled pacing
4. **ANSI Colors** — Rich terminal formatting with escape sequences
5. **Session Management** — Creating and using sessions via API
6. **WebSocket Streaming** — Real-time bidirectional communication
7. **Automation** — Using REST API to script the entire workflow

---

## Next Steps

- Try modifying the script to fetch from a different API
- Create your own example that monitors a long-running process
- Experiment with different themes in the web UI
- Add command-line arguments to customize the output
- Chain multiple commands together in a session

---

## Related Documentation

- [Main README](../../README.md) - Full tui-streamer documentation
- [CLAUDE.md](../../CLAUDE.md) - Architecture and development guide
- [lorem-api.com](https://lorem-api.com) - API documentation
