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
// Example (single Bundle):
//
//	---
//	apiVersion: v1
//	kind: Bundle
//	metadata:
//	  name: Diagnostics
//	spec:
//	  sessions:
//	    - name: ping
//	      command: ping -c 4 example.com
//	      autorun: true
//
// Example (BundleSet + Bundles in one file):
//
//	---
//	apiVersion: v1
//	kind: BundleSet
//	metadata:
//	  name: Network Troubleshooting
//	spec:
//	  bundles:
//	    - name: Diagnostics
//	---
//	apiVersion: v1
//	kind: Bundle
//	metadata:
//	  name: Diagnostics
//	spec:
//	  sessions: [...]
package bundle

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

// ── YAML document types ────────────────────────────────────────────────────

// rawDoc is a minimally-parsed YAML document used to identify the kind and
// decode the spec field into the appropriate concrete type.
type rawDoc struct {
	APIVersion string    `yaml:"apiVersion"`
	Kind       string    `yaml:"kind"`
	Metadata   struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Spec yaml.Node `yaml:"spec"`
}

type bundleSpec struct {
	Sessions []Entry `yaml:"sessions"`
}

type bundleSetSpec struct {
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

// File is the result of parsing a bundle YAML file.
type File struct {
	// Name is the top-level label: the BundleSet metadata.name when a
	// BundleSet document is present, otherwise the single Bundle's name.
	Name string
	// Bundles is the ordered list of resolved Bundle objects.
	Bundles []*Bundle
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

// Parse parses YAML bundle data. The data may contain multiple "---"-separated
// documents (BundleSet + Bundle, or multiple Bundles).
func Parse(data []byte) (*File, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))

	var bundleSetDoc *rawDoc
	// bundlesByName preserves insertion order via a slice of names.
	bundlesByName := map[string]*Bundle{}
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
			b := &Bundle{Name: d.Metadata.Name, Sessions: spec.Sessions}
			if _, exists := bundlesByName[d.Metadata.Name]; !exists {
				bundleOrder = append(bundleOrder, d.Metadata.Name)
			}
			bundlesByName[d.Metadata.Name] = b

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
		var setSpec bundleSetSpec
		if err := bundleSetDoc.Spec.Decode(&setSpec); err != nil {
			return nil, fmt.Errorf("bundleset %q spec: %w", bundleSetDoc.Metadata.Name, err)
		}
		for _, ref := range setSpec.Bundles {
			b, ok := bundlesByName[ref.Name]
			if !ok {
				return nil, fmt.Errorf("bundleset %q references unknown bundle %q", bundleSetDoc.Metadata.Name, ref.Name)
			}
			file.Bundles = append(file.Bundles, b)
		}
	} else {
		// No BundleSet: include all Bundle documents in document order.
		for _, name := range bundleOrder {
			b := bundlesByName[name]
			file.Bundles = append(file.Bundles, b)
			if file.Name == "" {
				file.Name = b.Name
			}
		}
	}

	return file, nil
}
