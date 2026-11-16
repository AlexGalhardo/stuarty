#!/usr/bin/env bash
# Supply-chain audit of a release (SDD-009 section 20 checks 4, 5, 11 and 14; NFR-041, NFR-043, RF-215):
#   bash scripts/release-audit.sh [vX.Y.Z | --dist DIR] [--image REF | --no-image] [--strict]
# With no argument it audits the latest published release. --dist audits a local folder (the dist/ of
# "task release:binaries SNAPSHOT=1", or any folder of artifacts) and downloads nothing.
#
# It opens every archive (.tar.gz, .zip), .deb, AppImage, .pkg and installer it can, and reports:
#   - any GPL, AGPL, SSPL or unknown licence: the licence texts of each artifact and the licences named
#     by each SBOM are judged by "internal/tools/notices -audit-tree" and "-audit-spdx", which use the
#     allow-list of the notices generator, so there is one source of truth (SDD-009 section 12.2);
#   - any artifact without THIRD_PARTY_NOTICES.md;
#   - the size of the largest archive against NFR-041 (60,000,000 bytes, SDD-009 section 20 check 5) and
#     the asset count.
# The container image is read with docker (REF defaults to ghcr.io/alexgalhardo/vukem-video:X.Y.Z); without a
# docker daemon, or with --no-image, it is reported as not inspected.
#
# The Inno Setup .exe is opened with innoextract, a CLI like 7z (apt install innoextract). An artifact that cannot
# be opened here is reported as not inspected; --strict turns every "not inspected" and every "unknown (scanner)"
# SBOM licence (a LicenseRef-* or NOASSERTION) into a failure.
#
# A reviewed waiver (.config/sbom-waivers.json, SDD-009 section 20) turns a wrong SBOM scanner claim into a
# "WAIVED" note while the text it names, inside the archive of that SBOM, still has the recorded sha256; when the
# text changes the waiver expires and the line is a finding again.
# Exit codes: 0 clean, 1 a finding, 2 usage or a missing tool.
# Needs: go, jq, tar, unzip, 7z (or 7zz) for .deb/.pkg/.AppImage, innoextract for the .exe (optional), and gh for a tag.
set -euo pipefail

repo="AlexGalhardo/vukem-video"
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$(cd "$script_dir/.." && pwd)"
max_archive=60000000

target=""
dist=""
image=""
check_image=1
strict=0

while [ $# -gt 0 ]; do
	case "$1" in
	--dist)
		dist="${2:-}"
		shift
		;;
	--image)
		image="${2:-}"
		shift
		;;
	--no-image) check_image=0 ;;
	--strict) strict=1 ;;
	v[0-9]*.[0-9]*.[0-9]*) target="$1" ;;
	*)
		echo "usage: release-audit.sh [vX.Y.Z | --dist DIR] [--image REF | --no-image] [--strict]" >&2
		exit 2
		;;
	esac
	shift
done

for tool in go jq tar unzip; do
	command -v "$tool" >/dev/null 2>&1 || {
		echo "release-audit: $tool is required" >&2
		exit 2
	}
done

sevenzip=""
for c in 7z 7zz; do
	if command -v "$c" >/dev/null 2>&1; then
		sevenzip="$c"
		break
	fi
done

innoextract=""
command -v innoextract >/dev/null 2>&1 && innoextract="innoextract"
waivers="$root/.config/sbom-waivers.json"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

if [ -n "$dist" ]; then
	[ -d "$dist" ] || {
		echo "release-audit: $dist is not a folder" >&2
		exit 2
	}
	dir="$(cd "$dist" && pwd)"
	version="${target#v}"
else
	command -v gh >/dev/null 2>&1 || {
		echo "release-audit: gh is required to read a release" >&2
		exit 2
	}
	[ -n "$target" ] || target="$(gh release view -R "$repo" --json tagName --jq .tagName)"
	version="${target#v}"
	dir="$work/release"
	mkdir -p "$dir"
	echo "release-audit: downloading $target"
	gh release download "$target" -R "$repo" -D "$dir"
fi

notices="$work/notices"
(cd "$root" && go build -o "$notices" ./internal/tools/notices)

