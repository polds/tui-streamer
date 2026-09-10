package bundle

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadIconSVGFromBundleDir(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "icon.svg"), []byte("<svg/>"), 0o644)
	got := LoadIconSVG(filepath.Join(dir, "bundle.yaml"), "./icon.svg")
	if string(got) != "<svg/>" {
		t.Fatalf("got %q", got)
	}
}

func TestLoadIconSVGMissingIsNil(t *testing.T) {
	if got := LoadIconSVG(filepath.Join(t.TempDir(), "bundle.yaml"), "./none.svg"); got != nil {
		t.Fatalf("expected nil, got %q", got)
	}
	if got := LoadIconSVG("", ""); got != nil {
		t.Fatalf("expected nil for empty inputs, got %q", got)
	}
}

// TestLoadIconSVGFallsBackToRepoRootAppIcon covers the third fallback: dev
// mode running from the repo root, with no packaged Resources dir and no
// bundle appIcon, still finds build/darwin/AppIcon.svg relative to cwd.
func TestLoadIconSVGFallsBackToRepoRootAppIcon(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "build", "darwin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "build", "darwin", "AppIcon.svg"), []byte("<svg stock/>"), 0o644); err != nil {
		t.Fatal(err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(cwd); err != nil {
			t.Fatal(err)
		}
	}()

	got := LoadIconSVG("", "")
	if string(got) != "<svg stock/>" {
		t.Fatalf("got %q", got)
	}
}
