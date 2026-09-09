package executor

import (
	"os"
	"strings"
)

// ExpandTUIPath replaces $(TUI_PATH), ${TUI_PATH}, and $TUI_PATH in each
// command token. This matches both Makefile-style bundle YAML and shell
// parameter expansion so scripts can write $(TUI_PATH)/gum.
func ExpandTUIPath(command []string, tuiPath string) []string {
	if tuiPath == "" || len(command) == 0 {
		return command
	}
	out := make([]string, len(command))
	for i, tok := range command {
		out[i] = expandTUIPathToken(tok, tuiPath)
	}
	return out
}

func expandTUIPathToken(s, tuiPath string) string {
	s = strings.ReplaceAll(s, "$(TUI_PATH)", tuiPath)
	s = strings.ReplaceAll(s, "${TUI_PATH}", tuiPath)
	s = strings.ReplaceAll(s, "$TUI_PATH", tuiPath)
	return s
}

func buildEnv(extra []string, tuiPath string) []string {
	if tuiPath == "" && extra == nil {
		return nil // inherit the process environment
	}
	env := extra
	if env == nil {
		env = os.Environ()
	} else {
		env = append([]string(nil), extra...)
	}
	if tuiPath == "" {
		return env
	}
	env = upsertEnv(env, "TUI_PATH", tuiPath)
	pathVal := lookupEnv(env, "PATH")
	if pathVal == "" {
		pathVal = os.Getenv("PATH")
	}
	env = upsertEnv(env, "PATH", tuiPath+string(os.PathListSeparator)+pathVal)
	return env
}

func lookupEnv(env []string, key string) string {
	prefix := key + "="
	for i := len(env) - 1; i >= 0; i-- {
		if strings.HasPrefix(env[i], prefix) {
			return env[i][len(prefix):]
		}
	}
	return ""
}

func upsertEnv(env []string, key, value string) []string {
	prefix := key + "="
	for i, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}
