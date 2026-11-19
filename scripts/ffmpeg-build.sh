#!/usr/bin/env bash
# Builds the own LGPL FFmpeg (D-017, ADR-0011) and packs it:
#   bash scripts/ffmpeg-build.sh <os> <arch> <out-dir>
#   bash scripts/ffmpeg-build.sh --print-flags <os>
# <os> is linux, darwin or windows; <arch> is amd64 or arm64 (windows: amd64 only). The
# archive is ffmpeg-<version>-<os>-<arch>.tar.xz (zip on windows) in <out-dir>, next to
# its SHA-256 file; its path is the last line of stdout, everything else goes to stderr.
#
# Inputs: .config/ffmpeg-build/sources.env (pinned tarballs with SHA-256) and
# .config/ffmpeg-build/configure-flags.txt (the flags, checked by internal/tools/ffmpegconf).
# Windows is cross compiled from Linux with mingw-w64 (x86_64-w64-mingw32-gcc, posix threads).
# Needs: curl, tar, make, cmake (linux and windows, for libvpl), autoconf, automake, libtool, a C and C++ compiler, nasm (x86), xz (zip on windows), GNU tar (gtar on macOS).
#
# Reproducibility: SOURCE_DATE_EPOCH is the commit time of HEAD (or 0 outside git), paths
# are remapped, no build time is embedded, and the archive has sorted entries, fixed owners and
# a fixed mtime. The result still depends on the compiler of the runner; the workflow builds twice
# and compares the digests (P3-REL-001).
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
build_dir="$root/.config/ffmpeg-build"

log() { printf '%s\n' "$*" >&2; }
die() { log "ffmpeg-build: error: $*"; exit 1; }

# flags_of <section>: the flags of one section of configure-flags.txt, one per line.
flags_of() {
	awk -v want="[$1]" '
		/^#/ || /^[[:space:]]*$/ { next }
		/^\[/ { on = ($0 == want); next }
		on { print }
	' "$build_dir/configure-flags.txt"
}

if [ "${1:-}" = "--print-flags" ]; then
	os="${2:?usage: ffmpeg-build.sh --print-flags <os>}"
	{ flags_of common; flags_of "$os"; } | tr '\n' ' '
	printf '\n'
	exit 0
fi

os="${1:?usage: ffmpeg-build.sh <os> <arch> <out-dir>}"
arch="${2:?usage: ffmpeg-build.sh <os> <arch> <out-dir>}"
out="${3:?usage: ffmpeg-build.sh <os> <arch> <out-dir>}"

# shellcheck source=../.config/ffmpeg-build/sources.env
. "$build_dir/sources.env"

sha256_of() {
	if command -v sha256sum >/dev/null 2>&1; then sha256sum <"$1" | cut -d ' ' -f 1
	else shasum -a 256 <"$1" | cut -d ' ' -f 1; fi
}

jobs="$( (nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 2) | head -n 1)"

tar_cmd=tar
if [ "$os" = darwin ]; then
	command -v gtar >/dev/null 2>&1 || die "GNU tar is required on macOS (brew install gnu-tar)"
	tar_cmd=gtar
fi

host_cc=cc
cross_prefix=""
target_os=""
configure_target=()
exe=""
case "$os" in
linux)
	case "$arch" in
	amd64 | arm64) ;;
	*) die "unsupported architecture: $arch" ;;
	esac
	target_os=linux
	;;
darwin)
	case "$arch" in
	arm64) darwin_arch=arm64 ;;
	amd64) darwin_arch=x86_64 ;;
	*) die "unsupported architecture: $arch" ;;
	esac
	export MACOSX_DEPLOYMENT_TARGET=13.0
	target_os=darwin
	# The only bytes that differed between two macOS builds of the same commit (ffmpeg-9.0.2-2) were the 16 bytes of the
	# LC_UUID load command (and, on arm64, the ad hoc code signature that hashes them). The linker derives that UUID from
	# its inputs, not from the output, and it moved once the static LAME, libogg, libvorbis and libwebp archives joined
	# the link. -no_uuid is not an option: dyld refuses an arm64 executable without LC_UUID, so every configure test
	# program aborts. The load command stays; after the final link internal/tools/machouuid rewrites the UUID from the
	# SHA-256 of the binary and the binary is signed again (see below).
	configure_target=("--extra-cflags=-arch $darwin_arch -mmacosx-version-min=13.0" "--extra-ldflags=-arch $darwin_arch")
	;;
