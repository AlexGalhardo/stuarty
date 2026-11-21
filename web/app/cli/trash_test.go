package cli_test

import (
	"io/fs"
	"path/filepath"
	"testing"
)

func countFiles(t *testing.T, dir string) int {
	t.Helper()

	n := 0

	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			n++
		}

		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	return n
}

func trashedInList(t *testing.T, listed map[string]any, asset string) bool {
	t.Helper()

	entries, _ := listed["assets"].([]any)

	for _, e := range entries {
		m, _ := e.(map[string]any)
		if m["asset_id"] == asset {
			return m["trashed"] == true
		}
	}

	t.Fatalf("%s is not in the list: %v", asset, listed)

	return false
}

// TestCLITrashNeverDeletes: media:trash then media:inspect shows the asset as
// trashed, media:restore brings it back, and no file leaves the workspace
// (RF-131).
func TestCLITrashNeverDeletes(t *testing.T) {
	s := newSession(t)

	project := str(
		t,
		s.ok(
			"",
			"project:create",
			"--name",
			"Trash",
			"--description",
			ctxDescription,
			"--specific",
			ctxSpecific,
			"--measurable",
			ctxMeasurable,
			"--achievable",
			ctxAchievable,
			"--relevant",
			ctxRelevant,
			"--time-bound",
			ctxTimeBound,
			"--due",
			ctxDue,
		),
		"project_id",
	)
	s.ok("", "item:add", "--project", project, "--kind", "timeline", "--title", "Trash")

	imported := s.ok("", "media:import", s.clip(), "--project", project)
	assets, _ := imported["assets"].([]any)

	if len(assets) != 1 {
		t.Fatalf("media:import: %v", imported)
	}

	first, _ := assets[0].(map[string]any)
	asset := str(t, first, "asset_id")

	before := countFiles(t, s.home)

	if trashed := s.ok("", "media:trash", asset, "--project", project); trashed["state"] != "trashed" {
		t.Fatalf("media:trash: %v", trashed)
	}

	if got := countFiles(t, s.home); got != before {
		t.Errorf("the workspace had %d files before the trash and %d after", before, got)
	}

	if !trashedInList(t, s.ok("", "media:list", "--project", project), asset) {
		t.Error("media:list does not show the asset as trashed")
	}

	if restored := s.ok("", "media:restore", asset, "--project", project); restored["state"] != "active" {
		t.Fatalf("media:restore: %v", restored)
	}

	if got := countFiles(t, s.home); got < before {
		t.Errorf("files left the workspace: %d before, %d after the restore", before, got)
	}

	if trashedInList(t, s.ok("", "media:list", "--project", project), asset) {
		t.Error("the asset is still trashed after the restore")
	}

	code, missing := s.do("", "media:trash", "ast_0000000000", "--project", project)
	if code != 1 || missing["code"] != "E_NOT_FOUND" {
		t.Errorf("an unknown asset: exit %d, %v", code, missing)
	}
}
