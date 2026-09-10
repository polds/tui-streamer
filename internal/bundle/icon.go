package bundle

import (
	"os"
	"path/filepath"
)

// LoadIconSVG returns the app icon SVG for splash rendering, trying in
// order: the packaged Contents/Resources/AppIcon.svg when running inside a
// .app, appIcon resolved relative to bundlePath, and (dev mode, running from
// the repo root) build/darwin/AppIcon.svg relative to the current working
// directory. Returns nil when none of these are found.
func LoadIconSVG(bundlePath, appIcon string) []byte {
	if dir := PackagedResourcesDir(); dir != "" {
		if b, err := os.ReadFile(filepath.Join(dir, "AppIcon.svg")); err == nil {
			return b
		}
	}
	if appIcon != "" {
		p := appIcon
		if !filepath.IsAbs(p) && bundlePath != "" {
			p = filepath.Join(filepath.Dir(bundlePath), p)
		}
		if !filepath.IsAbs(p) && bundlePath == "" {
			p = ""
		}
		if p != "" {
			if b, err := os.ReadFile(p); err == nil {
				return b
			}
		}
	}
	if b, err := os.ReadFile(filepath.Join("build", "darwin", "AppIcon.svg")); err == nil {
		return b
	}
	return nil
}
