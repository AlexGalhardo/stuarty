package servicetest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/engine/ingest"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/pathguard"
)

// A file that enters the content store through ImageImport is read, ingested and made a layer (SDD-021
// section 10.1) on the real store, the real content store and the real path guard. The ingest is replaced by one
// that answers an image of 64 by 48 pixels for any file, so these tests need no FFmpeg; the one that runs the
// real ingest is TestImageImportRunsTheRealIngest.

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()

	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}

	return buf.Bytes()
}

func fileHash(data []byte) string {
	sum := sha256.Sum256(data)

	return "sha256:" + hex.EncodeToString(sum[:])
}

// pictureIngest answers a ready image of w by h for any file.
func pictureIngest(w, h int) service.Ingester {
	return func(context.Context, ingest.Input) (*ingest.Result, error) {
		return &ingest.Result{
			Kind: ingest.KindImage, Summary: &ingest.Summary{Version: 1, Kind: ingest.KindImage},
			Facts: ingest.Facts{Width: w, Height: h},
		}, nil
	}
}

func imageCount(t *testing.T, m *mediaEnv) int {
	t.Helper()

	res, err := m.svc.ImageList(m.as(ui), service.ImageListRequest{})
	if err != nil {
		t.Fatal(err)
	}

	return len(res.Images)
}

// TestImageImportFromAPath is acceptance 1: the document has one image layer whose asset is the content hash of
// the file, fitted and centred, and a second import of the same bytes is deduplicated into the same hash.
func TestImageImportFromAPath(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{ingest: pictureIngest(64, 48)})
	data := pngBytes(t, 64, 48)
	path := m.write("Holiday Photo.png", string(data))

	res, err := m.svc.ImageImport(m.as(ui), service.ImageImportRequest{Path: path})
	if err != nil {
		t.Fatal(err)
	}

	if res.State != service.ImageImportReady || !res.Created || res.Revision != 1 || res.Hash != fileHash(data) {
		t.Fatalf("%+v", res)
	}

	d, err := m.svc.ImageDescribe(m.as(ui), service.ImageDescribeRequest{ImageID: res.ImageID})
	if err != nil {
		t.Fatal(err)
	}

	if d.Name != "Holiday Photo" || d.Width != 64 || d.Height != 48 || len(d.Layers) != 1 {
		t.Fatalf("%+v", d)
	}

	if l := d.Layers[0]; l.Kind != "image" || l.Asset != fileHash(data) || l.W != 64 || l.H != 48 || l.X != 0 ||
		l.Y != 0 {
		t.Fatalf("layer %+v", l)
	}

	again, err := m.svc.ImageImport(m.as(ui), service.ImageImportRequest{Path: path, Preset: "square_post"})
	if err != nil || again.Hash != res.Hash || again.ImageID == res.ImageID || again.Width != 1080 {
		t.Fatalf("%v %+v", err, again)
	}

	if m.ingests.Load() != 0 || imageCount(t, m) != 2 {
		t.Fatalf("two documents from one file: %d", imageCount(t, m))
	}

	// the same file, now by its hash, into the first document
	into, err := m.svc.ImageImport(m.as(ui), service.ImageImportRequest{
		Hash: res.Hash, ImageID: res.ImageID, BaseRevision: 1, Name: "Again",
	})
	if err != nil || into.Created || into.Revision != 2 || into.LayerID == "" {
		t.Fatalf("%v %+v", err, into)
	}
}

func TestImageImportIntoAnImageFromAPath(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{ingest: pictureIngest(64, 48)})

	made, err := m.svc.ImageCreate(m.as(ui), service.ImageCreateRequest{Preset: "video_thumbnail"})
	if err != nil {
		t.Fatal(err)
	}

	res, err := m.svc.ImageImport(m.as(agent), service.ImageImportRequest{
		Path: m.write("logo.png", string(pngBytes(t, 64, 48))), ImageID: made.ImageID, BaseRevision: 1,
	})
	if err != nil || res.Created || !res.Applied || res.Revision != 2 {
		t.Fatalf("%v %+v", err, res)
	}

	d, _ := m.svc.ImageDescribe(m.as(ui), service.ImageDescribeRequest{ImageID: made.ImageID})
	if len(d.Layers) != 1 || d.Layers[0].Name != "logo.png" {
		t.Fatalf("the layer carries the name of the file: %+v", d.Layers)
	}

	if imageCount(t, m) != 1 {
		t.Fatal("a file put in a document made another one")
	}
}

