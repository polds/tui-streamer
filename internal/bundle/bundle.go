// Package bundle defines the tui-streamer bundle format and loader.
//
// Bundle files are YAML and support two document kinds:
//
//	kind: Bundle    – a named collection of sessions
//	kind: BundleSet – an ordered list of Bundle references (resolved within the same file)
//
// A single file may contain multiple YAML documents separated by "---".
// BundleSet documents reference Bundle documents by metadata.name.
//
// File-level options (theme, allow, files, appIcon) are taken from the
// BundleSet when one is present, otherwise from the first Bundle document.
package bundle

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ── YAML document types ────────────────────────────────────────────────────

// rawDoc is a minimally-parsed YAML document used to identify the kind and
// decode the spec field into the appropriate concrete type.
type rawDoc struct {
	APIVersion string   `yaml:"apiVersion"`
	Kind       string   `yaml:"kind"`
	Metadata   metadata `yaml:"metadata"`
	Spec       yaml.Node `yaml:"spec"`
}

type metadata struct {
	Name    string `yaml:"name"`
	AppIcon string `yaml:"appIcon"`
}

type bundleSpec struct {
	Theme    string      `yaml:"theme"`
	Themes   []string    `yaml:"themes"`
	Allow    []string    `yaml:"allow"`
	Files    []FileEntry `yaml:"files"`
	Sessions []Entry     `yaml:"sessions"`
}

type bundleSetSpec struct {
	Theme   string      `yaml:"theme"`
	Themes  []string    `yaml:"themes"`
	Allow   []string    `yaml:"allow"`
	Files   []FileEntry `yaml:"files"`
	Bundles []struct {
		Name string `yaml:"name"`
	} `yaml:"bundles"`
}

// ── Public types ───────────────────────────────────────────────────────────

// Bundle is the resolved internal representation of a Bundle document.
type Bundle struct {
	// Name is the value of metadata.name in the YAML document.
	Name string
	// Sessions is the ordered list of session definitions.
	Sessions []Entry
}

// Entry is a single session definition within a Bundle.
type Entry struct {
	// Name is the session display name shown in the sidebar.
	Name string `yaml:"name"`
	// Description is an optional Markdown string describing the session.
	// It is rendered in the web UI next to the terminal output.
	Description string `yaml:"description"`
	// Command is the shell command to pre-populate in the input bar.
	// For autorun sessions it is also executed immediately on startup.
	Command string `yaml:"command"`
	// Autorun, when true, causes the command to be executed automatically when
	// the bundle is loaded. When false (the default) the command is
	// pre-loaded into the input bar but requires the user to press Run.
	Autorun bool `yaml:"autorun"`
}

// FileEntry describes an extra file or directory copied into TUI_PATH.
//
// YAML accepts either a mapping:
//
//	source: ./bin/gum
//	dest: gum
//
// or a bare string treated as source (dest defaults to the basename):
//
//	./bin/gum
type FileEntry struct {
	// Source is a path relative to the bundle YAML file (or absolute).
	Source string `yaml:"source"`
	// Dest is the relative path under TUI_PATH. Empty means the basename of Source.
	Dest string `yaml:"dest,omitempty"`
}

// UnmarshalYAML accepts either a string or a {source, dest} mapping.
func (e *FileEntry) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		e.Source = value.Value
		e.Dest = ""
		return nil
	}
	type raw FileEntry
	var decoded raw
	if err := value.Decode(&decoded); err != nil {
		return err
	}
	*e = FileEntry(decoded)
	return nil
}

// File is the result of parsing a bundle YAML file.
type File struct {
	// Name is the top-level label: the BundleSet metadata.name when a
	// BundleSet document is present, otherwise the single Bundle's name.
	Name string
	// Bundles is the ordered list of resolved Bundle objects.
	Bundles []*Bundle
	// AppIcon is a path (relative to the bundle file) to an SVG used as the
	// macOS app icon when packaging. Empty means the default icon.
	AppIcon string
	// Theme is the default UI theme name (e.g. "nord"). Empty means the
	// built-in default (catppuccin-macchiato).
	Theme string
	// Themes is the allowlist of theme names shown in the picker. Empty
	// means all built-in themes remain available.
	Themes []string
	// Allow is a list of binary names permitted for execution. Empty means
	// this file does not add an allowlist (CLI -allow still applies).
	Allow []string
	// Files are extra files/directories staged into TUI_PATH.
	Files []FileEntry
}

