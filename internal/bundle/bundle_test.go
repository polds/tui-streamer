package bundle

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseBundleSetFeatures(t *testing.T) {
	const yaml = `
apiVersion: v1
kind: BundleSet
metadata:
  name: Network Troubleshooting
  appIcon: ./icon.svg
spec:
  theme: nord
  themes:
    - nord
    - dracula
  allow:
    - ping
    - dig
    - gum
  files:
    - source: ./bin/gum
      dest: gum
    - ./bin/helper
  bundles:
    - name: Connectivity
---
apiVersion: v1
kind: Bundle
metadata:
  name: Connectivity
spec:
  sessions:
    - name: Ping
      command: ping -c 4 example.com
      autorun: true
`
	f, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if f.Name != "Network Troubleshooting" {
		t.Errorf("Name = %q", f.Name)
	}
	if f.AppIcon != "./icon.svg" {
		t.Errorf("AppIcon = %q", f.AppIcon)
	}
	if f.Theme != "nord" {
		t.Errorf("Theme = %q", f.Theme)
	}
	if len(f.Themes) != 2 || f.Themes[0] != "nord" || f.Themes[1] != "dracula" {
		t.Errorf("Themes = %#v", f.Themes)
	}
	if len(f.Allow) != 3 || f.Allow[0] != "ping" || f.Allow[2] != "gum" {
		t.Errorf("Allow = %#v", f.Allow)
	}
	if len(f.Files) != 2 {
		t.Fatalf("Files len = %d", len(f.Files))
	}
	if f.Files[0].Source != "./bin/gum" || f.Files[0].Dest != "gum" {
		t.Errorf("Files[0] = %+v", f.Files[0])
	}
	if f.Files[1].Source != "./bin/helper" || f.Files[1].Dest != "" {
		t.Errorf("Files[1] = %+v", f.Files[1])
	}
	if len(f.Bundles) != 1 || f.Bundles[0].Name != "Connectivity" {
		t.Errorf("Bundles = %#v", f.Bundles)
	}
	if len(f.Bundles[0].Sessions) != 1 || f.Bundles[0].Sessions[0].Name != "Ping" {
		t.Errorf("Sessions = %#v", f.Bundles[0].Sessions)
	}
}

func TestParseStandaloneBundleConfig(t *testing.T) {
	const yaml = `
apiVersion: v1
kind: Bundle
metadata:
  name: Deploy
  appIcon: ../icons/app.svg
spec:
  theme: matrix
  themes: [matrix]
  allow: [make]
  files:
    - source: ./tools
      dest: tools
  sessions:
    - name: Build
      command: make build
`
	f, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if f.Name != "Deploy" {
		t.Errorf("Name = %q", f.Name)
	}
	if f.AppIcon != "../icons/app.svg" {
		t.Errorf("AppIcon = %q", f.AppIcon)
	}
	if f.Theme != "matrix" {
		t.Errorf("Theme = %q", f.Theme)
	}
	if len(f.Themes) != 1 || f.Themes[0] != "matrix" {
		t.Errorf("Themes = %#v", f.Themes)
	}
	if len(f.Allow) != 1 || f.Allow[0] != "make" {
		t.Errorf("Allow = %#v", f.Allow)
	}
	if len(f.Files) != 1 || f.Files[0].Dest != "tools" {
		t.Errorf("Files = %#v", f.Files)
	}
}

func TestParseBundleSetWinsOverBundleConfig(t *testing.T) {
	const yaml = `
apiVersion: v1
kind: BundleSet
metadata:
  name: Top
  appIcon: ./set.svg
spec:
  theme: nord
  bundles:
    - name: Inner
---
apiVersion: v1
kind: Bundle
metadata:
  name: Inner
  appIcon: ./inner.svg
spec:
  theme: dracula
  sessions:
    - name: A
      command: echo hi
`
	f, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if f.AppIcon != "./set.svg" {
		t.Errorf("AppIcon = %q, want set icon", f.AppIcon)
	}
	if f.Theme != "nord" {
		t.Errorf("Theme = %q, want BundleSet theme", f.Theme)
	}
}

func TestParseUnknownKind(t *testing.T) {
	_, err := Parse([]byte("kind: Widget\nmetadata:\n  name: x\n"))
	if err == nil {
		t.Fatal("expected error for unknown kind")
	}
}

func TestParseUnknownBundleRef(t *testing.T) {
	const yaml = `
kind: BundleSet
metadata:
  name: Set
spec:
  bundles:
    - name: Missing
`
	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for missing bundle ref")
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bundle.yaml")
	if err := os.WriteFile(path, []byte(`
kind: Bundle
metadata:
  name: Solo
spec:
  sessions:
    - name: S
      command: echo
`), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if f.Name != "Solo" {
		t.Errorf("Name = %q", f.Name)
	}
}