// TestImageImportIsAtomicOverTheRealStore is acceptance 2 with the real store: whatever refuses the import, no
// document is made.
func TestImageImportIsAtomicOverTheRealStore(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		name string
		cfg  mediaCfg
		path func(m *mediaEnv) string
		req  func(path string) service.ImageImportRequest
		code string
	}{
		{
			name: "a file that is not an image (the ingest says a video)",
			cfg:  mediaCfg{},
			path: func(m *mediaEnv) string { return m.write("clip.mp4", clipBytes) },
			code: "E_UNSUPPORTED_MEDIA",
		},
		{
			name: "text that is not media at all (the real ingest)",
			cfg:  mediaCfg{realIngest: true},
			path: func(m *mediaEnv) string { return m.write("notes.png", "this is not a picture, only text") },
			code: "E_UNSUPPORTED_MEDIA",
		},
		{
			name: "a file over the import limit",
			cfg:  mediaCfg{ingest: pictureIngest(64, 48), limits: service.Limits{MaxImportBytes: 64}},
			path: func(m *mediaEnv) string { return m.write("big.png", strings.Repeat("x", 200)) },
			code: "E_LIMIT_EXCEEDED",
		},
		{
			name: "an empty file",
			cfg:  mediaCfg{ingest: pictureIngest(64, 48)},
			path: func(m *mediaEnv) string { return m.write("empty.png", "") },
			code: "E_UNSUPPORTED_MEDIA",
		},
		{
			name: "a path that does not exist",
			cfg:  mediaCfg{ingest: pictureIngest(64, 48)},
			path: func(m *mediaEnv) string { return filepath.Join(m.files, "missing.png") },
			code: pathguard.CodeFileNotFound,
		},
		{
			name: "a path that is not absolute",
			cfg:  mediaCfg{ingest: pictureIngest(64, 48)},
			path: func(*mediaEnv) string { return "photos/a.png" },
			code: pathguard.CodePathNotAllowed,
		},
		{
			name: "a path outside the import roots",
			cfg: mediaCfg{
				ingest: pictureIngest(64, 48), guard: &pathguard.Config{Network: true, Roots: []string{root}},
			},
			path: func(m *mediaEnv) string { return m.write("outside.png", "x") },
			code: pathguard.CodePathNotAllowed,
		},
		{
			name: "a source over the canvas with no size",
			cfg:  mediaCfg{ingest: pictureIngest(5000, 3000)},
			path: func(m *mediaEnv) string { return m.write("huge.png", string(pngBytes(t, 8, 8))) },
			code: "E_LIMIT",
		},
		{
			name: "an unknown preset",
			cfg:  mediaCfg{ingest: pictureIngest(64, 48)},
			path: func(m *mediaEnv) string { return m.write("a.png", string(pngBytes(t, 8, 8))) },
			req: func(p string) service.ImageImportRequest {
				return service.ImageImportRequest{Path: p, Preset: "poster"}
			},
			code: "E_VALIDATION",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := newMediaEnv(t, c.cfg)
			path := c.path(m)

			req := service.ImageImportRequest{Path: path}
			if c.req != nil {
				req = c.req(path)
			}

			res, err := m.svc.ImageImport(m.as(ui), req)
			if res != nil {
				t.Fatalf("a refusal returns no result: %+v", res)
			}

			if got := code(err); got != c.code {
				t.Fatalf("code %q, want %q: %v", got, c.code, err)
			}

			if n := imageCount(t, m); n != 0 {
				t.Fatalf("%d documents after a refused import", n)
			}
		})
	}
}

