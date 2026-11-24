package servicetest

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/asset"
	enginestore "github.com/AlexGalhardo/stuarty/engine/store"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/testproject"
)

// removeStored deletes the original of a file from the content store, as someone clearing the folder by hand would.
func (m *mediaEnv) removeStored(hash string) {
	m.t.Helper()

	p, err := enginestore.New(m.ws).Path(strings.TrimPrefix(hash, "sha256:"))
	if err != nil {
		m.t.Fatal(err)
	}

	if err := os.Remove(p); err != nil {
		m.t.Fatal(err)
	}
}

func entryOf(t *testing.T, res *service.AssetListResult, hash string) service.LibraryAsset {
	t.Helper()

	for i := range res.Assets {
		if res.Assets[i].Hash == hash {
			return res.Assets[i]
		}
	}

	t.Fatalf("hash %s is not in the list %v", hash, assetIDs(res))

	return service.LibraryAsset{}
}

// Epic 94 acceptance 1: a file held by two projects weighs once, each library says its count and total, and the
// detail says how many projects use the file.
func TestAssetLibrarySizeCountsASharedFileOnce(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	ctx := m.as(ui)
	one, two := m.newProject("Teaser"), m.newProject("Trailer")

	shared := m.importOne(one, m.write("shared.mp4", clipBytes)).Assets[0].Hash
	m.importOne(two, m.write("shared.mp4", clipBytes))

	other := "a second clip with other bytes entirely, to weigh differently"
	second := m.importOne(one, m.write("second.mp4", other)).Assets[0].Hash

	list, err := m.svc.AssetList(ctx, service.AssetListRequest{Section: "video"})
	if err != nil {
		t.Fatal(err)
	}

	want := int64(len(clipBytes) + len(other))
	if list.FileCount != 2 || list.Total != 2 || list.TotalBytes != want || list.Unsized != 0 {
		t.Errorf(
			"list = files %d total %d bytes %d unsized %d, want 2 2 %d 0",
			list.FileCount, list.Total, list.TotalBytes, list.Unsized, want,
		)
	}

	if list.TotalSizeText != asset.FormatSize(&want) {
		t.Errorf("total text = %q, want %q", list.TotalSizeText, asset.FormatSize(&want))
	}

	e := entryOf(t, list, shared)
	if e.Bytes == nil || *e.Bytes != int64(len(clipBytes)) || e.State != "ready" || e.SizeText == "" ||
		e.Projects != 2 {
		t.Errorf("shared entry = %+v bytes %v", e, e.Bytes)
	}

	if e2 := entryOf(t, list, second); e2.Projects != 1 {
		t.Errorf("second entry projects = %d", e2.Projects)
	}

	detail, err := m.svc.AssetGet(ctx, service.AssetRef{Hash: shared})
	if err != nil || detail.Projects != 2 || detail.State != "ready" || !strings.Contains(detail.Text, "counted once") {
		t.Errorf("detail = %+v, %v; the detail says the file is counted once", detail.LibraryAsset, err)
	}

	// The total is of every match, not of the page.
	paged, err := m.svc.AssetList(ctx, service.AssetListRequest{Section: "video", Limit: 1})
	if err != nil || len(paged.Assets) != 1 || paged.TotalBytes != want || paged.FileCount != 2 {
		t.Errorf(
			"paged = %d entries, bytes %d, files %d, %v",
			len(paged.Assets),
			paged.TotalBytes,
			paged.FileCount,
			err,
		)
	}

	// A section with no file weighs 0 B and is not an error (a library may be empty).
	visual, err := m.svc.AssetList(ctx, service.AssetListRequest{Section: "visual"})
	if err != nil || visual.FileCount != 0 || visual.TotalBytes != 0 || visual.TotalSizeText != "0 B" ||
		len(visual.Assets) != 0 {
		t.Errorf("visual = %+v, %v", visual, err)
	}
}

