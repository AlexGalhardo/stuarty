package servicetest

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	stdpng "image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/ingest"
	"github.com/AlexGalhardo/stuarty/engine/lut"
	"github.com/AlexGalhardo/stuarty/service"
)

func cubeFile(size int, header string, rows int) string {
	var b strings.Builder

	b.WriteString("# a table\nTITLE \"grade\"\n")
	b.WriteString(header)
	fmt.Fprintf(&b, "LUT_3D_SIZE %d\n", size)

	for range rows {
		b.WriteString("0.1 0.2 0.3\n")
	}

	return b.String()
}

// TASK-0254: a .cube file is ingested as an asset of kind lut, found by its content, with no artifact.
func TestImportingACubeFileMakesALutAsset(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project := m.newProject("p")

	res := m.importOne(project, m.write("warm.cube", cubeFile(2, "DOMAIN_MIN 0 0 0\nDOMAIN_MAX 1 1 1\n", 8)))
	a := res.Assets[0]

	if a.State != "ready" || a.Kind != "lut" || res.Failed != 0 {
		t.Fatalf("entry = %+v", a)
	}

	doc, err := timeline.Decode(m.document(project))
	if err != nil {
		t.Fatal(err)
	}

	got, ok := doc.Assets[a.AssetID]
	if !ok || got.Kind != timeline.AssetLUT || got.Width != 0 || got.SampleCount != 0 {
		t.Fatalf("document asset = %+v, %v", got, ok)
	}

	list, err := m.svc.MediaList(m.as(ui), service.MediaListRequest{ProjectID: project})
	if err != nil || len(list.Assets) != 1 || list.Assets[0].Kind != "lut" {
		t.Fatalf("list = %+v, %v", list, err)
	}

	if _, err := m.svc.MediaInspect(
		m.as(ui),
		service.MediaInspectRequest{ProjectID: project, AssetID: a.AssetID},
	); err != nil {
		t.Errorf("inspect of a lut: %v", err)
	}
}

func TestImportingAnInvalidCubeFileFails(t *testing.T) {
	cases := map[string]string{
		"too few lines":    cubeFile(2, "", 7),
		"too many lines":   cubeFile(2, "", 9),
		"size too small":   cubeFile(1, "", 1),
		"size too large":   "LUT_3D_SIZE 66\n",
		"a 1D table":       "LUT_1D_SIZE 4\n0 0 0\n0 0 0\n0 0 0\n0 0 0\n",
		"no size":          "TITLE \"x\"\n0 0 0\n",
		"not a number":     "LUT_3D_SIZE 2\n" + strings.Repeat("0.1 nan 0.3\n", 8),
		"two numbers":      "LUT_3D_SIZE 2\n" + strings.Repeat("0.1 0.3\n", 8),
		"header after":     cubeFile(2, "", 8) + "TITLE \"late\"\n",
		"empty domain":     cubeFile(2, "DOMAIN_MIN 1 1 1\nDOMAIN_MAX 0 0 0\n", 8),
		"unknown keyword":  "LUT_3D_SIZE 2\nWHATEVER 1\n",
		"a very long line": "TITLE x\n# " + strings.Repeat("x", 5000) + "\nLUT_3D_SIZE 2\n",
	}

	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			m := newMediaEnv(t, mediaCfg{})
			project := m.newProject("p")

			res := m.importOne(project, m.write("bad.cube", body))
			a := res.Assets[0]

			if a.State != "failed" || a.Error == nil || a.Error.Code != "E_INGEST_FAILED" || res.Failed != 1 {
				t.Fatalf("entry = %+v", a)
			}

			if doc, _ := timeline.Decode(m.document(project)); len(doc.Assets) != 0 {
				t.Errorf("assets in the document: %v", doc.Assets)
			}
		})
	}
}

// A text file that is not a .cube table is not taken for one: the media ingest refuses it as before.
func TestATextFileIsNotACubeTable(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{realIngest: true})
	project := m.newProject("p")

	a := m.importOne(project, m.write("notes.cube", "this is a text file that is not a lookup table at all\n")).Assets[0]
	if a.State != "failed" || a.Error == nil || a.Error.Code != "E_UNSUPPORTED_MEDIA" {
		t.Fatalf("entry = %+v", a)
	}
}

// invertingCube is a 2x2x2 table that maps every colour to its complement (red varies fastest).
func invertingCube() string {
	var b strings.Builder

	b.WriteString("TITLE \"invert\"\nLUT_3D_SIZE 2\n")

	for blue := range 2 {
		for green := range 2 {
			for red := range 2 {
				fmt.Fprintf(&b, "%d %d %d\n", 1-red, 1-green, 1-blue)
			}
		}
	}

	return b.String()
}

