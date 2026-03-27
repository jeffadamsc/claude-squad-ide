#!/usr/bin/env bash
#
# Bundle universal-ctags and its dynamic libraries into the .app bundle.
# This makes the DMG self-contained — no brew dependency at runtime.
#
# Usage:
#   ./scripts/bundle-ctags.sh [path-to-app-bundle]
#
# Defaults to build/bin/claude-squad.app if no argument given.
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
APP_BUNDLE="${1:-$PROJECT_DIR/build/bin/claude-squad.app}"
RESOURCES_DIR="$APP_BUNDLE/Contents/Resources/ctags"

# Find universal-ctags
CTAGS_BIN=""
for name in uctags ctags; do
    candidate=$(command -v "$name" 2>/dev/null || true)
    if [ -n "$candidate" ] && "$candidate" --version 2>/dev/null | grep -q "Universal Ctags"; then
        CTAGS_BIN="$candidate"
        break
    fi
done

if [ -z "$CTAGS_BIN" ]; then
    echo "ERROR: universal-ctags not found. Install with: brew install universal-ctags"
    exit 1
fi

echo "==> Bundling ctags from $CTAGS_BIN"

# Create the destination directory
rm -rf "$RESOURCES_DIR"
mkdir -p "$RESOURCES_DIR/lib"

# Copy the ctags binary
cp "$CTAGS_BIN" "$RESOURCES_DIR/ctags"
chmod +x "$RESOURCES_DIR/ctags"

# Find and copy non-system dylibs, then rewrite load paths
otool -L "$CTAGS_BIN" | tail -n +2 | awk '{print $1}' | while read -r dylib; do
    # Skip system libraries — they're on every macOS install
    case "$dylib" in
        /usr/lib/*|/System/*) continue ;;
    esac

    dylib_name=$(basename "$dylib")
    echo "    copying $dylib_name"
    cp "$dylib" "$RESOURCES_DIR/lib/$dylib_name"

    # Rewrite the ctags binary to look for this dylib in @executable_path/lib/
    install_name_tool -change "$dylib" "@executable_path/lib/$dylib_name" "$RESOURCES_DIR/ctags"

    # Also fix the dylib's own id
    install_name_tool -id "@executable_path/lib/$dylib_name" "$RESOURCES_DIR/lib/$dylib_name"

    # Check if this dylib itself links to other non-system dylibs
    otool -L "$dylib" | tail -n +2 | awk '{print $1}' | while read -r subdylib; do
        case "$subdylib" in
            /usr/lib/*|/System/*) continue ;;
        esac
        subdylib_name=$(basename "$subdylib")
        if [ ! -f "$RESOURCES_DIR/lib/$subdylib_name" ]; then
            echo "    copying transitive dep $subdylib_name"
            cp "$subdylib" "$RESOURCES_DIR/lib/$subdylib_name"
            install_name_tool -id "@executable_path/lib/$subdylib_name" "$RESOURCES_DIR/lib/$subdylib_name"
        fi
        install_name_tool -change "$subdylib" "@executable_path/lib/$subdylib_name" "$RESOURCES_DIR/lib/$dylib_name"
    done
done

# Ad-hoc re-sign everything (required on Apple Silicon)
codesign --force --sign - "$RESOURCES_DIR/ctags"
for lib in "$RESOURCES_DIR/lib/"*.dylib; do
    [ -f "$lib" ] && codesign --force --sign - "$lib"
done

# Verify it works
echo "==> Verifying bundled ctags..."
"$RESOURCES_DIR/ctags" --version 2>&1 | head -1

echo "==> Done. Bundled to $RESOURCES_DIR"
