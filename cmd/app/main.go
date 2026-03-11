// tui-streamer app – starts the WebSocket server and opens the web UI inside
// a native WKWebView window (macOS) instead of relying on an external browser.
//
// Build requirements:
//   - macOS 12+ with Xcode command-line tools
//   - CGO enabled (CGO_ENABLED=1, the default on macOS)
//   - The webview/webview_go dependency (added via 'go get'):
//       go get github.com/webview/webview_go
//
// Build:
//   GOOS=darwin CGO_ENABLED=1 go build -o dist/tui-streamer-app ./cmd/app
//
// The resulting binary is placed inside a macOS .app bundle –
// use 'make app' to do this automatically.
//
//go:build darwin

package main

import (
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	webview "github.com/webview/webview_go"

	"github.com/polds/tui-streamer/internal/server"
	"github.com/polds/tui-streamer/internal/session"
	"github.com/polds/tui-streamer/web"
)

// multiFlag allows a flag to be specified more than once.
type multiFlag []string

func (f *multiFlag) String() string     { return strings.Join(*f, ", ") }
func (f *multiFlag) Set(v string) error { *f = append(*f, v); return nil }

func main() {
	port  := flag.String("port",  "0",            "TCP port to listen on (0 = random free port)")
	dir   := flag.String("dir",   ".",             "default working directory for commands")
	title := flag.String("title", "TUI Streamer",  "window title")
	debug := flag.Bool("debug",   false,            "enable WKWebView inspector / DevTools")

	var allowed multiFlag
	flag.Var(&allowed, "allow", "whitelist a binary name (repeat for multiple)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: tui-streamer-app [flags]\n\nFlags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	// ── pick a free port when 0 is requested ────────────────────────────────
	if *port == "0" {
		ln, err := net.Listen("tcp", ":0")
		if err != nil {
			log.Fatalf("port: %v", err)
		}
		*port = fmt.Sprintf("%d", ln.Addr().(*net.TCPAddr).Port)
		ln.Close()
	}

	staticFS, err := fs.Sub(web.Static, "static")
	if err != nil {
		log.Fatalf("embed: %v", err)
	}

	manager := session.NewManager()
	cfg := server.Config{
		Stdout:          true,
		Stderr:          true,
		Dir:             *dir,
		AllowedCommands: []string(allowed),
	}

	srv := server.New(manager, cfg, staticFS)
	addr := ":" + *port
	url  := "http://localhost" + addr

	// ── start HTTP server in the background ─────────────────────────────────
	go func() {
		log.Printf("tui-streamer listening on %s", url)
		if err := http.ListenAndServe(addr, srv.Handler()); err != nil {
			log.Fatal(err)
		}
	}()

	// ── create the native window ─────────────────────────────────────────────
	wv := webview.New(*debug)
	defer wv.Destroy()

	wv.SetTitle(*title)
	wv.SetSize(1280, 800, webview.HintNone)

	// Show a loading splash immediately so the user has feedback while the
	// HTTP server finishes binding its socket.
	wv.SetHtml(splashHTML())

	// Once the server is ready, navigate to it on the main (UI) thread.
	go func() {
		waitForServer(url, 10*time.Second)
		wv.Dispatch(func() {
			wv.Navigate(url)
		})
	}()

	wv.Run()
}

// waitForServer polls the local HTTP server until it responds or the timeout elapses.
func waitForServer(url string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 200 * time.Millisecond}
	for time.Now().Before(deadline) {
		if resp, err := client.Get(url); err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	log.Printf("warning: server did not respond within %s; navigating anyway", timeout)
}

// splashHTML returns an inline HTML page that is displayed in the WKWebView
// window while the embedded HTTP server is starting up.  It intentionally
// matches the app's default dark terminal theme so the transition is seamless.
func splashHTML() string {
	return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<style>
  *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }

  body {
    background: #1a1b26;
    color: #c0caf5;
    font-family: 'SF Mono', 'Fira Code', 'Cascadia Code', 'JetBrains Mono', monospace;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    height: 100vh;
    user-select: none;
    -webkit-user-select: none;
  }

  .logo {
    font-size: 5rem;
    font-weight: 700;
    color: #7aa2f7;
    letter-spacing: -0.02em;
    line-height: 1;
    margin-bottom: 2.5rem;
  }
  .logo .cursor {
    color: #9ece6a;
    animation: blink-cursor 1s step-end infinite;
  }

  .label {
    font-size: 0.8rem;
    letter-spacing: 0.18em;
    text-transform: uppercase;
    color: #414868;
    margin-bottom: 2.5rem;
  }

  .progress {
    width: 220px;
    height: 2px;
    background: #24283b;
    border-radius: 1px;
    overflow: hidden;
    position: relative;
  }
  .progress-bar {
    position: absolute;
    top: 0; left: 0;
    height: 100%;
    width: 45%;
    background: linear-gradient(90deg, transparent, #7aa2f7 50%, transparent);
    border-radius: 1px;
    animation: slide 1.4s cubic-bezier(0.4, 0, 0.6, 1) infinite;
  }

  @keyframes blink-cursor {
    0%, 100% { opacity: 1; }
    50%       { opacity: 0; }
  }
  @keyframes slide {
    0%   { transform: translateX(-100%); }
    100% { transform: translateX(520px); }
  }
</style>
</head>
<body>
  <div class="logo">&gt;<span class="cursor">_</span></div>
  <div class="label">TUI Streamer &mdash; Starting&hellip;</div>
  <div class="progress"><div class="progress-bar"></div></div>
</body>
</html>`
}
