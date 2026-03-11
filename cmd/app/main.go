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
// The resulting binary can be placed inside a macOS .app bundle just like the
// standard server binary – use 'make app-webview' to do this automatically.
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

func (f *multiFlag) String() string        { return strings.Join(*f, ", ") }
func (f *multiFlag) Set(v string) error    { *f = append(*f, v); return nil }

func main() {
	port   := flag.String("port", "0", "TCP port to listen on (0 = random free port)")
	dir    := flag.String("dir",  ".",  "default working directory for commands")
	stdout := flag.Bool("stdout", true, "capture stdout")
	stderr := flag.Bool("stderr", true, "capture stderr")
	title  := flag.String("title", "TUI Streamer", "window title")

	var allowed multiFlag
	flag.Var(&allowed, "allow", "whitelist a binary name (repeat for multiple)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: tui-streamer-app [flags]\n\nFlags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	// Pick a free port when 0 is requested.
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
		Stdout:          *stdout,
		Stderr:          *stderr,
		Dir:             *dir,
		AllowedCommands: []string(allowed),
	}

	srv := server.New(manager, cfg, staticFS)
	addr := ":" + *port
	url  := "http://localhost" + addr

	// Start the HTTP server in the background.
	go func() {
		log.Printf("tui-streamer listening on %s", url)
		if err := http.ListenAndServe(addr, srv.Handler()); err != nil {
			log.Fatal(err)
		}
	}()

	// Wait briefly for the server to be ready.
	waitForServer(url, 3*time.Second)

	// Open a native WKWebView window.
	wv := webview.New(true) // true = debug mode (DevTools enabled)
	defer wv.Destroy()

	wv.SetTitle(*title)
	wv.SetSize(1280, 800, webview.HintNone)
	wv.Navigate(url)
	wv.Run()
}

// waitForServer polls until the local HTTP server responds or the timeout elapses.
func waitForServer(url string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 200 * time.Millisecond}
	for time.Now().Before(deadline) {
		if resp, err := client.Get(url); err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	log.Printf("warning: server did not respond within %s; opening WebView anyway", timeout)
}
