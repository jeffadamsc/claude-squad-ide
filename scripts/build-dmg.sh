#!/usr/bin/env bash
#
# Build a .dmg installer for Claude Squad IDE.
#
# Usage:
#   ./scripts/build-dmg.sh          # build first, then package
#   ./scripts/build-dmg.sh --skip-build   # package an existing build
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
BUILD_DIR="$PROJECT_DIR/build/bin"
APP_NAME="claude-squad"
APP_BUNDLE="$BUILD_DIR/$APP_NAME.app"
DMG_NAME="Claude Squad"
DMG_PATH="$BUILD_DIR/$DMG_NAME.dmg"
VOLUME_NAME="Claude Squad"

SKIP_BUILD=false
for arg in "$@"; do
    case "$arg" in
        --skip-build) SKIP_BUILD=true ;;
    esac
done

# Step 1: Build the app bundle (unless skipped)
if [ "$SKIP_BUILD" = false ]; then
    echo "==> Building app with wails..."
    cd "$PROJECT_DIR"
    wails build
fi

# Verify the app bundle exists
if [ ! -d "$APP_BUNDLE" ]; then
    echo "ERROR: App bundle not found at $APP_BUNDLE"
    echo "Run 'wails build' first or remove --skip-build."
    exit 1
fi

if [ ! -f "$APP_BUNDLE/Contents/MacOS/cs" ]; then
    echo "ERROR: Binary not found inside app bundle."
    exit 1
fi

# Step 1.5: Bundle universal-ctags into the app (for go-to-definition)
echo "==> Bundling ctags..."
"$SCRIPT_DIR/bundle-ctags.sh" "$APP_BUNDLE"

# Step 2: Remove any previous DMG
rm -f "$DMG_PATH"

# Step 3: Create a temporary directory for the DMG contents
DMG_STAGING=$(mktemp -d)
trap 'rm -rf "$DMG_STAGING"' EXIT

cp -R "$APP_BUNDLE" "$DMG_STAGING/"
ln -s /Applications "$DMG_STAGING/Applications"

# Step 4: Create the DMG
echo "==> Creating DMG at $DMG_PATH..."
hdiutil create \
    -volname "$VOLUME_NAME" \
    -srcfolder "$DMG_STAGING" \
    -ov \
    -format UDZO \
    "$DMG_PATH"

echo ""
echo "DMG created: $DMG_PATH"
echo "Size: $(du -h "$DMG_PATH" | cut -f1)"
