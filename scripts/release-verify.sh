#!/usr/bin/env bash
# Verify a published release (SDD-009 section 7 step 11, within 15 minutes of the tag):
#   bash scripts/release-verify.sh vX.Y.Z [--no-image] [--no-pages] [--dispatched]
# Needs gh (authenticated), cosign, sha256sum and, for the image, docker with buildx.
# --no-image skips the image checks (a release before the first one that publishes the image,
# or a pre-release). --no-pages skips the comparison with the feed that GitHub Pages serves (it
# is deployed a few minutes after the release). A stable release also has feed.json, which is
# checked against checksums.txt and the public key UPDATE_FEED_PUBLIC_KEY (the repository
# variable, read with gh when it is not in the environment). --dispatched accepts the signature
# identity of a workflow_dispatch
# recovery run, which signs as @refs/heads/main instead of @refs/tags/v (section 7).
set -euo pipefail

version="${1:-}"
shift || true

check_image=1
check_pages=1
identity_ref='refs/tags/v'

for flag in "$@"; do
	case "$flag" in
	--no-image) check_image=0 ;;
	--no-pages) check_pages=0 ;;
	--dispatched) identity_ref='refs/heads/main$' ;;
	*)
		echo "release-verify: unknown flag $flag" >&2
		exit 2
		;;
	esac
done

case "$version" in
v[0-9]*.[0-9]*.[0-9]*) ;;
*)
	echo "usage: release-verify.sh vX.Y.Z [--no-image] [--no-pages] [--dispatched]" >&2
	exit 2
	;;
esac

v="${version#v}"
repo="AlexGalhardo/vukem-video"
archive="vukem-video_${v}_linux_amd64.tar.gz"
identity="^https://github\.com/AlexGalhardo/vukem-video/\.github/workflows/release\.yml@${identity_ref}"
issuer="https://token.actions.githubusercontent.com"

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
cd "$work"

# From v0.16.0 the release lists 6 archives, 6 SBOMs, 2 .deb, 4 installers, the 4 installers of the desktop
# application and their SBOM, checksums.txt, its bundle and the provenance: 26 assets, and a
# stable release adds feed.json: 27 (release.yml, steps "Check the release assets" and "Attach the feed").
# Before v0.16.0 there is no desktop application and no feed: v0.15.0 lists 21 assets (the 26 minus the 5 of
# the desktop application), and the desktop and feed checks below are skipped. Releases older than v0.15.0
# have other counts and are not verified by this script. From v0.18.0 the release adds the 3 launcher
# artifacts of the desktop application (the AppImage and one zipped .app per macOS architecture): 29, and
# 30 for a stable release.
core="${v%%-*}"
minor="${core#*.}"
minor="${minor%%.*}"
desktop=1
if [ "${core%%.*}" = "0" ] && [ "$minor" -lt 16 ]; then
	desktop=0
fi
want_assets=26
[ "$desktop" -eq 1 ] || want_assets=21
if [ "${core%%.*}" != "0" ] || [ "$minor" -ge 18 ]; then
	want_assets=29
fi
stable=0
case "$v" in
*-*) ;;
*)
	[ "$desktop" -eq 0 ] || want_assets=$((want_assets + 1))
	stable=1
	;;
esac
read -r has_body assets < <(gh release view "$version" -R "$repo" --json body,assets \
	--jq '"\(.body | length > 0) \(.assets | length)"')
echo "release: body=$has_body assets=$assets (want $want_assets)"
[ "$has_body" = "true" ] && [ "$assets" -eq "$want_assets" ]

downloads=(-p checksums.txt -p checksums.txt.sigstore.json -p "$archive" -p "$archive.sbom.json")
[ "$desktop" -eq 0 ] || downloads+=(-p 'vukem-video-desktop_*')
gh release download "$version" -R "$repo" "${downloads[@]}"

cosign verify-blob checksums.txt \
	--bundle checksums.txt.sigstore.json \
	--certificate-identity-regexp "$identity" \
	--certificate-oidc-issuer "$issuer"

