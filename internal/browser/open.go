// Package browser provides a cross-platform helper for opening URLs in the
// default web browser.
package browser

import (
	"fmt"
	"os/exec"
	"runtime"
)

// Open opens the given URL in the default browser.
// It is best-effort: any error is returned but the caller may safely ignore it.
func Open(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default: // linux, freebsd, …
		// Try xdg-open first, fall back to common alternatives.
		if path, err := exec.LookPath("xdg-open"); err == nil {
			cmd = exec.Command(path, url)
		} else if path, err := exec.LookPath("gnome-open"); err == nil {
			cmd = exec.Command(path, url)
		} else if path, err := exec.LookPath("kde-open"); err == nil {
			cmd = exec.Command(path, url)
		} else {
			return fmt.Errorf("browser: no suitable open command found on %s", runtime.GOOS)
		}
	}

	return cmd.Start()
}
