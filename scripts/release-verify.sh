#!/usr/bin/env bash
# Verify a published release (SDD-009 section 7 step 11, within 15 minutes of the tag):
#   bash scripts/release-verify.sh vX.Y.Z [--no-image] [--dispatched]
# Needs gh (authenticated), cosign, sha256sum and, for the image, docker with buildx.
# --no-image skips the image checks (a release before the first one that publishes the image,
# or a pre-release). --dispatched accepts the signature identity of a workflow_dispatch
# recovery run, which signs as @refs/heads/main instead of @refs/tags/v (section 7).
set -euo pipefail

version="${1:-}"
shift || true

check_image=1
identity_ref='refs/tags/v'

for flag in "$@"; do
	case "$flag" in
	--no-image) check_image=0 ;;
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
	echo "usage: release-verify.sh vX.Y.Z [--no-image] [--dispatched]" >&2
	exit 2
	;;
esac

v="${version#v}"
repo="AlexGalhardo/vukem-video"
archive="vukem-video_${v}_linux_amd64.tar.gz"
identity="^https://github\.com/AlexGalhardo/vukem-video/\.github/workflows/release\.yml@${identity_ref}"
issuer="https://token.actions.githubusercontent.com"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
cd "$work"

# The release lists 6 archives, 6 SBOMs, 2 .deb, 4 installers, checksums.txt, its
# bundle and the provenance (release.yml, step "Check the release assets").
read -r has_body assets < <(gh release view "$version" -R "$repo" --json body,assets \
	--jq '"\(.body | length > 0) \(.assets | length)"')
echo "release: body=$has_body assets=$assets"
[ "$has_body" = "true" ] && [ "$assets" -eq 21 ]

gh release download "$version" -R "$repo" \
	-p checksums.txt -p checksums.txt.sigstore.json -p "$archive" -p "$archive.sbom.json"

cosign verify-blob checksums.txt \
	--bundle checksums.txt.sigstore.json \
	--certificate-identity-regexp "$identity" \
	--certificate-oidc-issuer "$issuer"

sha256sum --ignore-missing -c checksums.txt
gh attestation verify "$archive" -R "$repo"

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
