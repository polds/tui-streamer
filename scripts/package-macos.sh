#!/usr/bin/env bash
# package-macos.sh – build a macOS .app bundle and optionally a .dmg.
#
# Usage:
#   scripts/package-macos.sh \
#       --binary  dist/tui-streamer-darwin-universal \
#       --name    TuiStreamer \
#       --version 1.2.3 \
#       --out-dir dist \
#       [--dmg] [--sign "Developer ID Application: …"] [--webview]
#
# Flags:
#   --binary   PATH     Path to the compiled macOS binary (required)
#   --name     NAME     Application name (default: TuiStreamer)
#   --version  VER      Bundle version string (default: dev)
#   --out-dir  DIR      Output directory (default: dist)
#   --dmg               Also create a .dmg after the .app bundle
#   --sign     IDENTITY Code-sign with this identity (optional)
#   --webview           Set LSUIElement=false (show Dock icon) for WebView builds
#
set -euo pipefail

# ── defaults ─────────────────────────────────────────────────────────────────
BINARY=""
APP_NAME="TuiStreamer"
VERSION="dev"
OUT_DIR="dist"
CREATE_DMG=false
SIGN_IDENTITY=""
WEBVIEW_MODE=false

# ── parse arguments ───────────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
  case "$1" in
    --binary)   BINARY="$2";        shift 2 ;;
    --name)     APP_NAME="$2";      shift 2 ;;
    --version)  VERSION="$2";       shift 2 ;;
    --out-dir)  OUT_DIR="$2";       shift 2 ;;
    --dmg)      CREATE_DMG=true;    shift   ;;
    --sign)     SIGN_IDENTITY="$2"; shift 2 ;;
    --webview)  WEBVIEW_MODE=true;  shift   ;;
    *) echo "Unknown option: $1" >&2; exit 1 ;;
  esac
done

if [[ -z "$BINARY" ]]; then
  echo "Error: --binary is required" >&2
  exit 1
fi

if [[ ! -f "$BINARY" ]]; then
  echo "Error: binary not found: $BINARY" >&2
  exit 1
fi

# ── derive short version (strip leading 'v', drop git suffix) ─────────────────
SHORT_VERSION="${VERSION#v}"                      # strip leading 'v'
SHORT_VERSION="${SHORT_VERSION%%-*}"              # strip -g<hash>-dirty suffix
[[ -z "$SHORT_VERSION" ]] && SHORT_VERSION="0.0.0"

# ── paths ─────────────────────────────────────────────────────────────────────
APP_BUNDLE="${OUT_DIR}/${APP_NAME}.app"
CONTENTS="${APP_BUNDLE}/Contents"
MACOS_DIR="${CONTENTS}/MacOS"
RESOURCES_DIR="${CONTENTS}/Resources"
PLIST_TEMPLATE="build/darwin/Info.plist"
ENTITLEMENTS="build/darwin/entitlements.plist"
BINARY_NAME="tui-streamer"
DMG_PATH="${OUT_DIR}/${APP_NAME}-${VERSION}.dmg"

# ── create bundle structure ───────────────────────────────────────────────────
echo "→ Creating .app bundle: ${APP_BUNDLE}"

rm -rf "${APP_BUNDLE}"
mkdir -p "${MACOS_DIR}" "${RESOURCES_DIR}"

# Copy binary
cp "${BINARY}" "${MACOS_DIR}/${BINARY_NAME}"
chmod +x "${MACOS_DIR}/${BINARY_NAME}"

# Build Info.plist from template
PLIST_OUT="${CONTENTS}/Info.plist"
LSUIELEMENT="true"
[[ "$WEBVIEW_MODE" == "true" ]] && LSUIELEMENT="false"
if [[ -f "${PLIST_TEMPLATE}" ]]; then
  sed \
    -e "s/__VERSION__/${VERSION}/g" \
    -e "s/__SHORT_VERSION__/${SHORT_VERSION}/g" \
    -e "s/__LSUIELEMENT__/${LSUIELEMENT}/g" \
    "${PLIST_TEMPLATE}" > "${PLIST_OUT}"
else
  # Fallback: generate a minimal Info.plist inline
  cat > "${PLIST_OUT}" <<PLIST_EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
    "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleIdentifier</key>
    <string>io.github.polds.tui-streamer</string>
    <key>CFBundleName</key>
    <string>${APP_NAME}</string>
    <key>CFBundleDisplayName</key>
    <string>TUI Streamer</string>
    <key>CFBundleExecutable</key>
    <string>${BINARY_NAME}</string>
    <key>CFBundleVersion</key>
    <string>${VERSION}</string>
    <key>CFBundleShortVersionString</key>
    <string>${SHORT_VERSION}</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>LSMinimumSystemVersion</key>
    <string>12.0</string>
    <key>NSHighResolutionCapable</key>
    <true/>
    <key>LSUIElement</key>
    <${LSUIELEMENT}/>
