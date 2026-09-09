// bundlemeta prints packaging metadata from a YAML bundle file.
//
// Usage:
//
//	bundlemeta -name  <bundle.yaml>   # metadata.name (BundleSet or first Bundle)
//	bundlemeta -icon  <bundle.yaml>   # resolved absolute appIcon path (or empty)
//	bundlemeta -files <bundle.yaml>   # source<TAB>dest lines for spec.files
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/polds/tui-streamer/internal/bundle"
)

func main() {
	name := flag.Bool("name", false, "print the bundle display name")
	icon := flag.Bool("icon", false, "print the resolved appIcon path")
	files := flag.Bool("files", false, "print source\\tdest lines for spec.files")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: bundlemeta [-name|-icon|-files] <bundle.yaml>")
		os.Exit(2)
	}

	path := flag.Arg(0)
	abs, err := filepath.Abs(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bundlemeta: %v\n", err)
		os.Exit(1)
	}
	f, err := bundle.Load(abs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bundlemeta: %v\n", err)
		os.Exit(1)
	}
	base := filepath.Dir(abs)

	switch {
	case *name:
		fmt.Print(f.Name)
	case *icon:
		if f.AppIcon == "" {
			return
		}
		p := f.AppIcon
		if !filepath.IsAbs(p) {
			p = filepath.Join(base, p)
		}
		resolved, err := filepath.Abs(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "bundlemeta: %v\n", err)
			os.Exit(1)
		}
		fmt.Print(resolved)
	case *files:
		for _, e := range f.Files {
			src := e.Source
			if !filepath.IsAbs(src) {
				src = filepath.Join(base, src)
			}
			resolved, err := filepath.Abs(src)
			if err != nil {
				fmt.Fprintf(os.Stderr, "bundlemeta: %v\n", err)
				os.Exit(1)
			}
			dest, err := bundle.DestName(e)
			if err != nil {
				fmt.Fprintf(os.Stderr, "bundlemeta: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("%s\t%s\n", resolved, dest)
		}
	default:
		// Default to name so `bundlemeta file.yaml` matches bundle-name.py.
		fmt.Print(f.Name)
	}
}
