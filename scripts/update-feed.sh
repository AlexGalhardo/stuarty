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
repo="${GITHUB_REPOSITORY:-AlexGalhardo/stuarty}"

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
	for f in "$dir"/stuarty-desktop_"${version}"_* "$dir"/stuarty-desktop-"${dashed}"*; do
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
	# The newest stable release that carries a feed: a release without installers (a release train cut)
	# must not take the published feed away from the desktop applications that poll it.
	tag=""
	for t in $(gh release list -R "$repo" --exclude-pre-releases --limit 100 --json tagName --jq '.[].tagName'); do
		if gh release view "$t" -R "$repo" --json assets --jq '.assets[].name' | grep -qx feed.json; then
			tag="$t"
			break
		fi
	done
	if [ -z "$tag" ]; then
		echo "update-feed: no stable release has a feed.json yet, no updates directory"
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
