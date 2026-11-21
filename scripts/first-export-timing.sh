#!/usr/bin/env bash
# Times the path from docker run to the first verified export (NFR-040):
#   bash scripts/first-export-timing.sh [IMAGE] [--no-pull]
# It follows docs/self-hosting/docker ("Your first export from the container"):
# a fresh container and a fresh volume, an API key made with the first
# administrator, the CLI inside the container, one clip, one export whose checks
# must all pass. It removes the container and the volumes it creates, never the
# image. It exits 1 when a step fails or the total passes 600 seconds (NFR-040).
# Needs docker, curl, openssl and a port 7878 free on 127.0.0.1 (PORT overrides).
#
#   bash scripts/first-export-timing.sh --binary PATH_TO_STUARTY
#
# measures the same path for the binary install instead (epic 40 task 9): a clean workspace in a temporary
# folder, `stuarty serve` in local mode on a loopback port (PORT, default a random one in 20000 to 39999,
# never 7878 or 7979: those belong to the Stuarty of the person who runs it), the same project,
# import, edit and export, and the same verification. It needs no Docker and no network. ffmpeg and ffprobe
# come from STUARTY_TEST_FFMPEG_DIR or from PATH. The time is only a fair measure on an idle machine.
set -euo pipefail

image="ghcr.io/alexgalhardo/stuarty:latest"
pull=1
binary=""
while [ "$#" -gt 0 ]; do
	case "$1" in
	--no-pull) pull=0 ;;
	--binary)
		binary="${2:?--binary needs the path of a stuarty binary}"
		shift
		;;
	*) image="$1" ;;
	esac
	shift
done

# Git Bash on Windows rewrites /media into a host path in docker arguments.
export MSYS_NO_PATHCONV=1
if [ -n "$binary" ]; then
	port="${PORT:-$((20000 + RANDOM % 20000))}"
else
	port="${PORT:-7878}"
fi
suffix="$$"
name="vv-first-export-$suffix"
data="vv-first-export-data-$suffix"
media="vv-first-export-media-$suffix"
app="http://localhost:$port"
budget=600
work="$(mktemp -d)"

server_pid=""
cleanup() {
	if [ -n "$binary" ]; then
		[ -z "$server_pid" ] || kill "$server_pid" >/dev/null 2>&1 || true
		sleep 1
	else
		docker rm -f "$name" >/dev/null 2>&1 || true
		docker volume rm "$data" "$media" >/dev/null 2>&1 || true
	fi
	rm -rf "$work"
}
trap cleanup EXIT

field() { sed -n "s/.*\"$1\":\"\([^\"]*\)\".*/\1/p" | head -n 1; }
now() { echo "${EPOCHREALTIME:-$(date +%s).000000}"; }
labels=()
seconds=()
last="$(now)"
begin="$last"
mark() {
	local t
	t="$(now)"
	labels+=("$1")
	seconds+=("$(awk -v a="$last" -v b="$t" 'BEGIN { printf "%.1f", b - a }')")
	last="$t"
}
fail() {
	echo "first-export-timing: $*" >&2
	exit 1
}

if [ -n "$binary" ]; then
	case "$port" in 7878 | 7979) fail "port $port belongs to a person's own Stuarty: set PORT" ;; esac
	[ -x "$binary" ] || fail "no such binary: $binary"
	ffdir="${STUARTY_TEST_FFMPEG_DIR:-}"
	if [ -n "$ffdir" ]; then
		[ ! -d "$ffdir/bin" ] || ffdir="$ffdir/bin"
		export PATH="$ffdir:$PATH"
	fi
	command -v ffmpeg >/dev/null 2>&1 || fail "no ffmpeg: set STUARTY_TEST_FFMPEG_DIR or put it in PATH"
	# A native program on Windows needs a Windows path; MSYS_NO_PATHCONV (above) stops the shell converting it.
	native() { if command -v cygpath >/dev/null 2>&1; then cygpath -w "$1"; else printf '%s' "$1"; fi; }
	mkdir -p "$work/home"
	export STUARTY_HOME
	STUARTY_HOME="$(native "$work/home")"
	unset STUARTY_URL STUARTY_KEY STUARTY_PROJECT APP_KEY APP_ENV
	STUARTY_ADDR="127.0.0.1:$port" "$binary" serve >"$work/serve.log" 2>&1 &
	server_pid=$!
	for _ in $(seq 1 120); do
		curl -fsS "$app/up" >/dev/null 2>&1 && break
		kill -0 "$server_pid" 2>/dev/null || { cat "$work/serve.log" >&2; fail "the server stopped"; }
		sleep 0.5
	done
	curl -fsS "$app/up" >/dev/null 2>&1 || fail "the server did not answer /up in 60 seconds"
	mark "start the server on a clean workspace until /up answers"
	ffmpeg -hide_banner -loglevel error -f lavfi -i testsrc2=size=1080x1920:rate=30:duration=3 \
		-f lavfi -i sine=frequency=440:duration=3 -c:v mpeg4 -c:a aac -shortest "$(native "$work/clip.mp4")"
	mark "make a clip (stands in for the user's file)"
	vv() { STUARTY_ADDR="127.0.0.1:$port" "$binary" "$@"; }
	clip="$(native "$work/clip.mp4")"
	has_export() { test -s "$work/home/exports/$1/$2.mp4"; }
