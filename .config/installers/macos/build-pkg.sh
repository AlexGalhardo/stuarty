#!/usr/bin/env bash
# Builds the macOS installer package of stuarty (SDD-009 section 10.4, P1-REL-010). Runs on macOS:
# pkgbuild, sips and iconutil are part of the system and of the Xcode command line tools.
#
#   build-pkg.sh <stuarty binary for darwin> <version> <amd64|arm64> <output directory>
#
# The package installs /usr/local/bin/stuarty, "/Applications/Stuarty.app" (a launcher that runs
# "stuarty open": start the local server on 127.0.0.1 if none runs, then open the browser), and
# /usr/local/share/stuarty/uninstall.sh, which removes all of that and keeps the workspace.
# The package is not signed or notarized (no paid code signing): Gatekeeper warns at first open.
set -euo pipefail

[ $# -eq 4 ] || { echo "usage: build-pkg.sh <binary> <version> <amd64|arm64> <output directory>" >&2; exit 2; }
binary="$1" version="$2" arch="$3" out="$4"
case "$arch" in amd64 | arm64) ;; *) echo "build-pkg: unknown architecture $arch" >&2; exit 2 ;; esac
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo="$(cd "$here/../../.." && pwd)"
[ -f "$binary" ] || { echo "build-pkg: no binary at $binary" >&2; exit 1; }

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
root="$work/root"
app="$root/Applications/Stuarty.app"
mkdir -p "$root/usr/local/bin" "$root/usr/local/share/stuarty" "$app/Contents/MacOS" "$app/Contents/Resources" "$out"

install -m 0755 "$binary" "$root/usr/local/bin/stuarty"
install -m 0755 "$here/uninstall.sh" "$root/usr/local/share/stuarty/uninstall.sh"
install -m 0644 "$repo/LICENSE.md" "$root/usr/local/share/stuarty/LICENSE.md"
install -m 0644 "$repo/THIRD_PARTY_NOTICES.md" "$root/usr/local/share/stuarty/THIRD_PARTY_NOTICES.md"
cp -R "$repo/licenses" "$root/usr/local/share/stuarty/licenses"
install -m 0755 "$here/launcher.sh" "$app/Contents/MacOS/stuarty-launcher"
sed "s/@VERSION@/${version}/g" "$here/Info.plist" >"$app/Contents/Info.plist"

# The icon: an iconset from the PNG, then iconutil.
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

pkg="$out/stuarty_${version}_darwin_${arch}.pkg"
# The identifier keeps the identity of the former name (ADR-0044), as do the bundle identifiers of the
# Info.plist files: the package then upgrades an installed Vukem Video instead of sitting next to it.
pkgbuild --root "$root" --identifier dev.vukem.vukem-video --version "$version" --install-location / "$pkg"
echo "$pkg"
