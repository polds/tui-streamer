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
	// HTML comments are dropped before any extraction so a comment that mentions
	// <body> or <style> cannot become a false match.
	reComment = regexp.MustCompile(`(?s)<!--.*?-->`)
	reHead    = regexp.MustCompile(`(?is)<head[^>]*>(.*?)</head>`)
	reBody    = regexp.MustCompile(`(?is)<body[^>]*>(.*?)</body>`)
	reStyle   = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	reScript  = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	reLink    = regexp.MustCompile(`(?is)<link[^>]+rel=["']?stylesheet["']?[^>]*>`)
	reHref    = regexp.MustCompile(`(?i)href=["']([^"']+)["']`)
	// Attribute and CSS references that may point at a relative file.
	reAttrRef = regexp.MustCompile(`(?i)\b(src|href)=["']([^"']+)["']`)
	reCSSURL  = regexp.MustCompile(`(?i)url\(\s*["']?([^"')]+)["']?\s*\)`)
	// reOpenTag matches the opening of a <style> or <script> tag so a
	// data-splash marker attribute can be inserted right after the tag name,
	// leaving any existing attributes untouched.
	reOpenTag = regexp.MustCompile(`(?is)^<(style|script)([\s>])`)
)

// markDataSplash inserts a data-splash attribute into the opening <style> or
// <script> tag of a head block copied from a custom page, so app.js's
// splash:dismissed handler (web/static/app.js) can find and remove every
// element Render placed in Head, not just #splash itself.
func markDataSplash(tag string) string {
	return reOpenTag.ReplaceAllString(tag, "<$1 data-splash$2")
}

// isRelativeRef reports whether ref is a bundle-relative file reference (as
// opposed to a URL, data URI, fragment or absolute path).
func isRelativeRef(ref string) bool {
	r := strings.TrimSpace(ref)
	if r == "" || strings.HasPrefix(r, "#") || strings.HasPrefix(r, "/") || strings.HasPrefix(r, "data:") {
		return false
	}
	// A percent-encoded fragment (url(%23id) inside an SVG data URI) is still
	// a fragment, not a file.
	if strings.HasPrefix(strings.ToLower(r), "%23") {
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
	rootEval, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return "", fmt.Errorf("splash asset %q: %w", ref, err)
	}
	absEval := abs
	if _, statErr := os.Lstat(abs); statErr == nil {
		absEval, err = filepath.EvalSymlinks(abs)
		if err != nil {
			return "", fmt.Errorf("splash asset %q: %w", ref, err)
		}
	}
	rel, err := filepath.Rel(rootEval, absEval)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("splash asset %q escapes the splash directory", ref)
	}
	if !in.seen[absEval] {
		in.seen[absEval] = true
		in.files = append(in.files, absEval)
	}
	return absEval, nil
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
//
// Known limitations, none of which are validated at bundle-load time:
//   - srcset attributes are not processed (only src/href and CSS url()).
//   - @import rules without a url(...) wrapper (bare @import "x.css";) are
//     not followed or inlined.
//   - <link rel="preload stylesheet"> (a space-separated rel list) is not
//     recognised as a stylesheet link; only an exact rel="stylesheet" is.
//   - head <style> blocks are always re-emitted before head <script> blocks,
//     regardless of their original order in the source document.
//   - <a href="x.html"> relative links are treated the same as any other
//     relative reference and get their target inlined as a data URI, even
//     though the file is HTML, not an asset.
func renderCustom(cfg bundle.SplashConfig, in Inputs) (string, string, error) {
	raw, err := os.ReadFile(cfg.HTML)
	if err != nil {
		return "", "", fmt.Errorf("splash html: %w", err)
	}
	src := reComment.ReplaceAllString(string(raw), "")
	il := newInliner(cfg.HTML)

	headSrc := ""
	headIdx := reHead.FindStringSubmatchIndex(src)
	if headIdx != nil {
		headSrc = src[headIdx[2]:headIdx[3]]
	}
	bodySrc := src
	if m := reBody.FindStringSubmatch(src); m != nil {
		bodySrc = m[1]
	} else if headIdx != nil {
		// No <body> tag: the whole document is treated as the body, but the
		// matched <head>…</head> region must be cut out first or its style/
		// script blocks would be emitted twice (once in head, once in body).
		bodySrc = src[:headIdx[0]] + src[headIdx[1]:]
	}

	var head strings.Builder
	for _, link := range reLink.FindAllString(headSrc, -1) {
		hm := reHref.FindStringSubmatch(link)
		if hm == nil {
			continue
		}
		if !isRelativeRef(hm[1]) {
			head.WriteString(link)
			head.WriteString("\n")
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
		head.WriteString("<style data-splash>")
		head.WriteString(inlined)
		head.WriteString("</style>\n")
	}
	for _, block := range append(reStyle.FindAllString(headSrc, -1), reScript.FindAllString(headSrc, -1)...) {
		inlined, err := il.inlineRefs(block)
		if err != nil {
			return "", "", err
		}
		head.WriteString(markDataSplash(inlined))
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
// It walks the same references renderCustom inlines, so it shares that
// function's limitations (see its doc comment): srcset, bare @import,
// multi-token rel lists, and relative <a href> targets are not discovered.
func Assets(cfg bundle.SplashConfig) ([]string, error) {
	if cfg.HTML == "" {
		return nil, nil
	}
	raw, err := os.ReadFile(cfg.HTML)
	if err != nil {
		return nil, fmt.Errorf("splash html: %w", err)
	}
	src := reComment.ReplaceAllString(string(raw), "")
	il := newInliner(cfg.HTML)
	if _, err := il.inlineRefs(src); err != nil {
		return nil, err
	}
	// Stylesheets can reference further assets; walk them too.
	for _, link := range reLink.FindAllString(src, -1) {
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
