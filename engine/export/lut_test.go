package export

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
	"github.com/AlexGalhardo/vukem-video/engine/ingest"
	"github.com/AlexGalhardo/vukem-video/engine/lut"
)

// The lookup-table cases of SDD-004 section 3.6. A lut asset is a .cube file kept as the normalized artifact of
// its hash; the fixtures write one directly, because the ingest of a .cube file is not the subject here.

// lutFixture is the text of a generated table: "warm" is a 17 point table that warms and lifts, "teal" a 9 point
// table that mixes the channels. The values come from integer arithmetic printed with six decimals, so the file is
// the same on every machine.
func lutFixture(kind string) string {
	var sb strings.Builder

	n := map[string]int{"warm": 17, "teal": 9}[kind]
	fmt.Fprintf(&sb, "# generated for the golden cases\nTITLE %q\nLUT_3D_SIZE %d\n", kind, n)

	for b := range n {
		for g := range n {
			for r := range n {
				var o [3]int

				switch kind {
				case "warm":
					o = [3]int{
						min(r*11/10+n/10, n-1) * 1000000 / (n - 1),
						g * 1000000 / (n - 1),
						b * 85 / 100 * 1000000 / (n - 1),
					}
				default:
					o = [3]int{
						(7*g + 3*b) * 100000 / (n - 1),
						(8*r + 2*g) * 100000 / (n - 1),
						(5*r + 5*b) * 100000 / (n - 1),
					}
				}

				fmt.Fprintf(&sb, "%d.%06d %d.%06d %d.%06d\n",
					o[0]/1000000, o[0]%1000000, o[1]/1000000, o[1]%1000000, o[2]/1000000, o[2]%1000000)
			}
		}
	}

	return sb.String()
}

// useLUT writes the table of the spec as the normalized artifact of a lut asset and points the asset at it.
func useLUT(tb testing.TB, doc *timeline.Document, id string, spec fixtureSpec) {
	tb.Helper()
	setup(tb)

	text := lutFixture(spec.LUT)
	sum := sha256.Sum256([]byte(text))
	hash := hex.EncodeToString(sum[:])

	dir := filepath.Join(artifactsDir(), hash)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		tb.Fatal(err)
	}

	file := filepath.Join(dir, ingest.ArtifactName("normalized", ingest.ProfileNormalized, "", lutExt))
	if err := os.WriteFile(file, []byte(text), 0o600); err != nil {
		tb.Fatal(err)
	}

	a := doc.Assets[id]
	a.Hash = "sha256:" + hash
	doc.Assets[id] = a
}

// lutAsset adds a lut asset for the fixture.
func (b *caseBuilder) lutAsset(kind string) string {
	b.assets++
	id := fmt.Sprintf("ast_a%09d", b.assets)
	b.doc.Assets[id] = timeline.Asset{
		ID: id, Hash: "sha256:" + strings.Repeat("0", 64), Kind: timeline.AssetLUT, Name: kind + ".cube",
	}
	b.fixtures[id] = fixtureSpec{Name: kind + ".cube", LUT: kind}

	return id
}

// lutCase draws a table on a layer that stays and on a picture in picture that slides and rotates, both from the
// same clip, like the grade cases.
func lutCase(name, kind string, intensity float64, grade func(*timeline.Item)) alphaCase {
	b := newCase(name, rate(30, 1))
	asset, table := b.video(portrait), b.lutAsset(kind)

	set := func(it *timeline.Item) {
		it.Lut = &timeline.Lut{Asset: table, Intensity: intensity}
		if grade != nil {
			grade(it)
		}
	}

	b.clip(b.track(), asset, 0, 30, set)
	b.clip(b.track(), asset, 0, 30, func(it *timeline.Item) {
		set(it)
		it.Scale, it.Rotation = 0.5, 12
		it.Keyframes = timeline.Keyframes{"x": {timeline.NewKeyframe(0, 40), timeline.NewKeyframe(29, 140)}}
	})

	return b.build(0, 15, 29)
}

// The fixture text is a valid table of the sizes it claims, so the golden cases test the draw and not the parser.
func TestLUTFixturesAreValidTables(t *testing.T) {
	for kind, size := range map[string]int{"warm": 17, "teal": 9} {
		tb, err := lut.Parse(strings.NewReader(lutFixture(kind)))
		if err != nil || tb.Size != size {
			t.Errorf("%s: %v, size %v", kind, err, tb)
		}
	}
}

// The Rust path of the export uploads the table a layer names to each band's module and releases it after the frame
// (rustRenderer.frameLUTs): the frames it draws are the Go compositor's, in one band and in many.
func TestRustExportDrawsTheLookupTableLikeTheGoCompositor(t *testing.T) {
	setup(t)

	dir := goldenDir("lut-with-grade")
	doc := loadDir(t, dir)
	frames := readFrames(t, dir)
	want := renderDoc(t, doc, "lut-go", 1, frames)

	for _, bands := range []int{1, 8} {
		job := newJob(t, doc, fmt.Sprintf("lut-rust-%d", bands), "unused.mp4")
		job.PixelRenderer = PixelRendererRust
		s := testState(t, job, job.JobID)
		rd := s.newRenderer(bands)

		dst := image.NewRGBA(image.Rect(0, 0, doc.Settings.Width, doc.Settings.Height))

		for i, f := range frames {
			if err := rd.render(dst, f); err != nil {
				rd.close()
				t.Fatalf("%d bands, frame %d: %v", bands, f, err)
			}

			if !bytes.Equal(dst.Pix, want[i]) {
				t.Errorf("%d bands, frame %d: the Rust export differs from the Go compositor", bands, f)
			}
		}

		rd.close()
	}
}