windows)
	[ "$arch" = amd64 ] || die "unsupported architecture: $arch"
	command -v x86_64-w64-mingw32-gcc >/dev/null 2>&1 || die "mingw-w64 (x86_64-w64-mingw32-gcc) is required for windows"
	[ "$(uname -s)" = Linux ] || die "windows is cross compiled from Linux"
	cross_prefix=x86_64-w64-mingw32-
	target_os=mingw32
	exe=.exe
	;;
*) die "unsupported os: $os" ;;
esac

# A fixed path: FFmpeg embeds its configure line (with --prefix) in the binaries, so a mktemp name would differ per build.
work="${VUKEM_FFBUILD_DIR:-${RUNNER_TEMP:-${TMPDIR:-/tmp}}/vukem-ffbuild}"
case "$work" in ""|/) die "unsafe build directory" ;; esac
rm -rf "$work"
mkdir -p "$work"
trap 'rm -rf "$work"' EXIT
mkdir -p "$out"
out="$(cd "$out" && pwd)"

if [ -z "${SOURCE_DATE_EPOCH:-}" ]; then
	SOURCE_DATE_EPOCH="$(git -C "$root" log -1 --format=%ct 2>/dev/null || echo 0)"
fi
export SOURCE_DATE_EPOCH TZ=UTC LC_ALL=C

fetch() { # fetch <url> <sha256> <name>: downloads, verifies and unpacks into $work/src/<name>
	local url="$1" sum="$2" name="$3" file="$work/$3.tarball"
	curl -fsSL --retry 3 -o "$file" "$url" || die "download failed: $url"
	[ "$(sha256_of "$file")" = "$sum" ] || die "SHA-256 mismatch for $url"
	mkdir -p "$work/src/$name"
	tar -xf "$file" -C "$work/src/$name" --strip-components=1
}

fetch_file() { # fetch_file <url> <sha256> <dest>: downloads one file and verifies it
	local url="$1" sum="$2" dest="$3"
	curl -fsSL --retry 3 -o "$dest" "$url" || die "download failed: $url"
	[ "$(sha256_of "$dest")" = "$sum" ] || die "SHA-256 mismatch for $url"
}

fetch "$FFMPEG_SRC_URL" "$FFMPEG_SRC_SHA256" ffmpeg
fetch "$LIBVPX_SRC_URL" "$LIBVPX_SRC_SHA256" libvpx
fetch "$OPUS_SRC_URL" "$OPUS_SRC_SHA256" opus
fetch "$ZLIB_SRC_URL" "$ZLIB_SRC_SHA256" zlib
fetch "$LAME_SRC_URL" "$LAME_SRC_SHA256" lame
fetch "$OGG_SRC_URL" "$OGG_SRC_SHA256" ogg
fetch "$VORBIS_SRC_URL" "$VORBIS_SRC_SHA256" vorbis
fetch "$LIBWEBP_SRC_URL" "$LIBWEBP_SRC_SHA256" libwebp
fetch "$ZIMG_SRC_URL" "$ZIMG_SRC_SHA256" zimg
case "$os" in
linux | windows)
	fetch "$NVCODEC_SRC_URL" "$NVCODEC_SRC_SHA256" nvcodec
	# Quick Sync and AMF are x86 GPU technologies: the linux arm64 build keeps NVENC only.
	if [ "$arch" = amd64 ]; then
		fetch "$LIBVPL_SRC_URL" "$LIBVPL_SRC_SHA256" libvpl
		fetch "$AMF_SRC_URL" "$AMF_SRC_SHA256" amf
	fi
	;;
