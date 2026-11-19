#!/usr/bin/env bash
# End-to-end smoke test of the product, the way a person or an agent uses it:
#
#   bash scripts/smoke-e2e.sh
#
# It builds the binary, starts `stuarty serve` with a private workspace on a loopback port, and drives
# it through the CLI: create a project, import a generated clip, add tracks, the clip and a title, lint,
# export with --wait, and check the MP4 with ffprobe (duration, size, a video and an audio stream). The
# server is stopped by its pid and the work folder removed at the end. Prints SMOKE_OK or exits non-zero.
#
# Needs ffmpeg and ffprobe on PATH or in STUARTY_TEST_FFMPEG_DIR, and go (unless STUARTY_BIN is set).
#   STUARTY_BIN  the binary to test (default: built from this checkout)
#   SMOKE_PORT       loopback port (default: a random one in 20000 to 39999)
#   SMOKE_KEEP       set to 1 to keep the work folder
set -euo pipefail

die() { echo "smoke-e2e: error: $*" >&2; exit 1; }
log() { echo "smoke-e2e: $*" >&2; }

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
if [ -n "${STUARTY_TEST_FFMPEG_DIR:-}" ]; then
	ffdir="$STUARTY_TEST_FFMPEG_DIR"
	# A Windows path (C:/...) has a colon, the PATH separator of Git Bash: convert it first.
	if command -v cygpath >/dev/null 2>&1; then ffdir="$(cygpath -u "$ffdir")"; fi
	export PATH="$ffdir:$PATH"
fi
command -v ffmpeg >/dev/null 2>&1 || die "ffmpeg is required"
command -v ffprobe >/dev/null 2>&1 || die "ffprobe is required"

work="$(mktemp -d)"
server_pid=""
cleanup() {
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
mkdir -p "$work/bin" "$work/cwd" "$work/home"
if [ -n "${STUARTY_BIN:-}" ]; then
	cp "$STUARTY_BIN" "$work/bin/$exe"
else
	command -v go >/dev/null 2>&1 || die "go is required to build the binary (or set STUARTY_BIN)"
	log "building the binary"
	(cd "$root" && go build -o "$work/bin/$exe" ./web/cmd/app)
fi
chmod +x "$work/bin/$exe"
export PATH="$work/bin:$PATH"

# A private home: the run never touches the workspace of the person who runs it.
unset STUARTY_URL APP_KEY APP_ENV
export STUARTY_HOME="$work/home"
port="${SMOKE_PORT:-$((20000 + RANDOM % 20000))}"
STUARTY_ADDR="127.0.0.1:$port" "$work/bin/$exe" serve >"$work/serve.log" 2>&1 &
server_pid=$!
# Waiting for the server to announce itself is a readiness poll with a deadline, not a sleep in a test.
for _ in $(seq 1 120); do
	[ -s "$work/home/runtime.json" ] && break
	kill -0 "$server_pid" 2>/dev/null || { cat "$work/serve.log" >&2; die "the server exited at start"; }
	sleep 0.5
done
[ -s "$work/home/runtime.json" ] || { cat "$work/serve.log" >&2; die "the server did not announce itself in 60 s"; }
log "server up on 127.0.0.1:$port"

cd "$work/cwd"
ffmpeg -v error -f lavfi -i testsrc2=size=640x360:rate=30:duration=4 -f lavfi -i sine=frequency=220:duration=4 \
	-c:v libx264 -pix_fmt yuv420p -c:a aac -shortest clip.mp4 2>/dev/null \
	|| ffmpeg -v error -f lavfi -i testsrc2=size=640x360:rate=30:duration=4 -f lavfi -i sine=frequency=220:duration=4 \
		-c:v mpeg4 -c:a mp2 -shortest clip.mp4

step() {
	local name="$1"
	shift
	if ! "$@" >"$work/$name.out" 2>&1; then
		cat "$work/$name.out" >&2
		echo "--- server log (tail):" >&2
		tail -20 "$work/serve.log" >&2
		die "step $name failed: $*"
	fi
	log "ok: $name"
}

prj=prj_5m0k3t1e2s
step project stuarty project:create --name "Smoke" --id "$prj"
step import stuarty media:import clip.mp4 --project "$prj" --asset-id ast_5m0k3c7a2p
step edit stuarty edit --project "$prj" --ops - <<'JSON'
[{"op":"track.add","args":{"kind":"visual","name":"Video","id":"trk_5m0k3v1d2e"}},
 {"op":"track.add","args":{"kind":"visual","name":"Titles","id":"trk_5m0k3t1t2r"}},
 {"op":"clip.add","args":{"track_id":"trk_5m0k3v1d2e","asset_id":"ast_5m0k3c7a2p","start":"0s","id":"itm_5m0k3c1v2p"}},
 {"op":"text.add","args":{"track_id":"trk_5m0k3t1t2r","start":"0s","duration":"2s","text":"Smoke","id":"itm_5m0k3t1x2t"}}]
JSON
step lint stuarty lint --project "$prj"
step export stuarty export --project "$prj" --wait --out "$work/cwd/out.mp4"

[ -s out.mp4 ] || die "the export wrote no file"
probe="$(ffprobe -v error -show_entries format=duration:stream=codec_type,width,height -of default=nw=1 out.mp4)"
echo "$probe" | grep -q "codec_type=video" || die "the export has no video stream: $probe"
echo "$probe" | grep -q "codec_type=audio" || die "the export has no audio stream: $probe"
dur="$(echo "$probe" | awk -F= '/^duration=/ {print $2; exit}')"
awk -v d="$dur" 'BEGIN { exit !(d >= 3.9 && d <= 4.1) }' || die "the export lasts $dur s, expected 4 s"
log "export: $(echo "$probe" | tr '\n' ' ')"
echo SMOKE_OK
