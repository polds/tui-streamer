# TUI Path Demo

A small BundleSet that exercises the four file-level options:

| Field | What this example does |
|---|---|
| `metadata.appIcon` | Uses `icon.svg` when packaging `make app BUNDLE=...` |
| `spec.theme` / `spec.themes` | Locks the UI to the **nord** theme and hides the picker |
| `spec.allow` | Permits `echo`, `sh`, and `demo-tool` only |
| `spec.files` | Copies `bin/demo-tool` into `TUI_PATH` |

## Usage

```bash
# From the repository root
make build
./dist/tui-streamer -bundle examples/tui-path/bundle.yaml -open
```

The **Demo Tool** session autoruns:

```bash
$(TUI_PATH)/demo-tool hello from bundle
```

tui-streamer expands `$(TUI_PATH)` (and `$TUI_PATH` / `${TUI_PATH}`), sets the
`TUI_PATH` environment variable, and prepends that directory to `PATH`.

## Packaging

```bash
make app BUNDLE=./examples/tui-path/bundle.yaml
```

The `.app` is named **TUI Path Demo**. `icon.svg` is copied into
`Contents/Resources` (and converted to `AppIcon.icns` on macOS). `demo-tool`
is copied to `Contents/Resources/tui`, which becomes `TUI_PATH` at launch.

## Allowlist merge

CLI `-allow` and `spec.allow` are **unioned**. If neither is set, every
command is allowed. This example sets `spec.allow`, so only those binaries
(plus any extra `-allow` flags) can run.
