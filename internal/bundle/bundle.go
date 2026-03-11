// Package bundle defines the tui-streamer bundle format and loader.
// A bundle is a portable playbook/runbook that declares a set of named
// sessions, each with a pre-configured command that can be executed
// automatically on startup or left for manual execution.
package bundle

import (
	"encoding/json"
	"fmt"
	"os"
)

// Bundle describes a portable playbook of pre-configured sessions.
type Bundle struct {
	// Name is a human-readable label for the bundle (used in log output).
	Name string `json:"name"`
	// Sessions is the ordered list of session definitions in this bundle.
	Sessions []Entry `json:"sessions"`
}

// Entry is a single session definition within a Bundle.
type Entry struct {
	// Name is the session display name shown in the sidebar.
	Name string `json:"name"`
	// Command is the shell command to pre-populate in the input bar.
	// For auto=true sessions it is also executed immediately on startup.
	Command string `json:"command"`
	// Auto, when true, causes the command to be executed automatically when
	// the server starts. When false the command is pre-loaded into the input
	// bar but requires the user to press Run.
	Auto bool `json:"auto"`
}

// Load reads and parses a bundle JSON file at the given path.
func Load(path string) (*Bundle, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read bundle: %w", err)
	}
	var b Bundle
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("parse bundle: %w", err)
	}
	return &b, nil
}
