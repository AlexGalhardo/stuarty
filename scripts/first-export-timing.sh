#!/usr/bin/env bash
# Times the path from docker run to the first verified export (ALPHA exit
# criterion 7, NFR-040): bash scripts/first-export-timing.sh [IMAGE] [--no-pull]
# It follows docs/self-hosting/docker ("Your first export from the container"):
# a fresh container and a fresh volume, an API key made with the first
# administrator, the CLI inside the container, one clip, one export whose checks
# must all pass. It removes the container and the volumes it creates, never the
# image. It exits 1 when a step fails or the total passes 600 seconds (NFR-040).
# Needs docker, curl, openssl and a port 7878 free on 127.0.0.1 (PORT overrides).
set -euo pipefail

image="ghcr.io/alexgalhardo/vukem-video:latest"
pull=1
for arg in "$@"; do
	case "$arg" in
	--no-pull) pull=0 ;;
	*) image="$arg" ;;
	esac
done

# Git Bash on Windows rewrites /media into a host path in docker arguments.
export MSYS_NO_PATHCONV=1
port="${PORT:-7878}"
suffix="$$"
name="vv-first-export-$suffix"
data="vv-first-export-data-$suffix"
media="vv-first-export-media-$suffix"
app="http://localhost:$port"
budget=600
work="$(mktemp -d)"

cleanup() {
	docker rm -f "$name" >/dev/null 2>&1 || true
	docker volume rm "$data" "$media" >/dev/null 2>&1 || true
	rm -rf "$work"
}
trap cleanup EXIT

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

if [ "$pull" -eq 1 ]; then
	docker pull "$image" >/dev/null
	mark "pull the image"
fi

app_key="$(openssl rand -base64 32)"
docker run -d --name "$name" -p "127.0.0.1:$port:7878" \
	-v "$data:/data" -v "$media:/media" \
	-e VUKEM_VIDEO_IMPORT_ROOTS=/media \
	-e APP_KEY="$app_key" \
	-e VUKEM_VIDEO_ADMIN_EMAIL=admin@example.com \
	-e VUKEM_VIDEO_ADMIN_PASSWORD="choose-a-long-password" \
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

vv() { docker exec -i -e VUKEM_VIDEO_KEY="$secret" "$name" vukem-video "$@"; }
field() { sed -n "s/.*\"$1\":\"\([^\"]*\)\".*/\1/p" | head -n 1; }

project="$(vv project:create --name "First export" --json | field project_id)"
[ -n "$project" ] || fail "no project id"
asset="$(vv media:import /media/clip.mp4 --project "$project" --json | field asset_id)"
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
docker exec "$name" sh -c "test -s /data/exports/$project/$job.mp4" || fail "the exported file is missing"
mark "export and verify the file"

total="$(awk -v a="$begin" -v b="$(now)" 'BEGIN { printf "%.1f", b - a }')"
echo "first-export-timing: image $image"
for i in "${!labels[@]}"; do
	printf "  %-62s %7ss\n" "${labels[$i]}" "${seconds[$i]}"
done
printf "  %-62s %7ss\n" "total (budget ${budget}s, NFR-040)" "$total"
if awk -v t="$total" -v b="$budget" 'BEGIN { exit !(t > b) }'; then
	fail "the total is over $budget seconds"
fi
