#!/usr/bin/env bash
# Tests scripts/install-ffmpeg.sh, and on Windows scripts/install-ffmpeg.ps1, against fake archives made
# here (VV-227). Nothing is downloaded and no third-party binary is used: the fake ffmpeg is a shell
# script on Linux and a tiny Go program built here on Windows (Go is the toolchain of this repository),
# each archive sits in the download cache with its real SHA-256, and a curl first on PATH fails the
# test if the bash installer ever tries to download.
#   bash scripts/install-ffmpeg-test.sh [work-dir]
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
keep="${1:-}"
work="${keep:-$(mktemp -d "${TEMP:-${TMPDIR:-/tmp}}/install-ffmpeg-test.XXXXXX")}"
mkdir -p "$work"
work="$(cd "$work" && pwd)"

fail() { printf 'install-ffmpeg-test: FAIL: %s\n' "$*" >&2; exit 1; }
ok() { printf 'install-ffmpeg-test: ok: %s\n' "$*" >&2; }

os="$(uname -s)"
arch="$(uname -m)"
exe=""
windows=0
case "$os" in
Linux)
	case "$arch" in
	x86_64 | amd64) key=LINUX_AMD64 ;;
	aarch64 | arm64) key=LINUX_ARM64 ;;
	*) echo "unsupported architecture: $arch" >&2; exit 0 ;;
	esac
	ext=tar.xz
	;;
MINGW* | MSYS* | CYGWIN*) key=WINDOWS_AMD64 ext=zip exe=.exe windows=1 ;;
*) echo "install-ffmpeg-test: skipped on $os (the script installs Homebrew ffmpeg there)" >&2; exit 0 ;;
esac

sha256_of() { sha256sum <"$1" | cut -d ' ' -f 1; }

tag="faketag"
name="ffmpeg-fake-lgpl"
asset="$name.$ext"
base="$work/base"
dl="$base/download/$tag"
out="$base/$tag-$key"
digest=""

# A running fake ffmpeg.exe (Windows only), stopped by closing its standard input.
hold_pid=""
hold_in=""
release() {
	[ -n "$hold_pid" ] || return 0
	eval "exec $hold_in>&-"
	wait "$hold_pid" || true
	hold_pid=""
}
cleanup() {
	local status=$?
	release
	if [ "$status" = 0 ] && [ -z "$keep" ]; then rm -rf "$work"; fi
	return "$status"
}
trap cleanup EXIT

# The bash installer runs with these first on PATH: a curl that refuses, so a download fails the test,
# and an mv that fails when its source matches FAKE_MV_FAIL (a glob), so a failed swap can be forced.
real_mv="$(command -v mv)"
mkdir -p "$work/shim"
printf '#!/usr/bin/env bash\necho "install-ffmpeg-test: curl was called, a download was attempted" >&2\nexit 7\n' >"$work/shim/curl"
printf '#!/usr/bin/env bash\nif [ -n "${FAKE_MV_FAIL:-}" ]; then case "$1" in $FAKE_MV_FAIL) echo "mv: simulated failure for $1" >&2; exit 1 ;; esac; fi\nexec "%s" "$@"\n' \
	"$real_mv" >"$work/shim/mv"
chmod +x "$work/shim/curl" "$work/shim/mv"

if [ "$windows" = 1 ]; then
	command -v go >/dev/null 2>&1 || fail "go is needed on Windows to build the fake ffmpeg.exe"
	# The folder name starts with _ so that no go build ./... of an enclosing module scans it.
	mkdir -p "$work/_fake"
	printf 'module fakeffmpeg\n\ngo 1.21\n' >"$work/_fake/go.mod"
	cat >"$work/_fake/main.go" <<'EOF'
package main

import (
	"fmt"
	"io"
	"os"
)

var flags = ""

func main() {
	if len(os.Args) > 1 && os.Args[1] == "-hold" {
		fmt.Println("ready")
		_, _ = io.Copy(io.Discard, os.Stdin)

		return
	}

	fmt.Printf("ffmpeg version fake configuration: %s\n", flags)
}
EOF
fi

