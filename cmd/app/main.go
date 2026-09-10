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
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	webview "github.com/webview/webview_go"

	"github.com/polds/tui-streamer/internal/bundle"
	"github.com/polds/tui-streamer/internal/server"
	"github.com/polds/tui-streamer/internal/session"
	"github.com/polds/tui-streamer/internal/splash"
	"github.com/polds/tui-streamer/web"
)

// version is set at build time via -ldflags "-X main.version=<tag>" (see Makefile).
var version = "dev"

// multiFlag allows a flag to be specified more than once.
type multiFlag []string

func (f *multiFlag) String() string     { return strings.Join(*f, ", ") }
func (f *multiFlag) Set(v string) error { *f = append(*f, v); return nil }

func main() {
	defaultTitle := "TUI Streamer"
	var b *bundle.File
	tuiPath := bundle.PackagedTUIPath()
	if path := bundle.PackagedPath(); path != "" {
		if loaded, err := bundle.Load(path); err == nil {
			b = loaded
			if b.Name != "" {
				defaultTitle = b.Name
			}
			resolved, err := bundle.ResolveTUIPath(path, b.Files)
			if err != nil {
				log.Printf("bundle files: %v", err)
			} else if resolved != "" {
				tuiPath = resolved
			}
		}
	}

	port  := flag.String("port",  "0",            "TCP port to listen on (0 = random free port)")
	dir   := flag.String("dir",   ".",             "default working directory for commands")
	title := flag.String("title", defaultTitle,  "window title")
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

	bundleAllow := []string(nil)
	if b != nil {
		bundleAllow = b.Allow
	}
	effectiveAllow := bundle.MergeAllowlists([]string(allowed), bundleAllow)

	cfg := server.Config{
		Title:            *title,
		Stdout:           true,
		Stderr:           true,
		Dir:              *dir,
		AllowedCommands:  effectiveAllow,
		HasStartupBundle: b != nil,
		TUIPath:          tuiPath,
	}
	cfg.Version = version
	splashCfg := bundle.DefaultSplash()
	var iconSVG []byte
	if b != nil {
		cfg.Theme = b.Theme
		cfg.Themes = bundle.FilterThemes(b.Themes)
		splashCfg = b.Splash
		iconSVG = bundle.LoadIconSVG(bundle.PackagedPath(), b.AppIcon)
	} else {
		iconSVG = bundle.LoadIconSVG("", "")
	}
	cfg.Splash = splashCfg
	cfg.SplashIcon = iconSVG

	srv := server.New(manager, cfg, staticFS)
	if b != nil {
		srv.ImportFile(b)
	}
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

	const mainW, mainH = 1280, 800
	card := splashCfg.Window == bundle.SplashWindowCard
	wv.SetTitle(*title)
	if card {
		wv.SetSize(splashCfg.Size[0], splashCfg.Size[1], webview.HintNone)
		applyCardWindow(wv.Window(), splashCfg.Size[0], splashCfg.Size[1], splashCfg.Background)
	} else {
		wv.SetSize(mainW, mainH, webview.HintNone)
	}

	// Show the splash immediately so the user has feedback while the HTTP
	// server finishes binding its socket.
	doc, err := splash.Render(splashCfg, splash.Inputs{Title: *title, Version: version, IconSVG: iconSVG, Phase: splash.PhaseIntro})
	if err != nil {
		log.Fatalf("splash: %v", err)
	}
	splashShown := time.Now()
	wv.SetHtml(doc.HTML)

	// ── handoff state machine ────────────────────────────────────────────────
	// Navigate to the UI when the splash intro has finished and the server is
	// up (or after 10s regardless). In card mode, restore the main window once
	// the UI's overlay reports it has faded out.
	var (
		handoffMu sync.Mutex
		animated  bool
		serverUp  bool
		navigated bool
	)
	navigate := func() {
		handoffMu.Lock()
		defer handoffMu.Unlock()
		if navigated || !(animated && serverUp) {
			return
		}
		navigated = true
		remaining := splashCfg.MinDuration - time.Since(splashShown)
		if remaining < 0 {
			remaining = 0
		}
		target := url + "/?splash=final&remaining=" + strconv.FormatInt(remaining.Milliseconds(), 10)
		wv.Dispatch(func() { wv.Navigate(target) })
	}
	wv.Bind("__splashPost", func(name string) {
		switch name {
		case "animated":
			handoffMu.Lock()
			animated = true
			handoffMu.Unlock()
			navigate()
		case "dismissed":
			if card {
				wv.Dispatch(func() { restoreMainWindow(wv.Window(), mainW, mainH, *title) })
			}
		}
	})
	go func() {
		waitForServer(url, 10*time.Second)
		handoffMu.Lock()
		serverUp = true
		animated = animated || time.Since(splashShown) > 10*time.Second // give up waiting for the page
		handoffMu.Unlock()
		navigate()
	}()
	// Safety net: a custom page that never reports `animated` still hands off.
	time.AfterFunc(splashCfg.MinDuration+5*time.Second, func() {
		handoffMu.Lock()
		animated = true
		handoffMu.Unlock()
		navigate()
	})

	// WKWebView does not wire up the standard macOS Edit-menu responder actions
	// (copy:, paste:, selectAll:, etc.) because the app has no NSMenu with those
	// items.  Inject a capture-phase keydown listener so that the common macOS
	// keyboard shortcuts still work inside the web content.
	wv.Init(`(function () {
		document.addEventListener('keydown', function (e) {
			if (!e.metaKey) return;
			var handled = true;
			switch (e.key) {
				case 'a': document.execCommand('selectAll', false); break;
				case 'c': document.execCommand('copy',      false); break;
				case 'x': document.execCommand('cut',       false); break;
				case 'v': document.execCommand('paste',     false); break;
				case 'z':
					if (e.shiftKey) document.execCommand('redo', false);
					else            document.execCommand('undo', false);
					break;
				default: handled = false;
			}
			// Prevent the event from reaching the macOS responder chain after we
			// have handled it.  Without this the OS finds no matching responder
			// and plays the system "beep" even though the action succeeded.
			if (handled) e.preventDefault();
		}, true /* capture */);
	})();`)

	// Bind a function to open the native macOS file picker
	wv.Bind("openFileDialog", func() (string, error) {
		cmd := exec.Command("osascript", "-e", `POSIX path of (choose file with prompt "Select a bundle YAML file" of type {"public.yaml", "public.data"})`)
		out, err := cmd.Output()
		if err != nil {
			// user likely hit cancel
			return "", nil
		}
		path := strings.TrimSpace(string(out))
		if path == "" {
			return "", nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		return string(content), nil
	})

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
