package export

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const hashesEnv = "VUKEM_VIDEO_TEST_COMPOSE_HASHES"

// composeHashes is the SHA-256 of every sampled frame of every reference scene, one line each.
func composeHashes(t *testing.T) []string {
	t.Helper()

	var out []string

	for _, name := range referenceScenes {
		frames := sceneFrames(endFrame(loadScene(t, name)))
		for i, pix := range renderFrames(t, name, 0, frames) {
			sum := sha256.Sum256(pix)
			out = append(out, name+" "+strconv.Itoa(frames[i])+" "+hex.EncodeToString(sum[:]))
		}
	}

	return out
}

// TestComposeHashesChild is the second process of TestComposeTwice: it prints the hashes and does
// nothing otherwise.
func TestComposeHashesChild(t *testing.T) {
	if os.Getenv(hashesEnv) == "" {
		t.Skip("only runs as the child of TestComposeTwice")
	}

	for _, line := range composeHashes(t) {
		_, _ = os.Stdout.WriteString("HASH " + line + "\n")
	}
}

// NFR-010: two runs in one process and one run in another process give the same SHA-256 for every
// composed frame.
func TestComposeTwice(t *testing.T) {
	first, second := composeHashes(t), composeHashes(t)
	if !slices.Equal(first, second) {
		t.Fatal("two runs in one process differ")
	}

	cmd := exec.Command(os.Args[0], "-test.run", "^TestComposeHashesChild$", "-test.count=1")
	cmd.Env = append(os.Environ(), hashesEnv+"=1")

	raw, err := cmd.Output()
	if err != nil {
		t.Fatalf("the second process failed: %v", err)
	}

	var other []string

	for line := range strings.SplitSeq(string(raw), "\n") {
		if rest, ok := strings.CutPrefix(line, "HASH "); ok {
			other = append(other, rest)
		}
	}

	if !slices.Equal(first, other) {
		t.Fatalf("the hashes of a second process differ (%d against %d lines)", len(other), len(first))
	}
}
