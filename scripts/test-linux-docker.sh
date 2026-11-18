#!/usr/bin/env bash
# Runs the Go tests on Linux inside a pinned official Go image, from any host with Docker (Docker
# Desktop on Windows or macOS included). Used to see what fails only on Linux, such as the
# Pdeathsig crash test of desktop/shell (VV-006), without a Linux machine.
#
#   bash scripts/test-linux-docker.sh                      the default packages, with the pinned FFmpeg
#   bash scripts/test-linux-docker.sh ./desktop/shell/...  only these packages
#   bash scripts/test-linux-docker.sh --no-ffmpeg          skip the FFmpeg download (ffmpeg-bound tests skip)
#   bash scripts/test-linux-docker.sh --prune-cache        remove the named cache volume and exit
#
# The repository is mounted read-only at /src, so nothing in the working tree changes and a test
# that writes into the source tree fails here (it should use t.TempDir). Modules, the build cache
# and the FFmpeg download live in the named volume vukem-video-linux-cache, which makes a second run
# fast. The tests run as an unprivileged user, because a test of file permissions means nothing as root.
#
# Environment:
#   GO_IMAGE     the Go image; the exact tag of go.mod's toolchain, pinned by digest (never latest)
#   TEST_FLAGS   flags of go test; default -short -count=1 -timeout 30m
#   CACHE_VOLUME the cache volume; default vukem-video-linux-cache
set -euo pipefail

log() { printf '%s\n' "$*" >&2; }
die() { log "test-linux-docker: error: $*"; exit 1; }

GO_IMAGE="${GO_IMAGE:-golang:1.27.4-bookworm@sha256:4fa7c968ba93f1635670522dbb01a2c78150443e1385746924566079f36570ea}"
TEST_FLAGS="${TEST_FLAGS:--short -count=1 -timeout 30m}"
CACHE_VOLUME="${CACHE_VOLUME:-vukem-video-linux-cache}"
default_packages=(./core/... ./service/... ./web/app/... ./desktop/shell/... ./engine/...)

use_ffmpeg=1
packages=()
for arg in "$@"; do
	case "$arg" in
	--no-ffmpeg) use_ffmpeg=0 ;;
	--prune-cache)
		docker volume rm "$CACHE_VOLUME"
		exit 0
		;;
	-h | --help)
		sed -n '2,19p' "$0"
		exit 0
		;;
	-*) die "unknown option: $arg" ;;
	*) packages+=("$arg") ;;
	esac
done
[ "${#packages[@]}" -gt 0 ] || packages=("${default_packages[@]}")

command -v docker >/dev/null 2>&1 || die "docker is not installed"
docker info >/dev/null 2>&1 || die "the docker engine does not answer (start Docker Desktop)"
[ "$(docker info --format '{{.OSType}}')" = linux ] || die "docker must use a Linux engine"

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$(cd "$script_dir/.." && pwd)"
# Git Bash: the mount needs the Windows form of the path, and no conversion of the in-container ones.
if root_win="$(cd "$root" && pwd -W 2>/dev/null)"; then root="$root_win"; fi
export MSYS_NO_PATHCONV=1 MSYS2_ARG_CONV_EXCL='*'

want_go="$(sed -n 's/^go \([0-9][0-9.]*\)$/\1/p' "$script_dir/../go.mod" | head -n 1)"
case "$GO_IMAGE" in
*"$want_go"*) ;;
*) die "GO_IMAGE ($GO_IMAGE) does not name the toolchain of go.mod ($want_go)" ;;
esac

uid=1000
gid=1000

# The volume is created owned by root; hand it to the test user once.
docker run --rm --user 0 -v "$CACHE_VOLUME:/cache" "$GO_IMAGE" \
	sh -c "mkdir -p /cache/go /cache/gobuild /cache/ffmpeg /cache/home && chown -R $uid:$gid /cache"

# The pinned FFmpeg is installed once per pin, as root because the image has no xz (apt-get adds it),
# into the cache volume; the stamp holds the bin folder and names the release tag of the pin.
ffmpeg_prep='
set -euo pipefail
. .config/ffmpeg.env
stamp=/cache/ffmpeg/bin-dir
if [ -s "$stamp" ] && grep -q "$FFMPEG_RELEASE_TAG" "$stamp" && [ -x "$(cat "$stamp")/ffmpeg" ]; then
	echo "test-linux-docker: reusing $(cat "$stamp")" >&2
	exit 0
fi
apt-get update -qq >/dev/null
DEBIAN_FRONTEND=noninteractive apt-get install -y -qq --no-install-recommends xz-utils >/dev/null
FFMPEG_INSTALL_DIR=/cache/ffmpeg bash scripts/install-ffmpeg.sh | tail -n 1 >"$stamp"
chown -R 1000:1000 /cache/ffmpeg
'

inner='
set -euo pipefail
if [ "$USE_FFMPEG" = 1 ]; then
	bin_dir="$(cat /cache/ffmpeg/bin-dir)"
	export VUKEM_VIDEO_TEST_FFMPEG_DIR="$bin_dir"
	"$bin_dir/ffmpeg" -version | head -n 1
fi
go version
set +e
go test $TEST_FLAGS "$@"
status=$?
echo "go test exit status: $status"
exit $status
'

if [ "$use_ffmpeg" = 1 ]; then
	docker run --rm --user 0 -v "$root:/src:ro" -v "$CACHE_VOLUME:/cache" -w /src "$GO_IMAGE" bash -c "$ffmpeg_prep"
fi

log "test-linux-docker: $GO_IMAGE, packages: ${packages[*]}"

exec docker run --rm --user "$uid:$gid" \
	-v "$root:/src:ro" -v "$CACHE_VOLUME:/cache" -w /src \
	-e HOME=/cache/home -e GOPATH=/cache/go -e GOCACHE=/cache/gobuild -e GOFLAGS=-buildvcs=false \
	-e GOTOOLCHAIN=local -e CGO_ENABLED=0 -e GOWORK=off -e VUKEM_VIDEO_MIN_FREE_BYTES=1 \
	-e USE_FFMPEG="$use_ffmpeg" -e TEST_FLAGS="$TEST_FLAGS" \
	"$GO_IMAGE" bash -c "$inner" bash "${packages[@]}"