esac

patch_file="$build_dir/patches/$FFMPEG_PATCH_FILE"
[ "$(sha256_of "$patch_file")" = "$FFMPEG_PATCH_SHA256" ] || die "SHA-256 mismatch for $patch_file"
patch -p1 -d "$work/src/ffmpeg" <"$patch_file" >&2 || die "patch $patch_file does not apply"

prefix="$work/prefix"
mkdir -p "$prefix/include/wels"
# Only the four Cisco API headers, each pinned by SHA-256 at the commit of the v2.6.0 tag (the auto-generated
# tag archive is not byte-stable): libopenh264 is loaded at run time by the patched encoder, never linked.
fetch_file "$OPENH264_RAW_URL/codec_api.h" "$OPENH264_CODEC_API_SHA256" "$prefix/include/wels/codec_api.h"
fetch_file "$OPENH264_RAW_URL/codec_app_def.h" "$OPENH264_CODEC_APP_DEF_SHA256" "$prefix/include/wels/codec_app_def.h"
fetch_file "$OPENH264_RAW_URL/codec_def.h" "$OPENH264_CODEC_DEF_SHA256" "$prefix/include/wels/codec_def.h"
fetch_file "$OPENH264_RAW_URL/codec_ver.h" "$OPENH264_CODEC_VER_SHA256" "$prefix/include/wels/codec_ver.h"
remap="-ffile-prefix-map=$work=. -ffile-prefix-map=$root=."
# Only our prefix: every dependency installs its .pc under $prefix/lib/pkgconfig (--libdir above), nothing from the system.
unset PKG_CONFIG_PATH
export PKG_CONFIG_LIBDIR="$prefix/lib/pkgconfig"
export CFLAGS="-O2 $remap"

# --- dependencies, static, into $prefix ---
log "ffmpeg-build: zlib"
(
	cd "$work/src/zlib"
	if [ -n "$cross_prefix" ]; then
		CC="${cross_prefix}gcc" AR="${cross_prefix}ar" RANLIB="${cross_prefix}ranlib" ./configure --static --prefix="$prefix" --libdir="$prefix/lib" >&2
	else
		./configure --static --prefix="$prefix" --libdir="$prefix/lib" >&2
	fi
	make -j"$jobs" >&2 && make install >&2
)

log "ffmpeg-build: libvpx"
(
	cd "$work/src/libvpx"
	vpx_target=""
	case "$os-$arch" in
	linux-amd64) vpx_target=x86_64-linux-gcc ;;
	linux-arm64) vpx_target=arm64-linux-gcc ;;
	darwin-arm64) vpx_target=arm64-darwin20-gcc ;;
	darwin-amd64) vpx_target=x86_64-darwin20-gcc ;;
	windows-amd64) vpx_target=x86_64-win64-gcc ;;
	esac
	if [ -n "$cross_prefix" ]; then export CROSS="$cross_prefix"; fi
	./configure --target="$vpx_target" --prefix="$prefix" --libdir="$prefix/lib" --disable-shared --enable-static --enable-pic \
		--disable-examples --disable-tools --disable-docs --disable-unit-tests --enable-vp9-highbitdepth \
		--disable-install-bins >&2
	make -j"$jobs" >&2 && make install >&2
)

log "ffmpeg-build: opus"
(
	cd "$work/src/opus"
	host_arg=()
	[ -n "$cross_prefix" ] && host_arg=(--host=x86_64-w64-mingw32)
	./configure --prefix="$prefix" --libdir="$prefix/lib" --disable-shared --enable-static --disable-doc --disable-extra-programs \
		${host_arg[@]+"${host_arg[@]}"} >&2
	make -j"$jobs" >&2 && make install >&2
)

