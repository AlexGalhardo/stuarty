#!/usr/bin/env bash
# Builds the macOS disk image of the desktop application (SDD-009 section 10.4, P3-REL-003). Runs on macOS:
# hdiutil, sips and iconutil are part of the system.
#
#   build-dmg.sh <stuarty-desktop for darwin> <stuarty for darwin> <version> <amd64|arm64> <output directory>
#
# The image holds "Stuarty.app", whose executable is the desktop application, with the server next to
# it (the application looks for stuarty in its own folder), and a link to /Applications. The
# application is not signed or notarized (D-033): Gatekeeper warns at first open; control-click it and
# choose Open, or run: xattr -d com.apple.quarantine "/Applications/Stuarty.app"
#
# It also writes stuarty-desktop_<version>_darwin_<arch>.app.zip: a zip that holds exactly one
# "Stuarty.app" at its top level, the artifact the launcher installs updates from (SDD-009 section
# 10.5 item 6, the name the update feed expects).
set -euo pipefail

[ $# -eq 5 ] || { echo "usage: build-dmg.sh <desktop binary> <server binary> <version> <amd64|arm64> <output directory>" >&2; exit 2; }
desktop="$1" server="$2" version="$3" arch="$4" out="$5"
case "$arch" in amd64 | arm64) ;; *) echo "build-dmg: unknown architecture $arch" >&2; exit 2 ;; esac
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo="$(cd "$here/../../.." && pwd)"
[ -f "$desktop" ] || { echo "build-dmg: no desktop binary at $desktop" >&2; exit 1; }
[ -f "$server" ] || { echo "build-dmg: no server binary at $server" >&2; exit 1; }

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
stage="$work/stage"
app="$stage/Stuarty.app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources" "$out"

install -m 0755 "$desktop" "$app/Contents/MacOS/stuarty-desktop"
install -m 0755 "$server" "$app/Contents/MacOS/stuarty"
# An installed Vukem Video replaces itself with the bundle of the update and starts it again from the path of
# the program it ran, vukem-video-desktop, so that path must exist in the new bundle (ADR-0044). The shim
# only starts the real program; a bundle with no such file is refused by the launcher of Vukem Video.
cat >"$app/Contents/MacOS/vukem-video-desktop" <<'SHIM'
#!/bin/sh
exec "$(dirname "$0")/stuarty-desktop" "$@"
SHIM
chmod 0755 "$app/Contents/MacOS/vukem-video-desktop"
install -m 0644 "$repo/LICENSE.md" "$app/Contents/Resources/LICENSE.md"
install -m 0644 "$repo/THIRD_PARTY_NOTICES.md" "$app/Contents/Resources/THIRD_PARTY_NOTICES.md"
cp -R "$repo/licenses" "$app/Contents/Resources/licenses"
sed "s/@VERSION@/${version}/g" "$here/Info.plist" >"$app/Contents/Info.plist"
ln -s /Applications "$stage/Applications"

iconset="$work/stuarty.iconset"
mkdir -p "$iconset"
png="$repo/.config/installers/assets/stuarty.png"
for size in 16 32 64 128 256; do
	sips -z "$size" "$size" "$png" --out "$iconset/icon_${size}x${size}.png" >/dev/null
done
cp "$iconset/icon_32x32.png" "$iconset/icon_16x16@2x.png"
cp "$iconset/icon_64x64.png" "$iconset/icon_32x32@2x.png"
cp "$iconset/icon_256x256.png" "$iconset/icon_128x128@2x.png"
rm "$iconset/icon_64x64.png"
iconutil -c icns "$iconset" -o "$app/Contents/Resources/stuarty.icns"

dmg="$out/stuarty-desktop-${version//./-}-${arch}.dmg"
rm -f "$dmg"
hdiutil create -quiet -volname "Stuarty" -srcfolder "$stage" -ov -format UDZO "$dmg"

zip="$out/stuarty-desktop_${version}_darwin_${arch}.app.zip"
rm -f "$zip"
# ditto keeps the executable bits and the bundle layout; --keepParent puts the .app at the top of the zip.
ditto -c -k --keepParent "$app" "$zip"
echo "$dmg"
echo "$zip"
