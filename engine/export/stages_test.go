package export

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The golden frames depend on every stage before the compositor. This test pins the stages that run
// FFmpeg, so that a golden failure on another machine says which stage differs: the generated clip,
// or the normalized proxy that the ingest makes of it.
func TestFixtureStagesAreTheSameOnEveryMachine(t *testing.T) {
	setup(t)

	spec := fixtureSpec{Name: "stage-probe.mp4", Width: 180, Height: 320, FPS: 30, Seconds: 3}
	path, err := spec.generate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(path) }()

	got := map[string]string{"generated clip": fileDigest(t, path)}

	in := ingestClip(t, spec)
	dir := filepath.Join(artifactsDir(), in.hash)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "normalized-") {
			got["artifact "+e.Name()] = fileDigest(t, filepath.Join(dir, e.Name()))
		}
	}

	for stage, sum := range got {
		want, ok := stageDigests[stage]
		if !ok {
			t.Errorf("stage %q has no digest: %s", stage, sum)

			continue
		}
		if sum != want {
			t.Errorf("stage %q: %s, want %s", stage, sum, want)
		}
	}
}

func fileDigest(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := sha256.Sum256(b)

	return hex.EncodeToString(s[:])
}

var stageDigests = map[string]string{
	"generated clip":            "b2e9dcb54719ff5275fdce16f20fcd39dfaa1fda0886232ad3eca05dbbde1799",
	"artifact normalized-1.mov": "346f47391a30d96e048e84fbedba81f3d2c98799abd065ba1524e40675d4264f",
}
