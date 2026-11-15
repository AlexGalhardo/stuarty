#!/usr/bin/env bash
# Verifies the FFmpeg that Vukem Video distributes (D-017, ADR-0011, NFR-043):
#   bash scripts/ffmpeg-verify.sh binary <ffmpeg> [<ffprobe>]   one built binary
#   bash scripts/ffmpeg-verify.sh archive <archive>              unpacks it, then `binary`
#   bash scripts/ffmpeg-verify.sh tree <dir>                     no GPL or nonfree marker in any file of a tree (archives, image export)
#   bash scripts/ffmpeg-verify.sh signatures <dir>               cosign verify-blob and gh attestation verify for every asset
# `binary` fails when: the configure line is not LGPL safe (internal/tools/ffmpegconf), -buildconf
# or -version names --enable-gpl, --enable-nonfree, libx264, libx265, libfdk_aac, the linker
# listing shows an OpenH264 library, the libopenh264 encoder (the dlopen patch) is missing, or a filter the engine references is missing (FILTERS_FILE,
# default scripts/ffmpeg-filters.txt; eq and boxblur must be absent).
# `signatures` needs cosign and gh, and REPO (owner/name) or GITHUB_REPOSITORY.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
allow="$root/.config/ffmpeg-build/configure-allowlist.txt"
filters_file="${FILTERS_FILE:-$root/scripts/ffmpeg-filters.txt}"

die() { printf 'ffmpeg-verify: error: %s\n' "$*" >&2; exit 1; }

# Strings that must appear in no distributed ffmpeg or ffprobe (NFR-043).
forbidden_markers=(--enable-gpl --enable-nonfree --enable-libx264 --enable-libx265 --enable-libfdk-aac libfdk_aac)

check_binary() {
	local ffmpeg="$1" ffprobe="${2:-}" version buildconf listing
	[ -f "$ffmpeg" ] || die "not found: $ffmpeg"
	version="$("$ffmpeg" -hide_banner -version)"
	buildconf="$("$ffmpeg" -hide_banner -buildconf)"
	printf '%s\n' "$version" | head -n 1
	# engine/ffmpeg/flavour.go knows the own build by this flag (SDD-011 8.2).
	case "$version" in *--extra-version=vukem*) ;; *) die "the configure line has no --extra-version=vukem<N>" ;; esac

	for m in --enable-gpl --enable-nonfree --enable-libx264 --enable-libx265 --enable-libfdk-aac --enable-libopenh264; do
		case "$version$buildconf" in *"$m"*) die "the build carries $m" ;; esac
	done
	printf '%s\n' "$version" | (cd "$root" && go run ./internal/tools/ffmpegconf -allow "$allow" -line -) >&2 \
		|| die "the configure line is not LGPL safe"

	# No OpenH264 library linked (the dynamic dependencies); the patched encoder, which loads it at run time, must be there.
	if command -v ldd >/dev/null 2>&1 && ldd "$ffmpeg" >/dev/null 2>&1; then listing="$(ldd "$ffmpeg")"
	elif command -v otool >/dev/null 2>&1; then listing="$(otool -L "$ffmpeg")"
	elif command -v objdump >/dev/null 2>&1; then listing="$(objdump -p "$ffmpeg" 2>/dev/null | grep -i 'DLL Name\|NEEDED' || true)"
	else listing=""; fi
	case "$(printf '%s' "$listing" | tr 'A-Z' 'a-z')" in *openh264*) die "an OpenH264 library is linked: $listing" ;; esac
	"$ffmpeg" -hide_banner -encoders | grep -q libopenh264 || die "the libopenh264 encoder (the run time loader patch) is missing"
	grep -aq VUKEM_VIDEO_OPENH264_DIR "$ffmpeg" || die "the binary does not read VUKEM_VIDEO_OPENH264_DIR"

	if [ -f "$filters_file" ]; then
		local have missing=""
		have="$("$ffmpeg" -hide_banner -filters)"
		while read -r f; do
			case "$f" in "" | \#*) continue ;; esac
			printf '%s\n' "$have" | awk '{print $2}' | grep -qx "$f" || missing="$missing $f"
		done <"$filters_file"
		[ -z "$missing" ] || die "filters missing:$missing"
		for gpl in eq boxblur; do
			if printf '%s\n' "$have" | awk '{print $2}' | grep -qx "$gpl"; then die "the GPL filter $gpl is present"; fi
		done
	fi
	if [ -n "$ffprobe" ]; then "$ffprobe" -hide_banner -version | head -n 1 >/dev/null; fi
	printf 'ffmpeg-verify: %s is LGPL, with no OpenH264 linked\n' "$ffmpeg"
}

