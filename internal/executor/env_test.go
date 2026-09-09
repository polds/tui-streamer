package executor

import (
	"strings"
	"testing"
)

func TestExpandTUIPath(t *testing.T) {
	got := ExpandTUIPath([]string{"$(TUI_PATH)/gum", "spin", "--title", "${TUI_PATH}"}, "/opt/tui")
	if got[0] != "/opt/tui/gum" {
		t.Errorf("binary = %q", got[0])
	}
	if got[3] != "/opt/tui" {
		t.Errorf("arg = %q", got[3])
	}
	got = ExpandTUIPath([]string{"$TUI_PATH/demo-tool"}, "/cache/tui")
	if got[0] != "/cache/tui/demo-tool" {
		t.Errorf("$TUI_PATH expand = %q", got[0])
	}
	orig := []string{"echo", "hi"}
	if g := ExpandTUIPath(orig, ""); g[0] != "echo" {
		t.Errorf("empty TUI_PATH should leave command unchanged")
	}
}

func TestBuildEnv(t *testing.T) {
	if buildEnv(nil, "") != nil {
		t.Fatal("expected nil env (inherit) when TUI_PATH unset")
	}
	env := buildEnv(nil, "/opt/tui")
	var tui, path string
	for _, kv := range env {
		if strings.HasPrefix(kv, "TUI_PATH=") {
			tui = strings.TrimPrefix(kv, "TUI_PATH=")
		}
		if strings.HasPrefix(kv, "PATH=") {
			path = strings.TrimPrefix(kv, "PATH=")
		}
	}
	if tui != "/opt/tui" {
		t.Errorf("TUI_PATH = %q", tui)
	}
	if !strings.HasPrefix(path, "/opt/tui") {
		t.Errorf("PATH should start with TUI_PATH, got %q", path)
	}
}