# make_archive <configure flags>: builds the fake archive, caches it and writes the pin file. Each call
# gets other flags, so each archive has its own digest.
make_archive() {
	local flags="$1" src="$work/src"
	rm -rf "$src" "$dl"
	mkdir -p "$src/$name/bin" "$dl"
	if [ "$windows" = 1 ]; then
		(cd "$work/_fake" && go build -trimpath -ldflags "-X main.flags=$flags" -o "$src/$name/bin/ffmpeg.exe" .)
	else
		printf '#!/bin/sh\necho "ffmpeg version fake configuration: %s"\n' "$flags" >"$src/$name/bin/ffmpeg"
		chmod +x "$src/$name/bin/ffmpeg"
	fi
	cp "$src/$name/bin/ffmpeg$exe" "$src/$name/bin/ffprobe$exe"
	printf 'LGPL fake\n' >"$src/$name/LICENSE.txt"
	if [ "$ext" = zip ]; then
		powershell.exe -NoProfile -NonInteractive -Command \
			"Compress-Archive -LiteralPath '$(cygpath -w "$src/$name")' -DestinationPath '$(cygpath -w "$dl/$asset")' -Force" >&2
	else
		tar -cJf "$dl/$asset" -C "$src" "$name"
	fi
	digest="$(sha256_of "$dl/$asset")"
	printf 'FFMPEG_SOURCE="btbn"\nFFMPEG_RELEASE_TAG="%s"\nFFMPEG_VERSION="fake"\nFFMPEG_ASSET_%s="%s"\nFFMPEG_SHA256_%s="%s"\n' \
		"$tag" "$key" "$asset" "$key" "$digest" >"$work/ffmpeg.env"
}

# install [base]: runs the bash installer and prints the last line of its stdout.
install() {
	PATH="$work/shim:$PATH" FAKE_MV_FAIL="${FAKE_MV_FAIL:-}" \
		FFMPEG_ENV_FILE="$work/ffmpeg.env" FFMPEG_INSTALL_DIR="${1:-$base}" GITHUB_PATH="" \
		bash "$here/install-ffmpeg.sh" 2>"$work/stderr.log" | tail -n 1
}

# install_ps [base]: the same with the PowerShell installer; prints its last line in Unix form. Its
# download cannot be replaced by a fake, so the archive must already be in the cache of that base.
install_ps() {
	local target="${1:-$base}" line
	[ -f "$target/download/$tag/$asset" ] || fail "the archive is not in the cache of $target: the installer would download"
	line="$(FFMPEG_ENV_FILE="$(cygpath -w "$work/ffmpeg.env")" FFMPEG_INSTALL_DIR="$(cygpath -w "$target")" GITHUB_PATH="" \
		powershell.exe -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "$(cygpath -w "$here/install-ffmpeg.ps1")" \
		2>"$work/stderr.log" | tr -d '\r' | tail -n 1)" || return 1
	cygpath -u "$line"
}

# hold <folder>: starts the ffmpeg.exe of an installed folder and waits until it runs.
hold() {
	local line
	coproc HOLDER { "$1/$name/bin/ffmpeg.exe" -hold; }
	hold_pid="$HOLDER_PID"
	hold_in="${HOLDER[1]}"
	read -r line <&"${HOLDER[0]}"
	[ "$line" = ready ] || fail "the held ffmpeg.exe did not start: $line"
}

no_staging() {
	[ -z "$(find "$1" -maxdepth 1 \( -name '.*.new.*' -o -name '.*.old.*' \))" ] || fail "a staging folder was left in $1: $2"
}

# marker_is <folder> <digest>: the folder records that it was unpacked from that archive, as one LF line.
marker_is() {
	[ "$(od -An -c "$1/.archive-sha256" | tr -d ' \n')" = "$2\\n" ] || fail "the marker of $1 is not $2 and a line feed"
}