log "ffmpeg-build: lame"
(
	cd "$work/src/lame"
	host_arg=()
	[ -n "$cross_prefix" ] && host_arg=(--host=x86_64-w64-mingw32)
	# Only libmp3lame: the lame executable and the mpg123 decoder are not needed.
	./configure --prefix="$prefix" --libdir="$prefix/lib" --disable-shared --enable-static --disable-frontend --disable-decoder \
		--disable-gtktest ${host_arg[@]+"${host_arg[@]}"} >&2
	make -j"$jobs" -C libmp3lame >&2 && make -C libmp3lame install >&2 && make -C include install >&2
)

log "ffmpeg-build: ogg"
(
	cd "$work/src/ogg"
	host_arg=()
	[ -n "$cross_prefix" ] && host_arg=(--host=x86_64-w64-mingw32)
	./configure --prefix="$prefix" --libdir="$prefix/lib" --disable-shared --enable-static ${host_arg[@]+"${host_arg[@]}"} >&2
	make -j"$jobs" >&2 && make install >&2
)

log "ffmpeg-build: vorbis"
(
	cd "$work/src/vorbis"
	# libvorbis 1.3.7 adds -force_cpusubtype_ALL on macOS, which the current Apple
	# linker refuses ("ld: unknown options"); drop it before configuring.
	if [ "$os" = darwin ]; then
		perl -pi -e 's/-force_cpusubtype_ALL//g' configure
	fi
	host_arg=()
	[ -n "$cross_prefix" ] && host_arg=(--host=x86_64-w64-mingw32)
	./configure --prefix="$prefix" --libdir="$prefix/lib" --disable-shared --enable-static --disable-docs --disable-examples \
		--disable-oggtest --with-ogg="$prefix" ${host_arg[@]+"${host_arg[@]}"} >&2
	make -j"$jobs" >&2 && make install >&2
)

log "ffmpeg-build: libwebp"
(
	cd "$work/src/libwebp"
	host_arg=()
	[ -n "$cross_prefix" ] && host_arg=(--host=x86_64-w64-mingw32)
	# Only the libraries (sharpyuv and src): the command line tools need image libraries the build does not have.
	./configure --prefix="$prefix" --libdir="$prefix/lib" --disable-shared --enable-static --disable-gl --disable-sdl \
		--disable-png --disable-jpeg --disable-tiff --disable-gif --disable-wic ${host_arg[@]+"${host_arg[@]}"} >&2
	make -j"$jobs" -C sharpyuv >&2 && make -C sharpyuv install >&2
	make -j"$jobs" -C src >&2 && make -C src install >&2
)

log "ffmpeg-build: zimg"
(
	cd "$work/src/zimg"
	host_arg=()
	[ -n "$cross_prefix" ] && host_arg=(--host=x86_64-w64-mingw32)
	./autogen.sh >&2
	CXXFLAGS="-O2 $remap" ./configure --prefix="$prefix" --libdir="$prefix/lib" --disable-shared --enable-static ${host_arg[@]+"${host_arg[@]}"} >&2
	make -j"$jobs" >&2 && make install >&2
)

if [ -d "$work/src/nvcodec" ]; then
	log "ffmpeg-build: nv-codec-headers"
	make -C "$work/src/nvcodec" PREFIX="$prefix" install >&2
fi