findings=0
notinspected=0
finding() {
	echo "FINDING: $*"
	findings=$((findings + 1))
}
skipped() {
	echo "NOT INSPECTED: $*"
	notinspected=$((notinspected + 1))
}

# unpack FILE OUT extracts a file, then the archives inside it that the installers nest
# (data.tar.* of a .deb, the cpio Payload of a .pkg), up to four levels.
unpack() {
	local file="$1" out="$2" round f
	mkdir -p "$out"
	case "$file" in
	*.tar.gz) tar -xzf "$file" -C "$out" ;;
	*.zip) unzip -q -o "$file" -d "$out" ;;
	*.exe)
		[ -n "$innoextract" ] || return 1
		"$innoextract" --silent --output-dir "$out" "$file" >/dev/null 2>"$work/innoextract.err" || true
		if [ -z "$(find "$out" -type f -print -quit)" ]; then
			echo "release-audit: innoextract failed on ${file##*/}: $(head -c 300 "$work/innoextract.err" | tr '\n' ' ')" >&2
			return 1
		fi
		;;
	*)
		[ -n "$sevenzip" ] || return 1
		# 7z exits non-zero for a squashfs symlink it cannot create on Windows; what matters is that files came out.
		"$sevenzip" x -y -bd -o"$out" "$file" >/dev/null 2>&1 || true
		[ -n "$(find "$out" -type f -print -quit)" ] || return 1
		;;
	esac
	[ -n "$sevenzip" ] || return 0
	for round in 1 2 3 4; do
		local more=0
		while IFS= read -r f; do
			[ -e "$f.done" ] && continue
			: >"$f.done"
			"$sevenzip" x -y -bd -o"$f.d" "$f" >/dev/null 2>&1 && more=1
		done < <(find "$out" -type f \( -name 'data.tar*' -o -name 'Payload*' -o -name '*.cpio' -o -name '*.tar' \) ! -name '*.done' ! -path '*.d/*.done' 2>/dev/null)
		[ "$more" -eq 1 ] || break
	done
}

audit_licenses() {
	local label="$1" tree="$2" out rc=0
	# Exit 1 means findings, listed on stdout; anything else means the tool itself failed.
	out="$("$notices" -audit-tree "$tree" 2>"$work/notices.err")" || rc=$?
	if [ "$rc" -gt 1 ]; then
		finding "$label: the licence text audit failed: $(head -n 1 "$work/notices.err")"
		return
	fi
	if [ -n "$out" ]; then
		while IFS=$'\t' read -r verdict subject reason; do
			finding "$label: licence text $verdict: $subject ($reason)"
		done <<<"$out"
	fi
}

# audit_extracted LABEL DIR: the notices file must be there, and the licence texts beside it must pass.
audit_extracted() {
	local label="$1" tree="$2" nf roots
	nf="$(find "$tree" -type f -name THIRD_PARTY_NOTICES.md ! -path '*.d/*.done' | head -n 20)"
	if [ -z "$nf" ]; then
		finding "$label: no THIRD_PARTY_NOTICES.md"
		return
	fi
	roots="$(printf '%s\n' "$nf" | while IFS= read -r p; do dirname "$p"; done | sort -u)"
	while IFS= read -r r; do
		if [ -d "$r/licenses" ]; then
			audit_licenses "$label" "$r"
		else
			finding "$label: THIRD_PARTY_NOTICES.md has no licenses/ folder beside it ($r)"
		fi
	done <<<"$roots"
}

