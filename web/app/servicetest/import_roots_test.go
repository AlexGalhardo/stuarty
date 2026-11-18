package servicetest

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/service"
	"github.com/AlexGalhardo/vukem-video/service/pathguard"
)

// VV-062: a refused import names the import roots only for the caller who may see server paths.
func TestRefusedImportShowsTheRootsOnlyToAnAdministrator(t *testing.T) {
	t.Parallel()

	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	m := newMediaEnv(t, mediaCfg{guard: &pathguard.Config{Network: true, Roots: []string{root}}})

	member := service.Caller{
		Actor: "user:alice", Scopes: service.RoleScopes(service.RoleMember), OwnerUserID: "alice",
	}
	admin := service.Caller{
		Actor: "user:alice", Scopes: service.RoleScopes(service.RoleAdmin), OwnerUserID: "alice",
	}

	ctx := service.WithCaller(t.Context(), member)

	created, err := m.svc.ProjectCreate(ctx, service.ProjectCreateRequest{Name: "p"})
	if err != nil {
		t.Fatal(err)
	}

	hint := func(c service.Caller) string {
		_, err := m.svc.MediaImport(service.WithCaller(t.Context(), c), service.MediaImportRequest{
			ProjectID: created.ProjectID, Files: []service.ImportFile{{Path: filepath.Join(m.files, "outside.mp4")}},
		})

		se, ok := errors.AsType[*service.Error](err)
		if !ok || se.Code != "E_PATH_NOT_ALLOWED" {
			t.Fatalf("import = %v, want E_PATH_NOT_ALLOWED", err)
		}

		return se.Hint + se.Message
	}

	if h := hint(member); strings.Contains(h, root) {
		t.Errorf("a member was shown the root: %q", h)
	}

	if h := hint(admin); !strings.Contains(h, root) {
		t.Errorf("an administrator was not shown the root: %q", h)
	}
}
