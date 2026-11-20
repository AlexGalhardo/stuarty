#!/usr/bin/env bash
# Runs the command blocks of the tutorial pages against a fresh local server (P3-DOC-001, SDD-010 §14
# check 18):
#
#   bash scripts/docs-commands-smoke.sh [page.mdx ...]
#
# With no argument it takes the install pages of site/content/docs/en/getting-started/ and the tutorials of
# site/content/docs/en/guide/ (the first short video, then tutorial-vertical-short: it creates the project that
# the next tutorial reads). It extracts every fenced bash block, starts
# `stuarty serve` with a private workspace on a loopback port, runs the blocks one after the other in a
# folder that holds the sample media (made with ffmpeg) and stops at the first non-zero exit. The server and
# the work folder are removed at the end.
#
# A block that needs what this machine does not have (Docker, root, a server of its own, a git clone from
# the network) is not run: it is checked instead, and reported as "check". bash -n must accept it, every
# STUARTY_* and APP_* variable it names must be in the configuration reference, and a deploy/ script it calls
# must exist and know the flags the block passes. A block that starts the server (`stuarty serve`) is skipped:
# this script already runs that server. SMOKE_DOCKER=1 runs the Docker blocks too.
#
# A block is skipped, and reported, when it cannot run offline: it installs a tool or a model (needs the
# network, D-016; a transcription runs with the stub of the test condition) or it starts another agent client (`claude`). SMOKE_NETWORK=1 runs the model blocks too.
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
	start="$root/site/content/docs/en/getting-started"
	guide="$root/site/content/docs/en/guide"
	pages=("$start/install-windows.mdx" "$start/install-docker.mdx" "$start/install-server.mdx")
	[ ! -f "$guide/first-short-video.mdx" ] || pages+=("$guide/first-short-video.mdx")
	pages+=("$guide/tutorial-vertical-short.mdx" "$guide/tutorial-agent-edit.mdx")
	pages+=("$guide/image-editor.mdx" "$guide/asset-library.mdx" "$guide/memory-skills-prompts.mdx")
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
# APP_ENV=test with the stub transcriber (four fixed words) lets the pages run a transcription with no model
# and no network; nothing else of the test condition is used.
APP_ENV=test STUARTY_TEST_TRANSCRIBER=stub STUARTY_ADDR="127.0.0.1:$port" "$work/bin/$exe" serve >"$work/serve.log" 2>&1 &
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
checked=0
config="$root/site/content/generated/reference/configuration.mdx"

# check_block verifies a block that cannot run here: its syntax, its variables and the deploy scripts it calls.
check_block() {
	local b="$1" label="$2" v flag script
	bash -n "$b" || die "the block $label is not valid bash"
	for v in $(grep -oE '(STUARTY|APP)_[A-Z_]+' "$b" | sort -u); do
		grep -q "\`$v\`" "$config" || die "the block $label names $v, which the configuration reference does not list"
	done
	for script in $(grep -oE 'deploy/[a-z]+\.sh' "$b" | sort -u); do
		[ -f "$root/$script" ] || die "the block $label calls $script, which does not exist"
		for flag in $(grep -E "bash $script" "$b" | grep -oE ' --[a-z-]+' | sort -u); do
			grep -q -- "$flag" "$root/$script" || die "the block $label passes$flag to $script, which does not know it"
		done
	done
}
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
		if grep -qE '^[[:space:]]*stuarty serve' "$b"; then
			log "skip $label (starts the server: this script already runs one)"; skipped=$((skipped + 1)); continue
		fi
		if [ "${SMOKE_DOCKER:-}" != 1 ] && grep -qE '^[[:space:]]*(docker|sudo|git clone|bash deploy/)' "$b"; then
			check_block "$b" "$label"
			log "check $label (needs Docker or a server: syntax, variables and flags only; SMOKE_DOCKER=1 runs it)"
			checked=$((checked + 1)); continue
		fi
		if [ "${SMOKE_NETWORK:-}" != 1 ] && grep -qE 'tools:install' "$b"; then
			log "skip $label (installs a model, needs the network; SMOKE_NETWORK=1 runs it)"; skipped=$((skipped + 1)); continue
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

# The steps that need an account of the reader (an AI provider, a social network, a person's approval) are
# written as console blocks, which are not run. Every command they show must be a command of the CLI, and
# the whole flow, with those very steps, is run against the fake services by scripts/first-flow.sh.
cli_names="$(grep -oE '"name": "[a-z:-]+"' "$root/site/generated/cli.json" | cut -d'"' -f4)"
for page in "${pages[@]}"; do
	name="$(basename "$page" .mdx)"
	awk '{ sub(/$/, "") } /^[ ]*```console[ ]*$/ { inb = 1; next } inb && /^[ ]*```[ ]*$/ { inb = 0; next } inb { print }' "$page" |
		{ grep -oE '(^|[|;] *)[ ]*stuarty [a-z][a-z:-]*' || true; } | sed -E 's/.*stuarty //' | sort -u |
		while read -r cmd; do
			echo "$cli_names" | grep -qx "$cmd" || die "$name: a console block runs stuarty $cmd, which is not a command of the CLI"
		done
	if [ "$name" = first-short-video ]; then
		log "running scripts/first-flow.sh (the fake provider, the fake network and a person approval)"
		STUARTY_BIN="$work/bin/$exe" bash "$root/scripts/first-flow.sh" >"$work/first-flow.out" 2>&1 ||
			{ tail -n 20 "$work/first-flow.out" >&2; die "scripts/first-flow.sh failed"; }
		log "first-flow: $(tail -n 1 "$work/first-flow.out")"
		ran=$((ran + 1))
	fi
done

[ "$((ran + checked))" -gt 0 ] || die "no block ran"
log "ok: $ran block(s) ran, $checked checked without running, $skipped skipped"
