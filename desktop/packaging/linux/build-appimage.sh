#!/usr/bin/env bash
# Builds the AppImage of the desktop application, the artifact the launcher installs updates from on Linux
# (SDD-009 sections 10.5 item 6 and 10.6, EPIC-55):
#
#   build-appimage.sh <stuarty-desktop> <stuarty> <version> <output directory>
#
# The file is stuarty-desktop-<version with dashes>.AppImage (0.28.0 becomes
# stuarty-desktop-0-28-0.AppImage), the name the update feed expects
# (internal/tools/updatefeed). Both programs go to usr/bin, so the application finds the server next to its
# own executable. appimagetool is the pinned one of .config/installers/tools.env, fetched and verified by
# scripts/fetch-installer-tool.sh (set APPIMAGETOOL to use another path). It is not signed (D-033).
set -euo pipefail

[ $# -eq 4 ] || { echo "usage: build-appimage.sh <desktop binary> <server binary> <version> <output directory>" >&2; exit 2; }
desktop="$1" server="$2" version="$3" out="$4"
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo="$(cd "$here/../../.." && pwd)"
[ -f "$desktop" ] || { echo "build-appimage: no desktop binary at $desktop" >&2; exit 1; }
[ -f "$server" ] || { echo "build-appimage: no server binary at $server" >&2; exit 1; }

tool="${APPIMAGETOOL:-$(bash "$repo/scripts/fetch-installer-tool.sh" appimagetool)}"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
appdir="$work/stuarty-desktop.AppDir"
mkdir -p "$appdir/usr/bin" "$out"
install -m 0755 "$desktop" "$appdir/usr/bin/stuarty-desktop"
install -m 0755 "$server" "$appdir/usr/bin/stuarty"
install -m 0755 "$here/AppRun" "$appdir/AppRun"
install -m 0644 "$here/stuarty-desktop-appimage.desktop" "$appdir/stuarty-desktop.desktop"
install -m 0644 "$repo/.config/installers/assets/stuarty.png" "$appdir/stuarty-desktop.png"
install -m 0644 "$repo/LICENSE.md" "$appdir/LICENSE.md"
install -m 0644 "$repo/THIRD_PARTY_NOTICES.md" "$appdir/THIRD_PARTY_NOTICES.md"
cp -R "$repo/licenses" "$appdir/licenses"

file="$out/stuarty-desktop-${version//./-}.AppImage"
# Runners have no FUSE: appimagetool is itself an AppImage, so it extracts itself and runs.
ARCH=x86_64 APPIMAGE_EXTRACT_AND_RUN=1 "$tool" --no-appstream "$appdir" "$file" >&2
echo "$file"