stderr_has() { grep -q "$1" "$work/stderr.log" || { cat "$work/stderr.log" >&2; fail "$2"; }; }

# 1. A fresh install prints the bin folder, records the archive and leaves no staging folder.
make_archive "--disable-gpl"
bin_dir="$(install)" || { cat "$work/stderr.log" >&2; fail "fresh install failed"; }
[ "$bin_dir" = "$out/$name/bin" ] || fail "bin folder is $bin_dir, want $out/$name/bin"
[ -f "$out/$name/bin/ffprobe$exe" ] || fail "ffprobe is missing after a fresh install"
marker_is "$out" "$digest"
no_staging "$base" "fresh install"
ok "fresh install"

# 2. A matching folder is reused and never rewritten, without a download even when the cache is gone.
printf 'keep\n' >"$out/$name/bin/sentinel"
rm -f "$dl/$asset"
install >/dev/null || { cat "$work/stderr.log" >&2; fail "reinstall failed"; }
[ -f "$out/$name/bin/sentinel" ] || fail "a matching install was deleted and rewritten"
stderr_has "reusing the installed folder" "the reuse was not reported"
ok "a matching folder is reused, not deleted, and nothing is downloaded"

# 3. A broken folder (no marker, ffprobe gone) is replaced by a rename and the old tree removed.
make_archive "--disable-gpl,r2"
rm -f "$out/.archive-sha256" "$out/$name/bin/ffprobe$exe"
install >/dev/null || { cat "$work/stderr.log" >&2; fail "repair failed"; }
[ -f "$out/$name/bin/ffprobe$exe" ] || fail "the broken install was not repaired"
[ ! -e "$out/$name/bin/sentinel" ] || fail "the old tree was not replaced"
marker_is "$out" "$digest"
no_staging "$base" "repair"
ok "a broken folder is replaced by rename"

# 4. A rejected archive (GPL build) leaves the installed folder untouched.
installed="$digest"
printf 'keep\n' >"$out/$name/bin/sentinel"
make_archive "--enable-gpl"
if install >/dev/null; then fail "a GPL build was accepted"; fi
stderr_has "not LGPL" "the GPL refusal was not reported"
[ -f "$out/$name/bin/sentinel" ] || fail "a rejected archive damaged the installed folder"
marker_is "$out" "$installed"
no_staging "$base" "GPL refusal"
ok "a rejected archive leaves the install untouched"

# 5. The installed folder cannot be moved aside: the install stops and the folder stays as it was.
make_archive "--disable-gpl,r3"
if FAKE_MV_FAIL="*/$tag-$key" install >/dev/null; then fail "a failed move aside was reported as a success"; fi
stderr_has "it was left untouched" "the failed move aside was not reported"
[ -f "$out/$name/bin/sentinel" ] || fail "a failed move aside damaged the installed folder"
marker_is "$out" "$installed"
no_staging "$base" "failed move aside"
ok "a failed move aside leaves the install untouched"

# 6. The new tree cannot be moved in: the old folder is put back as it was.
if FAKE_MV_FAIL="*.new.*" install >/dev/null; then fail "a failed swap was reported as a success"; fi
stderr_has "the installed folder was left as it was" "the failed swap was not reported"
[ -f "$out/$name/bin/sentinel" ] || fail "a failed swap lost the installed folder"
[ -f "$out/$name/bin/ffprobe$exe" ] || fail "a failed swap left a broken folder"
marker_is "$out" "$installed"
no_staging "$base" "failed swap"
ok "a failed swap puts the old install back"

if [ "$windows" = 0 ]; then
	printf 'install-ffmpeg-test: all passed\n' >&2
	exit 0
fi

