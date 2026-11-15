#!/usr/bin/env bash
# Smoke test of a built image: bash scripts/image-smoke.sh <image> [<version>]
# With a version (no leading v), `version` must print exactly vukem-video v<version>.
# It starts the container the way the documentation says (published on the
# loopback of the host) and removes it and its volume at the end.
set -euo pipefail
img="${1:?usage: image-smoke.sh <image> [<version>]}"
want="${2:-}"
name="vukem-video-smoke-$$"
vol="vukem-video-smoke-$$"

cleanup() {
	docker rm -f "$name" >/dev/null 2>&1 || true
	docker volume rm -f "$vol" >/dev/null 2>&1 || true
}
trap cleanup EXIT

got="$(docker run --rm "$img" version)"
if [ -n "$want" ] && [ "$got" != "vukem-video v$want" ]; then
	echo "image-smoke: version printed [$got], want [vukem-video v$want]" >&2
	exit 1
fi
case "$got" in "vukem-video "*) ;; *) echo "image-smoke: version printed [$got]" >&2; exit 1 ;; esac

docker run --rm --entrypoint sh "$img" -c "id -u | grep -qx 10001"
docker run --rm --entrypoint sh "$img" -c "ffmpeg -version | head -n 1"
docker run --rm --entrypoint sh "$img" -c "ffmpeg -hide_banner -encoders | grep -q libopenh264"
docker run --rm --entrypoint sh "$img" -c "ffmpeg -v error -hide_banner -f lavfi -i testsrc=duration=1:size=320x240:rate=25 -c:v libopenh264 -pix_fmt yuv420p /tmp/t.mp4 && ffprobe -v error -show_entries stream=codec_name -of csv=p=0 /tmp/t.mp4 | grep -qx h264"
docker run --rm --entrypoint sh "$img" -c "if ffmpeg -version | grep -q -e --enable-gpl -e --enable-nonfree; then exit 1; fi"
docker run --rm --entrypoint sh "$img" -c "test -s /usr/share/doc/vukem-video/ffmpeg/LICENSE.txt && test -s /usr/share/doc/vukem-video/ffmpeg/configure.txt && test -s /usr/share/doc/vukem-video/ffmpeg/SOURCE.txt"
docker image inspect --format "{{json .Config.Healthcheck.Test}}" "$img" | grep -q 7878
docker image inspect --format "{{json .Config.Env}}" "$img" | grep -q "VUKEM_VIDEO_HOME=/data"
docker image inspect --format "{{json .Config.Volumes}}" "$img" | grep -q "\"/data\""
docker run --rm --entrypoint sh "$img" -c "test -s /usr/share/doc/vukem-video/LICENSE.md && test -s /usr/share/doc/vukem-video/THIRD_PARTY_NOTICES.md && test -d /usr/share/doc/vukem-video/licenses && test -s /usr/share/doc/vukem-video/base-image-packages.txt"
docker run --rm --entrypoint prlimit "$img" --version >/dev/null

# RF-206: with no network at all, the product still runs the part of the
# Prototype scenario that needs no media, through the CLI in the container.
net=(--rm --network none -v "$vol:/data")
created="$(docker run "${net[@]}" "$img" project:create --name smoke --json)"
id="$(printf "%s" "$created" | grep -o "\"project_id\":\"[^\"]*\"" | head -n 1 | cut -d\" -f4)"
test -n "$id"
docker run "${net[@]}" "$img" edit --project "$id" --json track.add kind=visual name=Video | grep -q "\"applied\":true"
docker run "${net[@]}" "$img" lint --project "$id" --json | grep -q "\"project_id\":\"$id\""
docker run "${net[@]}" "$img" history --project "$id" --json | grep -q "\"entries\""

# The server boots and answers /up. Until network mode (P2-SRV-014, SDD-007
# 4.3 item 4) the image serves in local mode, which refuses every peer that is
# not loopback, and a published port reaches the server from the bridge
# address; so /up is asked from inside the container, as the HEALTHCHECK does.
docker run -d --name "$name" "$img" >/dev/null
for _ in $(seq 1 30); do
	if docker exec "$name" curl -fsS http://127.0.0.1:7878/up >/dev/null 2>&1; then
		echo "image-smoke: /up answered inside the container"
		exit 0
	fi
	sleep 1
done
docker logs "$name" >&2
echo "image-smoke: /up did not answer in 30 seconds" >&2
exit 1
