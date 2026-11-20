package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	assetlib "github.com/AlexGalhardo/stuarty/service/assets"
)

const (
	videoHash = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	bigHash   = "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	slowHash  = "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	badHash   = "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
)

// withImportAssets adds to the prepared workspace a video, a 5000 by 3000 picture, a file still being ingested
// and one whose ingest failed.
func withImportAssets(s *svc) *memStore {
	st := s.store.(*memStore)
	big, bigH, vw, vh := 5000, 3000, 640, 360

	st.assets.rows[videoHash] = AssetRow{
		Hash: videoHash, Kind: "video", OriginalName: "clip.mp4", IngestState: IngestReady,
		Facts: AssetFacts{Width: &vw, Height: &vh},
	}
	st.assets.rows[bigHash] = AssetRow{
		Hash: bigHash, Kind: "image", OriginalName: "/home/me/poster.tiff", IngestState: IngestReady,
		Facts: AssetFacts{Width: &big, Height: &bigH},
	}
	st.assets.rows[slowHash] = AssetRow{
		Hash:         slowHash,
		Kind:         "",
		OriginalName: "slow.png",
		IngestState:  IngestIngesting,
	}
	st.assets.rows[badHash] = AssetRow{
		Hash: badHash, OriginalName: "bad.png", IngestState: IngestFailed,
		IngestError: json.RawMessage(`{"code":"E_INGEST_FAILED","message":"the file could not be decoded."}`),
	}

	return st
}

// TestImageImportFromAHash is acceptance 1 at the service: the document has one image layer, fitted and
// centred, whose asset is the content hash of the file, and the canvas is the size of the file.
func TestImageImportFromAHash(t *testing.T) {
	s, _ := newImageService(t)
	ctx := asActor("local:cli")

	res, err := s.ImageImport(ctx, ImageImportRequest{Hash: imgHash})
	if err != nil {
		t.Fatal(err)
	}

	if res.State != ImageImportReady || !res.Created || !res.Applied || res.Revision != 1 || res.Width != 64 ||
		res.Height != 48 || res.LayerID == "" || res.Hash != imgHash {
		t.Fatalf("%+v", res)
	}

	d, err := s.ImageDescribe(ctx, ImageDescribeRequest{ImageID: res.ImageID})
	if err != nil {
		t.Fatal(err)
	}

	if d.Name != "face" || d.Width != 64 || d.Height != 48 || len(d.Layers) != 1 {
		t.Fatalf("%+v", d)
	}

	l := d.Layers[0]
	if l.Kind != "image" || l.Asset != imgHash || l.X != 0 || l.Y != 0 || l.W != 64 || l.H != 48 ||
		l.ID != res.LayerID {
		t.Fatalf("layer %+v", l)
	}

	if !strings.Contains(res.Text, res.ImageID) {
		t.Fatalf("the text names the document: %q", res.Text)
	}
}

func TestImageImportTakesTheCanvasAndTheNameOfTheCaller(t *testing.T) {
	s, _ := newImageService(t)
	withImportAssets(s)
	ctx := asActor("local:cli")

	res, err := s.ImageImport(ctx, ImageImportRequest{Hash: imgHash, Preset: "video_thumbnail", Name: "Cover"})
	if err != nil || res.Width != 1280 || res.Height != 720 {
		t.Fatalf("%v %+v", err, res)
	}

	d, _ := s.ImageDescribe(ctx, ImageDescribeRequest{ImageID: res.ImageID})
	if d.Name != "Cover" || len(d.Layers) != 1 || d.Layers[0].W != 960 || d.Layers[0].H != 720 || d.Layers[0].X != 160 {
		t.Fatalf("the 64x48 picture is contained and centred in 1280x720: %+v", d)
	}

	big, err := s.ImageImport(ctx, ImageImportRequest{Hash: bigHash, Width: 2000, Height: 1200})
	if err != nil || big.Width != 2000 || big.Height != 1200 {
		t.Fatalf("%v %+v", err, big)
	}
}

