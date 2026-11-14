#!/usr/bin/env bash
# Build whisper-cli from the pinned whisper.cpp source and pack it:
#   bash scripts/whisper-build.sh <os> <arch> <out-dir>
# <os> is linux, darwin or windows; <arch> is amd64 or arm64. It prints the path of
# the archive: whisper-cli-<tag>-<os>-<arch>.tar.gz (zip on windows) in <out-dir>,
# and writes its SHA-256 next to it. This script is the one place that holds the
# build options (SDD-011 section 10.2): CPU only, no GPU or BLAS backend, no OpenMP
# (one binary with no runtime library), no SDL, no curl, no FFmpeg, portable code
# (no -march=native), and only the MIT and permissive code of whisper.cpp and ggml.
# Needs cmake, a C++ compiler, curl and tar (Visual Studio on windows).
set -euo pipefail
os="${1:?usage: whisper-build.sh <os> <arch> <out-dir>}"
arch="${2:?usage: whisper-build.sh <os> <arch> <out-dir>}"
out="${3:?usage: whisper-build.sh <os> <arch> <out-dir>}"

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=../.config/whisper.env
. "$root/.config/whisper.env"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
mkdir -p "$out"
out="$(cd "$out" && pwd)"

curl -fsSL -o "$work/src.tar.gz" "$WHISPER_SOURCE_URL"
echo "${WHISPER_SOURCE_SHA256}  $work/src.tar.gz" | sha256sum -c - >&2
tar -xzf "$work/src.tar.gz" -C "$work"
src="$work/whisper.cpp-${WHISPER_COMMIT}"

options=(
	-DCMAKE_BUILD_TYPE=Release
	-DBUILD_SHARED_LIBS=OFF
	-DGGML_NATIVE=OFF
	-DGGML_OPENMP=OFF
	-DGGML_METAL=OFF
	-DGGML_BLAS=OFF
	-DGGML_ACCELERATE=OFF
	-DGGML_CUDA=OFF
	-DGGML_VULKAN=OFF
	-DGGML_OPENCL=OFF
	-DGGML_SYCL=OFF
	-DGGML_HIP=OFF
	-DGGML_RPC=OFF
	-DWHISPER_COREML=OFF
	-DWHISPER_OPENVINO=OFF
	-DWHISPER_SDL2=OFF
	-DWHISPER_CURL=OFF
	-DWHISPER_BUILD_TESTS=OFF
	-DWHISPER_BUILD_SERVER=OFF
	-DWHISPER_BUILD_EXAMPLES=ON
)

exe=whisper-cli
# examples/common.cpp of v1.9.2 uses std::partial_sort without including
# <algorithm>; a libc++ that no longer includes it transitively (it did not for
# llvm-mingw 19, which is how this was found) fails to compile. Harmless where it
# is not needed. MSVC gets it from its own headers.
case "$os" in
	darwin|linux) options+=("-DCMAKE_CXX_FLAGS=-include algorithm") ;;
esac

case "$os" in
	darwin)
		case "$arch" in
			arm64) options+=(-DCMAKE_OSX_ARCHITECTURES=arm64) ;;
			amd64) options+=(-DCMAKE_OSX_ARCHITECTURES=x86_64) ;;
			*) echo "unsupported architecture: $arch" >&2; exit 1 ;;
		esac
		options+=(-DCMAKE_OSX_DEPLOYMENT_TARGET=13.0)
		;;
	windows)
		exe=whisper-cli.exe
		# A static runtime, so the helper needs no Visual C++ redistributable.
		options+=(-DCMAKE_MSVC_RUNTIME_LIBRARY=MultiThreaded)
		case "$arch" in
			arm64) options+=(-A ARM64) ;;
			amd64) options+=(-A x64) ;;
			*) echo "unsupported architecture: $arch" >&2; exit 1 ;;
		esac
		;;
	linux) ;;
	*) echo "unsupported os: $os" >&2; exit 1 ;;
esac

cmake -S "$src" -B "$work/build" "${options[@]}" >&2
cmake --build "$work/build" --config Release --target whisper-cli --parallel >&2

bin="$(find "$work/build" -type f -name "$exe" | head -n 1)"
test -n "$bin" || { echo "whisper-build: $exe was not built" >&2; exit 1; }

pack="$work/pack/whisper-cli-${WHISPER_TAG}-${os}-${arch}"
mkdir -p "$pack"
cp "$bin" "$pack/$exe"
cp "$src/LICENSE" "$pack/LICENSE"
printf "whisper.cpp %s (%s)\nBuilt by scripts/whisper-build.sh for %s-%s: CPU only.\nSource: %s\n" \
	"$WHISPER_TAG" "$WHISPER_COMMIT" "$os" "$arch" "$WHISPER_SOURCE_URL" > "$pack/SOURCE.txt"

base="whisper-cli-${WHISPER_TAG}-${os}-${arch}"
if [ "$os" = windows ]; then
	archive="$out/$base.zip"
	(cd "$work/pack" && powershell -NoProfile -Command "Compress-Archive -Path \"$base\" -DestinationPath \"$(cygpath -w "$archive")\" -Force")
else
	archive="$out/$base.tar.gz"
	tar -czf "$archive" -C "$work/pack" "$base"
fi

(cd "$out" && sha256sum "$(basename "$archive")" > "$(basename "$archive").sha256")
echo "$archive"
