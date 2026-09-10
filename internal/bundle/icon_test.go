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
