package splash

import (
	"encoding/base64"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/polds/tui-streamer/internal/bundle"
)

// maxInlineBytes caps the total size of assets inlined into a custom page.
const maxInlineBytes = 10 << 20

var (
	reHead   = regexp.MustCompile(`(?is)<head[^>]*>(.*?)</head>`)
	reBody   = regexp.MustCompile(`(?is)<body[^>]*>(.*?)</body>`)
	reStyle  = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	reScript = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	reLink   = regexp.MustCompile(`(?is)<link[^>]+rel=["']?stylesheet["']?[^>]*>`)
	reHref   = regexp.MustCompile(`(?i)href=["']([^"']+)["']`)
	// Attribute and CSS references that may point at a relative file.
	reAttrRef = regexp.MustCompile(`(?i)\b(src|href)=["']([^"']+)["']`)
	reCSSURL  = regexp.MustCompile(`(?i)url\(\s*["']?([^"')]+)["']?\s*\)`)
)

// isRelativeRef reports whether ref is a bundle-relative file reference (as
// opposed to a URL, data URI, fragment or absolute path).
func isRelativeRef(ref string) bool {
	r := strings.TrimSpace(ref)
	if r == "" || strings.HasPrefix(r, "#") || strings.HasPrefix(r, "/") || strings.HasPrefix(r, "data:") {
		return false
	}
	if strings.Contains(r, "://") || strings.HasPrefix(r, "//") || strings.HasPrefix(r, "javascript:") {
		return false
	}
	return true
}

// inliner resolves relative references against root, enforcing containment
// and the size cap, and remembers every file it touched.
type inliner struct {
	root  string
	total int
	seen  map[string]bool
	files []string
}

func newInliner(htmlPath string) *inliner {
	return &inliner{root: filepath.Dir(htmlPath), seen: map[string]bool{}}
}

func (in *inliner) resolve(ref string) (string, error) {
	p := filepath.Join(in.root, filepath.FromSlash(strings.SplitN(ref, "?", 2)[0]))
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("splash asset %q: %w", ref, err)
	}
	rootAbs, _ := filepath.Abs(in.root)
	rel, err := filepath.Rel(rootAbs, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("splash asset %q escapes the splash directory", ref)
	}
	if !in.seen[abs] {
		in.seen[abs] = true
		in.files = append(in.files, abs)
	}
	return abs, nil
}

func (in *inliner) dataURI(ref string) (string, error) {
	abs, err := in.resolve(ref)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return "", fmt.Errorf("splash asset %q: %w", ref, err)
	}
	in.total += len(b)
	if in.total > maxInlineBytes {
		return "", fmt.Errorf("splash assets exceed 10 MiB")
	}
	mt := mime.TypeByExtension(filepath.Ext(abs))
	if mt == "" {
		mt = "application/octet-stream"
	}
	return "data:" + mt + ";base64," + base64.StdEncoding.EncodeToString(b), nil
}

// inlineRefs rewrites relative src/href attributes and css url() values.
func (in *inliner) inlineRefs(s string) (string, error) {
	var firstErr error
	s = reAttrRef.ReplaceAllStringFunc(s, func(m string) string {
		parts := reAttrRef.FindStringSubmatch(m)
		if !isRelativeRef(parts[2]) {
			return m
		}
		uri, err := in.dataURI(parts[2])
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			return m
		}
		return parts[1] + `="` + uri + `"`
	})
	if firstErr != nil {
		return "", firstErr
	}
	s = reCSSURL.ReplaceAllStringFunc(s, func(m string) string {
		parts := reCSSURL.FindStringSubmatch(m)
		if !isRelativeRef(parts[1]) {
			return m
		}
		uri, err := in.dataURI(parts[1])
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			return m
		}
		return `url("` + uri + `")`
	})
	return s, firstErr
}

// renderCustom loads cfg.HTML and produces head/body fragments compatible
// with the built-in styles: styles and scripts from <head> (linked
// stylesheets inlined as <style>), body content wrapped in #splash.
func renderCustom(cfg bundle.SplashConfig, in Inputs) (string, string, error) {
	raw, err := os.ReadFile(cfg.HTML)
	if err != nil {
		return "", "", fmt.Errorf("splash html: %w", err)
	}
	src := string(raw)
	il := newInliner(cfg.HTML)

	headSrc := ""
	if m := reHead.FindStringSubmatch(src); m != nil {
		headSrc = m[1]
	}
	bodySrc := src
	if m := reBody.FindStringSubmatch(src); m != nil {
		bodySrc = m[1]
	}

	var head strings.Builder
	for _, link := range reLink.FindAllString(headSrc, -1) {
		hm := reHref.FindStringSubmatch(link)
		if hm == nil || !isRelativeRef(hm[1]) {
			continue
		}
		abs, err := il.resolve(hm[1])
		if err != nil {
			return "", "", err
		}
		css, err := os.ReadFile(abs)
		if err != nil {
			return "", "", fmt.Errorf("splash stylesheet %q: %w", hm[1], err)
		}
		il.total += len(css)
		if il.total > maxInlineBytes {
			return "", "", fmt.Errorf("splash assets exceed 10 MiB")
		}
		inlined, err := il.inlineRefs(string(css))
		if err != nil {
			return "", "", err
		}
		head.WriteString("<style>")
		head.WriteString(inlined)
		head.WriteString("</style>\n")
	}
	for _, block := range append(reStyle.FindAllString(headSrc, -1), reScript.FindAllString(headSrc, -1)...) {
		inlined, err := il.inlineRefs(block)
		if err != nil {
			return "", "", err
		}
		head.WriteString(inlined)
		head.WriteString("\n")
	}

	body, err := il.inlineRefs(bodySrc)
	if err != nil {
		return "", "", err
	}
	phase := in.Phase
	if phase == "" {
		phase = PhaseIntro
	}
	wrapped := `<div id="splash" data-style="custom" data-phase="` + string(phase) + `" style="--splash-accent: ` +
		cfg.Accent + `; --splash-bg: ` + cfg.Background + `;">` + "\n" + strings.TrimSpace(body) + "\n</div>"
	return strings.TrimSpace(head.String()), wrapped, nil
}

// Assets returns the absolute paths of every relative file a custom splash
// page references (stylesheets, images, fonts). Empty for built-in styles.
func Assets(cfg bundle.SplashConfig) ([]string, error) {
	if cfg.HTML == "" {
		return nil, nil
	}
	raw, err := os.ReadFile(cfg.HTML)
	if err != nil {
		return nil, fmt.Errorf("splash html: %w", err)
	}
	il := newInliner(cfg.HTML)
	if _, err := il.inlineRefs(string(raw)); err != nil {
		return nil, err
	}
	// Stylesheets can reference further assets; walk them too.
	for _, link := range reLink.FindAllString(string(raw), -1) {
		if hm := reHref.FindStringSubmatch(link); hm != nil && isRelativeRef(hm[1]) {
			abs, err := il.resolve(hm[1])
			if err != nil {
				return nil, err
			}
			if css, err := os.ReadFile(abs); err == nil {
				if _, err := il.inlineRefs(string(css)); err != nil {
					return nil, err
				}
			}
		}
	}
	return il.files, nil
}
