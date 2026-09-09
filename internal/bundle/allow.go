package bundle

import (
	"path/filepath"
	"strings"
)

// MergeAllowlists combines CLI and bundle allowlists.
//
// Semantics:
//   - If neither list contains any entries, the result is empty, meaning all
//     commands are allowed (historical default).
//   - If either list is non-empty, the result is the union of both. CLI
//     -allow and bundle spec.allow are therefore additive: each may introduce
//     additional permitted binaries, and neither silently drops the other.
func MergeAllowlists(cli, fromBundle []string) []string {
	if len(cli) == 0 && len(fromBundle) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(cli)+len(fromBundle))
	out := make([]string, 0, len(cli)+len(fromBundle))
	for _, list := range [][]string{cli, fromBundle} {
		for _, name := range list {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			out = append(out, name)
		}
	}
	return out
}

// CommandAllowed reports whether command[0] is permitted by allowed.
// An empty allowed list means every command is permitted.
//
// Matching is by exact token or by filepath.Base so that
// "$(TUI_PATH)/gum", "/usr/bin/ping", and "ping" all match an allow
// entry of "ping" or "gum".
func CommandAllowed(allowed []string, command []string) bool {
	if len(allowed) == 0 {
		return true
	}
	if len(command) == 0 {
		return false
	}
	token := command[0]
	base := filepath.Base(token)
	for _, a := range allowed {
		if a == token || a == base || filepath.Base(a) == base {
			return true
		}
	}
	return false
}