// A path in the import roots of a network server is read; the document belongs to the caller, who may use the
// file in it even though no project holds it, and another user may not.
func TestImageImportInNetworkModeBelongsToTheCaller(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	m := newMediaEnv(t, mediaCfg{
		ingest: pictureIngest(64, 48), guard: &pathguard.Config{Network: true, Roots: []string{root}},
	})

	file := filepath.Join(root, "face.png")
	if err := os.WriteFile(file, pngBytes(t, 8, 8), 0o600); err != nil {
		t.Fatal(err)
	}

	member := func(user string) service.Caller {
		return service.Caller{
			Actor: "user:" + user, Scopes: service.RoleScopes(service.RoleMember), OwnerUserID: user,
		}
	}
	alice, bob := service.WithCaller(t.Context(), member("alice")), service.WithCaller(t.Context(), member("bob"))

	res, err := m.svc.ImageImport(alice, service.ImageImportRequest{Path: file})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := m.svc.ImageDescribe(alice, service.ImageDescribeRequest{ImageID: res.ImageID}); err != nil {
		t.Fatal(err)
	}

	// she can put the same file in a second document by its hash, and export it
	second, err := m.svc.ImageImport(alice, service.ImageImportRequest{Hash: res.Hash, Preset: "square_post"})
	if err != nil {
		t.Fatalf("the user who imported the file may use its hash: %v", err)
	}

	if _, err := m.svc.ImageDescribe(
		bob,
		service.ImageDescribeRequest{ImageID: second.ImageID},
	); code(
		err,
	) != "E_NOT_FOUND" {
		t.Fatalf("another user does not see the document: %v", err)
	}

	_, err = m.svc.ImageImport(bob, service.ImageImportRequest{Hash: res.Hash})
	if se := serviceError(t, err); se.Code != "E_VALIDATION" || se.Path != "hash" {
		t.Fatalf("another user's hash is as unknown as one that does not exist: %+v", se)
	}
}

// A file whose ingest is still running when the call stops waiting makes no document; the hash imported again
// after the ingest makes it.
func TestImageImportWhilstTheIngestRuns(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{ingest: pictureIngest(64, 48), noQueue: true})

	res, err := m.svc.ImageImport(m.as(ui), service.ImageImportRequest{
		Path: m.write("slow.png", string(pngBytes(t, 8, 8))), WaitMS: 30,
	})
	if err != nil {
		t.Fatal(err)
	}

	if res.State != service.ImageImportPending || res.Hash == "" || res.ImageID != "" || imageCount(t, m) != 0 {
		t.Fatalf("%+v", res)
	}
}

func TestImageImportDryRunReadsAndWritesNothing(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{ingest: pictureIngest(64, 48)})

	res, err := m.svc.ImageImport(m.as(ui), service.ImageImportRequest{
		Path: m.write("a.png", string(pngBytes(t, 8, 8))), DryRun: true,
	})
	if err != nil || !res.DryRun || res.ImageID != "" || imageCount(t, m) != 0 {
		t.Fatalf("%v %+v", err, res)
	}

	if got, _ := os.ReadDir(filepath.Join(string(m.dir), "store")); len(got) != 0 {
		t.Fatalf("a dry run copied the file into the store: %v", got)
	}
}

// TestImageImportRunsTheRealIngest brings a real PNG through the real ingest and the pinned FFmpeg.
func TestImageImportRunsTheRealIngest(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{realExport: true, ingest: ingest.Run})
	data := pngBytes(t, 120, 80)

	res, err := m.svc.ImageImport(m.as(ui), service.ImageImportRequest{Path: m.write("real.png", string(data))})
	if err != nil {
		t.Fatal(err)
	}

	if res.Width != 120 || res.Height != 80 || res.Hash != fileHash(data) {
		t.Fatalf("%+v", res)
	}

	out, err := m.svc.ImageExport(m.as(ui), service.ImageExportRequest{ImageID: res.ImageID})
	if err != nil || out.Width != 120 || out.Height != 80 {
		t.Fatalf("%v %+v", err, out)
	}
}
