#!/bin/sh
# Removes Vukem Video from this Mac: the program, the launcher app and this script. The workspace
# (~/.vukem-video, or VUKEM_VIDEO_HOME) is never touched: it holds your projects and media.
# Run with: sudo /usr/local/share/vukem-video/uninstall.sh
set -eu

if [ "$(id -u)" -ne 0 ]; then
	echo "Run this with sudo: sudo $0" >&2
	exit 1
fi

# Stop the local server that "vukem-video open" started in the background.
pkill -x vukem-video 2>/dev/null || true

rm -f /usr/local/bin/vukem-video
rm -rf "/Applications/Vukem Video.app"
pkgutil --forget dev.vukem.vukem-video >/dev/null 2>&1 || true
rm -rf /usr/local/share/vukem-video
echo "Vukem Video was removed. Your workspace was kept."