audit_sbom() {
	local file="$1" label out archive wflags="" rc=0
	label="$(basename "$file")"
	# A waiver is checked against the text inside the archive this SBOM describes.
	archive="${file%.sbom.json}"
	if [ -f "$waivers" ]; then
		wflags="-waivers $waivers"
		if [ -f "$archive" ] && unpack "$archive" "$work/waiver/$label"; then
			wflags="$wflags -waiver-root $work/waiver/$label"
		fi
	fi
	# One line per package: the concluded licence, or the declared one when nothing was concluded.
	out="$(jq -r '
		(.packages[]? | [.name, (if (.licenseConcluded // "NOASSERTION") != "NOASSERTION" then .licenseConcluded else (.licenseDeclared // "NOASSERTION") end)]),
		(.components[]? | [.name, ([.licenses[]? | (.license.id // .license.name // .expression)] | join(" AND ") | if . == "" then "NOASSERTION" else . end)])
		| .[0] + "\t" + .[1]' "$file" | "$notices" -audit-spdx $wflags 2>"$work/notices.err")" || rc=$?
	if [ "$rc" -gt 1 ]; then
		finding "$label: the SBOM licence audit failed: $(head -n 1 "$work/notices.err")"
		rm -rf "$work/waiver/$label"
		return 0
	fi
	if [ -z "$out" ]; then
		rm -rf "$work/waiver/$label"
		return 0
	fi
	while IFS=$'\t' read -r verdict subject reason; do
		if [ "$verdict" = "waived" ]; then
			echo "WAIVED: $label: $subject: $reason"
			continue
		fi
		if [ "$verdict" = "unknown" ] && [ "$strict" -eq 0 ]; then
			# The scanner could not identify a text: the licences/ scan of the archive is the judge.
			echo "NOTE: $label: $subject: $reason is not identified by the SBOM scanner"
			continue
		fi
		finding "$label: SBOM licence $verdict: $subject ($reason)"
	done <<<"$out"
	rm -rf "$work/waiver/$label"
}

echo "release-audit: ${target:-$dir}"
assets=0
largest=0
largest_name=""
n=0

while IFS= read -r f; do
	name="$(basename "$f")"
	assets=$((assets + 1))
	size="$(wc -c <"$f" | tr -d ' ')"
	case "$name" in
	*.tar.gz | *.zip | *.deb | *.AppImage | *.pkg | *.exe | *.msi | *.dmg)
		n=$((n + 1))
		printf 'artifact: %s (%s bytes)\n' "$name" "$size"
		case "$name" in
		*.tar.gz | *.zip)
			if [ "$size" -gt "$largest" ]; then
				largest="$size"
				largest_name="$name"
			fi
			;;
		esac
		if unpack "$f" "$work/ex/$n"; then
			audit_extracted "$name" "$work/ex/$n"
		else
			skipped "$name cannot be opened with the tools here (an Inno Setup .exe needs innoextract; .deb, .pkg and AppImage need 7z)"
		fi
		rm -rf "$work/ex/$n"
		;;
	*.sbom.json)
		printf 'sbom: %s\n' "$name"
		audit_sbom "$f"
		;;
	esac
done < <(find "$dir" -maxdepth 1 -type f | sort)

if [ -n "$largest_name" ]; then
	echo "largest archive: $largest_name $largest bytes (limit $max_archive)"
	[ "$largest" -lt "$max_archive" ] || finding "$largest_name is $largest bytes, over the $max_archive of NFR-041"
fi
echo "assets: $assets"

if [ "$check_image" -eq 1 ] && [ -n "$version" ]; then
	image="${image:-ghcr.io/alexgalhardo/vukem-video:$version}"
	if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1 && docker pull "$image" >/dev/null 2>&1; then
		cid="$(docker create "$image")"
		mkdir -p "$work/image"
		if docker cp "$cid:/usr/share/doc/vukem-video/." "$work/image/" >/dev/null 2>&1; then
			audit_extracted "image $image" "$work/image"
		else
			finding "image $image: /usr/share/doc/vukem-video is missing"
		fi
		docker rm "$cid" >/dev/null
		flags="$(docker run --rm --entrypoint ffmpeg "$image" -version 2>/dev/null | grep -E -o -e '--enable-(gpl|nonfree|version3)' | sort -u | tr '\n' ' ' || true)"
		case "$flags" in
		*gpl* | *nonfree*) finding "image $image: FFmpeg is built with $flags" ;;
		esac
	else
		skipped "image ${image}: no docker daemon, or the image cannot be pulled"
	fi
elif [ "$check_image" -eq 0 ]; then
	echo "image: skipped (--no-image)"
fi

if [ "$strict" -eq 1 ] && [ "$notinspected" -gt 0 ]; then
	findings=$((findings + notinspected))
fi

echo "release-audit: $findings finding(s), $notinspected not inspected"
[ "$findings" -eq 0 ]
