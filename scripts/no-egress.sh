#!/usr/bin/env bash
# The no-egress check of SDD-007 section 10.2 item 3 (check 25, the `no-egress` job of ci.yml):
#
#   bash scripts/no-egress.sh
#
# Runs the product with every VUKEM_VIDEO_* variable unset through a reference session, each process under
# `strace -f -e trace=connect,sendto,sendmsg,sendmmsg`, so the children (FFmpeg, ffprobe) are traced too:
#
#   1. `serve` (the owner, in the background, bound to loopback);
#   2. the CLI, the reference session: project:create, media:import, edit, export, view (the owner runs the
#      jobs and FFmpeg);
#   3. the CLI, the BETA scenario (TASK-0149): every export format (mp4, gif, wav, aac, png), OTIO export and
#      import, bundle export and import, source import (media and a font file), a template saved, written
#      and imported, the brand kit (colours, a font and a logo) and the helper install, which must stop at
#      its question or be refused, so it connects to nothing;
#   4. `mcp` on stdio: initialize and tools/list.
#
# It fails on any socket address that is not loopback (127.0.0.0/8, ::1, ::ffff:127.x) or a Unix socket, and
# fails when the trace shows no loopback connect at all (then strace saw nothing and the result is void).
# Linux only. The desktop binary has no headless mode, so it is not covered here (SDD-007 section 14).
# The helper download itself (`tools:install`) is the one outbound path the product has (SDD-007 section
# 6.8: HTTPS to the hosts of the embedded manifest), so it is not run here: the check is that it stops at its
# question and that `--yes` from a session that is not a person is refused, both without a connection.
# Needs: go (or VUKEM_VIDEO_BIN), ffmpeg on PATH, strace. Environment:
#   VUKEM_VIDEO_BIN   the binary to test (default: built from this checkout)
#   NOEGRESS_PORT     loopback port of the server (default a random one in 20000 to 39999)
#   NOEGRESS_KEEP     set to 1 to keep the work folder and the traces
set -euo pipefail

die() { echo "no-egress: error: $*" >&2; exit 1; }
log() { echo "no-egress: $*" >&2; }

[ "$(uname -s)" = Linux ] || die "Linux only (strace)"
command -v strace >/dev/null 2>&1 || die "strace is required"
[ "$(id -u)" -ne 0 ] || die "do not run as root: the owner refuses to start FFmpeg as root"
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
	local attempt
	for attempt in 1 2 3; do
		if strace -f --seccomp-bpf -e trace=connect,sendto,sendmsg,sendmmsg -o "$work/traces/$label.log" -- "$@" \
			>"$work/$label.out" 2>&1; then
			return 0
		fi
		# The heavy-call ceiling is ten a minute. A full session crosses it; wait out the window and retry.
		if grep -q 'E_RATE_LIMITED' "$work/$label.out"; then
			log "$label: the heavy-call limit answered, waiting 65s (attempt $attempt)"
			sleep 65
			continue
		fi
		cat "$work/$label.out" >&2
		die "$label failed"
	done
	cat "$work/$label.out" >&2
	die "$label failed"
}
vv() { traced "$1" "$bin" "${@:2}"; }
# vv_refused <label> <args...>: the command must exit non-zero, and its refusal must not connect anywhere either.
vv_refused() {
	local label="$1"
	shift
	if strace -f --seccomp-bpf -e trace=connect,sendto,sendmsg,sendmmsg -o "$work/traces/$label.log" -- "$bin" "$@" 		>"$work/$label.out" 2>&1; then
		cat "$work/$label.out" >&2
		die "$label was expected to be refused and succeeded"
	fi
}
# json_field <name> <file>: the first string member with that name of a one-line JSON document.
json_field() { sed -n "s/.*\"$1\":\"\([^\"]*\)\".*/\1/p" "$2" | head -n 1; }

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
vv cli-create project:create --name "No egress" --id "$project"
vv cli-import media:import "$work/clip.mp4" --project "$project" --asset-id ast_3c5v8n2w6d
vv cli-edit edit --project "$project" --ops - <<'JSON'
[{"op":"track.add","args":{"kind":"visual","name":"Video","id":"trk_m1n2p3q4r5"}},
 {"op":"clip.add","args":{"track_id":"trk_m1n2p3q4r5","asset_id":"ast_3c5v8n2w6d","start":"0s","duration":"2s","id":"itm_s6t7v8w9x0"}}]
JSON
vv cli-export export --project "$project" --wait --timeout 3m
vv cli-view view --project "$project" --at 1s --json

log "CLI session: every export format"
for format in gif wav aac; do
	vv "cli-export-$format" export --project "$project" --format "$format" --wait --timeout 3m
done
vv cli-export-png export --project "$project" --format png --at 1s --wait --timeout 3m

log "CLI session: OTIO export and import"
project2=prj_8d3f6g1h4j
vv cli-otio-export otio:export --project "$project" --out "$work/cut.otio"
vv cli-otio-create project:create --name "From OTIO" --id "$project2"
vv cli-otio-media media:import "$work/clip.mp4" --project "$project2" --asset-id ast_9w8x7y6z5a
vv cli-otio-import otio:import "$work/cut.otio" --project "$project2"

log "CLI session: bundle export and import"
vv cli-bundle-export bundle:export --project "$project" --out "$work/project.vvbundle"
vv cli-bundle-import bundle:import "$work/project.vvbundle" --as-new

log "CLI session: font import, template, brand kit"
vv cli-font-import font:import "$root/renderer/fonts/Anton-Regular.ttf" --json
font_hash="$(json_field hash "$work/cli-font-import.out")"
[ -n "$font_hash" ] || die "font:import printed no hash"
# A template carries no media, so it comes from an empty project.
vv cli-template-project project:create --name "Template source" --id prj_2c4e6g8j0k
vv cli-template-save template:save --project prj_2c4e6g8j0k --name "No egress" --json
template_id="$(json_field template_id "$work/cli-template-save.out")"
[ -n "$template_id" ] || die "template:save printed no id"
vv cli-template-export template:export "$template_id" --out "$work/template.vvtemplate"
vv cli-template-import template:import "$work/template.vvtemplate"
ffmpeg -v error -f lavfi -i color=c=red:size=64x64:duration=1 -frames:v 1 "$work/logo.png"
vv cli-logo-import media:import "$work/logo.png" --project "$project"
# The logo of the kit is named by the SHA-256 of the image file.
logo_hash="sha256:$(sha256sum "$work/logo.png" | cut -d' ' -f1)"
printf '{"logo":"%s","colors":{"primary":"#ff0000","text":"#ffffff"},"fonts":{"Brand Display":{"files":[{"weight":400,"hash":"%s"}]}}}
' 	"$logo_hash" "$font_hash" >"$work/brand.json"
vv cli-brand-set brand:set --kit "$work/brand.json"
vv cli-brand-show brand:show --json

log "CLI session: the helper install stops at its question and connects to nothing"
# Without --yes the command asks; an empty standard input answers no.
vv cli-tools-install tools:install whisper </dev/null
grep -q "nothing was downloaded" "$work/cli-tools-install.out" || { cat "$work/cli-tools-install.out" >&2; die "tools:install did not stop at the question"; }

# With --yes a session that is not a person at the editor is refused before any download (E_FORBIDDEN,
# SDD-007 section 6.8): the helper download itself is the one allowed outbound path (HTTPS to the hosts of
# the embedded manifest) and is not run here, so the trace stays free of non-loopback addresses.
vv_refused cli-tools-install-yes tools:install whisper --yes </dev/null

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
bash "$root/scripts/no-egress-scan.sh" "${logs[@]}"
