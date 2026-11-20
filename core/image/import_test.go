package image

import (
	"errors"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
)

func TestImportName(t *testing.T) {
	long := strings.Repeat("é", 200)

	for _, c := range []struct{ in, want string }{
		{"face.png", "face"},
		{`C:\photos\My Face.final.JPG`, "My Face.final"},
		{"/home/me/.hidden", ".hidden"},
		{"  .png", "Untitled"},
		{"", "Untitled"},
		{long + ".png", strings.Repeat("é", 120)},
	} {
		if got := ImportName(c.in); got != c.want {
			t.Errorf("ImportName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestImportBatchMakesADocumentWithOneCentredLayer(t *testing.T) {
	src := ImportSource{Hash: hashFace, Width: 1600, Height: 900, Name: "face.png"}

	ops, err := ImportBatch(src, ImportCanvas{}, "")
	if err != nil {
		t.Fatal(err)
	}

	out, aerr := ApplyBatch(New(docID), ops, testEnv(lid('a')))
	if aerr != nil {
		t.Fatal(aerr)
	}

	d := out.Doc
	if d.Width != 1600 || d.Height != 900 || d.Name != "face" || len(d.Layers) != 1 {
		t.Fatalf("%dx%d %q %d layers", d.Width, d.Height, d.Name, len(d.Layers))
	}

	l := d.Layers[0]
	if l.Kind != KindImage || l.Asset != hashFace || l.X != 0 || l.Y != 0 || l.W != 1600 || l.H != 900 {
		t.Fatalf("layer %+v", l)
	}
}

func TestImportBatchCanvas(t *testing.T) {
	big := ImportSource{Hash: hashFace, Width: 5000, Height: 3000, Name: "big.png"}
	small := ImportSource{Hash: hashLogo, Width: 400, Height: 400, Name: "logo.png"}

	for _, c := range []struct {
		name         string
		src          ImportSource
		canvas       ImportCanvas
		w, h         int
		code, detail string
	}{
		{name: "source size", src: small, w: 400, h: 400},
		{name: "exactly the limit", src: ImportSource{Hash: hashLogo, Width: 4096, Height: 4096, Name: "x"}, w: 4096, h: 4096},
		{name: "a size wins", src: small, canvas: ImportCanvas{Width: 1280, Height: 720}, w: 1280, h: 720},
		{name: "a preset wins", src: small, canvas: ImportCanvas{Preset: "square_post"}, w: 1080, h: 1080},
		{name: "big with a preset", src: big, canvas: ImportCanvas{Preset: "video_thumbnail"}, w: 1280, h: 720},
		{name: "big with a size", src: big, canvas: ImportCanvas{Width: 2000, Height: 1200}, w: 2000, h: 1200},
		{name: "big alone", src: big, code: command.CodeLimit, detail: "canvas"},
		{name: "one side over", src: ImportSource{Hash: hashFace, Width: 4097, Height: 10}, code: command.CodeLimit},
		{
			name: "preset and size", src: small, canvas: ImportCanvas{Preset: "square_post", Width: 10, Height: 10},
			code: command.CodeValidation,
		},
		{name: "one side only", src: small, canvas: ImportCanvas{Width: 10}, code: command.CodeValidation},
		{name: "a side over", src: small, canvas: ImportCanvas{Width: 5000, Height: 10}, code: command.CodeValidation},
		{name: "unknown preset", src: small, canvas: ImportCanvas{Preset: "poster"}, code: command.CodeValidation},
		{name: "no size", src: ImportSource{Hash: hashLogo}, code: command.CodeValidation},
	} {
		t.Run(c.name, func(t *testing.T) {
			ops, err := ImportBatch(c.src, c.canvas, "")
			if c.code != "" {
				var ce *command.Error
				if !errors.As(err, &ce) || ce.Code != c.code {
					t.Fatalf("want %s, got %v", c.code, err)
				}

				if c.detail != "" && ce.Details["limit"] != c.detail {
					t.Fatalf("details %v", ce.Details)
				}

				if c.code == command.CodeLimit && ce.Hint == "" {
					t.Fatal("a limit answer carries a hint")
				}

				return
			}

			if err != nil {
				t.Fatal(err)
			}

			// the asset known to the test environment is the one the source names
			env := testEnv(lid('a'))
			env.Asset = func(string) (AssetInfo, bool) {
				return AssetInfo{Name: c.src.Name, Width: c.src.Width, Height: c.src.Height}, true
			}

			out, aerr := ApplyBatch(New(docID), ops, env)
			if aerr != nil {
				t.Fatal(aerr)
			}

			if out.Doc.Width != c.w || out.Doc.Height != c.h {
				t.Fatalf("canvas %dx%d, want %dx%d", out.Doc.Width, out.Doc.Height, c.w, c.h)
			}
		})
	}
}

func TestImportBatchTakesTheNameOfTheCaller(t *testing.T) {
	ops, err := ImportBatch(
		ImportSource{Hash: hashLogo, Width: 10, Height: 10, Name: "logo.png"},
		ImportCanvas{},
		"Cover",
	)
	if err != nil {
		t.Fatal(err)
	}

	out, aerr := ApplyBatch(New(docID), ops, testEnv(lid('a')))
	if aerr != nil || out.Doc.Name != "Cover" {
		t.Fatalf("%v %q", aerr, out.Doc.Name)
	}
}

func TestImportLayerFitsAndCentresTheSource(t *testing.T) {
	create := []command.Operation{op("image.create", `{"width": 1000, "height": 1000}`)}

	first, err := ApplyBatch(New(docID), create, testEnv())
	if err != nil {
		t.Fatal(err)
	}

	add, err := ImportLayer(hashFace, "Photo")
	if err != nil {
		t.Fatal(err)
	}

	out, aerr := ApplyBatch(first.Doc, []command.Operation{add}, testEnv(lid('b')))
	if aerr != nil {
		t.Fatal(aerr)
	}

	l := out.Doc.Layers[0]
	// 1600x900 contained in 1000x1000: 1000x562 (rounded), centred
	if l.Name != "Photo" || l.W != 1000 || l.X != 0 || l.Y < 218 || l.Y > 220 {
		t.Fatalf("layer %+v", l)
	}
}
