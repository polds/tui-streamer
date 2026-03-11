# Network Diagnostics Bundle

An example tui-streamer bundle that pre-creates a set of network diagnostic
sessions on startup.

## Usage

```bash
tui-streamer -bundle bundle.json -open
```

## Sessions

| Name | Command | Auto-execute |
|------|---------|--------------|
| 00-ping-test | `ping -c 4 example.com` | yes |
| 01-dns-test | `dig +short example.com` | yes |
| 02-traceroute | `traceroute example.com` | no (manual) |
| 03-flush-dns | `sudo dscacheutil -flushdns` | no (manual) |

Sessions marked **yes** run immediately when the server starts.
Sessions marked **no** have their command pre-loaded into the input bar — click
**Run** (or press Enter) to execute them.
