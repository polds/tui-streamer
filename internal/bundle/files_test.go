package bundle

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestStageFiles(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	tool := filepath.Join(bin, "demo-tool")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\necho hi\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	scripts := filepath.Join(dir, "scripts")
	if err := os.Mkdir(scripts, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scripts, "helper.sh"), []byte("echo helper\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	bundlePath := filepath.Join(dir, "bundle.yaml")
	if err := os.WriteFile(bundlePath, []byte("kind: Bundle\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tuiPath, err := StageFiles(bundlePath, []FileEntry{
		{Source: "./bin/demo-tool", Dest: "demo-tool"},
		{Source: "./scripts", Dest: "scripts"},
	})
	if err != nil {
		t.Fatalf("StageFiles: %v", err)
	}
	staged := filepath.Join(tuiPath, "demo-tool")
	info, err := os.Stat(staged)
	if err != nil {
		t.Fatalf("staged binary: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
		t.Errorf("expected execute bit on staged binary, mode=%v", info.Mode())
	}
	if _, err := os.Stat(filepath.Join(tuiPath, "scripts", "helper.sh")); err != nil {
		t.Fatalf("staged dir file: %v", err)
	}
}

func TestSanitizeDestRejectsTraversal(t *testing.T) {
	_, err := sanitizeDest("../etc/passwd", "/tmp/src")
	if err == nil {
		t.Fatal("expected traversal dest to be rejected")
	}
	_, err = sanitizeDest("/abs", "/tmp/src")
	if err == nil {
		t.Fatal("expected absolute dest to be rejected")
	}
	got, err := sanitizeDest("", "/tmp/src/gum")
	if err != nil {
		t.Fatal(err)
	}
	if got != "gum" {
		t.Errorf("default dest = %q", got)
	}
}

func TestResolveTUIPathNoFiles(t *testing.T) {
	p, err := ResolveTUIPath("/tmp/bundle.yaml", nil)
	if err != nil {
		t.Fatal(err)
	}
	if p != "" {
		t.Errorf("expected empty TUI_PATH, got %q", p)
	}
}

func TestResolveTUIPathRequiresPath(t *testing.T) {
	_, err := ResolveTUIPath("", []FileEntry{{Source: "./x"}})
	if err == nil {
		t.Fatal("expected error when files listed without a bundle path")
	}
}
