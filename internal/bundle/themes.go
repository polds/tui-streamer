package bundle

// BuiltInThemes is the ordered list of theme names shipped in the web UI.
// Keep in sync with <select id="theme-select"> in web/static/index.html.
var BuiltInThemes = []string{
	"catppuccin-macchiato",
	"catppuccin-latte",
	"catppuccin-frappe",
	"catppuccin-mocha",
	"dark",
	"dracula",
	"matrix",
	"nord",
	"solarized",
	"light",
}

// DefaultTheme is used when a bundle does not set spec.theme.
const DefaultTheme = "catppuccin-macchiato"

// FilterThemes returns the intersection of requested names with BuiltInThemes,
// preserving the requested order. Unknown names are dropped.
func FilterThemes(requested []string) []string {
	if len(requested) == 0 {
		return nil
	}
	known := make(map[string]struct{}, len(BuiltInThemes))
	for _, t := range BuiltInThemes {
		known[t] = struct{}{}
	}
	out := make([]string, 0, len(requested))
	seen := make(map[string]struct{}, len(requested))
	for _, t := range requested {
		if _, ok := known[t]; !ok {
			continue
		}
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}

// ResolveDefaultTheme picks a theme that is present in allowed.
// If allowed is empty, all built-in themes are treated as allowed.
func ResolveDefaultTheme(requested string, allowed []string) string {
	pool := allowed
	if len(pool) == 0 {
		pool = BuiltInThemes
	}
	inPool := func(name string) bool {
		for _, t := range pool {
			if t == name {
				return true
			}
		}
		return false
	}
	if requested != "" && inPool(requested) {
		return requested
	}
	if inPool(DefaultTheme) {
		return DefaultTheme
	}
	if len(pool) > 0 {
		return pool[0]
	}
	return DefaultTheme
}
