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

	"github.com/polds/tui-streamer/internal/server"
	"github.com/polds/tui-streamer/internal/session"
	"github.com/polds/tui-streamer/web"
)

// multiFlag allows a flag to be specified more than once.
type multiFlag []string

func (f *multiFlag) String() string  { return strings.Join(*f, ", ") }
func (f *multiFlag) Set(v string) error { *f = append(*f, v); return nil }

func main() {
	port   := flag.String("port", "8080", "TCP port to listen on")
	dir    := flag.String("dir",  ".",    "default working directory for commands")
	stdout := flag.Bool("stdout", true,  "capture stdout (default true)")
	stderr := flag.Bool("stderr", true,  "capture stderr (default true)")

	var allowed multiFlag
	flag.Var(&allowed, "allow", "whitelist a binary name (repeat for multiple);\n\t\tomit to allow all commands")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: tui-streamer [flags]\n\nFlags:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Examples:
  tui-streamer                          # allow all commands, port 8080
  tui-streamer -port 3000 -dir /app     # custom port and working dir
  tui-streamer -allow make -allow npm   # whitelist specific binaries
`)
	}
	flag.Parse()

	// Strip the "static" prefix so files are served from "/".
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
	log.Printf("tui-streamer listening on http://localhost%s", addr)
	if len(allowed) > 0 {
		log.Printf("allowed commands: %s", strings.Join(allowed, ", "))
	} else {
		log.Printf("all commands allowed (use -allow to restrict)")
	}

	if err := http.ListenAndServe(addr, srv.Handler()); err != nil {
		log.Fatal(err)
	}
}
