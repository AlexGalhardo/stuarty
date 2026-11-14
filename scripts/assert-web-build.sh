#!/usr/bin/env bash
# Fails when the web build produced no JavaScript. The embed directive of
# web/resources/resources.go also matches the tracked public/robots.txt, so a
# binary compiled without the web build still builds and silently ships
# without the UI (SDD-009 section 10). Run it after `bun run build` in web/.
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

if [ -z "$(find web/resources/public/build -type f -name '*.js' 2>/dev/null | head -n 1)" ]; then
	echo "assert-web-build: web/resources/public/build has no .js file: run 'bun run build' in web/" >&2
	exit 1
fi
