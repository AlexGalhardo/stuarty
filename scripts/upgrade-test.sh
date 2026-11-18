#!/usr/bin/env bash
# Upgrade path test across released versions (P4-REL-002, RF-208, RF-209, ADR-0025 item 1).
#
#   bash scripts/upgrade-test.sh [vX.Y.Z ...]
#
# For each released version (the arguments, else every published release from v0.3.0 on) it downloads the
# archive of this OS and architecture, checks it against checksums.txt, builds a workspace with
# service/testdata/upgrade/build-workspace.sh, builds the candidate (this checkout), sets
#   VUKEM_UPGRADE_WORKSPACES   one folder per version
#   VUKEM_UPGRADE_BINARIES     <version>/vukem-video[.exe]
#   VUKEM_UPGRADE_CANDIDATE    the binary of this checkout
# and runs TestUpgradeMatrix and TestUpgradeOlderBinaryRefusesCandidate. Exit 0 means every released
# workspace opens with the candidate and every older binary refuses a workspace of the candidate.
#
# v0.1.0 (the legacy Studio) and v0.2.0 (no command that creates a project) cannot be in the matrix.
# Needs: go, ffmpeg on PATH, curl or gh, jq, tar or unzip. Environment:
#   UPGRADE_REPO   owner/name of the releases (default $GITHUB_REPOSITORY, else AlexGalhardo/vukem-video)
#   UPGRADE_WORK   work folder (default a new temporary one, removed at the end)
#   UPGRADE_KEEP   set to 1 to keep the work folder
set -euo pipefail

die() { echo "upgrade-test: error: $*" >&2; exit 1; }
log() { echo "upgrade-test: $*" >&2; }

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
repo="${UPGRADE_REPO:-${GITHUB_REPOSITORY:-AlexGalhardo/vukem-video}}"

for tool in go ffmpeg ffprobe jq; do
	command -v "$tool" >/dev/null 2>&1 || die "$tool is required"
done

case "$(uname -s)" in
Linux) os=linux ext=tar.gz exe=vukem-video ;;
Darwin) os=darwin ext=tar.gz exe=vukem-video ;;
MINGW* | MSYS* | CYGWIN*) os=windows ext=zip exe=vukem-video.exe ;;
*) die "unsupported OS $(uname -s)" ;;
esac
case "$(uname -m)" in
x86_64 | amd64) arch=amd64 ;;
arm64 | aarch64) arch=arm64 ;;
*) die "unsupported architecture $(uname -m)" ;;
esac

if [ -n "${UPGRADE_WORK:-}" ]; then
	work="$UPGRADE_WORK"
	mkdir -p "$work"
else
	work="$(mktemp -d)"
fi
if [ "${UPGRADE_KEEP:-}" != 1 ]; then
	trap 'rm -rf "$work"' EXIT
fi
work="$(cd "$work" && pwd)"

if [ "$#" -gt 0 ]; then
	versions=("$@")
else
	command -v gh >/dev/null 2>&1 || die "gh is required to list the releases (or pass the versions)"
	# A while loop instead of mapfile: macOS ships bash 3.2, which has no mapfile.
	versions=()
	while IFS= read -r v; do
		versions+=("$v")
	done < <(gh release list -R "$repo" --limit 200 --json tagName -q '.[].tagName' | tr -d '\r' |
		grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' |
		awk -F'[v.]' '($2 > 0) || ($2 == 0 && $3 >= 3)' | sort -t. -k1.2,1n -k2,2n -k3,3n)
fi
[ "${#versions[@]}" -gt 0 ] || die "no version to test"
log "versions: ${versions[*]}"

sha256_of() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum <"$1" | cut -d ' ' -f 1
	else
		shasum -a 256 <"$1" | cut -d ' ' -f 1
	fi
}

download() { # download <tag> <asset> <destination>
	if command -v gh >/dev/null 2>&1; then
		gh release download "$1" -R "$repo" -p "$2" -O "$3" --clobber
	else
		curl --fail --silent --show-error --location -o "$3" "https://github.com/$repo/releases/download/$1/$2"
	fi
}

# The media fixture: the engine sniffer refuses an x264 MP4 whose moov box is at the end.
media="$work/clip.mp4"
ffmpeg -v error -y -f lavfi -i testsrc=size=320x240:rate=30:duration=2 -f lavfi -i sine=duration=2 \
	-c:v mpeg4 -pix_fmt yuv420p -c:a aac -shortest -movflags +faststart "$media"

workspaces="$work/workspaces"
binaries="$work/binaries"
mkdir -p "$workspaces" "$binaries"

for version in "${versions[@]}"; do
	number="${version#v}"
	archive="vukem-video_${number}_${os}_${arch}.${ext}"
	dl="$work/download/$version"
	mkdir -p "$dl" "$binaries/$version"

	# A release can be published before its archives exist (v0.25.0 has notes and no binaries).
	# There is nothing to open, so the matrix continues with the releases that shipped this OS.
	if command -v gh >/dev/null 2>&1; then
		names="$(gh release view "$version" -R "$repo" --json assets -q '.assets[].name' || true)"
		if ! printf '%s\n' "$names" | grep -qx "$archive"; then
			log "$version: no $archive on the release, skipping"
			rm -rf "$dl"
			continue
		fi
	fi

	log "$version: downloading $archive"
	download "$version" "$archive" "$dl/$archive"
	download "$version" checksums.txt "$dl/checksums.txt"

	want="$(tr -d '\r' <"$dl/checksums.txt" | awk -v a="$archive" '$2 == a { print $1 }')"
	[ -n "$want" ] || die "$version: $archive is not in checksums.txt"
	[ "$(sha256_of "$dl/$archive")" = "$want" ] || die "$version: the SHA-256 of $archive differs from checksums.txt"

	if [ "$ext" = zip ]; then
		unzip -q -o "$dl/$archive" -d "$dl/unpacked"
	else
		mkdir -p "$dl/unpacked"
		tar -xzf "$dl/$archive" -C "$dl/unpacked"
	fi
	found="$(find "$dl/unpacked" -name "$exe" -type f | head -n 1)"
	[ -n "$found" ] || die "$version: $exe is not in $archive"
	cp "$found" "$binaries/$version/$exe"
	chmod +x "$binaries/$version/$exe"

	log "$version: building the workspace"
	bash "$root/service/testdata/upgrade/build-workspace.sh" "$binaries/$version/$exe" "$workspaces/$version" "$media" ||
		die "$version: the scenario did not run (see $workspaces/$version/scenario.out)"
	rm -rf "$dl"
done

if ! find "$binaries" -mindepth 1 -type d | grep -q .; then
	die "every release was missing the archive for this OS"
fi

log "building the candidate"
candidate="$work/candidate/$exe"
mkdir -p "$work/candidate"
(cd "$root" && go build -trimpath -o "$candidate" ./web/cmd/app)

export VUKEM_UPGRADE_WORKSPACES="$workspaces"
export VUKEM_UPGRADE_BINARIES="$binaries"
export VUKEM_UPGRADE_CANDIDATE="$candidate"

cd "$root"
go test -count=1 -v -run 'TestUpgradeMatrix|TestUpgradeOlderBinaryRefusesCandidate' ./service/
