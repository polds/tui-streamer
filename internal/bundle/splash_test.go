package bundle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSplashDefaults(t *testing.T) {
	f, err := Parse([]byte(`
apiVersion: v1
kind: Bundle
metadata:
  name: Plain
spec:
  sessions: []
`))
	if err != nil {
		t.Fatal(err)
	}
	s := f.Splash
	if s.Style != SplashStyleMinimal || s.Window != SplashWindowFull {
		t.Errorf("defaults: style=%q window=%q", s.Style, s.Window)
	}
	if s.MinDuration != 1200*time.Millisecond {
		t.Errorf("minDuration = %v", s.MinDuration)
	}
	if s.Size != [2]int{640, 400} {
		t.Errorf("size = %v", s.Size)
	}
	if s.Accent == "" || s.Background == "" {
		t.Errorf("colours should have defaults: %+v", s)
	}
}

func TestSplashParseFields(t *testing.T) {
	f, err := Parse([]byte(`
apiVersion: v1
kind: BundleSet
metadata:
  name: Set
  splash:
    style: arc
    window: card
    tagline: Hello there
    accent: "#ff0000"
    background: "rgb(1, 2, 3)"
    minDuration: 2s
    size: [800, 500]
spec:
  bundles: []
`))
	if err != nil {
		t.Fatal(err)
	}
	s := f.Splash
	if s.Style != SplashStyleArc || s.Window != SplashWindowCard || s.Tagline != "Hello there" {
		t.Errorf("parsed = %+v", s)
	}
	if s.Accent != "#ff0000" || s.Background != "rgb(1, 2, 3)" {
		t.Errorf("colours = %q %q", s.Accent, s.Background)
	}
	if s.MinDuration != 2*time.Second || s.Size != [2]int{800, 500} {
		t.Errorf("duration/size = %v %v", s.MinDuration, s.Size)
	}
}

func TestSplashJetBrainsDefaultsToCard(t *testing.T) {
	f, err := Parse([]byte(`
apiVersion: v1
kind: Bundle
metadata:
  name: JB
  splash:
    style: jetbrains
spec:
  sessions: []
`))
	if err != nil {
		t.Fatal(err)
	}
	if f.Splash.Window != SplashWindowCard {
		t.Errorf("jetbrains window = %q, want card", f.Splash.Window)
	}
}

func TestSplashBundleSetWinsOverBundle(t *testing.T) {
	f, err := Parse([]byte(`
apiVersion: v1
kind: BundleSet
metadata:
  name: Set
  splash:
    style: dia
spec:
  bundles:
    - name: One
---
apiVersion: v1
kind: Bundle
metadata:
  name: One
  splash:
    style: arc
spec:
  sessions: []
`))
	if err != nil {
		t.Fatal(err)
	}
	if f.Splash.Style != SplashStyleDia {
		t.Errorf("style = %q, want dia (BundleSet wins)", f.Splash.Style)
	}
}

func TestSplashValidation(t *testing.T) {
	cases := map[string]string{
		"bad style":    "style: neon",
		"bad window":   "window: popup",
		"bad accent":   `accent: "red; } </style><script>"`,
		"bad duration": "minDuration: soon",
		"size small":   "size: [10, 400]",
		"size len":     "size: [640]",
	}
	for name, field := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte("apiVersion: v1\nkind: Bundle\nmetadata:\n  name: X\n  splash:\n    " + field + "\nspec:\n  sessions: []\n"))
			if err == nil {
				t.Fatalf("expected error for %s", field)
			}
		})
	}
}

func TestSplashHTMLResolvedByLoad(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "splash.html"), []byte("<html></html>"), 0o644)
	bundlePath := filepath.Join(dir, "bundle.yaml")
	os.WriteFile(bundlePath, []byte("apiVersion: v1\nkind: Bundle\nmetadata:\n  name: X\n  splash:\n    html: ./splash.html\nspec:\n  sessions: []\n"), 0o644)

	f, err := Load(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	if f.Splash.HTML != filepath.Join(dir, "splash.html") {
		t.Errorf("HTML = %q", f.Splash.HTML)
	}
}

func TestSplashHTMLMustStayInsideBundleDir(t *testing.T) {
	dir := t.TempDir()
	bundlePath := filepath.Join(dir, "bundle.yaml")
	os.WriteFile(bundlePath, []byte("apiVersion: v1\nkind: Bundle\nmetadata:\n  name: X\n  splash:\n    html: ../outside.html\nspec:\n  sessions: []\n"), 0o644)
	_, err := Load(bundlePath)
	if err == nil || !strings.Contains(err.Error(), "splash html") {
		t.Fatalf("expected splash html error, got %v", err)
	}
}

func TestSplashHTMLMissingFile(t *testing.T) {
	dir := t.TempDir()
	bundlePath := filepath.Join(dir, "bundle.yaml")
	os.WriteFile(bundlePath, []byte("apiVersion: v1\nkind: Bundle\nmetadata:\n  name: X\n  splash:\n    html: ./nope.html\nspec:\n  sessions: []\n"), 0o644)
	if _, err := Load(bundlePath); err == nil {
		t.Fatal("expected error for missing splash html")
	}
}
