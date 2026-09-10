package bundle

import (
	"os"
	"path/filepath"
)

// LoadIconSVG returns the app icon SVG for splash rendering: the packaged
// Contents/Resources/AppIcon.svg when running inside a .app, otherwise
// appIcon resolved relative to bundlePath. Returns nil when nothing is found.
func LoadIconSVG(bundlePath, appIcon string) []byte {
	if dir := PackagedResourcesDir(); dir != "" {
		if b, err := os.ReadFile(filepath.Join(dir, "AppIcon.svg")); err == nil {
			return b
		}
	}
	if appIcon == "" {
		return nil
	}
	p := appIcon
	if !filepath.IsAbs(p) {
		if bundlePath == "" {
			return nil
		}
		p = filepath.Join(filepath.Dir(bundlePath), p)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	return b
}
