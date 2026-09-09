// tui-streamer – stream command output over WebSocket with a beautiful web UI.
package main

import (
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/polds/tui-streamer/internal/browser"
	"github.com/polds/tui-streamer/internal/bundle"
	"github.com/polds/tui-streamer/internal/server"
	"github.com/polds/tui-streamer/internal/session"
	"github.com/polds/tui-streamer/web"
)

// version is set at build time via -ldflags "-X main.version=<tag>".
var version = "dev"

// multiFlag allows a flag to be specified more than once.
type multiFlag []string

func (f *multiFlag) String() string  { return strings.Join(*f, ", ") }
func (f *multiFlag) Set(v string) error { *f = append(*f, v); return nil }

func main() {
	port   := flag.String("port", "8080", "TCP port to listen on")
	title  := flag.String("title", "",    "window title (defaults to tui-streamer or bundle name)")
	dir    := flag.String("dir",  ".",    "default working directory for commands")
	stdout := flag.Bool("stdout", true,  "capture stdout (default true)")
	stderr := flag.Bool("stderr", true,  "capture stderr (default true)")

	// Default -open to true when launched as a macOS .app bundle so the browser
	// opens automatically and the user sees feedback (the app has no Dock icon).
	open := flag.Bool("open", insideAppBundle(), "open the web UI in the default browser after startup")

	var allowed multiFlag
	flag.Var(&allowed, "allow", "whitelist a binary name (repeat for multiple);\n\t\tomit to allow all commands")

	var defaultBundle string
	if insideAppBundle() {
		defaultBundle = bundle.PackagedPath()
	}
	bundlePath := flag.String("bundle", defaultBundle, "path to a YAML bundle file that pre-creates sessions with\n\t\toptional auto-execution (see docs for format)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: tui-streamer [flags]\n\nFlags:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Examples:
  tui-streamer                          # allow all commands, port 8080
  tui-streamer -port 3000 -dir /app     # custom port and working dir
  tui-streamer -allow make -allow npm   # whitelist specific binaries
  tui-streamer -open                    # launch browser automatically
  tui-streamer -bundle runbook.yaml     # load a pre-configured session bundle
`)
	}
	flag.Parse()

	// Strip the "static" prefix so files are served from "/".
	staticFS, err := fs.Sub(web.Static, "static")
	if err != nil {
		log.Fatalf("embed: %v", err)
	}

	manager := session.NewManager()

	var loaded *bundle.File
	tuiPath := bundle.PackagedTUIPath()
	bundleAllow := []string(nil)
	if *bundlePath != "" {
		f, err := bundle.Load(*bundlePath)
		if err != nil {
			log.Fatalf("bundle: %v", err)
		}
		loaded = f
		if *title == "" && f.Name != "" {
			*title = f.Name
		}
		resolved, err := bundle.ResolveTUIPath(*bundlePath, f.Files)
		if err != nil {
			log.Fatalf("bundle files: %v", err)
		}
		if resolved != "" {
			tuiPath = resolved
		}
		bundleAllow = f.Allow
	}

	if *title == "" {
		*title = "TUI Streamer"
	}

	effectiveAllow := bundle.MergeAllowlists([]string(allowed), bundleAllow)

	cfg := server.Config{
		Title:            *title,
		Stdout:           *stdout,
		Stderr:           *stderr,
		Dir:              *dir,
		AllowedCommands:  effectiveAllow,
		HasStartupBundle: *bundlePath != "",
		TUIPath:          tuiPath,
	}
	if loaded != nil {
		cfg.Theme = loaded.Theme
		cfg.Themes = bundle.FilterThemes(loaded.Themes)
	}

	srv := server.New(manager, cfg, staticFS)
	if loaded != nil {
		srv.ImportFile(loaded)
	}

	addr := ":" + *port
	url := "http://localhost" + addr
	log.Printf("tui-streamer %s listening on %s", version, url)
	if len(effectiveAllow) > 0 {
		log.Printf("allowed commands: %s", strings.Join(effectiveAllow, ", "))
	} else {
		log.Printf("all commands allowed (use -allow or bundle spec.allow to restrict)")
	}
	if tuiPath != "" {
		log.Printf("TUI_PATH=%s", tuiPath)
	}



	if *open {
		// Give the HTTP listener a moment to bind before opening the browser.
		go func() {
			time.Sleep(150 * time.Millisecond)
			if err := browser.Open(url); err != nil {
				log.Printf("browser: could not open %s: %v", url, err)
			}
		}()
	}

	if err := http.ListenAndServe(addr, srv.Handler()); err != nil {
		// When running as a .app and the port is already taken, a previous
		// instance is likely running.  Open the browser to that instance and
		// exit cleanly so macOS doesn't report the process as "not responding".
		if insideAppBundle() {
			log.Printf("could not start server (%v); opening browser to existing instance", err)
			_ = browser.Open(url)
			os.Exit(0)
		}
		log.Fatal(err)
	}
}

// insideAppBundle reports whether the current process was launched from inside
// a macOS .app bundle (i.e. its executable path contains ".app/Contents/MacOS/").
func insideAppBundle() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	return strings.Contains(exe, ".app/Contents/MacOS/")
}
