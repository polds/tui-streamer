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
#       [--bundle PATH] [--icon-svg PATH]
#
# Flags:
#   --binary   PATH     Path to the compiled macOS binary (required)
#   --name     NAME     Application name (default: TuiStreamer)
#   --version  VER      Bundle version string (default: dev)
#   --out-dir  DIR      Output directory (default: dist)
#   --dmg               Also create a .dmg after the .app bundle
#   --sign     IDENTITY Code-sign with this identity (optional)
#   --webview           Set LSUIElement=false (show Dock icon) for WebView builds
#   --bundle   PATH     YAML bundle copied to Contents/Resources/bundle.yaml
#   --icon-svg PATH     Custom SVG used as the app icon (overrides AppIcon.icns)
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
BUNDLE_FILE=""
ICON_SVG=""
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

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
    --bundle)   BUNDLE_FILE="$2";   shift 2 ;;
    --icon-svg) ICON_SVG="$2";      shift 2 ;;
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

# Copy bundle YAML to Contents/Resources so the app finds it at launch.
# Also stage spec.files into Contents/Resources/tui (TUI_PATH at runtime).
if [[ -n "${BUNDLE_FILE}" ]]; then
  if [[ ! -f "${BUNDLE_FILE}" ]]; then
    echo "Error: bundle file not found: ${BUNDLE_FILE}" >&2
    exit 1
  fi
  BUNDLE_FILE="$(cd "$(dirname "${BUNDLE_FILE}")" && pwd)/$(basename "${BUNDLE_FILE}")"
  dest="bundle.yaml"
  [[ "${BUNDLE_FILE##*.}" == "yml" ]] && dest="bundle.yml"
  cp "${BUNDLE_FILE}" "${RESOURCES_DIR}/${dest}"
  echo "  ✓ Bundled configuration: ${BUNDLE_FILE} → Contents/Resources/${dest}"

  if [[ -z "${ICON_SVG}" ]] && command -v go >/dev/null; then
    ICON_SVG="$(cd "${REPO_ROOT}" && go run ./cmd/bundlemeta -icon "${BUNDLE_FILE}" 2>/dev/null || true)"
  fi

  if command -v go >/dev/null; then
    files_list=""
    if ! files_list="$(cd "${REPO_ROOT}" && go run ./cmd/bundlemeta -files "${BUNDLE_FILE}")"; then
      echo "Error: could not read spec.files from ${BUNDLE_FILE}" >&2
      exit 1
    fi
    TUI_DIR="${RESOURCES_DIR}/tui"
    while IFS=$'\t' read -r src dest_rel || [[ -n "${src}" ]]; do
      [[ -z "${src}" ]] && continue
      if [[ ! -e "${src}" ]]; then
        echo "Error: bundle file source not found: ${src}" >&2
        exit 1
      fi
      dest_path="${TUI_DIR}/${dest_rel}"
      mkdir -p "$(dirname "${dest_path}")"
      if [[ -d "${src}" ]]; then
        mkdir -p "${dest_path}"
        cp -R "${src}/." "${dest_path}/"
      else
        cp "${src}" "${dest_path}"
      fi
      if [[ -f "${src}" && -x "${src}" ]]; then
        chmod +x "${dest_path}"
      fi
      echo "  ✓ Bundled file: ${src} → Contents/Resources/tui/${dest_rel}"
    done <<< "${files_list}"

    # Custom splash page + assets, keeping paths relative to the bundle so the
    # packaged bundle.yaml's `html:` still resolves under Resources/.
    bundle_dir="$(cd "$(dirname "${BUNDLE_FILE}")" && pwd -P)"
    splash_html="$(cd "${REPO_ROOT}" && go run ./cmd/bundlemeta -splash "${BUNDLE_FILE}" 2>/dev/null || true)"
    if [[ -n "${splash_html}" ]]; then
      { echo "${splash_html}"; cd "${REPO_ROOT}" && go run ./cmd/bundlemeta -splash-assets "${BUNDLE_FILE}"; } | while IFS= read -r src; do
        [[ -z "${src}" ]] && continue
        # bundlemeta -splash-assets prints symlink-resolved (physical) paths
        # while ${bundle_dir} above is likewise physical (pwd -P); resolve
        # src's directory the same way so the prefix strip below compares
        # like with like (e.g. a bundle under /tmp -> /private/tmp on macOS).
        if ! src_dir="$(cd "$(dirname "${src}")" 2>/dev/null && pwd -P)"; then
          echo "Error: splash asset source not found: ${src}" >&2
          exit 1
        fi
        src="${src_dir}/$(basename "${src}")"
        rel="${src#"${bundle_dir}"/}"
        if [[ "${rel}" == "${src}" ]]; then
          echo "Error: splash asset ${src} is outside the bundle directory" >&2
          exit 1
        fi
        mkdir -p "${RESOURCES_DIR}/$(dirname "${rel}")"
        cp "${src}" "${RESOURCES_DIR}/${rel}"
        echo "  ✓ Bundled splash: ${rel} → Contents/Resources/${rel}"
      done
    fi
  fi
