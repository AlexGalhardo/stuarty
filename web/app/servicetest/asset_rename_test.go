package servicetest

import (
	"context"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/asset"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/testproject"
	"github.com/AlexGalhardo/stuarty/service/untrusted"
)

func (m *mediaEnv) shownName(ctx context.Context, project string) string {
	m.t.Helper()

	list, err := m.svc.MediaList(ctx, service.MediaListRequest{ProjectID: project})
	if err != nil || len(list.Assets) != 1 {
		m.t.Fatalf("media of %s = %+v, %v", project, list, err)
	}

	return list.Assets[0].Name
}

// The same bytes imported as clip.mp4 and as clip.mov: whichever name the person was looking at, each project keeps
// its own extension and never gets a second one.
func TestAssetRenameKeepsTheExtensionOfEachProject(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	ctx := m.as(ui)
	p1, p2 := m.newProject("one"), m.newProject("two")

	hash := m.importOne(p1, m.write("clip.mp4", clipBytes)).Assets[0].Hash
	m.importOne(p2, m.write("clip.mov", clipBytes))

	for _, typed := range []string{"Beach.mp4", "Beach.mov", "Beach"} {
		if _, err := m.svc.AssetRename(ctx, service.AssetRenameRequest{Hash: hash, Name: typed}); err != nil {
			t.Fatalf("rename to %q: %v", typed, err)
		}

		if got := m.shownName(ctx, p1); got != "Beach.mp4" {
			t.Errorf("after %q the first project shows %q", typed, got)
		}

		if got := m.shownName(ctx, p2); got != "Beach.mov" {
			t.Errorf("after %q the second project shows %q", typed, got)
		}
	}
}

// A use in the trash is renamed with the others, so a restore does not bring the old name back.
func TestAssetRenameReachesAUseInTheTrash(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	ctx := m.as(ui)
	p1, p2 := m.newProject("one"), m.newProject("two")

	hash := m.importOne(p1, m.write("clip.mp4", clipBytes)).Assets[0].Hash
	trashed := m.importOne(p2, m.write("clip.mp4", clipBytes)).Assets[0].AssetID

	if _, err := m.svc.MediaTrash(ctx, service.MediaTrashRequest{ProjectID: p2, AssetID: trashed}); err != nil {
		t.Fatal(err)
	}

	if _, err := m.svc.AssetRename(ctx, service.AssetRenameRequest{Hash: hash, Name: "Beach"}); err != nil {
		t.Fatal(err)
	}

	if _, err := m.svc.MediaRestore(ctx, service.MediaTrashRequest{ProjectID: p2, AssetID: trashed}); err != nil {
		t.Fatal(err)
	}

	if got := m.shownName(ctx, p2); got != "Beach.mp4" {
		t.Errorf("the restored use is named %q", got)
	}
}

// Two users hold the same bytes: a rename is of the caller's own projects.
func TestAssetRenameLeavesTheSameFileOfAnotherUserAlone(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	as := func(user string) context.Context {
		return service.WithCaller(context.Background(), service.Caller{
			Actor: "user:" + user, Scopes: service.AllScopes, OwnerUserID: user,
		})
	}
	alice, bob := as("alice"), as("bob")

	project := func(ctx context.Context, name string) (string, string) {
		res, err := testproject.CreateWithTimeline(ctx, m.svc, name)
		if err != nil {
			t.Fatal(err)
		}

		in, err := m.svc.MediaImport(ctx, service.MediaImportRequest{
			ProjectID: res.ProjectID, Files: []service.ImportFile{{Path: m.write(name+".mp4", clipBytes)}},
		})
		if err != nil || in.Failed != 0 {
			t.Fatalf("import = %+v, %v", in, err)
		}

		return res.ProjectID, in.Assets[0].Hash
	}

	mine, hash := project(alice, "Alice clips")
	theirs, same := project(bob, "Bobby clips")

	if hash != same {
		t.Fatalf("the two imports differ: %s, %s", hash, same)
	}

	if _, err := m.svc.AssetRename(alice, service.AssetRenameRequest{Hash: hash, Name: "Renamed"}); err != nil {
		t.Fatal(err)
	}

	if got := m.shownName(alice, mine); got != "Renamed.mp4" {
		t.Errorf("alice sees %q", got)
	}

	if got := m.shownName(bob, theirs); got != "Bobby clips.mp4" {
		t.Errorf("bob sees %q", got)
	}
}

// A key limited to a project renames the file in that project only.
func TestAssetRenameByAKeyLimitedToAProject(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	ctx := m.as(ui)
	mine, other := m.newProject("Mine"), m.newProject("Other")

	hash := m.importOne(mine, m.write("clip.mp4", clipBytes)).Assets[0].Hash
	m.importOne(other, m.write("clip.mp4", clipBytes))

	key := service.WithCaller(context.Background(), service.Caller{
		Actor: "key:k", Scopes: service.AllScopes, ProjectID: mine,
	})

	if _, err := m.svc.AssetRename(key, service.AssetRenameRequest{Hash: hash, Name: "Keyed"}); err != nil {
		t.Fatal(err)
	}

	if got := m.shownName(ctx, mine); got != "Keyed.mp4" {
		t.Errorf("the project of the key shows %q", got)
	}

	if got := m.shownName(ctx, other); got != "clip.mp4" {
		t.Errorf("the other project shows %q", got)
	}
}

// An editor open on the project is told, so that a rename by an agent shows without a reload.
func TestAssetRenameTellsTheOpenEditors(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	ctx := m.as(ui)
	project := m.newProject("one")
	in := m.importOne(project, m.write("clip.mp4", clipBytes)).Assets[0]

	count := func() int {
		n := 0

		for _, ev := range m.eventsOf("asset.updated") {
			if ev.ProjectID == project && ev.AssetID == in.AssetID && ev.State == "ready" {
				n++
			}
		}

		return n
	}

	before := count()

	if _, err := m.svc.AssetRename(ctx, service.AssetRenameRequest{Hash: in.Hash, Name: "Beach"}); err != nil {
		t.Fatal(err)
	}

	if count() != before+1 {
		t.Errorf("asset.updated ready events: %d, want %d", count(), before+1)
	}

	if _, err := m.svc.AssetRename(ctx, service.AssetRenameRequest{Hash: in.Hash, Name: "Beach"}); err != nil {
		t.Fatal(err)
	}

	if count() != before+1 {
		t.Errorf("a rename that changes nothing told the editors")
	}
}

// The rule of core/asset cannot import service/untrusted: this keeps the two in step, so a rename never stores a
// character the import of a file name removes.
func TestAssetRenameRefusesEveryCharacterAnImportRemoves(t *testing.T) {
	for r := rune(0); r <= 0x10FFFF; r++ {
		if r == '\n' || r >= 0xD800 && r <= 0xDFFF || !untrusted.Removed(r) {
			continue
		}

		if _, bad := asset.RenameAll([]string{"a.wav"}, "x"+string(r)+"y"); bad == nil {
			t.Errorf("U+%04X is removed at import and accepted by a rename", r)
		}
	}
}
