#!/bin/sh
# Removes Stuarty from this Mac: the program, the launcher app and this script. The workspace
# (~/.stuarty, or STUARTY_HOME) is never touched: it holds your projects and media.
# Run with: sudo /usr/local/share/stuarty/uninstall.sh
set -eu

if [ "$(id -u)" -ne 0 ]; then
	echo "Run this with sudo: sudo $0" >&2
	exit 1
fi

# Stop the local server that "stuarty open" started in the background.
pkill -x stuarty 2>/dev/null || true

rm -f /usr/local/bin/stuarty /usr/local/bin/vukem-video
rm -rf "/Applications/Stuarty.app"
pkgutil --forget dev.vukem.vukem-video >/dev/null 2>&1 || true
rm -rf /usr/local/share/stuarty
echo "Stuarty was removed. Your workspace was kept."