# 7. Windows, the case of VV-227: an install while the installed ffmpeg.exe runs leaves a complete tree
# and the running program untouched.
hold "$out"
install >/dev/null || { cat "$work/stderr.log" >&2; fail "an install while ffmpeg.exe runs failed"; }
[ -f "$out/$name/bin/ffprobe$exe" ] && [ -f "$out/$name/LICENSE.txt" ] || fail "an install while ffmpeg.exe runs left a broken folder"
[ ! -e "$out/$name/bin/sentinel" ] || fail "the old tree was not replaced"
marker_is "$out" "$digest"
kill -0 "$hold_pid" 2>/dev/null || fail "the running ffmpeg.exe was stopped"
release
no_staging "$base" "install while ffmpeg.exe runs"
ok "bash: an install while ffmpeg.exe runs leaves a complete tree"

# 8. The PowerShell installer: a fresh install in a folder whose name has a space.
base_ps="$work/base ps"
out_ps="$base_ps/$tag-$key"
mkdir -p "$base_ps/download/$tag"
cp "$dl/$asset" "$base_ps/download/$tag/$asset"
bin_dir="$(install_ps "$base_ps")" || { cat "$work/stderr.log" >&2; fail "PowerShell: fresh install failed"; }
[ "$bin_dir" = "$out_ps/$name/bin" ] || fail "PowerShell: bin folder is $bin_dir, want $out_ps/$name/bin"
marker_is "$out_ps" "$digest"
no_staging "$base_ps" "PowerShell fresh install"
ok "PowerShell: fresh install"

# 9. It reuses the folder the bash installer made, and the bash installer reuses its folder.
printf 'keep\n' >"$out/$name/bin/sentinel"
install_ps >/dev/null || { cat "$work/stderr.log" >&2; fail "PowerShell: reuse failed"; }
stderr_has "reusing the installed folder" "PowerShell: the reuse was not reported"
[ -f "$out/$name/bin/sentinel" ] || fail "PowerShell: a matching install was deleted and rewritten"
printf 'keep\n' >"$out_ps/$name/bin/sentinel"
install "$base_ps" >/dev/null || { cat "$work/stderr.log" >&2; fail "bash over the PowerShell folder failed"; }
stderr_has "reusing the installed folder" "bash did not reuse the PowerShell folder"
[ -f "$out_ps/$name/bin/sentinel" ] || fail "bash rewrote the PowerShell folder"
ok "PowerShell and bash reuse each other's folder"

# 10. A rejected archive leaves the PowerShell install untouched.
installed="$digest"
make_archive "--enable-gpl"
if install_ps >/dev/null; then fail "PowerShell: a GPL build was accepted"; fi
stderr_has "not LGPL" "PowerShell: the GPL refusal was not reported"
[ -f "$out/$name/bin/sentinel" ] || fail "PowerShell: a rejected archive damaged the installed folder"
marker_is "$out" "$installed"
no_staging "$base" "PowerShell GPL refusal"
ok "PowerShell: a rejected archive leaves the install untouched"

# 11. While the installed ffmpeg.exe runs, the PowerShell install still leaves a complete tree. It
# cannot delete the running file, so the old folder may stay beside it, and it says so.
make_archive "--disable-gpl,r4"
hold "$out"
install_ps >/dev/null || { cat "$work/stderr.log" >&2; fail "PowerShell: an install while ffmpeg.exe runs failed"; }
[ -f "$out/$name/bin/ffprobe$exe" ] && [ -f "$out/$name/LICENSE.txt" ] || fail "PowerShell: an install while ffmpeg.exe runs left a broken folder"
[ ! -e "$out/$name/bin/sentinel" ] || fail "PowerShell: the old tree was not replaced"
marker_is "$out" "$digest"
kill -0 "$hold_pid" 2>/dev/null || fail "PowerShell: the running ffmpeg.exe was stopped"
release
[ -z "$(find "$base" -maxdepth 1 -name '.*.new.*')" ] || fail "PowerShell: a staging folder was left"
if [ -n "$(find "$base" -maxdepth 1 -name '.*.old.*')" ]; then
	stderr_has "delete it by hand" "PowerShell: an old folder was left without a word"
fi
ok "PowerShell: an install while ffmpeg.exe runs leaves a complete tree"

printf 'install-ffmpeg-test: all passed\n' >&2
