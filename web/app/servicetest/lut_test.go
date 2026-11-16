package servicetest

import (
	"fmt"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
	"github.com/AlexGalhardo/vukem-video/service"
)

func cubeFile(size int, header string, rows int) string {
	var b strings.Builder

	b.WriteString("# a table\nTITLE \"grade\"\n")
	b.WriteString(header)
	b.WriteString(fmt.Sprintf("LUT_3D_SIZE %d\n", size))

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

	if _, err := m.svc.MediaInspect(m.as(ui), service.MediaInspectRequest{ProjectID: project, AssetID: a.AssetID}); err != nil {
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
