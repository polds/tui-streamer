# Splash Screen — Design

**Date:** 2026-09-09
**Status:** approved in chat, pending spec review

## Goal

Let a bundle configure an animated splash screen — Arc/Dia-style full-window,
or JetBrains-style chromeless card — that plays identically in the native
macOS `.app` (WKWebView) and in browser mode, and is dismissed only when its
intro animation has finished **and** the UI is connected.

## Background

`cmd/app` uses `webview/webview_go`: one `WKWebView` inside one `NSWindow`.
It already shows a hard-coded HTML splash via `SetHtml` while the HTTP server
binds, then `Navigate`s to the UI. `wv.Window()` exposes the `NSWindow*`, so
the window's style mask and frame can be changed from cgo. Browser mode
(`cmd/server`, `make app-server`) has no splash today.

## Non-goals

- Splash on Windows/Linux native windows (cgo code is darwin-only; the web
  overlay works everywhere).
- Per-session or per-bundle (as opposed to per-file) splash configuration.
- Video or Lottie players. Templates are CSS/SVG animation; custom HTML may
  bring its own.

## 1. Schema

`splash` lives on `metadata`, next to `appIcon`. A `BundleSet`'s value wins
over a standalone `Bundle`'s, matching the existing file-level option rule.

```yaml
metadata:
  name: iCloud Photo Sync
  appIcon: ./icon.svg
  splash:
    style: jetbrains          # minimal | arc | dia | jetbrains   (default: minimal)
    window: card              # full | card   (default: full; style jetbrains defaults to card)
    html: ./splash.html       # optional custom page; overrides `style`
    tagline: Syncing your memories
    accent: "#5b8def"         # CSS color
    background: "#0b1020"     # CSS color; also the crossfade color
    minDuration: 1200ms       # Go duration; default 1200ms; 0 = dismiss as soon as ready
    size: [640, 400]          # card only; default 640×400
```

Parsed into:

```go
type SplashConfig struct {
    Style       string        // enum
    Window      string        // enum
    HTML        string        // resolved absolute path or ""
    Tagline     string
    Accent      string
    Background  string
    MinDuration time.Duration
    Size        [2]int
}
```

Validation (in `internal/bundle`, at parse time):

- `style` ∈ {minimal, arc, dia, jetbrains}; `window` ∈ {full, card}.
- `accent` / `background` must match `^[#a-zA-Z0-9(),.% -]+$` (passed to CSS
  verbatim, so they must not be able to close a style attribute or tag).
- `html` must resolve inside the bundle's directory (same `isWithin` rule as
  `spec.files`); missing file is an error at load time.
- `size` both values in [200, 4000].
- `tagline` is HTML-escaped when rendered.
- `splash: {}` or absent → defaults. **The built-in `minimal` style replaces
  today's hard-coded splash**, so every app uses the new pipeline.

`bundle.File` gains `Splash SplashConfig` (always populated, defaults applied).
`GET /api/config` gains `"splash": {style, window, tagline, accent,
background, minDurationMs}` — no HTML.

## 2. `internal/splash`

```go
type Phase string // "intro" | "final"

type Inputs struct {
    Title   string
    Version string
    IconSVG []byte // may be nil → template's fallback glyph
    Phase   Phase
}

func Render(cfg bundle.SplashConfig, in Inputs) (string, error)
```

Returns one self-contained HTML document: no external requests, icon inlined
as a `data:image/svg+xml;base64` URI, config injected as
`window.SPLASH = {...}` (JSON-encoded, `</` escaped) and the **shim** (below)
injected right after `<head>`.

Built-in styles are `embed`ded Go templates (`html/template`, so user strings
are escaped by context):

| style | look |
|---|---|
| `minimal` | today's `>_` cursor + indeterminate line, colored by `accent`/`background` |
| `arc` | full-bleed animated gradient mesh, icon rises and settles, tagline fades in |
| `dia` | soft light gradient, breathing orb behind the icon, title tracks in |
| `jetbrains` | card: artwork panel (gradient + icon) left, name/tagline/version right, indeterminate bar along the bottom |

`Phase == "final"` renders the same layout with intro animations disabled
(templates gate `@keyframes` classes on `data-phase="intro"`); indeterminate
progress keeps running until dismissal.

Custom `html:`: file is read, the `<script>window.SPLASH…</script>` + shim
are injected after `<head>` (or prepended if none), and relative `src` /
`href` / `url()` references are inlined as data URIs (max 10 MiB total,
error otherwise). A custom page is responsible for calling
`splash.animated()`; if it never does, the shim auto-fires after 5 s.

### Readiness protocol

Identical on both hosts. The shim defines `window.splash`:

```
page → host : splash.animated()    intro finished (auto: when all CSS animations on
                                    [data-splash-intro] elements end, or after 5 s)
host → page : splash.dismiss()     host calls when animated ∧ connected ∧ minDuration elapsed;
                                    shim adds .splash-dismissing (400 ms fade to `background`)
page → host : splash.dismissed()   fade complete (auto after 400 ms if the page doesn't call it)
```

Transport: the shim posts events through `window.__splashHost.post(name)` if
present (native host binds it), else dispatches `CustomEvent('splash:'+name)`
on `document` (browser host). Templates only call `splash.*`.

`connected` means:
- native: `/api/config` responded **and** the UI page reported its first
  WebSocket open (`splashConnected` bind), or the UI has no sessions;
