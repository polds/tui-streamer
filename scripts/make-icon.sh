#!/usr/bin/env bash
# make-icon.sh – convert build/darwin/AppIcon.svg into an AppIcon.icns file.
#
# Requirements (macOS only):
#   - rsvg-convert   (brew install librsvg)
#   - iconutil       (ships with Xcode command-line tools)
#
# Usage:
#   bash scripts/make-icon.sh
#
# Output:
#   build/darwin/AppIcon.icns   (ready to be bundled by package-macos.sh)
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

SVG="${REPO_ROOT}/build/darwin/AppIcon.svg"
ICONSET="${REPO_ROOT}/build/darwin/AppIcon.iconset"
ICNS="${REPO_ROOT}/build/darwin/AppIcon.icns"

# ── sanity checks ─────────────────────────────────────────────────────────────
if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "Error: make-icon.sh requires macOS (iconutil is macOS-only)." >&2
  exit 1
fi

if [[ ! -f "${SVG}" ]]; then
  echo "Error: SVG source not found: ${SVG}" >&2
  exit 1
fi

if ! command -v rsvg-convert &>/dev/null; then
  echo "Error: rsvg-convert not found." >&2
  echo "       Install it with:  brew install librsvg" >&2
  exit 1
fi

if ! command -v iconutil &>/dev/null; then
  echo "Error: iconutil not found. Install Xcode command-line tools:" >&2
  echo "       xcode-select --install" >&2
  exit 1
fi

# ── generate PNG sizes required by macOS iconset ─────────────────────────────
echo "→ Generating PNG sizes from ${SVG}"

rm -rf "${ICONSET}"
mkdir -p "${ICONSET}"

# iconset requires: 16, 32, 128, 256, 512 at 1x and 2x (@2x)
for SIZE in 16 32 128 256 512; do
  DOUBLE=$(( SIZE * 2 ))
  rsvg-convert -w "${SIZE}"   -h "${SIZE}"   "${SVG}" -o "${ICONSET}/icon_${SIZE}x${SIZE}.png"
  rsvg-convert -w "${DOUBLE}" -h "${DOUBLE}" "${SVG}" -o "${ICONSET}/icon_${SIZE}x${SIZE}@2x.png"
  echo "  ✓ ${SIZE}x${SIZE} + @2x"
done

# ── build the .icns ───────────────────────────────────────────────────────────
echo "→ Building AppIcon.icns"
iconutil -c icns "${ICONSET}" -o "${ICNS}"

# Clean up intermediate iconset directory
rm -rf "${ICONSET}"

echo "✓ Icon ready: ${ICNS}"
