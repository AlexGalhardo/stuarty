package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/engine/ingest"
)

// The lookup table of an image layer through the service (SDD-021 section 3.1, epic 49): the service resolves the
// hash of a lut asset for the document and the export reads its normalized .cube file.

const (
	lutHash  = "sha256:9999999999999999999999999999999999999999999999999999999999999999"
	clipHash = "sha256:8888888888888888888888888888888888888888888888888888888888888888"
)

// invertCube is a 2 point table that inverts the colours.
const invertCube = "TITLE \"invert\"\nLUT_3D_SIZE 2\n1 1 1\n0 1 1\n1 0 1\n0 0 1\n1 1 0\n0 1 0\n1 0 0\n0 0 0\n"

// holdLUT adds a ready lookup table to the store and its normalized file to the workspace.
func holdLUT(t *testing.T, s *svc, ws string) {
	t.Helper()

	hash, body := lutHash, invertCube
	st := s.store.(*memStore)
	st.assets.rows[hash] = AssetRow{
		Hash:         hash,
		Kind:         "lut",
		OriginalName: "/grading/invert.cube",
		IngestState:  IngestReady,
	}

	dir := filepath.Join(ws, "artifacts", strings.TrimPrefix(hash, "sha256:"))
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}

	file := filepath.Join(dir, ingest.ArtifactName("normalized", ingest.ProfileNormalized, "", "cube"))
	if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// photoImage is an image with one photo that fills the canvas.
func photoImage(ctx context.Context, t *testing.T, s *svc) string {
	t.Helper()

	c, err := s.ImageCreate(ctx, ImageCreateRequest{Width: 64, Height: 48, Background: "#101820"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.ImageEdit(ctx, ImageEditRequest{ImageID: c.ImageID, BaseRevision: 1, Operations: ops(
		"layer.add_image", `{"asset": "`+imgHash+`", "id": "lyr_aaaaaaaaaa"}`,
	)}); err != nil {
		t.Fatal(err)
	}

	return c.ImageID
}

func TestImageLUT(t *testing.T) {
	s, ws := newImageService(t)
	holdLUT(t, s, ws)

	ctx := asActor("local:agent")
	id := photoImage(ctx, t, s)

	plain, err := s.ImageExport(ctx, ImageExportRequest{ImageID: id})
	if err != nil {
		t.Fatal(err)
	}

	edit, err := s.ImageEdit(ctx, ImageEditRequest{ImageID: id, BaseRevision: 2, Operations: ops(
		"layer.adjust", `{"layer_id": "lyr_aaaaaaaaaa", "lut": {"asset": "`+lutHash+`", "intensity": 100}}`,
	)})
	if err != nil || !edit.Applied {
		t.Fatalf("%v %+v", err, edit)
	}

	if !strings.Contains(edit.Description, "adjust") {
		t.Errorf("description %q", edit.Description)
	}

	// the table is a file of the document, with its kind and no size, and the text view names it
	st := s.store.(*memStore)

	doc := st.images.rows[id].Document
	if !strings.Contains(string(doc), `"kind":"lut"`) && !strings.Contains(string(doc), `"kind": "lut"`) {
		t.Errorf("the stored document does not list the table as a lut: %s", doc)
	}

	d, err := s.ImageDescribe(ctx, ImageDescribeRequest{ImageID: id})
	if err != nil || !strings.Contains(d.Text, "lut=sha256:999999999999@100") {
		t.Fatalf("%v: the text view does not name the table: %s", err, d.Text)
	}

	graded, err := s.ImageExport(ctx, ImageExportRequest{ImageID: id})
	if err != nil {
		t.Fatal(err)
	}

	if graded.SHA256 == plain.SHA256 {
		t.Fatal("the export is the picture of the document without the table")
	}

	// the same step undone gives the first file again, and the table leaves the document
	if _, err := s.ImageUndo(ctx, ImageStepRequest{ImageID: id}); err != nil {
		t.Fatal(err)
	}

	back, err := s.ImageExport(ctx, ImageExportRequest{ImageID: id})
	if err != nil || back.SHA256 != plain.SHA256 {
		t.Fatalf("the undo did not restore the picture: %v", err)
	}

	if strings.Contains(string(st.images.rows[id].Document), "lut") {
		t.Errorf("the undone document still names the table: %s", st.images.rows[id].Document)
	}

	// the view of the editor is the same render
	if _, err := s.ImageRedo(ctx, ImageStepRequest{ImageID: id}); err != nil {
		t.Fatal(err)
	}

	frame, err := s.ImageFrame(ctx, ImageFrameRequest{ImageID: id, Width: 64})
	if err != nil || len(frame.Image) == 0 {
		t.Fatalf("%v", err)
	}
}

func TestImageLUTRefusals(t *testing.T) {
	s, ws := newImageService(t)
	holdLUT(t, s, ws)

	st := s.store.(*memStore)
	st.assets.rows[clipHash] = AssetRow{Hash: clipHash, Kind: "video", OriginalName: "a.mp4", IngestState: IngestReady}

	pending := "sha256:" + strings.Repeat("e", 64)
	st.assets.rows[pending] = AssetRow{Hash: pending, Kind: "lut", OriginalName: "b.cube", IngestState: IngestPending}

	ctx := asActor("local:agent")
	id := photoImage(ctx, t, s)

	for name, c := range map[string]struct{ hash, code string }{
		"an image":              {imgHash, command.CodeTypeMismatch},
		"a video":               {clipHash, command.CodeTypeMismatch},
		"a table not yet ready": {pending, command.CodeNotFound},
		"an unknown hash":       {"sha256:" + strings.Repeat("f", 64), command.CodeNotFound},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := s.ImageEdit(ctx, ImageEditRequest{ImageID: id, BaseRevision: 2, Operations: ops(
				"layer.adjust", `{"layer_id": "lyr_aaaaaaaaaa", "lut": {"asset": "`+c.hash+`"}}`,
			)})
			mustCode(t, err, c.code)
		})
	}

	// a table is not an image: the file that only a table can be is refused by the image operations
	_, err := s.ImageEdit(ctx, ImageEditRequest{ImageID: id, BaseRevision: 2, Operations: ops(
		"layer.add_image", `{"asset": "`+lutHash+`"}`,
	)})
	mustCode(t, err, command.CodeNotFound)
}