// ── Parsing ────────────────────────────────────────────────────────────────

// Load reads and parses a YAML bundle file at the given path.
func Load(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read bundle: %w", err)
	}
	f, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("parse bundle %q: %w", path, err)
	}
	return f, nil
}

type parsedBundle struct {
	bundle  *Bundle
	appIcon string
	theme   string
	themes  []string
	allow   []string
	files   []FileEntry
}

// Parse parses YAML bundle data. The data may contain multiple "---"-separated
// documents (BundleSet + Bundle, or multiple Bundles).
func Parse(data []byte) (*File, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))

	var bundleSetDoc *rawDoc
	// bundlesByName preserves insertion order via a slice of names.
	bundlesByName := map[string]*parsedBundle{}
	var bundleOrder []string

	for {
		var d rawDoc
		if err := dec.Decode(&d); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("yaml decode: %w", err)
		}

		switch d.Kind {
		case "Bundle":
			var spec bundleSpec
			if err := d.Spec.Decode(&spec); err != nil {
				return nil, fmt.Errorf("bundle %q spec: %w", d.Metadata.Name, err)
			}
			pb := &parsedBundle{
				bundle:  &Bundle{Name: d.Metadata.Name, Sessions: spec.Sessions},
				appIcon: d.Metadata.AppIcon,
				theme:   spec.Theme,
				themes:  spec.Themes,
				allow:   spec.Allow,
				files:   spec.Files,
			}
			if _, exists := bundlesByName[d.Metadata.Name]; !exists {
				bundleOrder = append(bundleOrder, d.Metadata.Name)
			}
			bundlesByName[d.Metadata.Name] = pb

		case "BundleSet":
			copy := d // avoid loop-variable capture
			bundleSetDoc = &copy

		default:
			if d.Kind != "" {
				return nil, fmt.Errorf("unknown kind %q (expected Bundle or BundleSet)", d.Kind)
			}
			// Skip empty / blank documents (e.g. leading ---).
		}
	}

	if len(bundlesByName) == 0 && bundleSetDoc == nil {
		return nil, fmt.Errorf("no Bundle or BundleSet documents found")
	}

	file := &File{}

	if bundleSetDoc != nil {
		file.Name = bundleSetDoc.Metadata.Name
		file.AppIcon = bundleSetDoc.Metadata.AppIcon
		var setSpec bundleSetSpec
		if err := bundleSetDoc.Spec.Decode(&setSpec); err != nil {
			return nil, fmt.Errorf("bundleset %q spec: %w", bundleSetDoc.Metadata.Name, err)
		}
		file.Theme = setSpec.Theme
		file.Themes = setSpec.Themes
		file.Allow = setSpec.Allow
		file.Files = setSpec.Files
		for _, ref := range setSpec.Bundles {
			pb, ok := bundlesByName[ref.Name]
			if !ok {
				return nil, fmt.Errorf("bundleset %q references unknown bundle %q", bundleSetDoc.Metadata.Name, ref.Name)
			}
			file.Bundles = append(file.Bundles, pb.bundle)
		}
	} else {
		// No BundleSet: include all Bundle documents in document order.
		// File-level options come from the first Bundle.
		for i, name := range bundleOrder {
			pb := bundlesByName[name]
			file.Bundles = append(file.Bundles, pb.bundle)
			if file.Name == "" {
				file.Name = pb.bundle.Name
			}
			if i == 0 {
				file.AppIcon = pb.appIcon
				file.Theme = pb.theme
				file.Themes = pb.themes
				file.Allow = pb.allow
				file.Files = pb.files
			}
		}
	}

	return file, nil
}

// ── Packaged paths ─────────────────────────────────────────────────────────

// PackagedResourcesDir returns Contents/Resources next to the executable,
// or "" if the executable path cannot be resolved.
func PackagedResourcesDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(exe), "..", "Resources")
}

// PackagedPath returns Contents/Resources/bundle.yaml (or .yml) next to the
// executable, or "" if neither file exists.
func PackagedPath() string {
	dir := PackagedResourcesDir()
	if dir == "" {
		return ""
	}
	for _, name := range []string{"bundle.yaml", "bundle.yml"} {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}