- browser: `/api/sessions` loaded **and** the first `SessionSocket` opened, or
  there are no sessions.

`minDuration` is measured from the splash's first paint
(`performance.timeOrigin` of the splash document; the UI page receives the
remaining time via `?splash=final&remaining=<ms>` so a native handoff does not
double-count).

## 3. Native host (`cmd/app`)

- Build `Inputs` (title from bundle/flags, `version` from ldflags, icon from
  `Resources/AppIcon.svg` → bundle `appIcon` → `build/darwin/AppIcon.svg`),
  `Render(cfg, intro)`, `SetHtml(doc)` immediately. Server starts in the
  background as today.
- `cmd/app/window_darwin.go` (cgo, `#cgo LDFLAGS: -framework Cocoa`,
  `objc_msgSend` only):
  - `applyCard(win, w, h)`: `styleMask = NSWindowStyleMaskBorderless`,
    `setFrame:display:` to `w×h` centred, `setMovableByWindowBackground:YES`,
    `setHasShadow:YES`, `setBackgroundColor:` from `background`.
  - `restoreMain(win)`: `styleMask = Titled|Closable|Miniaturizable|Resizable`,
    `setFrame:display:animate:YES` to a centred 1280×800 (current default),
    `setTitle:` again (borderless windows drop it), `makeKeyAndOrderFront:`.
  - Both run via `wv.Dispatch` on the UI thread. `full` mode never calls them.
- Binds: `__splashHost.post(name)` receives `animated` / `dismissed`;
  `splashConnected()` is called by `app.js` on first WebSocket open.
- Handoff sequence: `animated ∧ connected(server) ∧ minDuration` →
  `Eval("splash.dismiss()")` → on `dismissed` → `Navigate(url?splash=final&remaining=N)`
  → (card) `restoreMain` → UI overlay (final frame, same background) is
  dismissed by `app.js` when its own `connected` holds.
- Timeout: if the server never answers within 10 s, dismiss anyway and
  navigate (today's behaviour), logging a warning.

## 4. Web host (`web/static`, `internal/server`)

- `index.html`: `<div id="splash" hidden></div>` as the first child of
  `<body>`.
- `handleIndex`: renders `Render(cfg, phase)` with `phase = final` if
  `?splash=final`, extracts the document's `<style>` and `<body>` content,
  scopes styles under `#splash` (prefix selectors; templates are written to
  tolerate this), injects them into `#splash`, removes `hidden`, and adds
  `window.SPLASH_CONFIG` (`phase`, `minDurationMs`, `remainingMs`,
  `background`). Custom HTML is injected the same way (its `<script>`s run in
  the page; documented as trusted bundle content, like commands).
- `app.js` `Splash` controller: waits for `splash:animated`, tracks
  `connected` (see §2), honours `minDuration`/`remaining`, calls
  `splash.dismiss()`, and on `splash:dismissed` removes `#splash`; when
  `window.__splashHost` exists it also posts `dismissed` and calls
  `splashConnected()` on first socket open. No other `App` code changes.
- `prefers-reduced-motion`: built-in templates skip intro animations and
  call `animated()` immediately.

## 5. Packaging

- `bundlemeta -splash <bundle.yaml>` prints the resolved custom `html` path
  (or empty). `package-macos.sh` copies it to `Contents/Resources/splash.html`
  and its inlinable assets alongside (paths printed by
  `bundlemeta -splash-assets`). `bundle.Load` on a packaged app resolves
  `html` against `Resources/` when the bundle path is the packaged one.
- Built-in styles ship nothing extra. The icon is already at
  `Resources/AppIcon.svg`.
- `examples/tui-path` gets `splash: {style: arc}`; `examples/icloud-sync` gets
  `splash: {style: jetbrains, tagline: …, accent: "#5b8def"}` so both window
  modes are exercised by shipped examples. README + CLAUDE.md schema tables
  updated.

## 6. Testing

- `internal/bundle`: defaults; `jetbrains → card`; bad enum; bad colour;
  `html` outside bundle dir; missing `html`; BundleSet-over-Bundle
  precedence; `size` bounds.
- `internal/splash`: each style renders with config values present and user
  strings escaped; `final` phase has no `data-splash-intro` animation
  classes; icon becomes a data URI; custom HTML gets shim + inlined assets;
  oversized asset set errors.
- `internal/server`: `/api/config.splash`; `handleIndex` injects `#splash` +
  `SPLASH_CONFIG`; `?splash=final` flips phase and forwards `remaining`.
- `cmd/app`: cgo window code is thin and verified manually with `make app` on
  both examples (card grows into the main window; full crossfades). `go vet`
  covers the darwin build tag.
- Browser: Chrome/Playwright check that `#splash` is removed only after both
  `splash:animated` and the first WebSocket open, and that `?splash=final`
  shows no intro animation.

## Decisions

- **Shared document, two hosts** (over web-only or native-only): only option
  that gives instant native paint before the server binds, a chromeless card,
  and one implementation for both modes.
- **Readiness = animated ∧ connected ∧ minDuration** as an explicit protocol,
  not a timer — per Peter's requirement that the overlay signal when it can be
  unloaded.
- **`minimal` replaces the hard-coded splash** so there is one code path.
- **Card mode restyles the existing window** rather than creating a second
  `NSWindow`: avoids WKWebView re-creation and keeps webview_go's event loop
  untouched.
