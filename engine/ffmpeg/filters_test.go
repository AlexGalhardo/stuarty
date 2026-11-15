package ffmpeg

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// TestFiltersPresent checks that every filter the argv checker accepts, and that SDD-004 and SDD-011 name,
// exists in the build: over the listing fixture of the own build, and over the pinned build for real.
func TestFiltersPresent(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "builds", "own", "filters.txt"))
	if err != nil {
		t.Fatal(err)
	}
	filters := parseListing(string(b))
	if m := MissingFilters(filters); len(m) != 0 {
		t.Errorf("the own build fixture lacks %v", m)
	}

	// A build without scale or ebur128 is reported by name; the optional ones never are.
	delete(filters, "scale")
	delete(filters, "ebur128")
	delete(filters, "zscale")
	if m := MissingFilters(filters); !slices.Equal(m, []string{"ebur128", "scale"}) {
		t.Errorf("missing = %v", m)
	}
}

func TestFiltersPresentOnThePinnedBuild(t *testing.T) {
	r, dir := pinnedRunner(t)
	filters, err := r.list(context.Background(), dir, "-filters")
	if err != nil {
		t.Fatal(err)
	}
	if m := MissingFilters(filters); len(m) != 0 {
		t.Errorf("the pinned build lacks %v", m)
	}
}

// The encoders listing of the own build fixture has what detection and the required items read.
func TestOwnBuildEncoderListing(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "builds", "own", "encoders.txt"))
	if err != nil {
		t.Fatal(err)
	}
	enc := parseListing(string(b))
	for _, n := range append([]string{"libopenh264"}, requiredEncoders...) {
		if !enc[n] {
			t.Errorf("encoder %s is not in the fixture", n)
		}
	}
	if enc["libx264"] || enc["libx265"] {
		t.Error("an LGPL build lists libx264 or libx265")
	}
}