func TestImageImportIntoAnImage(t *testing.T) {
	s, _ := newImageService(t)
	ctx := asActor("local:cli")

	made, err := s.ImageCreate(ctx, ImageCreateRequest{Preset: "square_post", Name: "Post"})
	if err != nil {
		t.Fatal(err)
	}

	req := ImageImportRequest{
		Hash: imgHash, ImageID: made.ImageID, BaseRevision: 1, Name: "Portrait", IdempotencyKey: "import-key-1",
	}

	res, err := s.ImageImport(ctx, req)
	if err != nil {
		t.Fatal(err)
	}

	if res.Created || !res.Applied || res.Revision != 2 || res.LayerID == "" || res.ImageID != made.ImageID {
		t.Fatalf("%+v", res)
	}

	again, err := s.ImageImport(ctx, req)
	if err != nil || !again.Replayed || again.Revision != 2 {
		t.Fatalf("the same key is replayed, not applied twice: %v %+v", err, again)
	}

	d, _ := s.ImageDescribe(ctx, ImageDescribeRequest{ImageID: made.ImageID})
	if d.Revision != 2 || len(d.Layers) != 1 || d.Layers[0].Name != "Portrait" || d.Layers[0].Asset != imgHash {
		t.Fatalf("%+v", d)
	}

	dry, err := s.ImageImport(
		ctx,
		ImageImportRequest{Hash: imgHash, ImageID: made.ImageID, BaseRevision: 2, DryRun: true},
	)
	if err != nil || dry.Applied || !dry.DryRun || dry.Revision != 3 {
		t.Fatalf("%v %+v", err, dry)
	}

	if d, _ = s.ImageDescribe(ctx, ImageDescribeRequest{ImageID: made.ImageID}); d.Revision != 2 {
		t.Fatalf("a dry run writes nothing: r%d", d.Revision)
	}
}

func TestImageImportDryRunOfANewDocumentWritesNothing(t *testing.T) {
	s, _ := newImageService(t)

	res, err := s.ImageImport(asActor("local:cli"), ImageImportRequest{Hash: imgHash, DryRun: true})
	if err != nil || !res.DryRun || res.Applied || res.ImageID != "" || res.Width != 64 {
		t.Fatalf("%v %+v", err, res)
	}

	if n := len(s.store.(*memStore).images.rows); n != 0 {
		t.Fatalf("a dry run made %d documents", n)
	}
}

// The refusals of acceptance 2 and of the rules of SDD-021 section 10.1, one row each.
var importRefusals = []struct {
	name string
	req  ImageImportRequest
	code string
	path string
}{
	{"neither a path nor a hash", ImageImportRequest{}, command.CodeValidation, "path"},
	{"a path and a hash", ImageImportRequest{Path: "/a.png", Hash: imgHash}, command.CodeValidation, "path"},
	{"a hash that is not one", ImageImportRequest{Hash: "sha256:zz"}, command.CodeValidation, "hash"},
	{"an unknown hash", ImageImportRequest{Hash: "sha256:" + strings.Repeat("1", 64)}, command.CodeValidation, "hash"},
	{"a file that is not an image", ImageImportRequest{Hash: videoHash}, "E_UNSUPPORTED_MEDIA", ""},
	{"a source over the canvas with no size", ImageImportRequest{Hash: bigHash}, command.CodeLimit, ""},
	{
		"a size with a preset",
		ImageImportRequest{Hash: imgHash, Preset: "square_post", Width: 10, Height: 10},
		command.CodeValidation, "preset",
	},
	{"one side of a size", ImageImportRequest{Hash: imgHash, Width: 10}, command.CodeValidation, "width"},
	{"an unknown preset", ImageImportRequest{Hash: imgHash, Preset: "poster"}, command.CodeValidation, "preset"},
	{
		"a key with no image",
		ImageImportRequest{Hash: imgHash, IdempotencyKey: "import-key-1"},
		command.CodeValidation, "idempotency_key",
	},
	{
		"a base revision with no image",
		ImageImportRequest{Hash: imgHash, BaseRevision: 3},
		command.CodeValidation, "idempotency_key",
	},
	{"a negative wait", ImageImportRequest{Hash: imgHash, WaitMS: -1}, command.CodeValidation, "wait_ms"},
	{"a wait over the limit", ImageImportRequest{Hash: imgHash, WaitMS: 25001}, command.CodeValidation, "wait_ms"},
	{"a file whose ingest failed", ImageImportRequest{Hash: badHash}, "E_INGEST_FAILED", ""},
	{
		"an image that does not exist",
		ImageImportRequest{Hash: imgHash, ImageID: "img_zzzzzzzzzz"},
		command.CodeNotFound, "",
	},
	{"an id that is not one", ImageImportRequest{Hash: imgHash, ImageID: "nope"}, command.CodeValidation, "image_id"},
	{
		"a size for a layer",
		ImageImportRequest{Hash: imgHash, ImageID: "img_zzzzzzzzzz", Width: 10, Height: 10},
		command.CodeValidation, "width",
	},
}

// TestImageImportIsAtomic is acceptance 2: a refused import, whatever the reason, creates no document, writes no
// log row and leaves the documents that exist as they were.
func TestImageImportIsAtomic(t *testing.T) {
	for _, c := range importRefusals {
		t.Run(c.name, func(t *testing.T) {
			s, _ := newImageService(t)
			st := withImportAssets(s)
			ctx := asActor("local:cli")

			if _, err := s.ImageCreate(ctx, ImageCreateRequest{Width: 10, Height: 10}); err != nil {
				t.Fatal(err)
			}

			ops0 := len(st.images.ops)

			res, err := s.ImageImport(ctx, c.req)
			if res != nil {
				t.Fatalf("a refusal returns no result: %+v", res)
			}

			if se := mustCode(t, err, c.code); c.path != "" && se.Path != c.path {
				t.Fatalf("path %q, want %q: %v", se.Path, c.path, se)
			}

			if n := len(st.images.rows); n != 1 || len(st.images.ops) != ops0 {
				t.Fatalf("%d documents, %d logs after a refusal", n, len(st.images.ops))
			}

			for _, r := range st.images.rows {
				if r.Revision != 1 {
					t.Fatalf("a document was changed by a refused import: r%d", r.Revision)
				}
			}
		})
	}
}

