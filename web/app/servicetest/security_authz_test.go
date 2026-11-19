package servicetest

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
)

// T-24: a key replayed with different operations is E_IDEMPOTENCY_MISMATCH and
// writes nothing; a malformed key is refused before any work.
func TestSecurity_T24_IdempotencyMismatch(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	project, track := e.withTrack()
	key := "mismatch-key-1"

	send := func(text string) (*service.EditResult, error) {
		return e.svc.Edit(e.as(ui), service.EditRequest{
			ProjectID: project, BaseRevision: e.head(project).Revision, IdempotencyKey: key,
			Operations: []command.Operation{op("text.add",
				fmt.Sprintf(`{"track_id":%q,"start":"0s","duration":"1s","text":%q}`, track, text))},
		})
	}

	if _, err := send("first"); err != nil {
		t.Fatal(err)
	}

	rows, revision := e.operationCount(project), e.head(project).Revision

	_, err := send("second")
	if code(err) != command.CodeIdempotencyMismatch {
		t.Fatalf("the same key with other operations = %v, want E_IDEMPOTENCY_MISMATCH", err)
	}

	if e.operationCount(project) != rows || e.head(project).Revision != revision {
		t.Error("a mismatched replay wrote something")
	}

	for _, bad := range []string{"short", strings.Repeat("a", 65), "has space in it", "semi;colon-key", "new\nline-key"} {
		key = bad

		if _, err := send("third"); code(err) != command.CodeValidation {
			t.Errorf("idempotency key %q = %v, want E_VALIDATION", bad, err)
		}
	}
}

// T-06: an asset is reached only through project_assets. User B, whose project
// does not hold the asset of user A, cannot open or inspect it by its id, and
// importing the same bytes gives B an asset of B's own.
func TestSecurity_T06_AssetByHashNeedsMembership(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	alice, bob := network("user:alice", "alice"), network("user:bob", "bob")

	aliceProject := mustCreate(alice, t, m, "a")
	bobProject := mustCreate(bob, t, m, "b")
	path := m.write("shared.mp4", clipBytes)

	imported, err := m.svc.MediaImport(alice, service.MediaImportRequest{
		ProjectID: aliceProject, Files: []service.ImportFile{{Path: path}},
	})
	if err != nil || len(imported.Assets) != 1 {
		t.Fatalf("alice's import = %+v, %v", imported, err)
	}

	asset := imported.Assets[0].AssetID

	for _, kind := range []string{"proxy", "chunks", "thumbnail", "strip", "peaks", "lut"} {
		_, err := m.svc.OpenFile(bob, service.FileRef{Kind: kind, ProjectID: bobProject, ID: asset})
		if code(err) != command.CodeNotFound {
			t.Errorf("bob opening %s of alice's asset through his project = %v, want E_NOT_FOUND", kind, err)
		}

		_, err = m.svc.OpenFile(bob, service.FileRef{Kind: kind, ProjectID: aliceProject, ID: asset})
		if code(err) != command.CodeNotFound {
			t.Errorf("bob opening %s through alice's project = %v, want E_NOT_FOUND", kind, err)
		}
	}

	if _, err := m.svc.MediaInspect(
		bob,
		service.MediaInspectRequest{ProjectID: bobProject, AssetID: asset},
	); code(
		err,
	) !=
		command.CodeNotFound {
		t.Errorf("bob inspecting alice's asset = %v, want E_NOT_FOUND", err)
	}

	if _, err := m.svc.MediaTrash(bob, service.MediaTrashRequest{ProjectID: bobProject, AssetID: asset}); code(err) !=
		command.CodeNotFound {
		t.Errorf("bob trashing alice's asset = %v, want E_NOT_FOUND", err)
	}

	// The same bytes under bob's project is his own membership, not alice's.
	again, err := m.svc.MediaImport(bob, service.MediaImportRequest{
		ProjectID: bobProject, Files: []service.ImportFile{{Path: path}},
	})
	if err != nil || len(again.Assets) != 1 {
		t.Fatalf("bob's import = %+v, %v", again, err)
	}

	if _, err := m.svc.OpenFile(bob, service.FileRef{
		Kind: "proxy", ProjectID: bobProject, ID: again.Assets[0].AssetID,
	}); err != nil {
		t.Errorf("bob opening the asset of his own import = %v", err)
	}
}

func mustCreate(ctx context.Context, t *testing.T, m *mediaEnv, name string) string {
	t.Helper()

	res, err := m.svc.ProjectCreate(ctx, service.ProjectCreateRequest{Name: name})
	if err != nil {
		t.Fatal(err)
	}

	return res.ProjectID
}