# The desktop installers are not signed with a paid certificate (D-033): their integrity is checksums.txt,
# which cosign signed above, and the provenance attestation of every file.
sha256sum --ignore-missing -c checksums.txt
gh attestation verify "$archive" -R "$repo"
for installer in vukem-video-desktop_"${v}"_* vukem-video-desktop-"${v//./-}"*; do
	[ -e "$installer" ] || continue
	case "$installer" in *.sbom.json) continue ;; esac
	gh attestation verify "$installer" -R "$repo"
done

# The archive bundles no FFmpeg (NFR-041), so the digests of "doctor --verify" read "your own build,
# not compared" until the pinned FFmpeg of the release sits next to the binary. Put it there, from the
# pin file at the tag, and require "matches the release" for ffmpeg and ffprobe. Linux amd64 only.
if [ "$(uname -s)" = "Linux" ] && [ "$(uname -m)" = "x86_64" ]; then
	mkdir -p extracted ffmpeg-pin
	tar -xzf "$archive" -C extracted
	gh api "repos/$repo/contents/.config/ffmpeg.env?ref=$version" -H "Accept: application/vnd.github.raw" >ffmpeg-pin/ffmpeg.env
	ffmpeg_bin="$(FFMPEG_ENV_FILE="$work/ffmpeg-pin/ffmpeg.env" FFMPEG_INSTALL_DIR="$work/ffmpeg-pin/install" bash "$script_dir/install-ffmpeg.sh" | tail -n 1)"
	cp "$ffmpeg_bin/ffmpeg" "$ffmpeg_bin/ffprobe" extracted/
	verify_out="$(VUKEM_VIDEO_HOME="$work/doctor-home" ./extracted/vukem-video doctor --verify --json || true)"
	matched="$(printf '%s' "$verify_out" | grep -o 'matches the release' | wc -l)"
	if [ "$matched" -ne 2 ]; then
		echo "release-verify: doctor --verify did not report 'matches the release' for ffmpeg and ffprobe ($matched of 2)" >&2
		printf '%s\n' "$verify_out" >&2
		exit 1
	fi
	echo "release-verify: doctor --verify matches the release for ffmpeg and ffprobe"
fi

# The update feed of the desktop application (SDD-009 section 10.5): every digest in it is the one in
# checksums.txt, and its signature and the signature of each artifact verify with the public key.
if [ "$stable" -eq 1 ] && [ "$desktop" -eq 1 ]; then
	public_key="${UPDATE_FEED_PUBLIC_KEY:-$(gh variable get UPDATE_FEED_PUBLIC_KEY -R "$repo")}"
	gh release download "$version" -R "$repo" -p feed.json
	bash "$script_dir/update-feed.sh" verify "$public_key" checksums.txt feed.json
	if [ "$check_pages" -eq 1 ]; then
		site="${SITE_URL:-https://video.vukem.dev}"
		curl -fsSL "${site%/}/updates/feed.json" -o pages-feed.json
		cmp feed.json pages-feed.json || {
			echo "release-verify: GitHub Pages serves another feed than the release (deploy not finished? use --no-pages to skip)" >&2
			exit 1
		}
	fi
fi

if [ "$check_image" -eq 0 ]; then
	echo "release-verify: ok (image checks skipped)"
	exit 0
fi

digests=()

for image in ghcr.io/alexgalhardo/vukem-video alexgalhardo/vukem-video; do
	digest="$(docker buildx imagetools inspect "$image:$v" --format '{{.Manifest.Digest}}')"
	cosign verify "$image@$digest" \
		--certificate-identity-regexp "$identity" \
		--certificate-oidc-issuer "$issuer"
	echo "$image $digest"
	digests+=("$digest")
done

# One build is pushed to both registries, so the digests are the same.
if [ "${digests[0]}" != "${digests[1]}" ]; then
	echo "release-verify: the registries serve different digests" >&2
	exit 1
fi

docker run --rm "ghcr.io/alexgalhardo/vukem-video:$v" version | grep -F -- "$v"
echo "release-verify: ok"