if [ -d "$work/src/libvpl" ]; then
	# Only the dispatcher: it finds the Intel runtime of the GPU driver at run time (D-034).
	log "ffmpeg-build: libvpl"
	cmake_cross=()
	if [ -n "$cross_prefix" ]; then
		cmake_cross=(-DCMAKE_SYSTEM_NAME=Windows -DCMAKE_C_COMPILER="${cross_prefix}gcc"
			-DCMAKE_CXX_COMPILER="${cross_prefix}g++-posix" -DCMAKE_RC_COMPILER="${cross_prefix}windres")
	fi
	cmake -S "$work/src/libvpl" -B "$work/libvpl-build" -DCMAKE_INSTALL_PREFIX="$prefix" -DCMAKE_INSTALL_LIBDIR=lib \
		-DCMAKE_BUILD_TYPE=Release -DBUILD_SHARED_LIBS=OFF -DBUILD_TESTS=OFF -DBUILD_EXAMPLES=OFF -DBUILD_TOOLS=OFF \
		-DBUILD_PREVIEW=OFF -DBUILD_EXPERIMENTAL=OFF -DINSTALL_EXAMPLE_CODE=OFF -DENABLE_WARNINGS_AS_ERRORS=OFF \
		-DCMAKE_C_FLAGS="$CFLAGS" -DCMAKE_CXX_FLAGS="$CFLAGS" ${cmake_cross[@]+"${cmake_cross[@]}"} >&2
	cmake --build "$work/libvpl-build" -j "$jobs" >&2 && cmake --install "$work/libvpl-build" >&2
fi

if [ -d "$work/src/amf" ]; then
	log "ffmpeg-build: AMF headers"
	mkdir -p "$prefix/include/AMF"
	cp -R "$work/src/amf/AMF/." "$prefix/include/AMF/"
fi

# --- FFmpeg ---
name="ffmpeg-${FFMPEG_BUILD_VERSION}-${os}-${arch}"
flags=()
while IFS= read -r f; do
	# libvpl and AMF are fetched on x86 only (see above): without their inputs their flags are dropped.
	case "$f" in --enable-libvpl | --enable-amf) [ -d "$work/src/libvpl" ] || continue ;; esac
	flags+=("$f")
done < <({ flags_of common; flags_of "$os"; })
flags+=("--extra-version=vukem${FFMPEG_BUILD_TAG##*-}")

# The configure flags are checked before the build: a licence mistake costs nothing then.
(cd "$root" && go run ./internal/tools/ffmpegconf -allow .config/ffmpeg-build/configure-allowlist.txt \
	-flags .config/ffmpeg-build/configure-flags.txt) >&2 || die "the configure flags are not LGPL safe"

extra=()
case "$os" in
linux)
	extra+=("--extra-ldflags=-static-libgcc")
	[ "$arch" = arm64 ] && extra+=(--arch=aarch64)
	;;
darwin) extra+=(${configure_target[@]+"${configure_target[@]}"}) ;;
windows)
	extra+=(--arch=x86_64 --target-os=mingw32 --cross-prefix="$cross_prefix" --enable-cross-compile --pkg-config=pkg-config
		"--extra-ldflags=-static -Wl,--no-insert-timestamp")
	;;
esac

# zimg.pc does not list libm in Libs.private, yet libzimg.a needs it (log10f): the link test fails without it.
extra+=(--extra-libs=-lm)
# The libvpl dispatcher is C++: a static link needs the C++ runtime named explicitly.
[ -d "$work/src/libvpl" ] && extra+=(--extra-libs=-lstdc++)

log "ffmpeg-build: ffmpeg $FFMPEG_BUILD_VERSION for $os-$arch"
(
	cd "$work/src/ffmpeg"
	./configure --prefix="$work/install" --pkg-config-flags=--static \
		"--extra-cflags=-I$prefix/include $remap" "--extra-ldflags=-L$prefix/lib" \
		"${flags[@]}" ${extra[@]+"${extra[@]}"} >&2
	make -j"$jobs" >&2 && make install >&2
)

if [ "$os" = darwin ]; then
	# Runs on the installed (stripped) binaries, which are what is archived. The ad hoc signature carries no time and
	# its identifier comes from the file name, so signing the same bytes again gives the same signature.
	(cd "$root" && go run ./internal/tools/machouuid "$work/install/bin/ffmpeg" "$work/install/bin/ffprobe") >&2 \
		|| die "could not derive the Mach-O UUID"
	for bin in ffmpeg ffprobe; do
		codesign --force -s - "$work/install/bin/$bin" >&2 || die "codesign failed for $bin"
	done
