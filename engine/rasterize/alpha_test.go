package rasterize

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/plan"
)

// TestKeyStable: the key is identical on two runs and is the hash of the spec
// together with the version, so another version gives another key.
func TestKeyStable(t *testing.T) {
	t.Parallel()

	spec := captionSpec([]string{"a", "b"}, 1, plainStyle())
	first, second := spec.Key(), captionSpec([]string{"a", "b"}, 1, plainStyle()).Key()

	if first != second {
		t.Fatal("two equal specs have different keys")
	}

	hashWith := func(version int) string {
		data, err := json.Marshal(struct {
			Version int             `json:"version"`
			Spec    plan.BitmapSpec `json:"spec"`
		}{version, spec})
		if err != nil {
			t.Fatal(err)
		}

		sum := sha256.Sum256(data)

		return hex.EncodeToString(sum[:])
	}

	if got, want := spec.Key(), hashWith(plan.BitmapVersion); got != want {
		t.Errorf("key %s is not the hash of the spec and BitmapVersion (%s)", got, want)
	}

	if spec.Key() == hashWith(plan.BitmapVersion+1) {
		t.Error("the key does not depend on the version")
	}
}

// TestBundledFontsHaveLicence: every embedded font file has a row in FONTS.md
// that names a licence and the licence text exists in licenses/fonts.
func TestBundledFontsHaveLicence(t *testing.T) {
	t.Parallel()

	notes, err := os.ReadFile(filepath.Join(fontDir, "FONTS.md"))
	if err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(fontDir)
	if err != nil {
		t.Fatal(err)
	}

	var files []string

	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".ttf") {
			files = append(files, e.Name())
		}
	}

	if len(files) != len(bundledFamilies) {
		t.Errorf("%d embedded font files, %d families", len(files), len(bundledFamilies))
	}

	for _, name := range files {
		var row string

		for line := range strings.SplitSeq(string(notes), "\n") {
			if strings.HasPrefix(line, "|") && strings.Contains(line, name) {
				row = line
			}
		}

		if row == "" {
			t.Errorf("FONTS.md has no row for %s", name)
		}

		var fam *bundledFamily

		for i := range bundledFamilies {
			if bundledFamilies[i].file == name {
				fam = &bundledFamilies[i]
			}
		}

		if fam == nil {
			t.Errorf("%s is embedded but is in no family", name)

			continue
		}

		lic := strings.ReplaceAll(fam.name, " ", "") + "-OFL.txt"
		if !strings.Contains(row, "SIL OFL 1.1") || !strings.Contains(row, lic) {
			t.Errorf("the FONTS.md row of %s does not name its licence and %s", name, lic)
		}

		text, err := os.ReadFile(filepath.Join("..", "..", "licenses", "fonts", lic))
		if err != nil || !strings.Contains(string(text), "SIL OPEN FONT LICENSE Version 1.1") {
			t.Errorf("%s: licence file %s missing or not the OFL: %v", name, lic, err)
		}
	}
}

// TestWordBoxesInsideBitmap: the boxes of every caption golden lie inside the
// natural size of the bitmap.
func TestWordBoxesInsideBitmap(t *testing.T) {
	t.Parallel()

	n := 0

	for _, c := range goldenCases() {
		if c.spec.Kind != KindCaption {
			continue
		}

		n++

		b := render(t, c.spec)
		if len(b.WordBoxes) != len(c.spec.Words) {
			t.Errorf("%s: %d word boxes for %d words", c.name, len(b.WordBoxes), len(c.spec.Words))
		}

		for i, w := range b.WordBoxes {
			if w.W <= 0 || w.H <= 0 || w.X < 0 || w.Y < 0 || w.X+w.W > b.NaturalW || w.Y+w.H > b.NaturalH {
				t.Errorf("%s: word %d box %+v outside %.1fx%.1f", c.name, i, w, b.NaturalW, b.NaturalH)
			}
		}
	}

	if n < 3 {
		t.Fatalf("%d caption goldens, want at least 3", n)
	}
}