</dict>
</plist>
PLIST_EOF
fi

# Write a PkgInfo stub (required by older macOS launcher code)
printf 'APPL????' > "${CONTENTS}/PkgInfo"

echo "  ✓ Bundle structure created"

# ── code signing (optional) ───────────────────────────────────────────────────
if [[ -n "${SIGN_IDENTITY}" ]]; then
  echo "→ Signing with identity: ${SIGN_IDENTITY}"
  ENTITLEMENTS_ARG=""
  if [[ -f "${ENTITLEMENTS}" ]]; then
    ENTITLEMENTS_ARG="--entitlements ${ENTITLEMENTS}"
  fi
  codesign --force --deep --options runtime \
    ${ENTITLEMENTS_ARG} \
    --sign "${SIGN_IDENTITY}" \
    "${APP_BUNDLE}"
  echo "  ✓ Signed"
else
  # Ad-hoc sign so macOS Gatekeeper doesn't immediately reject unsigned code
  echo "→ Ad-hoc signing (no identity provided)"
  codesign --force --deep --sign - "${APP_BUNDLE}" 2>/dev/null || \
    echo "  ⚠ codesign not available; skipping (expected on Linux)"
fi

echo "✓ .app bundle ready: ${APP_BUNDLE}"

# ── .dmg creation ─────────────────────────────────────────────────────────────
if [[ "$CREATE_DMG" == "true" ]]; then
  echo "→ Creating .dmg: ${DMG_PATH}"

  if ! command -v hdiutil &>/dev/null; then
    echo "  ⚠ hdiutil not found – .dmg creation requires macOS. Skipping."
    exit 0
  fi

  # Staging directory for .dmg contents
  DMG_STAGE="${OUT_DIR}/.dmg-stage"
  rm -rf "${DMG_STAGE}"
  mkdir -p "${DMG_STAGE}"

  cp -R "${APP_BUNDLE}" "${DMG_STAGE}/"

  # Create a symlink to /Applications so the user can drag-and-drop install
  ln -s /Applications "${DMG_STAGE}/Applications"

  # Size estimate: add 20 MB headroom to the actual app size
  APP_SIZE_KB=$(du -sk "${APP_BUNDLE}" | awk '{print $1}')
  DMG_SIZE_KB=$(( APP_SIZE_KB + 20480 ))

  # Create a writable intermediate image
  TEMP_DMG="${OUT_DIR}/.tmp-${APP_NAME}.dmg"
  hdiutil create \
    -srcfolder "${DMG_STAGE}" \
    -volname "${APP_NAME}" \
    -fs HFS+ \
    -fsargs "-c c=65536,a=16,b=16" \
    -format UDRW \
    -size "${DMG_SIZE_KB}k" \
    "${TEMP_DMG}"

  # Mount, optionally set background / icon positions, then unmount
  MOUNT_DIR="$(mktemp -d)"
  hdiutil attach -readwrite -noverify -noautoopen \
    "${TEMP_DMG}" -mountpoint "${MOUNT_DIR}"

  # Basic window layout via AppleScript (best-effort; skipped if osascript fails)
  osascript 2>/dev/null <<APPLESCRIPT || true
tell application "Finder"
  tell disk "${APP_NAME}"
    open
    set current view of container window to icon view
    set toolbar visible of container window to false
    set statusbar visible of container window to false
    set the bounds of container window to {100, 100, 700, 440}
    set viewOptions to the icon view options of container window
    set arrangement of viewOptions to not arranged
    set icon size of viewOptions to 128
    set position of item "${APP_NAME}.app" of container window to {160, 180}
    set position of item "Applications" of container window to {460, 180}
    close
    open
    update without registering applications
    delay 2
  end tell
end tell
APPLESCRIPT

  hdiutil detach "${MOUNT_DIR}"

  # Convert to a compressed, read-only DMG
  rm -f "${DMG_PATH}"
  hdiutil convert "${TEMP_DMG}" \
    -format UDZO \
    -imagekey zlib-level=9 \
    -o "${DMG_PATH}"

  # Sign the .dmg too (if an identity was provided)
  if [[ -n "${SIGN_IDENTITY}" ]]; then
    codesign --sign "${SIGN_IDENTITY}" "${DMG_PATH}"
    echo "  ✓ DMG signed"
  fi

  rm -f "${TEMP_DMG}"
  rm -rf "${DMG_STAGE}"

  echo "✓ .dmg ready: ${DMG_PATH}"
fi