check_tree() {
	local dir="$1" f m
	[ -d "$dir" ] || die "not a directory: $dir"
	# Every configure.txt is checked as a configure line; every ffmpeg and ffprobe binary is searched for the markers.
	while IFS= read -r f; do
		(cd "$root" && go run ./internal/tools/ffmpegconf -allow "$allow" -line "$f") >&2 || die "$f is not LGPL safe"
	done < <(find "$dir" -type f -name configure.txt)
	while IFS= read -r f; do
		for m in "${forbidden_markers[@]}"; do
			if grep -aq -e "$m" "$f"; then die "$m found inside $f"; fi
		done
	done < <(find "$dir" -type f \( -name ffmpeg -o -name ffmpeg.exe -o -name ffprobe -o -name ffprobe.exe \))
	printf 'ffmpeg-verify: no GPL or nonfree marker under %s\n' "$dir"
}

check_signatures() {
	local dir="$1" repo="${REPO:-${GITHUB_REPOSITORY:-}}" ref="${GITHUB_REF:-refs/heads/main}"
	[ -n "$repo" ] || die "set REPO (owner/name) or GITHUB_REPOSITORY"
	local n=0
	for f in "$dir"/ffmpeg-*.tar.xz "$dir"/ffmpeg-*.zip; do
		[ -e "$f" ] || continue
		n=$((n + 1))
		(cd "$dir" && sha256sum -c "$(basename "$f").sha256")
		cosign verify-blob "$f" --bundle "$f.sigstore.json" \
			--certificate-identity "https://github.com/${repo}/.github/workflows/ffmpeg-build.yml@${ref}" \
			--certificate-oidc-issuer https://token.actions.githubusercontent.com
		gh attestation verify "$f" --repo "$repo" --signer-workflow "${repo}/.github/workflows/ffmpeg-build.yml"
	done
	[ "$n" -gt 0 ] || die "no ffmpeg-* asset in $dir"
	printf 'ffmpeg-verify: %s assets verified\n' "$n"
}

cmd="${1:-}"
case "$cmd" in
binary)
	check_binary "${2:?usage: ffmpeg-verify.sh binary <ffmpeg> [<ffprobe>]}" "${3:-}"
	;;
archive)
	archive="${2:?usage: ffmpeg-verify.sh archive <archive>}"
	tmp="$(mktemp -d)"
	trap 'rm -rf "$tmp"' EXIT
	case "$archive" in
	*.tar.xz) tar -xJf "$archive" -C "$tmp" ;;
	*.zip)
		if command -v unzip >/dev/null 2>&1; then unzip -q "$archive" -d "$tmp"
		else
			# Git Bash on Windows has no unzip.
			powershell.exe -NoProfile -NonInteractive -Command \
				"Expand-Archive -LiteralPath '$(cygpath -w "$archive")' -DestinationPath '$(cygpath -w "$tmp")' -Force" >&2
		fi
		;;
	*) die "unknown archive type: $archive" ;;
	esac
	bin="$(find "$tmp" -type f \( -name ffmpeg -o -name ffmpeg.exe \) | head -n 1)"
	[ -n "$bin" ] || die "no ffmpeg in $archive"
	for f in LICENSE.txt configure.txt SOURCE.txt; do
		[ -s "$(dirname "$(dirname "$bin")")/$f" ] || die "$f is missing or empty in $archive"
	done
	check_tree "$tmp"
	check_binary "$bin"
	;;
tree)
	check_tree "${2:?usage: ffmpeg-verify.sh tree <dir>}"
	;;
signatures)
	check_signatures "${2:?usage: ffmpeg-verify.sh signatures <dir>}"
	;;
*)
	die "usage: ffmpeg-verify.sh binary|archive|tree|signatures ..."
	;;
esac
