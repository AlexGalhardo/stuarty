package export

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	coreimage "github.com/AlexGalhardo/stuarty/core/image"
)

// The lookup table of an image layer (SDD-021 section 3.1, epic 49): the same .cube file and the same arithmetic
// as a clip (SDD-004 section 3.6), drawn by the same module.

// lutFile writes a table of lutFixture to a folder and gives its content hash and its path.
func lutFile(t *testing.T, text string) (hash, path string) {
	t.Helper()

	sum := sha256.Sum256([]byte(text))
	hash = "sha256:" + hex.EncodeToString(sum[:])
	path = filepath.Join(t.TempDir(), "table.cube")

	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}

	return hash, path
}

// lutEnv is the environment of a batch that knows the table as a lut asset.
func lutEnv(hash string) coreimage.Env {
	return coreimage.Env{Asset: func(h string) (coreimage.AssetInfo, bool) {
		if h == hash {
			return coreimage.AssetInfo{Name: "table.cube", Kind: coreimage.AssetLUT}, true
		}

		return coreimage.AssetInfo{}, false
	}}
}

func withLUT(t *testing.T, d *coreimage.Document, hash, layer, args string) *coreimage.Document {
	t.Helper()

	op := command.Operation{Op: "layer.adjust", Args: json.RawMessage(
		`{"layer_id": "` + layer + `", "lut": {"asset": "` + hash + `"` + args + `}}`)}

	res, err := coreimage.ApplyBatch(d, []command.Operation{op}, lutEnv(hash))
	if err != nil {
		t.Fatal(err)
	}

	return res.Doc
}

func renderWithLUT(t *testing.T, d *coreimage.Document, path string) (*image.RGBA, error) {
	t.Helper()

	src := sourcePNG(t)

	return RenderImage(context.Background(), d, ImageConfig{
		AssetPath: func(string) (string, error) { return src, nil },
		LUTPath:   func(string) (string, error) { return path, nil },
	})
}

func goldenPNG(t *testing.T, name string, img *image.RGBA) {
	t.Helper()

	got := pngBytes(t, img)
	path := filepath.Join("testdata", "image", name)

	if *update {
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatal(err)
		}

		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update)", err)
	}

	if !bytes.Equal(got, want) {
		t.Fatalf("%s differs from its golden file (%d bytes against %d)", name, len(got), len(want))
	}
}

// TestImageLUT: a 17 point table at 100 % and at 50 % on the photo changes the picture to the golden files, twice
// the same bytes; an intensity of 0 leaves no key and the picture of the document without it.
func TestImageLUT(t *testing.T) {
	hash, path := lutFile(t, lutFixture("warm"))
	base := lookBase()
	photo := firstLayerOfKind(base, coreimage.KindImage)
	plain, perr := renderWithLUT(t, base, path)

	if perr != nil {
		t.Fatal(perr)
	}

	for _, c := range []struct{ name, args, golden string }{
		{"full", ``, "lut-100.png"},
		{"half", `, "intensity": 50`, "lut-50.png"},
	} {
		t.Run(c.name, func(t *testing.T) {
			d := withLUT(t, base, hash, photo, c.args)

			a, err := renderWithLUT(t, d, path)
			if err != nil {
				t.Fatal(err)
			}

			if bytes.Equal(a.Pix, plain.Pix) {
				t.Fatal("the table left the picture as it was")
			}

			b, err := renderWithLUT(t, d, path)
			if err != nil {
				t.Fatal(err)
			}

			if !bytes.Equal(a.Pix, b.Pix) {
				t.Fatal("two renders of the same document differ")
			}

			goldenPNG(t, c.golden, a)
		})
	}

	t.Run("an intensity of 0 leaves the picture of the document without it", func(t *testing.T) {
		d := withLUT(t, base, hash, photo, ``)

		res, err := coreimage.ApplyBatch(d, []command.Operation{{Op: "layer.adjust", Args: json.RawMessage(
			`{"layer_id": "` + photo + `", "lut": {"intensity": 0}}`)}}, lutEnv(hash))
		if err != nil {
			t.Fatal(err)
		}

		if l, _ := coreimage.Find(res.Doc, photo); l.Adjust != nil {
			t.Fatalf("an intensity of 0 left an adjust: %+v", l.Adjust)
		}

		got, err := renderWithLUT(t, res.Doc, path)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(got.Pix, plain.Pix) {
			t.Error("the picture differs from the one of the document that never had a table")
		}
	})

	t.Run("a shape layer is graded too", func(t *testing.T) {
		d := withLUT(t, base, hash, firstLayerOfKind(base, coreimage.KindShape), ``)

		got, err := renderWithLUT(t, d, path)
		if err != nil {
			t.Fatal(err)
		}

		if bytes.Equal(got.Pix, plain.Pix) {
			t.Error("the table left the fill as it was")
		}
	})
}

// TestImageLUTRefusals: a table that cannot be had or read fails the export; the layer is never drawn without it.
func TestImageLUTRefusals(t *testing.T) {
	hash, path := lutFile(t, lutFixture("teal"))
	base := lookBase()
	d := withLUT(t, base, hash, firstLayerOfKind(base, coreimage.KindImage), ``)

	oneD := "LUT_1D_SIZE 4\n0 0 0\n0.3 0.3 0.3\n0.6 0.6 0.6\n1 1 1\n"
	tooBig := "LUT_3D_SIZE 66\n0 0 0\n"
	notACube := "this is not a lookup table\n"
	huge := "LUT_3D_SIZE 2\n" + strings.Repeat("# padding padding padding padding padding\n", 16<<20/42+1)

	for name, text := range map[string]string{
		"a 1D table": oneD, "a size over the limit": tooBig, "a file that is not a table": notACube,
		"a file over 16 MiB": huge, "an empty file": "",
	} {
		t.Run(name, func(t *testing.T) {
			_, bad := lutFile(t, text)

			if _, err := renderWithLUT(t, d, bad); err == nil || !strings.Contains(err.Error(), "lookup table") {
				t.Fatalf("the export was not refused for the table: %v", err)
			}
		})
	}

	t.Run("no files at all", func(t *testing.T) {
		src := sourcePNG(t)

		_, err := RenderImage(
			context.Background(),
			d,
			ImageConfig{AssetPath: func(string) (string, error) { return src, nil }},
		)
		if err == nil {
			t.Fatal("the export drew a layer without its table")
		}
	})

	t.Run("a file that is gone", func(t *testing.T) {
		if _, err := renderWithLUT(t, d, filepath.Join(filepath.Dir(path), "gone.cube")); err == nil {
			t.Fatal("the export drew a layer without its table")
		}
	})
}
