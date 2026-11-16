#!/usr/bin/env bash
# The no-egress check of SDD-007 section 10.2 item 3 (check 25, the `no-egress` job of ci.yml):
#
#   bash scripts/no-egress.sh
#
# Runs the product with every VUKEM_VIDEO_* variable unset through a reference session, each process under
# `strace -f -e trace=connect,sendto,sendmsg,sendmmsg`, so the children (FFmpeg, ffprobe) are traced too:
#
#   1. `serve` (the owner, in the background, bound to loopback);
#   2. the CLI: project:create, media:import, edit, export, view (the owner runs the jobs and FFmpeg);
#   3. `mcp` on stdio: initialize and tools/list.
#
# It fails on any socket address that is not loopback (127.0.0.0/8, ::1, ::ffff:127.x) or a Unix socket, and
# fails when the trace shows no loopback connect at all (then strace saw nothing and the result is void).
# Linux only. The desktop binary has no headless mode, so it is not covered here (SDD-007 section 14).
# Needs: go (or VUKEM_VIDEO_BIN), ffmpeg on PATH, strace. Environment:
#   VUKEM_VIDEO_BIN   the binary to test (default: built from this checkout)
#   NOEGRESS_PORT     loopback port of the server (default a random one in 20000 to 39999)
#   NOEGRESS_KEEP     set to 1 to keep the work folder and the traces
set -euo pipefail

die() { echo "no-egress: error: $*" >&2; exit 1; }
log() { echo "no-egress: $*" >&2; }

[ "$(uname -s)" = Linux ] || die "Linux only (strace)"
command -v strace >/dev/null 2>&1 || die "strace is required"
command -v ffmpeg >/dev/null 2>&1 || die "ffmpeg is required to make the sample media"

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
work="$(mktemp -d)"
server_pid=""
# stop_server ends the traced server: strace does not pass SIGTERM on, so the signal goes to its child.
stop_server() {
	[ -n "$server_pid" ] || return 0
	pkill -TERM -P "$server_pid" 2>/dev/null || true
	for _ in $(seq 1 40); do kill -0 "$server_pid" 2>/dev/null || break; sleep 0.5; done
	pkill -KILL -P "$server_pid" 2>/dev/null || true
	wait "$server_pid" 2>/dev/null || true
	server_pid=""
}
cleanup() {
	local status=$?
	set +e
	stop_server
	if [ "${NOEGRESS_KEEP:-}" = 1 ]; then log "kept $work"; else rm -rf "$work"; fi
	return "$status"
}
trap cleanup EXIT

if [ -n "${VUKEM_VIDEO_BIN:-}" ]; then
	bin="$work/vukem-video"
	cp "$VUKEM_VIDEO_BIN" "$bin"
else
	command -v go >/dev/null 2>&1 || die "go is required to build the binary (or set VUKEM_VIDEO_BIN)"
	log "building the binary"
	bin="$work/vukem-video"
	(cd "$root" && go build -o "$bin" ./web/cmd/app)
fi
chmod +x "$bin"

# Nothing may configure an outbound path, and no other workspace may be reached.
while IFS='=' read -r name _; do
	case "$name" in VUKEM_VIDEO_* | APP_KEY | APP_ENV | OTEL_*) unset "$name" ;; esac
done < <(env)
export VUKEM_VIDEO_HOME="$work/home"
mkdir -p "$VUKEM_VIDEO_HOME" "$work/traces"
port="${NOEGRESS_PORT:-$((20000 + RANDOM % 20000))}"
project=prj_7k2m9x4q1b

# traced <label> <command...>: runs the command under strace; the trace goes to traces/<label>.log.
traced() {
	local label="$1"
	shift
	strace -f --seccomp-bpf -e trace=connect,sendto,sendmsg,sendmmsg -o "$work/traces/$label.log" -- "$@"
}
vv() { traced "$1" "$bin" "${@:2}"; }

log "starting the server on 127.0.0.1:$port"
VUKEM_VIDEO_ADDR="127.0.0.1:$port" VUKEM_VIDEO_MIN_FREE_BYTES=1 \
	strace -f --seccomp-bpf -e trace=connect,sendto,sendmsg,sendmmsg -o "$work/traces/serve.log" -- \
	"$bin" serve >"$work/serve.log" 2>&1 &
server_pid=$!
for _ in $(seq 1 120); do
	[ -s "$VUKEM_VIDEO_HOME/runtime.json" ] && break
	kill -0 "$server_pid" 2>/dev/null || { cat "$work/serve.log" >&2; die "the server exited at start"; }
	sleep 0.5
done
[ -s "$VUKEM_VIDEO_HOME/runtime.json" ] || { cat "$work/serve.log" >&2; die "the server did not announce itself in 60 s"; }

log "CLI session: create, import, edit, export, view"
ffmpeg -v error -f lavfi -i testsrc2=size=320x180:rate=30:duration=2 -f lavfi -i sine=frequency=220:duration=2 \
	-c:v mpeg4 -c:a aac -shortest "$work/clip.mp4"
vv cli-create project:create --name "No egress" --id "$project" >/dev/null
vv cli-import media:import "$work/clip.mp4" --project "$project" --asset-id ast_3c5v8n2w6d >/dev/null
vv cli-edit edit --project "$project" --ops - >/dev/null <<'JSON'
[{"op":"track.add","args":{"kind":"visual","name":"Video","id":"trk_m1n2p3q4r5"}},
 {"op":"clip.add","args":{"track_id":"trk_m1n2p3q4r5","asset_id":"ast_3c5v8n2w6d","start":"0s","duration":"2s","id":"itm_s6t7v8w9x0"}}]
JSON
vv cli-export export --project "$project" --wait --timeout 3m >/dev/null
vv cli-view view --project "$project" --at 1s --json >/dev/null

log "MCP session on stdio: initialize, tools/list"
mcp_out="$work/mcp.out"
{
	printf '%s\n' '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"no-egress","version":"1"}}}'
	printf '%s\n' '{"jsonrpc":"2.0","method":"notifications/initialized"}'
	printf '%s\n' '{"jsonrpc":"2.0","id":2,"method":"tools/list"}'
	sleep 3
} | timeout 60 strace -f --seccomp-bpf -e trace=connect,sendto,sendmsg,sendmmsg -o "$work/traces/mcp.log" -- "$bin" mcp >"$mcp_out" || true
grep -q '"tools"' "$mcp_out" || { cat "$mcp_out" >&2; die "the MCP session did not list the tools"; }

log "stopping the server"
stop_server

logs=("$work"/traces/*.log)
loopback_re='inet_addr\("127\.[0-9.]+"\)|inet_pton\(AF_INET6, "(::1|::ffff:127\.[0-9.]+)"\)'
net_lines="$(grep -hE 'sa_family=AF_INET6?' "${logs[@]}" || true)"
bad="$(printf '%s\n' "$net_lines" | grep -vE "^$|$loopback_re" || true)"
if [ -n "$bad" ]; then
	echo "no-egress: socket addresses that are not loopback:" >&2
	printf '%s\n' "$bad" >&2
	die "a non-loopback address was used"
fi
good="$(printf '%s\n' "$net_lines" | grep -cE "$loopback_re" || true)"
[ "$good" -gt 0 ] || die "the traces hold no loopback connect: strace saw nothing, the result is void"
log "ok: $good loopback socket calls, no other address, in ${#logs[@]} traced processes"