func TestImageImportOfAFileStillIngestingIsPending(t *testing.T) {
	s, _ := newImageService(t)
	st := withImportAssets(s)

	res, err := s.ImageImport(asActor("local:cli"), ImageImportRequest{Hash: slowHash, WaitMS: 1})
	if err != nil {
		t.Fatal(err)
	}

	if res.State != ImageImportPending || res.Hash != slowHash || res.ImageID != "" || res.Created || res.Applied {
		t.Fatalf("%+v", res)
	}

	if len(st.images.rows) != 0 {
		t.Fatal("a pending import made a document")
	}

	// the file becomes ready: the same hash, imported again, makes the document
	w, h := 30, 20
	row := st.assets.rows[slowHash]
	row.Kind, row.IngestState, row.Facts = "image", IngestReady, AssetFacts{Width: &w, Height: &h}
	st.assets.rows[slowHash] = row

	res, err = s.ImageImport(asActor("local:cli"), ImageImportRequest{Hash: slowHash})
	if err != nil || res.State != ImageImportReady || res.Width != 30 || !res.Created {
		t.Fatalf("%v %+v", err, res)
	}
}

func TestImageImportRefusesAProjectLimitedKey(t *testing.T) {
	s, _ := newImageService(t)
	limited := WithCaller(context.Background(), Caller{
		Actor: "key:k", ProjectID: "prj_aaaaaaaaaa", Scopes: []string{ScopeRead, ScopeEdit},
	})

	_, err := s.ImageImport(limited, ImageImportRequest{Hash: imgHash})
	mustCode(t, err, CodeForbidden)
}

func TestImageImportNeedsTheEditScope(t *testing.T) {
	s, _ := newImageService(t)
	readOnly := WithCaller(context.Background(), Caller{Actor: "key:k", Scopes: []string{ScopeRead}})

	_, err := s.ImageImport(readOnly, ImageImportRequest{Hash: imgHash})
	mustCode(t, err, CodeForbidden)
}

// In network mode the file must be the caller own: held by one of their projects, or imported by them straight
// into an image (their library row). A hash another user holds is as unknown as one that does not exist, and
// the owner of the new document is the caller.
func TestImageImportInNetworkMode(t *testing.T) {
	s, _ := newImageService(t)
	st := withImportAssets(s)
	holdAsset(s, "alice")

	alice, bob := networkUser("alice"), networkUser("bob")

	res, err := s.ImageImport(alice, ImageImportRequest{Hash: imgHash})
	if err != nil {
		t.Fatalf("the user who holds the file may use it: %v", err)
	}

	if row := st.images.rows[res.ImageID]; row.OwnerUserID == nil || *row.OwnerUserID != "alice" {
		t.Fatalf("the owner is the caller: %+v", row.OwnerUserID)
	}

	d, _ := s.ImageDescribe(alice, ImageDescribeRequest{ImageID: res.ImageID})
	if len(d.Layers) != 1 || d.Layers[0].Name != "face" && d.Layers[0].Name != "face.png" {
		t.Fatalf("%+v", d.Layers)
	}

	_, err = s.ImageImport(bob, ImageImportRequest{Hash: imgHash})
	if se := mustCode(t, err, command.CodeValidation); se.Path != "hash" || strings.Contains(se.Message, "face") {
		t.Fatalf("another user's file is unknown, and its name is not returned: %+v", se)
	}

	_, err = s.ImageImport(bob, ImageImportRequest{Hash: imgHash, ImageID: res.ImageID, BaseRevision: 1})
	mustCode(t, err, command.CodeNotFound)

	// bob imported the file himself (a library row, no project): he may use it, under a neutral name
	st.library.rows["bob|"+imgHash] = assetlib.Row{Owner: "bob", Hash: imgHash}

	got, err := s.ImageImport(bob, ImageImportRequest{Hash: imgHash})
	if err != nil {
		t.Fatalf("the user who imported the file may use it: %v", err)
	}

	if bd, _ := s.ImageDescribe(bob, ImageDescribeRequest{ImageID: got.ImageID}); len(bd.Layers) != 1 {
		t.Fatalf("%+v", bd)
	}

	// and alice cannot see bob's document
	_, err = s.ImageDescribe(alice, ImageDescribeRequest{ImageID: got.ImageID})
	mustCode(t, err, command.CodeNotFound)
}
