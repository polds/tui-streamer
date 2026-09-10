// bundlemeta prints packaging metadata from a YAML bundle file.
//
// Usage:
//
//	bundlemeta -name          <bundle.yaml>   # metadata.name (BundleSet or first Bundle)
//	bundlemeta -icon          <bundle.yaml>   # resolved absolute appIcon path (or empty)
//	bundlemeta -files         <bundle.yaml>   # source<TAB>dest lines for spec.files
//	bundlemeta -splash        <bundle.yaml>   # resolved absolute custom splash html path (or empty)
//	bundlemeta -splash-assets <bundle.yaml>   # absolute asset paths referenced by the custom splash html
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/polds/tui-streamer/internal/bundle"
	"github.com/polds/tui-streamer/internal/splash"
)

func main() {
	name := flag.Bool("name", false, "print the bundle display name")
	icon := flag.Bool("icon", false, "print the resolved appIcon path")
	files := flag.Bool("files", false, "print source\\tdest lines for spec.files")
	splashHTML := flag.Bool("splash", false, "print the resolved custom splash html path (or empty)")
	splashAssets := flag.Bool("splash-assets", false, "print absolute paths of assets referenced by the custom splash html")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: bundlemeta [-name|-icon|-files|-splash|-splash-assets] <bundle.yaml>")
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
	case *splashHTML:
		fmt.Print(f.Splash.HTML)
	case *splashAssets:
		assets, err := splash.Assets(f.Splash)
		if err != nil {
			fmt.Fprintf(os.Stderr, "bundlemeta: %v\n", err)
			os.Exit(1)
		}
		for _, a := range assets {
			fmt.Println(a)
		}
	default:
		// Default to name so `bundlemeta file.yaml` matches bundle-name.py.
		fmt.Print(f.Name)
	}
}