// A file the library lists but the store lost is missing, with no size: never 0, and it adds nothing to the total.
func TestAssetLibraryAFileMissingFromTheStoreIsListedAsMissing(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	ctx := m.as(ui)
	project := m.newProject("p")

	kept := m.importOne(project, m.write("kept.mp4", clipBytes)).Assets[0].Hash
	lost := m.importOne(project, m.write("lost.mp4", "bytes of the clip that will be lost from the folder")).Assets[0].Hash
	m.removeStored(lost)

	list, err := m.svc.AssetList(ctx, service.AssetListRequest{})
	if err != nil {
		t.Fatal(err)
	}

	gone := entryOf(t, list, lost)
	if gone.State != "missing" || gone.Bytes != nil || gone.SizeText != "" {
		t.Errorf(
			"lost entry = state %q bytes %v text %q, want missing, null and no text",
			gone.State,
			gone.Bytes,
			gone.SizeText,
		)
	}

	if list.Unsized != 1 || list.FileCount != 2 || list.TotalBytes != int64(len(clipBytes)) {
		t.Errorf("list = unsized %d files %d bytes %d", list.Unsized, list.FileCount, list.TotalBytes)
	}

	if e := entryOf(t, list, kept); e.State != "ready" {
		t.Errorf("kept entry state = %q", e.State)
	}

	detail, err := m.svc.AssetGet(ctx, service.AssetRef{Hash: lost})
	if err != nil || detail.State != "missing" || detail.Bytes != nil {
		t.Errorf("detail = %+v, %v; the detail says the file is missing", detail.LibraryAsset, err)
	}
}

// A file still importing is listed with no size until it is ready.
func TestAssetLibraryAFileStillImportingHasNoSizeUntilItIsReady(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{noQueue: true})
	ctx := m.as(ui)
	project := m.newProject("p")

	hash := m.importOne(project, m.write("coming.mp4", clipBytes)).Assets[0].Hash

	list, err := m.svc.AssetList(ctx, service.AssetListRequest{})
	if err != nil {
		t.Fatal(err)
	}

	e := entryOf(t, list, hash)
	if e.State != "importing" || e.Bytes != nil || e.SizeText != "" {
		t.Errorf("entry = state %q bytes %v text %q, want importing, null and no text", e.State, e.Bytes, e.SizeText)
	}

	if list.Unsized != 1 || list.TotalBytes != 0 || list.FileCount != 1 {
		t.Errorf("list = unsized %d bytes %d files %d", list.Unsized, list.TotalBytes, list.FileCount)
	}
}

// A generated file is stored like an imported one and weighs the same way.
func TestAssetLibraryAGeneratedAssetHasASize(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	ctx := m.as(ui)
	project := m.newProject("p")

	res, err := m.svc.MediaImport(ctx, service.MediaImportRequest{
		ProjectID: project, Files: []service.ImportFile{{Path: m.write("gen.mp4", clipBytes)}},
		Origin: &asset.Origin{Kind: asset.Generated, Provider: "acme", Model: "clip-1"},
	})
	if err != nil || res.Failed != 0 {
		t.Fatalf("import = %+v, %v", res, err)
	}

	list, err := m.svc.AssetList(ctx, service.AssetListRequest{Origin: string(asset.Generated)})
	if err != nil || len(list.Assets) != 1 {
		t.Fatalf("list = %+v, %v", list, err)
	}

	if e := list.Assets[0]; e.Bytes == nil || *e.Bytes != int64(len(clipBytes)) || e.State != "ready" {
		t.Errorf("generated entry = %+v bytes %v", e, e.Bytes)
	}
}

// D-134: a workspace whose projects are text only has an empty library, which is a normal answer.
func TestAssetLibraryATextOnlyWorkspaceHasAnEmptyLibrary(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	ctx := m.as(ui)

	if _, err := testproject.CreateWithTimeline(ctx, m.svc, "Only words"); err != nil {
		t.Fatal(err)
	}

	list, err := m.svc.AssetList(ctx, service.AssetListRequest{})
	if err != nil {
		t.Fatal(err)
	}

	if list.FileCount != 0 || list.TotalBytes != 0 || list.Unsized != 0 || list.TotalSizeText != "0 B" ||
		len(list.Assets) != 0 {
		t.Errorf("list = %+v", list)
	}
}

// A locked studio answers E_LOCKED to the library read like to every other read (ADR-0052).
func TestAssetLibraryALockedStudioAnswersLocked(t *testing.T) {
	g := &gate{}
	e := newEnvWith(t, func(_ *env, o *service.Options) { o.Gate = g })

	g.locked.Store(true)

	for name, call := range map[string]func(context.Context) error{
		"AssetList": func(ctx context.Context) error {
			_, err := e.svc.AssetList(ctx, service.AssetListRequest{})

			return err
		},
		"AssetGet": func(ctx context.Context) error {
			_, err := e.svc.AssetGet(ctx, service.AssetRef{Hash: "sha256:" + strings.Repeat("a", 64)})

			return err
		},
	} {
		err := call(e.as(ui))

		se, ok := errors.AsType[*service.Error](err)
		if !ok || se.Code != service.CodeLocked {
			t.Errorf("%s while locked = %v, want %s", name, err, service.CodeLocked)
		}
	}
}