// exportedGIFCentre exports the project as a GIF and returns the colour of the middle of its first frame.
func (m *mediaEnv) exportedGIFCentre(project string) color.Color {
	m.t.Helper()

	job, err := m.startFormat(project, "gif")
	if err != nil || job.State != service.JobSucceeded || job.Result == nil || job.Result.Export == nil {
		m.t.Fatalf("export: %v, %s", err, jobSummary(job))
	}

	f, err := os.Open(job.Result.Export.Path)
	if err != nil {
		m.t.Fatal(err)
	}

	defer func() { _ = f.Close() }()

	g, err := gif.DecodeAll(f)
	if err != nil || len(g.Image) == 0 {
		m.t.Fatalf("decode: %v", err)
	}

	b := g.Image[0].Bounds()

	return g.Image[0].At((b.Min.X+b.Max.X)/2, (b.Min.Y+b.Max.Y)/2)
}

// TASK-0254: the table a real import keeps is the one the export reads, so a clip graded with an
// imported .cube changes in the exported picture.
func TestAnImportedCubeGradesTheExport(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{realExport: true, ingest: ingest.Run})
	project, track := m.withTrack()

	m.mustEdit(ui, project, m.head(project).Revision, op("settings.set", `{"width":64,"height":48}`))

	img := image.NewRGBA(image.Rect(0, 0, 64, 48))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = 220, 40, 40, 255
	}

	var png bytes.Buffer
	if err := stdpng.Encode(&png, img); err != nil {
		t.Fatal(err)
	}

	pic := m.importOne(project, m.write("red.png", png.String())).Assets[0].AssetID
	lutID := m.importOne(project, m.write("invert.cube", invertingCube())).Assets[0].AssetID

	hash := ""

	doc, err := timeline.Decode(m.document(project))
	if err != nil {
		t.Fatal(err)
	}

	hash = strings.TrimPrefix(doc.Assets[lutID].Hash, "sha256:")

	if _, err := os.Stat(filepath.Join(m.ws, "artifacts", hash, "normalized-1.cube")); err != nil {
		t.Fatalf("the lut has no normalized artifact: %v", err)
	}

	arts, err := m.store.Assets().Artifacts(context.Background(), doc.Assets[lutID].Hash)
	if err != nil || len(arts) != 1 || arts[0].Kind != "normalized" || arts[0].Ext != "cube" {
		t.Fatalf("artifacts = %+v, %v", arts, err)
	}

	added := m.mustEdit(ui, project, m.head(project).Revision, op("clip.add", fmt.Sprintf(
		`{"track_id":%q,"asset_id":%q,"start":"0s","duration":"1s"}`, track, pic,
	)))
	clip := added.Created[0]

	plain := m.exportedGIFCentre(project)

	m.mustEdit(ui, project, m.head(project).Revision, op("clip.set_color", fmt.Sprintf(
		`{"item_id":%q,"lut_asset":%q,"lut_intensity":1}`, clip, lutID,
	)))

	graded := m.exportedGIFCentre(project)

	pr, _, _, _ := plain.RGBA()
	gr, _, _, _ := graded.RGBA()

	if pr>>8 < 150 || gr>>8 > 110 {
		t.Errorf("centre red: without the table %d, with the inverting table %d", pr>>8, gr>>8)
	}
}

// TASK-0254: the preview reads the table the server parsed, never the text of the file.
func TestOpenFileServesTheParsedTableOfALutAsset(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project := m.newProject("p")
	body := invertingCube()
	a := m.importOne(project, m.write("invert.cube", body)).Assets[0]

	f, err := m.svc.OpenFile(m.as(ui), service.FileRef{Kind: "lut", ProjectID: project, ID: a.AssetID})
	if err != nil {
		t.Fatal(err)
	}

	want, err := lut.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}

	if f.ContentType != "application/octet-stream" || !f.Immutable || !strings.HasPrefix(f.ETag, `"lut-`) {
		t.Errorf("file = %+v", f)
	}

	if len(f.Body) != 7+6*8 || string(f.Body[:4]) != "VLUT" || f.Body[4] != 1 || f.Body[5] != 2 || f.Body[6] != 0 {
		t.Fatalf("header or length of % x", f.Body)
	}

	if !bytes.Equal(f.Body[7:], want.Bytes()) || f.Size != int64(len(f.Body)) {
		t.Errorf("entries % x, want % x", f.Body[7:], want.Bytes())
	}

	// Red varies fastest: the second entry is red 1, green 0, blue 0, which the table maps to (0, 1, 1).
	if got := f.Body[7+6 : 7+12]; !bytes.Equal(got, []byte{0, 0, 0xFF, 0xFF, 0xFF, 0xFF}) {
		t.Errorf("second entry % x", got)
	}
}

func TestOpenFileRefusesTheLutOfAnAssetThatIsNotOne(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project := m.newProject("p")
	a := m.importOne(project, m.write("a.mp4", clipBytes)).Assets[0]

	_, err := m.svc.OpenFile(m.as(ui), service.FileRef{Kind: "lut", ProjectID: project, ID: a.AssetID})
	if code(err) != command.CodeNotFound {
		t.Errorf("lut of a video: %v", err)
	}
}