// The export reads the file again with the limits of engine/lut: a table that is gone or that no longer parses
// fails the export, and the layer is not drawn without it.
func TestImageLUTExportFailsWithoutTheTable(t *testing.T) {
	s, ws := newImageService(t)
	holdLUT(t, s, ws)

	ctx := asActor("local:agent")
	id := photoImage(ctx, t, s)

	if _, err := s.ImageEdit(ctx, ImageEditRequest{ImageID: id, BaseRevision: 2, Operations: ops(
		"layer.adjust", `{"layer_id": "lyr_aaaaaaaaaa", "lut": {"asset": "`+lutHash+`"}}`,
	)}); err != nil {
		t.Fatal(err)
	}

	file := filepath.Join(ws, "artifacts", strings.TrimPrefix(lutHash, "sha256:"),
		ingest.ArtifactName("normalized", ingest.ProfileNormalized, "", "cube"))

	if err := os.WriteFile(file, []byte("LUT_1D_SIZE 4\n0 0 0\n1 1 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := s.ImageExport(ctx, ImageExportRequest{ImageID: id})
	mustCode(t, err, CodeExportFailed)

	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}

	_, err = s.ImageExport(ctx, ImageExportRequest{ImageID: id})
	mustCode(t, err, CodeExportFailed)

	if _, err := s.ImageFrame(ctx, ImageFrameRequest{ImageID: id, Width: 64}); err == nil {
		t.Fatal("the picture of the editor was drawn without the table")
	}
}

// In network mode a table can only be used by a user who holds it, as an image can only be used by one who holds it.
func TestSecurity_T93_LookupTableOfAnotherUserIsNotUsable(t *testing.T) {
	s, ws := newImageService(t)
	holdLUT(t, s, ws)
	holdAsset(s, "carol")

	st := s.store.(*memStore)
	st.projects.heads = append(st.projects.heads, ProjectHead{ID: "prj_bob", OwnerUserID: new("bob")})
	st.assets.projects["prj_carol"] = append(st.assets.projects["prj_carol"], ProjectAssetRow{
		ProjectID: "prj_carol", AssetHash: lutHash, Name: "invert.cube",
	})

	add := ops("layer.add_image", `{"asset": "`+imgHash+`", "id": "lyr_aaaaaaaaaa"}`)
	adjust := ops("layer.adjust", `{"layer_id": "lyr_aaaaaaaaaa", "lut": {"asset": "`+lutHash+`"}}`)

	carol, bob := networkUser("carol"), networkUser("bob")

	ca, err := s.ImageCreate(carol, ImageCreateRequest{Width: 100, Height: 100})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.ImageEdit(
		carol,
		ImageEditRequest{ImageID: ca.ImageID, BaseRevision: 1, Operations: add},
	); err != nil {
		t.Fatal(err)
	}

	if _, err := s.ImageEdit(
		carol,
		ImageEditRequest{ImageID: ca.ImageID, BaseRevision: 2, Operations: adjust},
	); err != nil {
		t.Fatalf("the user who holds the table may use it: %v", err)
	}

	cb, err := s.ImageCreate(bob, ImageCreateRequest{Width: 100, Height: 100})
	if err != nil {
		t.Fatal(err)
	}

	own := ops("layer.add_shape", `{"shape": "rect", "w": 50, "h": 50, "fill": "#FF0000", "id": "lyr_aaaaaaaaaa"}`)
	if _, err := s.ImageEdit(bob, ImageEditRequest{ImageID: cb.ImageID, BaseRevision: 1, Operations: own}); err != nil {
		t.Fatal(err)
	}

	_, err = s.ImageEdit(bob, ImageEditRequest{ImageID: cb.ImageID, BaseRevision: 2, Operations: adjust})
	if se := mustCode(t, err, command.CodeNotFound); strings.Contains(se.Message+se.Hint, "invert.cube") {
		t.Fatalf("the name of a file of another user is not returned: %+v", se)
	}

	// a document that names the table already (a template, an old revision) does not export it for another user
	st.images.rows[cb.ImageID].Document = st.images.rows[ca.ImageID].Document

	if _, err := s.ImageExport(bob, ImageExportRequest{ImageID: cb.ImageID}); err == nil {
		t.Fatal("bob exported with the table of carol")
	}
}

// TestSecurity_T94_LookupTableIsNamedByHashNeverByPath: a table is named by a content hash only. A path, or a hash
// with a separator, is refused when the key is set and again when the path of the artifact is built, and the name
// of the file (a label) never reaches the path.
func TestSecurity_T94_LookupTableIsNamedByHashNeverByPath(t *testing.T) {
	s, ws := newImageService(t)
	holdLUT(t, s, ws)

	ctx := asActor("local:agent")
	id := photoImage(ctx, t, s)

	for _, name := range []string{
		"../../etc/passwd", `C:\x.cube`, "sha256:../../../x", "sha256:" + strings.Repeat("9", 63) + "/",
		"sha256:" + strings.Repeat("9", 65), lutHash + "/..", "",
	} {
		_, err := s.ImageEdit(ctx, ImageEditRequest{ImageID: id, BaseRevision: 2, Operations: ops(
			"layer.adjust", `{"layer_id": "lyr_aaaaaaaaaa", "lut": {"asset": `+strconvQuote(name)+`}}`,
		)})
		if err == nil {
			t.Errorf("%q was accepted as the name of a table", name)
		}
	}

	// a name of 64 characters that climbs out of artifacts/ to a table that is really there: only the digits of a
	// hash may become a folder name, so its length alone is not enough
	outside := strings.Repeat("o", 61)
	escape := "sha256:../" + outside
	planted := filepath.Join(ws, outside)

	if err := os.MkdirAll(planted, 0o750); err != nil {
		t.Fatal(err)
	}

	cube := ingest.ArtifactName("normalized", ingest.ProfileNormalized, "", "cube")
	if err := os.WriteFile(filepath.Join(planted, cube), []byte(invertCube), 0o600); err != nil {
		t.Fatal(err)
	}

	path := s.imageLUTPath(ctx, Caller{Actor: "local:agent"})
	for _, name := range []string{
		"../../etc/passwd", "sha256:../x", "sha256:" + strings.Repeat("9", 65), escape,
	} {
		if p, err := path(name); err == nil {
			t.Errorf("the path of %q was built: %s", name, p)
		}
	}

	// the file is named by a path of its own: only its base name is a label, and the artifact is found by hash
	st := s.store.(*memStore)
	row := st.assets.rows[lutHash]
	row.OriginalName = "../../evil.cube"
	st.assets.rows[lutHash] = row

	if _, err := s.ImageEdit(ctx, ImageEditRequest{ImageID: id, BaseRevision: 2, Operations: ops(
		"layer.adjust", `{"layer_id": "lyr_aaaaaaaaaa", "lut": {"asset": "`+lutHash+`"}}`,
	)}); err != nil {
		t.Fatal(err)
	}

	if _, err := s.ImageExport(ctx, ImageExportRequest{ImageID: id}); err != nil {
		t.Fatalf("a label that is a path stopped a valid table: %v", err)
	}

	if strings.Contains(string(st.images.rows[id].Document), "..") {
		t.Errorf("the stored document keeps the path of the label: %s", st.images.rows[id].Document)
	}
}

func strconvQuote(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}

	return string(b)
}
