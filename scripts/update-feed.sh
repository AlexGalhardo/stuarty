#!/usr/bin/env bash
# The feed of the desktop auto-update (SDD-009 section 10.5, P3-REL-004), on top of
# internal/tools/updatefeed:
#   bash scripts/update-feed.sh sign <version> <base-url> <installers dir> <feed.json> [<release notes.md>]
#       signs a feed that lists every desktop installer and every launcher artifact (.AppImage, .app.zip,
#       _portable.zip) of the directory, with a short summary of the release notes when a file is given; needs UPDATE_FEED_SIGNING_KEY
#       (the 64 hex characters of the Ed25519 seed, a repository secret). A version with a suffix is refused.
#   bash scripts/update-feed.sh verify <public key hex> <checksums.txt> <feed.json>
#       the signature of the feed and of every artifact verify, and every digest equals checksums.txt
#   bash scripts/update-feed.sh pages <output directory>
#       puts the feed and the installers of the latest stable release that has a feed into
#       <output directory>/updates/, for GitHub Pages; needs gh (GH_TOKEN), jq and sha256sum. A release
#       asset URL redirects to another host and the client follows no redirect, so Pages serves both.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
repo="${GITHUB_REPOSITORY:-AlexGalhardo/vukem-video}"

die() { printf 'update-feed: error: %s\n' "$*" >&2; exit 1; }

# The tool runs from the repository root: give it absolute paths.
abs() { (cd "$(dirname "$1")" && printf '%s/%s' "$PWD" "$(basename "$1")"); }

case "${1:-}" in
sign)
	[ $# -eq 5 ] || [ $# -eq 6 ] || die "usage: update-feed.sh sign <version> <base-url> <installers dir> <feed.json> [<release notes.md>]"
	version="$2" base="$3" dir="$(abs "$4")" out="$(abs "$5")"
	notes=()
	[ $# -eq 6 ] && notes=(-notes-file "$(abs "$6")")
	case "$version" in *-*) die "$version is a pre-release: it never reaches the feed" ;; esac
	[ -d "$dir" ] || die "not a directory: $dir"
	files=()
	dashed="${version//./-}"
	for f in "$dir"/vukem-video-desktop_"${version}"_* "$dir"/vukem-video-desktop-"${dashed}"*; do
		case "$f" in *.sbom.json) continue ;; esac
		[ -e "$f" ] && files+=("$f")
	done
	[ ${#files[@]} -gt 0 ] || die "no desktop installer of $version in $dir"
	(cd "$root" && go run ./internal/tools/updatefeed sign -version "$version" -base-url "$base" ${notes[@]+"${notes[@]}"} -out "$out" "${files[@]}")
	;;
verify)
	[ $# -eq 4 ] || die "usage: update-feed.sh verify <public key hex> <checksums.txt> <feed.json>"
	checksums="$(abs "$3")" feed="$(abs "$4")"
	(cd "$root" && go run ./internal/tools/updatefeed verify -pub "$2" -checksums "$checksums" "$feed")
	;;
pages)
	[ $# -eq 2 ] || die "usage: update-feed.sh pages <output directory>"
	command -v gh >/dev/null 2>&1 || die "gh is required"
	command -v jq >/dev/null 2>&1 || die "jq is required"
	out="$2/updates"
	tag="$(gh release list -R "$repo" --exclude-pre-releases --limit 1 --json tagName --jq '.[0].tagName // empty')"
	if [ -z "$tag" ]; then
		echo "update-feed: no stable release yet, no updates directory"
		exit 0
	fi
	if ! gh release view "$tag" -R "$repo" --json assets --jq '.assets[].name' | grep -qx feed.json; then
		echo "update-feed: $tag has no feed.json (it predates the desktop update), no updates directory"
		exit 0
	fi
	mkdir -p "$out"
	gh release download "$tag" -R "$repo" -p feed.json -D "$out" --clobber
	# Every file the feed lists (installers and launcher artifacts), checked against the digest the feed signs, before it is served.
	jq -r '.payload' "$out/feed.json" | base64 -d | jq -r '(.platforms[], (.launchers // {})[]) | "\(.sha256) \(.url | split("/") | last)"' |
		while read -r sum name; do
			gh release download "$tag" -R "$repo" -p "$name" -D "$out" --clobber
			[ "$(sha256sum <"$out/$name" | cut -d ' ' -f 1)" = "$sum" ] || die "$name does not match the digest of the feed"
			echo "update-feed: $name ($sum)"
		done
	echo "update-feed: $out holds the feed of $tag"
	;;
*)
	die "usage: update-feed.sh sign|verify|pages ..."
	;;
esac
