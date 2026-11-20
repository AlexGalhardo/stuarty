#!/usr/bin/env bash
# The part the scripted recipe runs share (epic 39): a temporary workspace, a free port, the Stuarty server, the
# test FFmpeg, and the small helpers that read a JSON answer. A recipe script sets RECIPE before it sources this
# file (the name its messages carry) and then calls `recipe_start`. Nothing here needs a network: the commercial
# and the long video use no connection, and the transcript of the long video is the fixed stub.
#
# Environment (all optional):
#   STUARTY_BIN                a built stuarty binary; without it the script builds one
#   STUARTY_TEST_FFMPEG_DIR    the folder of the test FFmpeg (ffmpeg and ffprobe, or its bin/); without it
#                              ffmpeg and ffprobe are looked up in PATH
#   RECIPE_PORT                the port of the server (default: a random one between 40000 and 59999)
#   RECIPE_KEEP=1              keep the temporary folder and say where it is
#   TMPDIR                     where the temporary folder is made
#
# shellcheck shell=bash

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# The fake transcriber works only under APP_ENV=test, so a recipe runs under it and refuses another value.
if [ -n "${APP_ENV:-}" ] && [ "${APP_ENV}" != "test" ]; then
	echo "$RECIPE: APP_ENV is ${APP_ENV}; the fake transcriber needs APP_ENV=test" >&2
	exit 2
fi
export APP_ENV=test

native() {
	if command -v cygpath >/dev/null 2>&1; then cygpath -w "$1"; else printf '%s' "$1"; fi
}

die() {
	echo "$RECIPE: FAILED: $*" >&2
	exit 1
}

step() { echo "== $*" >&2; }

WORK="$(mktemp -d "${TMPDIR:-/tmp}/$RECIPE.XXXXXX")"
SERVER_PID=""

cleanup() {
	local code=$?
	if [ -n "$SERVER_PID" ]; then kill "$SERVER_PID" >/dev/null 2>&1 || true; fi
	# The server holds its files open until it is gone.
	sleep 1
	if [ "${RECIPE_KEEP:-}" = 1 ]; then
		echo "$RECIPE: kept $WORK" >&2
	else
		rm -rf "$WORK" >/dev/null 2>&1 || true
	fi
	exit "$code"
}
trap cleanup EXIT

# field NAME: the first string value of NAME in the JSON on standard input.
field() { grep -o "\"$1\":\"[^\"]*\"" | head -n 1 | cut -d'"' -f4; }
# number NAME: the first number value of NAME.
number() { grep -o "\"$1\":[0-9]*" | head -n 1 | cut -d: -f2; }

# need NAME VALUE CONTEXT: VALUE, or a failure that says NAME was not found in CONTEXT.
need() {
	[ -n "$2" ] || die "$1 not found in: $3"
	printf '%s' "$2"
}

stuarty() { "$STUARTY" "$@"; }

# recipe_start: find the tools, build the binary, start the server on its own workspace and port.
recipe_start() {
	local ffdir="${STUARTY_TEST_FFMPEG_DIR:-}" exe=""
	case "$(uname -s)" in MINGW* | MSYS* | CYGWIN*) exe=".exe" ;; esac
	if [ -n "$ffdir" ]; then
		if [ -x "$ffdir/bin/ffmpeg$exe" ]; then ffdir="$ffdir/bin"; fi
		FFMPEG="$ffdir/ffmpeg$exe"
		FFPROBE="$ffdir/ffprobe$exe"
	else
		FFMPEG="$(command -v ffmpeg || true)"
		FFPROBE="$(command -v ffprobe || true)"
	fi
	[ -x "$FFMPEG" ] && [ -x "$FFPROBE" ] || die "no ffmpeg and ffprobe: set STUARTY_TEST_FFMPEG_DIR or put them in PATH"

	mkdir -p "$WORK/bin"
	STUARTY="${STUARTY_BIN:-}"
	if [ -z "$STUARTY" ]; then
		step "building stuarty"
		(cd "$ROOT" && go build -o "$WORK/bin/stuarty$exe" ./web/cmd/app)
		STUARTY="$WORK/bin/stuarty$exe"
	fi

	HOME_DIR="$WORK/home"
	mkdir -p "$HOME_DIR"
	export STUARTY_HOME
	STUARTY_HOME="$(native "$HOME_DIR")"
	export STUARTY_MASTER_KEY_FILE
	STUARTY_MASTER_KEY_FILE="$(native "$WORK/master.key")"
	export STUARTY_FFMPEG STUARTY_FFPROBE
	STUARTY_FFMPEG="$(native "$FFMPEG")"
	STUARTY_FFPROBE="$(native "$FFPROBE")"
	export STUARTY_TEST_TRANSCRIBER=stub
	unset VUKEM_VIDEO_HOME STUARTY_PROJECT

	local port="${RECIPE_PORT:-$((40000 + RANDOM % 20000))}"
	case "$port" in 7878 | 7979) die "port $port belongs to a person's own Stuarty" ;; esac
	export STUARTY_ADDR="127.0.0.1:$port"
	BASE="http://127.0.0.1:$port"

	step "starting the server on $STUARTY_ADDR with the workspace $STUARTY_HOME"
	"$STUARTY" serve >"$WORK/serve.log" 2>&1 &
	SERVER_PID=$!
	local _
	for _ in $(seq 1 150); do
		curl -sf "$BASE/up" >/dev/null 2>&1 && break
		kill -0 "$SERVER_PID" 2>/dev/null || die "the server stopped: $(tail -n 5 "$WORK/serve.log")"
		sleep 0.2
	done
	curl -sf "$BASE/up" >/dev/null 2>&1 || die "the server did not answer: $(tail -n 5 "$WORK/serve.log")"
}

# revision PROJECT: the current revision of the project.
revision() { stuarty project:show "$1" --json | number revision; }

# export_file PROJECT: exports the project as it is, waits for the job, and prints the path of the file as the
# local machine writes it. It fails when the job fails or when a check of the export says it did not hold.
export_file() {
	local out path
	out="$(stuarty export --project "$1" --wait --json)" || die "the export of $1 failed: $out"
	case "$out" in *'"state":"succeeded"'*) ;; *) die "the export of $1 did not succeed: $out" ;; esac
	case "$out" in *'"ok":false'*) die "a check of the export of $1 failed: $out" ;; esac
	path="$(need path "$(printf '%s' "$out" | field path)" "$out")"
	# The path is JSON: a backslash of a Windows path comes doubled.
	printf '%s' "$path" | sed 's/\\\\/\\/g'
}

# probe FILE: width, height and duration of the first video stream, as "WIDTHxHEIGHT SECONDS".
probe() {
	"$FFPROBE" -v error -select_streams v:0 -show_entries stream=width,height:format=duration \
		-of csv=p=0:s=x "$(native "$1")" | tr -d '\r' | tr '\n' ' ' | sed 's/ *$//'
}
