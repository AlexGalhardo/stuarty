package crashharness

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// covered says, for every crash point of the code, the test that kills it and checks the recovery. A new point
// needs a line here, with a test that exists.
var covered = map[string]string{
	"commit.after_lock":             "web/app/servicetest TestCrashSuite",
	"commit.after_load":             "web/app/servicetest TestCrashSuite",
	"commit.after_apply":            "web/app/servicetest TestCrashSuite",
	"commit.after_operation_append": "web/app/servicetest TestCrashSuite",
	"commit.after_project_commit":   "web/app/servicetest TestCrashSuite",
	"commit.after_assets_sync":      "web/app/servicetest TestCrashSuite",
	"commit.after_idempotency_put":  "web/app/servicetest TestCrashSuite",
	"commit.before_commit":          "web/app/servicetest TestCrashSuite",
	"commit.after_commit":           "web/app/servicetest TestCrashSuite",
	"commit.after_publish":          "web/app/servicetest TestCrashSuite",
	"bundle_export.after_write":     "web/app/servicetest TestCrashBundleExport",
	"bundle_export.after_rename":    "web/app/servicetest TestCrashBundleExport",
	"bundle_import.after_copy":      "web/app/servicetest TestCrashBundleImport",
	"bundle_import.before_commit":   "web/app/servicetest TestCrashBundleImport",
	"bundle_import.after_commit":    "web/app/servicetest TestCrashBundleImport",
	"migrate.before_commit":         "web/app/servicetest TestCrashMigration",
	"migrate.after_commit":          "web/app/servicetest TestCrashMigration",
	"trash_empty.after_lock":        "web/app/servicetest TestCrashTrashEmpty",
	"trash_empty.before_commit":     "web/app/servicetest TestCrashTrashEmpty",
	"trash_empty.after_commit":      "web/app/servicetest TestCrashTrashEmpty",
	"vault.before_rename":           "internal/tools/crashharness TestSecurity_T101_VaultKilledMidWriteKeepsEveryAcknowledgedSecret",
	"vault.after_rename":            "internal/tools/crashharness TestSecurity_T101_VaultKilledMidWriteKeepsEveryAcknowledgedSecret",
}

var hitRe = regexp.MustCompile(`crashpoint\.Hit\("([^"]+)"\)`)

func repoRoot(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	return filepath.Join(wd, "..", "..", "..")
}

// codePoints is every crash point named in the Go code outside the tests.
func codePoints(t *testing.T, root string) []string {
	t.Helper()

	var names []string

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() && (d.Name() == "node_modules" || d.Name() == ".git" || d.Name() == "development") {
			return filepath.SkipDir
		}

		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		for _, m := range hitRe.FindAllStringSubmatch(string(raw), -1) {
			if !slices.Contains(names, m[1]) {
				names = append(names, m[1])
			}
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	return names
}

func TestEveryCrashPointHasARecoveryTest(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	points := codePoints(t, root)

	for _, p := range points {
		if covered[p] == "" {
			t.Errorf("the crash point %s has no test that kills it: add it to covered with its test", p)
		}
	}

	for p, test := range covered {
		if !slices.Contains(points, p) {
			t.Errorf("%s is listed as covered by %s but no code names it", p, test)

			continue
		}

		pkg, name, _ := strings.Cut(test, " ")

		found, named := false, false

		files, _ := filepath.Glob(filepath.Join(root, pkg, "*_test.go"))
		for _, f := range files {
			if raw, err := os.ReadFile(f); err == nil && strings.Contains(string(raw), "func "+name+"(") {
				found = true
				// The file of the test names the point it kills, so a line cannot point at a test that exists
				// but kills something else.
				named = named || strings.Contains(string(raw), `"`+p+`"`)
			}
		}

		switch {
		case !found:
			t.Errorf("%s: the test %s does not exist in %s", p, name, pkg)
		case !named:
			t.Errorf("%s: the file of %s in %s does not name the point", p, name, pkg)
		}
	}
}
