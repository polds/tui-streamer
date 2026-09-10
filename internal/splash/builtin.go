package splash

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"strings"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed assets/shim.js
var shimJS string

//go:embed assets/base.css
var baseCSS string

// templateData is what every built-in template receives.
type templateData struct {
	Title, Tagline, Version string
	Accent, Background      template.CSS // validated by bundle; safe to pass through
	Phase                   Phase
	IconDataURI             template.URL // "" when no icon
	HasIcon                 bool
}

// iconSrc renders a data-URI icon as a whole src="…" attribute. html/template's
// URL normalizer HTML-entity-encodes "+" even for an already-vetted
// template.URL substituted into an attribute *value* (it appears both in
// "image/svg+xml" and in standard base64 payloads), which would break exact
// data-URI matching. Emitting the complete attribute as template.HTMLAttr
// instead inserts it verbatim.
func iconSrc(u template.URL) template.HTMLAttr {
	return template.HTMLAttr(`src="` + string(u) + `"`)
}

var builtins = template.Must(template.New("").Funcs(template.FuncMap{"iconSrc": iconSrc}).ParseFS(templateFS, "templates/*.html"))

// renderBuiltin executes templates/<style>.html and splits the result into a
// <style> head fragment and the #splash body fragment.
func renderBuiltin(style string, d templateData) (head, body string, err error) {
	t := builtins.Lookup(style + ".html")
	if t == nil {
		return "", "", fmt.Errorf("splash style %q: no template", style)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, d); err != nil {
		return "", "", fmt.Errorf("splash style %q: %w", style, err)
	}
	out := buf.String()
	// Templates are written as <style>…</style> followed by the #splash div.
	i := strings.Index(out, "</style>")
	if i < 0 {
		return "", strings.TrimSpace(out), nil
	}
	i += len("</style>")
	return strings.TrimSpace(out[:i]), strings.TrimSpace(out[i:]), nil
}
