#!/usr/bin/env bash
# Smoke test of a built image: bash scripts/image-smoke.sh <image>
# It starts the container the way the documentation says (published on the
# loopback of the host) and removes it at the end.
set -euo pipefail
img="${1:?usage: image-smoke.sh <image>}"
name="vukem-video-smoke-$$"

cleanup() { docker rm -f "$name" >/dev/null 2>&1 || true; }
trap cleanup EXIT

docker run --rm --entrypoint sh "$img" -c "id -u | grep -qx 10001"
docker run --rm --entrypoint sh "$img" -c "ffmpeg -version | head -n 1"
docker run --rm --entrypoint sh "$img" -c "ffmpeg -hide_banner -encoders | grep -q libopenh264"
docker run --rm --entrypoint sh "$img" -c "ffmpeg -v error -hide_banner -f lavfi -i testsrc=duration=1:size=320x240:rate=25 -c:v libopenh264 -pix_fmt yuv420p /tmp/t.mp4 && ffprobe -v error -show_entries stream=codec_name -of csv=p=0 /tmp/t.mp4 | grep -qx h264"
docker run --rm --entrypoint sh "$img" -c "if ffmpeg -version | grep -q -e --enable-gpl -e --enable-nonfree; then exit 1; fi"
docker run --rm --entrypoint sh "$img" -c "test -s /usr/share/doc/vukem-video/ffmpeg/LICENSE.txt && test -s /usr/share/doc/vukem-video/ffmpeg/configure.txt && test -s /usr/share/doc/vukem-video/ffmpeg/SOURCE.txt"
docker image inspect --format "{{json .Config.Healthcheck.Test}}" "$img" | grep -q 7878

# The server boots and answers /up. A free host port is used so the script
# does not clash with a server already running on 7878.
docker run -d --name "$name" -p 127.0.0.1::7878 "$img" >/dev/null
port="$(docker port "$name" 7878/tcp | head -n 1 | sed "s/.*://")"
for _ in $(seq 1 30); do
	if curl -fsS "http://127.0.0.1:${port}/up" >/dev/null 2>&1; then
		echo "image-smoke: /up answered on 127.0.0.1:${port}"
		exit 0
	fi
	sleep 1
done
docker logs "$name" >&2
echo "image-smoke: /up did not answer in 30 seconds" >&2
exit 1
