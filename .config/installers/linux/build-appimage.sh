#!/usr/bin/env bash
# Builds the AppImage of stuarty (SDD-009 section 10.4, P1-REL-010).
#
#   build-appimage.sh <stuarty binary for linux amd64> <version> <output directory>
#
# appimagetool is the pinned one of .config/installers/tools.env, fetched by
# scripts/fetch-installer-tool.sh (set APPIMAGETOOL to use another path). The output is
# <output directory>/stuarty-<version with dashes>.AppImage (stuarty-0-28-0.AppImage).
# The AppImage carries the
# icon and the desktop entry, and starts "stuarty open" when it is run with no argument.
set -euo pipefail

[ $# -eq 3 ] || { echo "usage: build-appimage.sh <binary> <version> <output directory>" >&2; exit 2; }
binary="$1" version="$2" out="$3"
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo="$(cd "$here/../../.." && pwd)"
[ -f "$binary" ] || { echo "build-appimage: no binary at $binary" >&2; exit 1; }

tool="${APPIMAGETOOL:-$(bash "$repo/scripts/fetch-installer-tool.sh" appimagetool)}"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
appdir="$work/stuarty.AppDir"
mkdir -p "$appdir/usr/bin" "$out"
install -m 0755 "$binary" "$appdir/usr/bin/stuarty"
install -m 0755 "$here/AppRun" "$appdir/AppRun"
install -m 0644 "$here/stuarty.desktop" "$appdir/stuarty.desktop"
install -m 0644 "$repo/.config/installers/assets/stuarty.png" "$appdir/stuarty.png"
install -m 0644 "$repo/LICENSE.md" "$appdir/LICENSE.md"
install -m 0644 "$repo/THIRD_PARTY_NOTICES.md" "$appdir/THIRD_PARTY_NOTICES.md"
cp -R "$repo/licenses" "$appdir/licenses"

# Runners have no FUSE: appimagetool is itself an AppImage, so it extracts itself and runs.
file="$out/stuarty-${version//./-}.AppImage"
ARCH=x86_64 APPIMAGE_EXTRACT_AND_RUN=1 "$tool" --no-appstream "$appdir" "$file"
echo "$file"
