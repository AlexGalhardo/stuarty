package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// repoFile reads a file of the repository (the tests of this package run in
// internal/tools/release) with the line ends of every system turned into LF.
func repoFile(t *testing.T, rel string) string {
	t.Helper()

	raw, err := os.ReadFile("../../../" + rel)
	if err != nil {
		t.Fatal(err)
	}

	return strings.ReplaceAll(string(raw), "\r\n", "\n")
}

var (
	usesLine   = regexp.MustCompile(`(?m)^\s*(?:- )?uses: (\S+)(.*)$`)
	pinnedUses = regexp.MustCompile(`^[^@\s]+@[0-9a-f]{40}$`)
	pinComment = regexp.MustCompile(`^\s*# v\d`)
)

// TestSecurity_T27_ReleaseWorkflowSigns (SDD-007 T-27, section 13): the release
// workflow pins every action by commit, installs cosign at a fixed version, signs
// checksums.txt and verifies the signature, builds the SBOMs and the provenance,
// and signs the image by digest in both registries. A change that drops one of
// them fails here, not on a release day.
func TestSecurity_T27_ReleaseWorkflowSigns(t *testing.T) {
	t.Parallel()

	wf := repoFile(t, ".github/workflows/release.yml")

	t.Run("every action is pinned by a full commit SHA", func(t *testing.T) {
		t.Parallel()

		uses := usesLine.FindAllStringSubmatch(wf, -1)
		if len(uses) < 10 {
			t.Fatalf("found %d actions in release.yml, expected the whole workflow", len(uses))
		}

		for _, u := range uses {
			if strings.HasPrefix(u[1], "./") {
				continue
			}

			if !pinnedUses.MatchString(u[1]) || !pinComment.MatchString(u[2]) {
				t.Errorf("%s is not pinned by a 40-character commit SHA with a version comment", u[1])
			}
		}
	})

	t.Run("cosign is installed at a fixed release", func(t *testing.T) {
		t.Parallel()

		if !regexp.MustCompile(`sigstore/cosign-installer@[0-9a-f]{40}`).MatchString(wf) ||
			!regexp.MustCompile(`cosign-release: v\d+\.\d+\.\d+`).MatchString(wf) {
			t.Error("cosign is not installed by a pinned action at a fixed release")
		}
	})

	t.Run("checksums.txt is signed and the signature is verified", func(t *testing.T) {
		t.Parallel()

		for _, want := range []string{
			"cosign sign-blob --yes --bundle dist/checksums.txt.sigstore.json dist/checksums.txt",
			"cosign verify-blob dist/checksums.txt",
			"--bundle dist/checksums.txt.sigstore.json",
			"--certificate-identity",
			"--certificate-oidc-issuer https://token.actions.githubusercontent.com",
		} {
			if !strings.Contains(wf, want) {
				t.Errorf("release.yml does not contain %q", want)
			}
		}

		goreleaser := repoFile(t, ".config/goreleaser.yaml")
		if !strings.Contains(goreleaser, "artifacts: checksum") || !strings.Contains(goreleaser, `"sign-blob"`) {
			t.Error("goreleaser does not sign the checksum file")
		}
	})

	t.Run("SBOMs and provenance are produced", func(t *testing.T) {
		t.Parallel()

		goreleaser := repoFile(t, ".config/goreleaser.yaml")
		if !strings.Contains(goreleaser, "sboms:") || !strings.Contains(wf, "anchore/sbom-action/download-syft@") {
			t.Error("the archives get no SBOM")
		}

		for _, want := range []string{
			"actions/attest-build-provenance@", "subject-checksums: ./dist/checksums.txt",
			"sbom: true", "provenance: mode=max", "attestations: write", "id-token: write",
		} {
			if !strings.Contains(wf, want) {
				t.Errorf("release.yml does not contain %q", want)
			}
		}
	})

	t.Run("the image is signed and attested by digest in both registries", func(t *testing.T) {
		t.Parallel()

		for _, want := range []string{
			`cosign sign --yes "$GHCR_IMAGE@$DIGEST"`, `cosign sign --yes "$HUB_IMAGE@$DIGEST"`,
			`cosign verify "$image@$DIGEST"`, "subject-digest: ${{ steps.build.outputs.digest }}",
			"subject-name: ghcr.io/alexgalhardo/vukem-video", "subject-name: index.docker.io/alexgalhardo/vukem-video",
		} {
			if !strings.Contains(wf, want) {
				t.Errorf("release.yml does not contain %q", want)
			}
		}

		// A signature on a tag does not follow the image the tag is moved to.
		for line := range strings.SplitSeq(wf, "\n") {
			if strings.Contains(line, "cosign sign ") && !strings.Contains(line, "@$DIGEST") {
				t.Errorf("an image is signed by something other than its digest: %s", strings.TrimSpace(line))
			}
		}
	})
}
