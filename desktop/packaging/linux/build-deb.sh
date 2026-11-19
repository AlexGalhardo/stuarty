#!/usr/bin/env bash
# Builds the Debian package of the desktop application (SDD-009 section 10.4, P3-REL-003) with dpkg-deb:
#
#   build-deb.sh <stuarty-desktop> <stuarty> <version> <output directory>
#
# The package is stuarty-desktop_<version>_linux_amd64.deb. The application finds the server next to
# its own executable, so both go to /usr/lib/stuarty-desktop/ and /usr/bin/stuarty-desktop links
# to the application. The package does not contain /usr/bin/stuarty, so it can sit next to the
# package of the local server. The window needs GTK 3 and WebKitGTK 4.1, which it loads at run time.
# The package is not signed (D-033): verify it with checksums.txt (SECURITY.md).
set -euo pipefail

[ $# -eq 4 ] || { echo "usage: build-deb.sh <desktop binary> <server binary> <version> <output directory>" >&2; exit 2; }
desktop="$1" server="$2" version="$3" out="$4"
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo="$(cd "$here/../../.." && pwd)"
[ -f "$desktop" ] || { echo "build-deb: no desktop binary at $desktop" >&2; exit 1; }
[ -f "$server" ] || { echo "build-deb: no server binary at $server" >&2; exit 1; }
command -v dpkg-deb >/dev/null 2>&1 || { echo "build-deb: dpkg-deb is required" >&2; exit 1; }

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
root="$work/root"
lib="$root/usr/lib/stuarty-desktop"
mkdir -p "$lib" "$root/usr/bin" "$root/usr/share/applications" "$root/usr/share/icons/hicolor/256x256/apps" \
	"$root/usr/share/doc/stuarty-desktop" "$root/DEBIAN" "$out"

install -m 0755 "$desktop" "$lib/stuarty-desktop"
install -m 0755 "$server" "$lib/stuarty"
ln -s ../lib/stuarty-desktop/stuarty-desktop "$root/usr/bin/stuarty-desktop"
install -m 0644 "$here/stuarty-desktop.desktop" "$root/usr/share/applications/stuarty-desktop.desktop"
install -m 0644 "$repo/.config/installers/assets/stuarty.png" "$root/usr/share/icons/hicolor/256x256/apps/stuarty-desktop.png"
install -m 0644 "$repo/LICENSE.md" "$root/usr/share/doc/stuarty-desktop/copyright"
install -m 0644 "$repo/THIRD_PARTY_NOTICES.md" "$root/usr/share/doc/stuarty-desktop/THIRD_PARTY_NOTICES.md"
cp -R "$repo/licenses" "$root/usr/share/doc/stuarty-desktop/licenses"

size="$(du -sk "$root" | cut -f 1)"
cat >"$root/DEBIAN/control" <<CONTROL
Package: stuarty-desktop
Version: ${version}
Architecture: amd64
Maintainer: Stuarty <https://github.com/AlexGalhardo/stuarty/issues>
Installed-Size: ${size}
Depends: libgtk-3-0, libwebkit2gtk-4.1-0
Replaces: vukem-video-desktop
Conflicts: vukem-video-desktop
Section: video
Priority: optional
Homepage: https://github.com/AlexGalhardo/stuarty
Description: Stuarty, the desktop application
 A minimalist video editor for people and AI agents. This package holds the
 native window and the local server it starts; the workspace stays in the
 home folder and is never removed by the package.
CONTROL

deb="$out/stuarty-desktop_${version}_linux_amd64.deb"
dpkg-deb --root-owner-group --build "$root" "$deb" >&2
echo "$deb"
