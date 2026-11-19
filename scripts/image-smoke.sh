#!/usr/bin/env bash
# Smoke test of a built image: bash scripts/image-smoke.sh <image> [<version>]
# With a version (no leading v), `version` must print exactly stuarty <version>.
# It starts the container the way the documentation says (published on the
# loopback of the host) and removes it and its volume at the end.
set -euo pipefail
img="${1:?usage: image-smoke.sh <image> [<version>]}"
want="${2:-}"
name="stuarty-smoke-$$"
vol="stuarty-smoke-$$"

cleanup() {
	docker rm -f "$name" >/dev/null 2>&1 || true
	docker volume rm -f "$vol" >/dev/null 2>&1 || true
}
trap cleanup EXIT

got="$(docker run --rm "$img" version)"
if [ -n "$want" ] && [ "$got" != "stuarty $want" ]; then
	echo "image-smoke: version printed [$got], want [stuarty $want]" >&2
	exit 1
fi
case "$got" in "stuarty "*) ;; *) echo "image-smoke: version printed [$got]" >&2; exit 1 ;; esac

docker run --rm --entrypoint sh "$img" -c "id -u | grep -qx 10001"
docker run --rm --entrypoint sh "$img" -c "ffmpeg -version | head -n 1"
# The BtbN build (through ALPHA) has OpenH264 compiled in and encodes with it at once. The own
# build (D-017) has none: libopenh264 is absent until the explicit install step, and the
# image must not carry the Cisco binary. SMOKE_OPENH264_STEP=1 runs that step (it needs the
# network and downloads from Cisco) and then encodes with the downloaded library.
if docker run --rm --entrypoint sh "$img" -c "ffmpeg -hide_banner -encoders | grep -q libopenh264"; then
	docker run --rm --entrypoint sh "$img" -c "ffmpeg -v error -hide_banner -f lavfi -i testsrc=duration=1:size=320x240:rate=25 -c:v libopenh264 -pix_fmt yuv420p /tmp/t.mp4 && ffprobe -v error -show_entries stream=codec_name -of csv=p=0 /tmp/t.mp4 | grep -qx h264"
else
	echo "image-smoke: the build has no libopenh264 encoder (own build)"
	docker run --rm --entrypoint sh "$img" -c "! ffmpeg -hide_banner -version | grep -q -e --enable-libopenh264"
	docker run --rm --entrypoint sh "$img" -c "! find / -xdev -name 'libopenh264*' 2>/dev/null | grep -q ."
	docker run --rm --entrypoint sh "$img" -c "ffmpeg -hide_banner -encoders | grep -q libvpx-vp9"
	if [ "${SMOKE_OPENH264_STEP:-}" = 1 ]; then
		docker run --name "$name-step" -v "$vol:/data" "$img" tools:install openh264 --yes
		docker run --rm -v "$vol:/data" --entrypoint sh "$img" -c 'lib="$(find /data -name "libopenh264*" | head -n 1)"; test -n "$lib" && LD_LIBRARY_PATH="$(dirname "$lib")" ffmpeg -v error -hide_banner -f lavfi -i testsrc=duration=1:size=320x240:rate=25 -c:v libopenh264 -pix_fmt yuv420p /tmp/t.mp4 && ffprobe -v error -show_entries stream=codec_name -of csv=p=0 /tmp/t.mp4 | grep -qx h264'
		docker rm -f "$name-step" >/dev/null 2>&1 || true
	fi
fi
docker run --rm --entrypoint sh "$img" -c "if ffmpeg -version | grep -q -e --enable-gpl -e --enable-nonfree; then exit 1; fi"
docker run --rm --entrypoint sh "$img" -c "test -s /usr/share/doc/stuarty/ffmpeg/LICENSE.txt && test -s /usr/share/doc/stuarty/ffmpeg/configure.txt && test -s /usr/share/doc/stuarty/ffmpeg/SOURCE.txt"
docker image inspect --format "{{json .Config.Healthcheck.Test}}" "$img" | grep -q 7878
docker image inspect --format "{{json .Config.Env}}" "$img" | grep -q "STUARTY_HOME=/data"
docker image inspect --format "{{json .Config.Volumes}}" "$img" | grep -q "\"/data\""
docker run --rm --entrypoint sh "$img" -c "test -s /usr/share/doc/stuarty/LICENSE.md && test -s /usr/share/doc/stuarty/THIRD_PARTY_NOTICES.md && test -d /usr/share/doc/stuarty/licenses && test -s /usr/share/doc/stuarty/base-image-packages.txt"
docker run --rm --entrypoint prlimit "$img" --version >/dev/null
# The whisper.cpp helper installed by tools:install needs the OpenMP runtime (VV-012).
docker run --rm --entrypoint sh "$img" -c "ldconfig -p | grep -q libgomp.so.1"

# RF-206: with no network at all, the product still runs the part of the
# Prototype scenario that needs no media, through the CLI in the container.
net=(--rm --network none -v "$vol:/data")
created="$(docker run "${net[@]}" "$img" project:create --name smoke --json)"
id="$(printf "%s" "$created" | grep -o "\"project_id\":\"[^\"]*\"" | head -n 1 | cut -d\" -f4)"
test -n "$id"
docker run "${net[@]}" "$img" edit --project "$id" --json track.add kind=visual name=Video | grep -q "\"applied\":true"
docker run "${net[@]}" "$img" lint --project "$id" --json | grep -q "\"project_id\":\"$id\""
docker run "${net[@]}" "$img" history --project "$id" --json | grep -q "\"entries\""

# The server boots in network mode (SDD-007 4.3 item 4) and answers /up, asked
# from inside the container as the HEALTHCHECK does. With no user yet, the API
# answers 401 to everyone.
key="$(head -c 32 /dev/urandom | base64)"
docker run -d --name "$name" -e APP_KEY="$key" "$img" >/dev/null
for _ in $(seq 1 30); do
	if docker exec "$name" curl -fsS http://127.0.0.1:7878/up >/dev/null 2>&1; then
		echo "image-smoke: /up answered inside the container"
		code="$(docker exec "$name" curl -s -o /dev/null -w "%{http_code}" -H "Host: localhost:7878" http://127.0.0.1:7878/api/v1/projects)"
		if [ "$code" != "401" ]; then
			echo "image-smoke: the API answered $code with no user; want 401" >&2
			exit 1
		fi
		exit 0
	fi
	sleep 1
done
docker logs "$name" >&2
echo "image-smoke: /up did not answer in 30 seconds" >&2
exit 1
