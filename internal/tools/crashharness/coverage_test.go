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
	"library_seal.after_file":       "web/app/servicetest TestCrashLibrarySeal",
	"seal_documents.after_batch":    "web/app/servicetest TestCrashSealDocuments",
	"seal_documents.before_version": "web/app/servicetest TestCrashSealDocuments",
	"seal_documents.after_version":  "web/app/servicetest TestCrashSealDocuments",
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

// killCase is a case of the real-kill suite (SDD-008 section 13.3): a child process ended from outside, at a
// moment the parent observes, in a step the product runs. test is the test of web/app/servicetest that kills
// it; steps are the places in the code it protects, each a file and the text that names the step there, so
// that a step that is renamed or removed makes this list stale and a new step is added here in review.
type killCase struct {
	test  string
	steps [][2]string
}

var killCases = map[string]killCase{
	"publication.send": {
		test: "TestKillRealPublicationResumesAndPostsOnce",
		steps: [][2]string{
			{"service/publish/scheduler.go", "func (s *Scheduler) send("},
			{"service/publish/manager.go", "func (m *Manager) Claim("},
		},
	},
	"publication.interrupted": {
		test:  "TestKillRealPublicationInterruptedAfterMaxAttempts",
		steps: [][2]string{{"service/publish/manager.go", "func (m *Manager) Interrupt("}},
	},
	"upload.resumable": {
		test: "TestKillRealUploadResumesAtTheReceivedOffset",
		steps: [][2]string{
			{"service/social/youtube/upload.go", "func (c *Connector) sendFrom("},
			{"service/social/youtube/upload.go", "func (c *Connector) queryOffset("},
		},
	},
	"export.video": {
		test: "TestKillRealExportVideo",
		steps: [][2]string{
			{"engine/export/paths.go", "func publishUnder("},
			{"service/jobrun.go", "func (s *svc) Recover("},
		},
	},
	"export.video.given_up": {
		test:  "TestKillRealExportVideoGivenUpAfterTheAttempts",
		steps: [][2]string{{"service/jobrun.go", "func (s *svc) failInterrupted("}},
	},
	"export.image": {
		test:  "TestKillRealExportImage",
		steps: [][2]string{{"service/image_export.go", "func (s *svc) ImageExport("}},
	},
}

// TestEveryRealKillCaseExists holds the list of real-kill cases to the code on both sides: each test exists in
// a file with the crashtest tag that ends a child with Process.Kill, and each step it protects is still in the
// code.
func TestEveryRealKillCaseExists(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	dir := filepath.Join(root, "web", "app", "servicetest")

	files, err := filepath.Glob(filepath.Join(dir, "kill_real*_test.go"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no kill_real*_test.go in %s: %v", dir, err)
	}

	var killer bool

	sources := map[string]string{}

	for _, f := range files {
		raw, rerr := os.ReadFile(f)
		if rerr != nil {
			t.Fatal(rerr)
		}

		sources[f] = string(raw)
		killer = killer || strings.Contains(string(raw), "Process.Kill()")
	}

	if !killer {
		t.Error("no file of the real-kill suite ends a child with Process.Kill")
	}

	for name, c := range killCases {
		found := false

		for f, src := range sources {
			if !strings.Contains(src, "func "+c.test+"(") {
				continue
			}

			found = true

			if !strings.HasPrefix(src, "//go:build crashtest") {
				t.Errorf("%s: %s is not under the crashtest build tag", name, filepath.Base(f))
			}
		}

		if !found {
			t.Errorf("%s: the test %s does not exist in web/app/servicetest", name, c.test)
		}

		for _, step := range c.steps {
			raw, rerr := os.ReadFile(filepath.Join(root, filepath.FromSlash(step[0])))
			if rerr != nil || !strings.Contains(string(raw), step[1]) {
				t.Errorf("%s: the step %q is no longer in %s (%v): update the case", name, step[1], step[0], rerr)
			}
		}
	}
}
