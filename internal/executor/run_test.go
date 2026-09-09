package executor

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRunExportsTUIPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX sh")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	lines, err := Run(ctx, Options{
		Command: []string{"sh", "-c", "printf '%s\\n' \"$TUI_PATH\""},
		Stdout:  true,
		TUIPath: "/opt/tui-assets",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	var stdout []string
	for line := range lines {
		if line.Type == LineTypeStdout {
			stdout = append(stdout, line.Data)
		}
	}
	got := strings.Join(stdout, "\n")
	if !strings.Contains(got, "/opt/tui-assets") {
		t.Fatalf("stdout %q does not contain TUI_PATH", got)
	}
}