fi

# --- the package, with the layout of the BtbN archives the pins already read ---
pkg="$work/pkg/$name"
mkdir -p "$pkg/bin"
cp "$work/install/bin/ffmpeg$exe" "$work/install/bin/ffprobe$exe" "$pkg/bin/"
cp "$work/src/ffmpeg/COPYING.LGPLv2.1" "$pkg/LICENSE.txt"
"$work/install/bin/ffmpeg" -version 2>/dev/null | sed -n 's/^configuration: //p' >"$pkg/configure.txt" || true
if [ ! -s "$pkg/configure.txt" ]; then
	# A cross compiled windows binary cannot run here: take the line from the build itself.
	sed -n 's/^#define FFMPEG_CONFIGURATION "\(.*\)"$/\1/p' "$work/src/ffmpeg/config_components.h" "$work/src/ffmpeg/config.h" 2>/dev/null | head -n 1 >"$pkg/configure.txt"
	[ -s "$pkg/configure.txt" ] || printf '%s\n' "${flags[*]} ${extra[*]}" >"$pkg/configure.txt"
fi
{
	printf 'FFmpeg %s, own LGPL build %s, built from the source tarball below by scripts/ffmpeg-build.sh.\n' "$FFMPEG_BUILD_VERSION" "$FFMPEG_BUILD_TAG"
	printf 'Corresponding FFmpeg source: %s (SHA-256 %s)\n' "$FFMPEG_SRC_URL" "$FFMPEG_SRC_SHA256"
	printf 'libvpx: %s\nopus: %s\nzlib: %s\n' "$LIBVPX_SRC_URL" "$OPUS_SRC_URL" "$ZLIB_SRC_URL"
	printf 'lame: %s\nlibogg: %s\nlibvorbis: %s\nlibwebp: %s\n' "$LAME_SRC_URL" "$OGG_SRC_URL" "$VORBIS_SRC_URL" "$LIBWEBP_SRC_URL"
	printf 'OpenH264 (four API headers only, BSD-2-Clause; the library is loaded at run time, not included): %s/{codec_api,codec_app_def,codec_def,codec_ver}.h (SHA-256 %s %s %s %s)\n' "$OPENH264_RAW_URL" "$OPENH264_CODEC_API_SHA256" "$OPENH264_CODEC_APP_DEF_SHA256" "$OPENH264_CODEC_DEF_SHA256" "$OPENH264_CODEC_VER_SHA256"
	printf 'Patch: .config/ffmpeg-build/patches/%s (SHA-256 %s)\n' "$FFMPEG_PATCH_FILE" "$FFMPEG_PATCH_SHA256"
	printf 'Build scripts and flags:https://github.com/AlexGalhardo/stuarty/tree/%s/.config/ffmpeg-build\n' "$FFMPEG_BUILD_TAG"
} >"$pkg/SOURCE.txt"

touch -d "@$SOURCE_DATE_EPOCH" "$pkg" "$pkg/bin" "$pkg"/* "$pkg"/bin/* 2>/dev/null \
	|| find "$pkg" -exec touch -t "$(date -u -r "$SOURCE_DATE_EPOCH" +%Y%m%d%H%M.%S)" {} +

if [ "$os" = windows ]; then
	archive="$out/$name.zip"
	rm -f "$archive"
	(cd "$work/pkg" && find "$name" -type f | LC_ALL=C sort | zip -X -q "$archive" -@)
else
	archive="$out/$name.tar.xz"
	"$tar_cmd" -C "$work/pkg" --sort=name --owner=0 --group=0 --numeric-owner \
		--mtime="@$SOURCE_DATE_EPOCH" -cf - "$name" | xz -9e -T1 >"$archive"
fi
printf '%s  %s\n' "$(sha256_of "$archive")" "$(basename "$archive")" >"$archive.sha256"
log "ffmpeg-build: $(cat "$archive.sha256")"
printf '%s\n' "$archive"
