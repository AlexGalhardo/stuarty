#!/usr/bin/env bash
# Tests scripts/install-ffmpeg.sh against a fake archive made here (VV-227). Nothing is downloaded and
# no third-party binary is used: the fake "ffmpeg" is a shell script and the archive sits in the
# download cache with its real SHA-256, so the installer never opens a connection.
#   bash scripts/install-ffmpeg-test.sh [work-dir]
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
work="${1:-$(mktemp -d "${TEMP:-${TMPDIR:-/tmp}}/install-ffmpeg-test.XXXXXX")}"
mkdir -p "$work"
work="$(cd "$work" && pwd)"

fail() { printf 'install-ffmpeg-test: FAIL: %s\n' "$*" >&2; exit 1; }
ok() { printf 'install-ffmpeg-test: ok: %s\n' "$*" >&2; }

os="$(uname -s)"
arch="$(uname -m)"
exe=""
case "$os" in
Linux)
	case "$arch" in
	x86_64 | amd64) key=LINUX_AMD64 ;;
	aarch64 | arm64) key=LINUX_ARM64 ;;
	*) echo "unsupported architecture: $arch" >&2; exit 0 ;;
	esac
	ext=tar.xz
	;;
MINGW* | MSYS* | CYGWIN*) key=WINDOWS_AMD64 ext=zip exe=.exe ;;
*) echo "install-ffmpeg-test: skipped on $os (the script installs Homebrew ffmpeg there)" >&2; exit 0 ;;
esac

sha256_of() { sha256sum <"$1" | cut -d ' ' -f 1; }

tag="faketag"
name="ffmpeg-fake-lgpl"
asset="$name.$ext"
base="$work/base"
dl="$base/download/$tag"
out="$base/$tag-$key"

# make_archive <configure flags>: builds the fake archive and writes the pin file.
make_archive() {
	local flags="$1" src="$work/src"
	rm -rf "$src" "$dl"
	mkdir -p "$src/$name/bin" "$dl"
	printf '#!/bin/sh\necho "ffmpeg version fake configuration: %s"\n' "$flags" >"$src/$name/bin/ffmpeg$exe"
	cp "$src/$name/bin/ffmpeg$exe" "$src/$name/bin/ffprobe$exe"
	chmod +x "$src/$name/bin/ffmpeg$exe" "$src/$name/bin/ffprobe$exe"
	printf 'LGPL fake\n' >"$src/$name/LICENSE.txt"
	if [ "$ext" = zip ]; then
		powershell.exe -NoProfile -NonInteractive -Command \
			"Compress-Archive -LiteralPath '$(cygpath -w "$src/$name")' -DestinationPath '$(cygpath -w "$dl/$asset")' -Force" >&2
	else
		tar -cJf "$dl/$asset" -C "$src" "$name"
	fi
	digest="$(sha256_of "$dl/$asset")"
	printf 'FFMPEG_SOURCE=btbn\nFFMPEG_RELEASE_TAG=%s\nFFMPEG_VERSION=fake\nFFMPEG_ASSET_%s=%s\nFFMPEG_SHA256_%s=%s\n' \
		"$tag" "$key" "$asset" "$key" "$digest" >"$work/ffmpeg.env"
}

install() {
	FFMPEG_ENV_FILE="$work/ffmpeg.env" FFMPEG_INSTALL_DIR="$base" GITHUB_PATH="" \
		bash "$here/install-ffmpeg.sh" 2>"$work/stderr.log" | tail -n 1
}

# 1. A fresh install prints the bin folder and leaves no staging folder.
make_archive "--disable-gpl"
bin_dir="$(install)" || { cat "$work/stderr.log" >&2; fail "fresh install failed"; }
[ "$bin_dir" = "$out/$name/bin" ] || fail "bin folder is $bin_dir, want $out/$name/bin"
[ -f "$out/$name/bin/ffprobe$exe" ] || fail "ffprobe is missing after a fresh install"
[ -z "$(find "$base" -maxdepth 1 -name '.*.new.*' -o -maxdepth 1 -name '.*.old.*')" ] || fail "a staging folder was left"
ok "fresh install"

# 2. A matching folder is reused and never rewritten: a file added to it survives.
printf 'keep\n' >"$out/$name/bin/sentinel"
install >/dev/null || { cat "$work/stderr.log" >&2; fail "reinstall failed"; }
[ -f "$out/$name/bin/sentinel" ] || fail "a matching install was deleted and rewritten"
grep -q "reusing the installed folder" "$work/stderr.log" || fail "the reuse was not reported"
ok "a matching folder is reused, not deleted"

# 3. A folder that does not match (a broken one, no marker) is replaced by a rename, old one removed.
rm -f "$out/.archive-sha256"
rm -f "$out/$name/bin/ffprobe$exe"
install >/dev/null || { cat "$work/stderr.log" >&2; fail "repair failed"; }
[ -f "$out/$name/bin/ffprobe$exe" ] || fail "the broken install was not repaired"
[ ! -e "$out/$name/bin/sentinel" ] || fail "the old tree was not replaced"
[ -z "$(find "$base" -maxdepth 1 -name '.*.new.*' -o -maxdepth 1 -name '.*.old.*')" ] || fail "a staging folder was left after the repair"
ok "a broken folder is replaced by rename"

# 4. A new archive that is rejected (GPL build) leaves the installed folder untouched.
printf 'keep\n' >"$out/$name/bin/sentinel"
make_archive "--enable-gpl"
if install >/dev/null; then fail "a GPL build was accepted"; fi
grep -q "not LGPL" "$work/stderr.log" || fail "the GPL refusal was not reported"
[ -f "$out/$name/bin/sentinel" ] || fail "a rejected archive damaged the installed folder"
[ -z "$(find "$base" -maxdepth 1 -name '.*.new.*')" ] || fail "the staging folder was left after a refusal"
ok "a rejected archive leaves the install untouched"

[ -n "${1:-}" ] || rm -rf "$work"
printf 'install-ffmpeg-test: all passed\n' >&2
