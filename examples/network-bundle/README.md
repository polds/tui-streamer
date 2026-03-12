# Network Troubleshooting Bundle

An example tui-streamer bundle that pre-creates a set of network diagnostic
sessions on startup, split across two named bundles inside a `BundleSet`.

## Usage

Run directly from the CLI:

```bash
tui-streamer -bundle bundle.yaml -open
```

Or package as a standalone macOS application (the app will be automatically
named **Network Troubleshooting** from the BundleSet metadata):

```bash
make app BUNDLE=./examples/network-bundle/bundle.yaml
```

## Bundle Structure

The file uses a `BundleSet` to group two `Bundle` documents in a single YAML
file. The `BundleSet` declares the display order; the `Bundle` documents define
the sessions.

```
BundleSet: Network Troubleshooting
├── Bundle: Connectivity
│   ├── Ping           (autorun)
│   └── Traceroute     (manual)
└── Bundle: DNS
    ├── Dig (System Resolvers)   (autorun)
    ├── Dig (Google)             (autorun)
    ├── Dig (Cloudflare)         (autorun)
    └── NSLookup                 (autorun)
```

## Sessions

### Connectivity

| Name | Command | Auto-execute |
|------|---------|:------------:|
| Ping | `ping -c 4 example.com` | ✓ |
| Traceroute | `traceroute example.com` | — |

### DNS

| Name | Command | Auto-execute |
|------|---------|:------------:|
| Dig (System Resolvers) | `dig +short example.com` | ✓ |
| Dig (Google) | `dig @8.8.8.8 +short example.com` | ✓ |
| Dig (Cloudflare) | `dig @1.1.1.1 +short example.com` | ✓ |
| NSLookup | `nslookup google.com` | ✓ |

Sessions with **✓** run automatically when the bundle loads. Sessions with
**—** have their command pre-loaded in the input bar — press **Run** to execute.

## Description Rendering

Each session in this bundle includes a `description` field written in Markdown.
When you select a session in the UI, the description is rendered above the
terminal output — useful for explaining what the command does and how to
interpret its results.
