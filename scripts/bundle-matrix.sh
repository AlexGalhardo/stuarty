#!/usr/bin/env bash
# The cross-system check of BETA exit criterion 5 (P3-REL-005, the `bundle-matrix` job of ci.yml):
#
#   bash scripts/bundle-matrix.sh export <out.vvbundle>
#   bash scripts/bundle-matrix.sh verify <a.vvbundle> [<b.vvbundle> ...]
#
# `export` builds the reference project from fixed ids and bit-exact sample audio in a private workspace and
# writes its bundle (without the operation log, which holds times). `verify` imports each bundle into an
# empty private workspace, exports it again, and checks that the document hash and the media hashes (printed by
# internal/tools/bundlehash) of the re-export equal those of the bundle, and that every bundle given has the
# same hashes: bundles made on other systems must equal the one made here. Exit 0 means all of it holds.
# Needs: go, ffmpeg on PATH. Environment:
#   VUKEM_VIDEO_BIN   the binary to use (default: built from this checkout)
#   BUNDLE_KEEP       set to 1 to keep the work folder
set -euo pipefail

die() { echo "bundle-matrix: error: $*" >&2; exit 1; }
log() { echo "bundle-matrix: $*" >&2; }

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
mode="${1:-}"
[ "$#" -ge 2 ] && { [ "$mode" = export ] || [ "$mode" = verify ]; } || die "usage: bundle-matrix.sh export <out> | verify <bundle>..."
shift
[ "$mode" != export ] || [ "$#" -eq 1 ] || die "export takes one output path"

command -v go >/dev/null 2>&1 || die "go is required"
work="$(mktemp -d)"
cleanup() {
	local status=$?
	set +e
	if [ "${BUNDLE_KEEP:-}" = 1 ]; then log "kept $work"; else rm -rf "$work" 2>/dev/null; fi
	return "$status"
}
trap cleanup EXIT

case "$(uname -s)" in MINGW* | MSYS* | CYGWIN*) exe=vukem-video.exe ;; *) exe=vukem-video ;; esac
bin="$work/$exe"
if [ -n "${VUKEM_VIDEO_BIN:-}" ]; then
	cp "$VUKEM_VIDEO_BIN" "$bin"
else
	log "building the binary"
	(cd "$root" && go build -o "$bin" ./web/cmd/app)
fi
hashtool="$work/bundlehash"
(cd "$root" && go build -o "$hashtool" ./internal/tools/bundlehash)

unset VUKEM_VIDEO_URL APP_KEY APP_ENV
project=prj_7k2m9x4q1b

# vv <home> <args...>: the product CLI in a private workspace; commands run in-process, no server is needed.
vv() {
	local home="$1"
	shift
	VUKEM_VIDEO_HOME="$home" "$bin" "$@"
}

# export_bundle <home> <out>: exports the project of the workspace without the log and copies the file.
export_bundle() {
	vv "$1" bundle:export --project "$project" --no-log --wait >/dev/null
	local files=("$1"/exports/"$project"/bundle-*.vvbundle)
	[ "${#files[@]}" -eq 1 ] && [ -f "${files[0]}" ] || die "expected one bundle in $1/exports/$project"
	cp "${files[0]}" "$2"
}

if [ "$mode" = export ]; then
	command -v ffmpeg >/dev/null 2>&1 || die "ffmpeg is required to make the sample audio"
	home="$work/home-export"
	mkdir -p "$home"
	# Bit-exact: no encoder tag, no metadata, so the bytes (and the hash) are the same on every system.
	ffmpeg -v error -y -f lavfi -i sine=frequency=440:duration=2 -c:a pcm_s16le -bitexact -fflags +bitexact \
		-map_metadata -1 "$work/reference.wav"
	vv "$home" project:create --name "Reference" --id "$project" >/dev/null
	vv "$home" media:import "$work/reference.wav" --project "$project" --asset-id ast_3c5v8n2w6d >/dev/null
	vv "$home" edit --project "$project" --ops - >/dev/null <<'JSON'
[{"op":"track.add","args":{"kind":"audio","name":"Music","id":"trk_m1n2p3q4r5"}},
 {"op":"audio.add","args":{"track_id":"trk_m1n2p3q4r5","asset_id":"ast_3c5v8n2w6d","start":"0s","id":"itm_s6t7v8w9x0"}}]
JSON
	export_bundle "$home" "$1"
	log "wrote $1"
	"$hashtool" "$1" >&2
	exit 0
fi

first=""
for bundle in "$@"; do
	[ -f "$bundle" ] || die "no such bundle: $bundle"
	home="$work/home-$(basename "$bundle" .vvbundle)"
	mkdir -p "$home"
	want="$("$hashtool" "$bundle")"
	vv "$home" bundle:import "$bundle" >/dev/null
	export_bundle "$home" "$work/reexport.vvbundle"
	got="$("$hashtool" "$work/reexport.vvbundle")"
	if [ "$got" != "$want" ]; then
		echo "--- $bundle:" >&2
		echo "$want" >&2
		echo "--- after import and export here:" >&2
		echo "$got" >&2
		die "the hashes changed through an import on this system"
	fi
	if [ -z "$first" ]; then
		first="$want"
		first_name="$bundle"
	elif [ "$want" != "$first" ]; then
		echo "--- $first_name:" >&2
		echo "$first" >&2
		echo "--- $bundle:" >&2
		echo "$want" >&2
		die "the bundles do not have the same document and media hashes"
	fi
	log "ok $bundle"
done
log "ok: $# bundle(s), equal hashes"