fi

# Build Info.plist from template
PLIST_OUT="${CONTENTS}/Info.plist"
LSUIELEMENT="true"
[[ "$WEBVIEW_MODE" == "true" ]] && LSUIELEMENT="false"
if [[ -f "${PLIST_TEMPLATE}" ]]; then
  sed \
    -e "s/__VERSION__/${VERSION}/g" \
    -e "s/__SHORT_VERSION__/${SHORT_VERSION}/g" \
    -e "s/__LSUIELEMENT__/${LSUIELEMENT}/g" \
    -e "s/__APP_NAME__/${APP_NAME}/g" \
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
    <string>${APP_NAME}</string>
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

# Copy / generate the app icon. A bundle metadata.appIcon SVG (or --icon-svg)
# takes precedence over the stock build/darwin/AppIcon.icns.
ICON_SRC="${REPO_ROOT}/build/darwin/AppIcon.icns"
if [[ -n "${ICON_SVG}" ]]; then
  if [[ ! -f "${ICON_SVG}" ]]; then
    echo "Error: icon SVG not found: ${ICON_SVG}" >&2
    exit 1
  fi
  cp "${ICON_SVG}" "${RESOURCES_DIR}/AppIcon.svg"
  echo "  ✓ App icon SVG: ${ICON_SVG} → Contents/Resources/AppIcon.svg"
  if [[ "$(uname -s)" == "Darwin" ]]; then
    if bash "${SCRIPT_DIR}/make-icon.sh" "${ICON_SVG}" "${RESOURCES_DIR}/AppIcon.icns"; then
      echo "  ✓ App icon generated from bundle SVG"
    else
      echo "  ⚠ Could not generate .icns from ${ICON_SVG}"
    fi
  elif [[ -f "${ICON_SRC}" ]]; then
    cp "${ICON_SRC}" "${RESOURCES_DIR}/AppIcon.icns"
    echo "  ⚠ Not on macOS – bundled stock AppIcon.icns (SVG copied for reference)"
  else
    echo "  ⚠ No AppIcon.icns generated (iconutil is macOS-only); SVG copied"
  fi
elif [[ -f "${ICON_SRC}" ]]; then
  cp "${ICON_SRC}" "${RESOURCES_DIR}/AppIcon.icns"
  echo "  ✓ App icon bundled"
else
  echo "  ⚠ No AppIcon.icns found – run 'make icon' on macOS to generate one"
  echo "    (brew install librsvg, then: make icon)"
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

  # Build a compressed read-only DMG directly from the staging folder.
  # Using -format UDZO avoids the deprecated HFS+-with-fsargs two-step
  # (UDRW create → mount → convert) that fails with "Operation not permitted"
  # on macOS 13+ due to tightened kernel security controls.
  rm -f "${DMG_PATH}"
  hdiutil create \
    -volname "${APP_NAME}" \
    -srcfolder "${DMG_STAGE}" \
    -ov \
    -format UDZO \
    -o "${DMG_PATH}"

  # Sign the .dmg too (if an identity was provided)
  if [[ -n "${SIGN_IDENTITY}" ]]; then
    codesign --sign "${SIGN_IDENTITY}" "${DMG_PATH}"
    echo "  ✓ DMG signed"
  fi

  rm -rf "${DMG_STAGE}"

  echo "✓ .dmg ready: ${DMG_PATH}"
fi