else
if [ "$pull" -eq 1 ]; then
	docker pull "$image" >/dev/null
	mark "pull the image"
fi

app_key="$(openssl rand -base64 32)"
docker run -d --name "$name" -p "127.0.0.1:$port:$port" \
	-v "$data:/data" -v "$media:/media" \
	-e STUARTY_IMPORT_ROOTS=/media \
	-e APP_KEY="$app_key" \
	-e PORT="$port" -e APP_URL="$app" \
	-e STUARTY_ADMIN_EMAIL=admin@example.com \
	-e STUARTY_ADMIN_PASSWORD="choose-a-long-password" \
	"$image" >/dev/null
for _ in $(seq 1 120); do
	curl -fsS "$app/up" >/dev/null 2>&1 && break
	sleep 0.5
done
curl -fsS "$app/up" >/dev/null 2>&1 || fail "the server did not answer /up in 60 seconds"
mark "docker run until /up answers"

(cd "$work" && curl -fsS -c jar.txt -H "Content-Type: application/json" -H "Origin: $app" \
	-d '{"email": "admin@example.com", "password": "choose-a-long-password"}' "$app/auth/login" >/dev/null)
secret="$(cd "$work" && curl -fsS -b jar.txt -H "Content-Type: application/json" -H "Origin: $app" \
	-d '{"name": "first-export", "scopes": ["read", "edit", "media", "render"]}' "$app/api/v1/keys" |
	sed -n 's/.*"secret":"\([^"]*\)".*/\1/p')"
[ -n "$secret" ] || fail "no API key secret in the answer"
mark "sign in and create an API key"

# The clip stands in for the user's own file; FFmpeg of the image makes it.
docker exec -u 0 "$name" sh -c 'ffmpeg -hide_banner -loglevel error \
	-f lavfi -i testsrc2=size=1080x1920:rate=30:duration=3 -f lavfi -i sine=frequency=440:duration=3 \
	-c:v libopenh264 -pix_fmt yuv420p -c:a aac -shortest /media/clip.mp4 && chmod 644 /media/clip.mp4'
mark "make a clip in the import folder (stands in for the user's file)"

vv() { docker exec -i -e STUARTY_KEY="$secret" "$name" stuarty "$@"; }
clip=/media/clip.mp4
has_export() { docker exec "$name" sh -c "test -s /data/exports/$1/$2.mp4"; }
fi

# A project has a description and a SMART goal, and no timeline until one is added (SDD-005 section 5.12.2).
project="$(vv project:create --name "First export" --json \
	--description "The first export of a new user, timed from the install: one clip, cut and exported as a video." \
	--specific "Export the first video of a new user" --measurable "The time from install to the file" \
	--achievable "One clip made by FFmpeg is enough" --relevant "It is the first thing a person tries" \
	--time-bound "Done within the run of this script" --due 2999-12-31 | field project_id)"
[ -n "$project" ] || fail "no project id"
vv item:add --project "$project" --kind timeline --title "First export" >/dev/null
asset="$(vv media:import "$clip" --project "$project" --json | field asset_id)"
[ -n "$asset" ] || fail "no asset id"
mark "create a project and import the clip"

vv edit --project "$project" --ops - --json >/dev/null <<EOF
[{"op":"track.add","args":{"kind":"visual","name":"Video","id":"trk_f6g7h8j9k0"}},
 {"op":"clip.add","args":{"track_id":"trk_f6g7h8j9k0","asset_id":"$asset","start":"0s","duration":"2s","id":"itm_m1n2p3q4r5"}}]
EOF
mark "edit: one track, one clip"

result="$(vv export --project "$project" --json)"
job="$(printf "%s" "$result" | field job_id)"
[ -n "$job" ] || fail "no export job: $result"
shown="$(vv job:show "$job" --json)"
printf "%s" "$shown" | grep -q '"state":"succeeded"' || fail "the export did not succeed: $shown"
if printf "%s" "$shown" | grep -q '"ok":false'; then fail "an export check failed: $shown"; fi
printf "%s" "$shown" | grep -q '"name":"faststart","ok":true' || fail "the export carries no verification checks: $shown"
has_export "$project" "$job" || fail "the exported file is missing"
mark "export and verify the file"

total="$(awk -v a="$begin" -v b="$(now)" 'BEGIN { printf "%.1f", b - a }')"
if [ -n "$binary" ]; then echo "first-export-timing: binary $binary"; else echo "first-export-timing: image $image"; fi
for i in "${!labels[@]}"; do
	printf "  %-62s %7ss\n" "${labels[$i]}" "${seconds[$i]}"
done
printf "  %-62s %7ss\n" "total (budget ${budget}s, NFR-040)" "$total"
if awk -v t="$total" -v b="$budget" 'BEGIN { exit !(t > b) }'; then
	fail "the total is over $budget seconds"
fi
