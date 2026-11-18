#!/usr/bin/env bash
# Verifies a downloaded desktop installer of a release (SDD-009 section 10.6, P3-REL-003, D-033):
#   bash scripts/desktop-verify.sh vX.Y.Z <installer>
# The BETA installers are not signed with a paid certificate, so there is no codesign or signtool step.
# Integrity is checked three ways: the SHA-256 of the file against checksums.txt of the release, the
# cosign keyless signature of checksums.txt, and the provenance attestation of the file.
# Needs gh (authenticated), cosign and sha256sum (shasum on macOS). REPO overrides the repository.
set -euo pipefail

die() { printf 'desktop-verify: error: %s\n' "$*" >&2; exit 1; }

version="${1:-}"
file="${2:-}"
case "$version" in
v[0-9]*.[0-9]*.[0-9]*) ;;
*) die "usage: desktop-verify.sh vX.Y.Z <installer>" ;;
esac
[ -f "$file" ] || die "usage: desktop-verify.sh vX.Y.Z <installer>"
name="$(basename "$file")"
dashed="${version#v}"
dashed="${dashed//./-}"
case "$name" in
vukem-video-desktop_"${version#v}"_*|vukem-video-desktop-"${dashed}"*) ;;
*) die "$name is not a desktop installer of $version (vukem-video-desktop-${dashed}... or vukem-video-desktop_${version#v}_...)" ;;
esac

repo="${REPO:-AlexGalhardo/vukem-video}"
issuer="https://token.actions.githubusercontent.com"
identity="^https://github\.com/${repo//./\\.}/\.github/workflows/release\.yml@refs/(tags/v|heads/main$)"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
gh release download "$version" -R "$repo" -D "$work" -p checksums.txt -p checksums.txt.sigstore.json

cosign verify-blob "$work/checksums.txt" \
	--bundle "$work/checksums.txt.sigstore.json" \
	--certificate-identity-regexp "$identity" \
	--certificate-oidc-issuer "$issuer"

want="$(awk -v n="$name" '{ f = $2; sub(/^\*/, "", f); if (f == n) print $1 }' "$work/checksums.txt")"
[ -n "$want" ] || die "$name is not in checksums.txt of $version"
if command -v sha256sum >/dev/null 2>&1; then got="$(sha256sum <"$file" | cut -d ' ' -f 1)"
else got="$(shasum -a 256 <"$file" | cut -d ' ' -f 1)"; fi
[ "$got" = "$want" ] || die "the SHA-256 of $name is $got, checksums.txt says $want"
echo "desktop-verify: SHA-256 equals checksums.txt"

gh attestation verify "$file" -R "$repo"
echo "desktop-verify: ok ($name is the file release.yml built; it is not signed with a paid certificate)"
