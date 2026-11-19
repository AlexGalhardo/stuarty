#!/usr/bin/env bash
# Runs the command blocks of the tutorial pages against a fresh local server (P3-DOC-001, SDD-010 §14
# check 18):
#
#   bash scripts/docs-commands-smoke.sh [page.mdx ...]
#
# With no argument it takes the tutorials of site/content/docs/en/guide/ (tutorial-vertical-short first: it
# creates the project that the next tutorial reads). It extracts every fenced bash block, starts
# `stuarty serve` with a private workspace on a loopback port, runs the blocks one after the other in a
# folder that holds the sample media (made with ffmpeg) and stops at the first non-zero exit. The server and
# the work folder are removed at the end.
#
# A block is skipped, and reported, when it cannot run offline: it installs a tool or a model (needs the
# network, D-016) or it starts another agent client (`claude`). SMOKE_NETWORK=1 runs the model blocks too.
# Needs: ffmpeg on PATH, go (only when STUARTY_BIN is not set). Environment:
#   STUARTY_BIN   the binary to test (default: built from this checkout)
#   SMOKE_PORT        loopback port of the server (default a random one in 20000 to 39999)
#   SMOKE_KEEP        set to 1 to keep the work folder
set -euo pipefail

die() { echo "docs-commands-smoke: error: $*" >&2; exit 1; }
log() { echo "docs-commands-smoke: $*" >&2; }

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
command -v ffmpeg >/dev/null 2>&1 || die "ffmpeg is required to make the sample media"

if [ "$#" -gt 0 ]; then
	pages=("$@")
else
	guide="$root/site/content/docs/en/guide"
	pages=("$guide/tutorial-vertical-short.mdx" "$guide/tutorial-agent-edit.mdx")
fi
for p in "${pages[@]}"; do [ -f "$p" ] || die "no such page: $p"; done

work="$(mktemp -d)"
server_pid=""
cleanup() {
	local status=$?
	set +e
	if [ -n "$server_pid" ] && kill -0 "$server_pid" 2>/dev/null; then
		kill "$server_pid" 2>/dev/null || true
		if command -v taskkill >/dev/null 2>&1; then
			# Git Bash cannot signal a native Windows process: stop it by its Windows pid.
			winpid="$(ps -p "$server_pid" -W 2>/dev/null | awk 'NR==2 {print $4}')"
			[ -z "$winpid" ] || taskkill //F //T //PID "$winpid" >/dev/null 2>&1 || true
		fi
		wait "$server_pid" 2>/dev/null || true
	fi
	if [ "${SMOKE_KEEP:-}" = 1 ]; then log "kept $work"; else rm -rf "$work"; fi
}
trap cleanup EXIT

case "$(uname -s)" in MINGW* | MSYS* | CYGWIN*) exe=stuarty.exe ;; *) exe=stuarty ;; esac
mkdir -p "$work/bin" "$work/cwd" "$work/home" "$work/blocks"
if [ -n "${STUARTY_BIN:-}" ]; then
	cp "$STUARTY_BIN" "$work/bin/$exe"
else
	command -v go >/dev/null 2>&1 || die "go is required to build the binary (or set STUARTY_BIN)"
	log "building the binary"
	(cd "$root" && go build -o "$work/bin/$exe" ./web/cmd/app)
fi
chmod +x "$work/bin/$exe"
export PATH="$work/bin:$PATH"

# The private home keeps the run away from the workspace of the person who runs it. No server of another
# workspace can be reached: STUARTY_URL is cleared.
unset STUARTY_URL APP_KEY APP_ENV
export STUARTY_HOME="$work/home"
port="${SMOKE_PORT:-$((20000 + RANDOM % 20000))}"
STUARTY_ADDR="127.0.0.1:$port" "$work/bin/$exe" serve >"$work/serve.log" 2>&1 &
server_pid=$!
for _ in $(seq 1 120); do
	[ -s "$work/home/runtime.json" ] && break
	kill -0 "$server_pid" 2>/dev/null || { cat "$work/serve.log" >&2; die "the server exited at start"; }
	sleep 0.5
done
[ -s "$work/home/runtime.json" ] || { cat "$work/serve.log" >&2; die "the server did not announce itself in 60 s"; }
log "server up on 127.0.0.1:$port"

# The sample media of the tutorials (talk.mp4 with speech-like audio, song.mp3).
cd "$work/cwd"
ffmpeg -v error -f lavfi -i testsrc2=size=640x360:rate=30:duration=6 -f lavfi -i sine=frequency=220:duration=6 \
	-c:v mpeg4 -c:a aac -shortest talk.mp4
if ! ffmpeg -v error -f lavfi -i sine=frequency=440:duration=6 -c:a libmp3lame song.mp3 2>/dev/null; then
	ffmpeg -v error -f lavfi -i sine=frequency=440:duration=6 -c:a mp2 -f mp2 song.mp3
fi

ran=0
skipped=0
for page in "${pages[@]}"; do
	name="$(basename "$page" .mdx)"
	rm -f "$work"/blocks/"$name"-*.sh
	# One file per fenced bash block, with the indentation of the fence removed (blocks sit inside lists).
	awk -v dir="$work/blocks" -v name="$name" '
		{ sub(/\r$/, "") }
		!infence && match($0, /^[ ]*```(bash|sh)[ ]*$/) { infence = 1; indent = index($0, "`") - 1; n++; file = sprintf("%s/%s-%02d.sh", dir, name, n); next }
		infence && $0 ~ /^[ ]*```[ ]*$/ { infence = 0; close(file); next }
		infence { line = $0; if (substr(line, 1, indent) ~ /^ *$/) line = substr(line, indent + 1); print line >> file }
	' "$page"
	blocks=("$work"/blocks/"$name"-*.sh)
	[ -f "${blocks[0]}" ] || die "$page has no bash block"
	for b in "${blocks[@]}"; do
		label="$name $(basename "$b" .sh | sed 's/.*-//')"
		if grep -qE '^[[:space:]]*claude[[:space:]]' "$b"; then
			log "skip $label (starts another client)"; skipped=$((skipped + 1)); continue
		fi
		if [ "${SMOKE_NETWORK:-}" != 1 ] && grep -qE 'tools:install|--language|transcribe' "$b"; then
			log "skip $label (installs or runs a model, needs the network; SMOKE_NETWORK=1 runs it)"; skipped=$((skipped + 1)); continue
		fi
		log "run  $label"
		if ! bash -eo pipefail "$b"; then
			echo "--- failed block ($label):" >&2; cat "$b" >&2
			echo "--- server log (tail):" >&2; tail -n 20 "$work/serve.log" >&2
			die "the block $label exited non-zero"
		fi
		ran=$((ran + 1))
	done
done

[ "$ran" -gt 0 ] || die "no block ran"
log "ok: $ran block(s) ran, $skipped skipped"
