#!/usr/bin/env bash
# End-to-end smoke test of the product, the way a person or an agent uses it:
#
#   bash scripts/smoke-e2e.sh
#
# It builds the binary, starts `stuarty serve` with a private workspace on a loopback port, and drives
# it through the CLI: create a project, import a generated clip, add tracks, the clip and a title, lint,
# export with --wait, and check the MP4 with ffprobe (duration, size, a video and an audio stream). The
# server is stopped by its pid and the work folder removed at the end. It then makes a thumbnail, the reference
# scenario of an image (SDD-021): a template with slots, an image made from it with values, layers added and
# aligned, one step undone, a PNG exported twice with the same bytes, by the CLI and then by the MCP tool.
# Prints SMOKE_OK or exits non-zero.
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
# --- the reference scenario of an image (SDD-021) ---
# jsonfield <file> <name> prints the first string value of "name" in a JSON document.
jsonfield() { grep -o "\"$2\": *\"[^\"]*\"" "$1" | head -1 | sed -e 's/^[^:]*: *"//' -e 's/"$//'; }

ffmpeg -v error -f lavfi -i testsrc2=size=640x360:rate=1 -frames:v 1 face.png
step faceimport stuarty media:import face.png --project "$prj" --json
face="$(grep -o 'sha256:[0-9a-f]\{64\}' "$work/faceimport.out" | head -1)"
[ -n "$face" ] || die "the import of face.png gave no hash: $(cat "$work/faceimport.out")"

step tplcreate stuarty image:create --template --preset video_thumbnail --name "Thumbnail" --json
tpl="$(jsonfield "$work/tplcreate.out" image_id)"
[ -n "$tpl" ] || die "no template id"
step tpledit stuarty image:edit "$tpl" --ops - <<JSON
[{"op":"layer.add_image","args":{"asset":"$face","slot":"photo","fit":"cover"}},
 {"op":"layer.add_text","args":{"text":"","slot":"title","style":{"font_family":"Anton","size":120,"color":"#FFE600"},"y":440}}]
JSON
step imgcreate stuarty image:create --from-template "$tpl" --name "Episode 12" --values - --json <<'JSON'
{"title":{"text":"HOW IT WORKS"}}
JSON
img="$(jsonfield "$work/imgcreate.out" image_id)"
[ -n "$img" ] || die "no image id"
step imgedit stuarty image:edit "$img" --ops - <<'JSON'
[{"op":"layer.add_shape","args":{"shape":"rounded_rect","w":520,"h":90,"radius":24,"fill":"#E63946","id":"lyr_5m0k3b4r2s","index":1}},
 {"op":"layer.align","args":{"layer_ids":["lyr_5m0k3b4r2s"],"to":"canvas","horizontal":"center"}}]
JSON
step imgundo stuarty image:undo "$img"
step imgshow stuarty image:show "$img"
grep -q "layers=2" "$work/imgshow.out" || die "the undo left the wrong layers: $(cat "$work/imgshow.out")"
step imgexport1 stuarty image:export "$img" --json
step imgexport2 stuarty image:export "$img" --json
sum1="$(jsonfield "$work/imgexport1.out" sha256)"
sum2="$(jsonfield "$work/imgexport2.out" sha256)"
[ -n "$sum1" ] && [ "$sum1" = "$sum2" ] || die "two exports of the same image differ: $sum1 $sum2"
png="$(jsonfield "$work/imgexport1.out" path)"
if command -v cygpath >/dev/null 2>&1; then png="$(cygpath -u "$png")"; fi
iprobe="$(ffprobe -v error -show_entries stream=codec_name,width,height -of default=nw=1 "$png")"
echo "$iprobe" | grep -q "codec_name=png" && echo "$iprobe" | grep -q "width=1280" && echo "$iprobe" | grep -q "height=720" 	|| die "the exported thumbnail is not a 1280x720 PNG: $iprobe"
log "thumbnail: $(echo "$iprobe" | tr '
' ' ')"

# The same, through the MCP tool: one call per process, so the id of the image is read between calls.
mcp_image() {
	{
		printf '%s
' '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"smoke","version":"1"}}}'
		printf '%s
' '{"jsonrpc":"2.0","method":"notifications/initialized"}'
		printf '%s
' "{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"tools/call\",\"params\":{\"name\":\"image\",\"arguments\":$1}}"
	} | stuarty mcp 2>"$work/mcp.err" | grep '"id":2' || { cat "$work/mcp.err" >&2; return 1; }
}
mcp1="$(mcp_image "{\"action\":\"create\",\"from_template\":\"$tpl\",\"values\":{\"title\":{\"text\":\"BY MCP\"}}}")" || die "mcp create failed"
mimg="$(echo "$mcp1" | grep -o 'img_[0-9a-z]\{10\}' | head -1)"
[ -n "$mimg" ] || die "the MCP create gave no image id: $mcp1"
mcp_image "{\"action\":\"edit\",\"image_id\":\"$mimg\",\"base_revision\":1,\"operations\":[{\"op\":\"layer.add_shape\",\"args\":{\"shape\":\"ellipse\",\"w\":200,\"h\":200,\"fill\":\"#FFFFFF\"}}]}" >"$work/mcpedit.out" || die "mcp edit failed"
grep -q '"isError":true' "$work/mcpedit.out" && die "the MCP edit was refused: $(cat "$work/mcpedit.out")"
mcp_image "{\"action\":\"undo\",\"image_id\":\"$mimg\"}" >"$work/mcpundo.out" || die "mcp undo failed"
mcp_image "{\"action\":\"export\",\"image_id\":\"$mimg\"}" >"$work/mcpexport.out" || die "mcp export failed"
grep -q '"isError":true' "$work/mcpexport.out" && die "the MCP export failed: $(cat "$work/mcpexport.out")"
grep -q 'exported img_' "$work/mcpexport.out" || die "the MCP export did not say it exported: $(cat "$work/mcpexport.out")"
log "ok: the thumbnail through the MCP tool"

echo SMOKE_OK
