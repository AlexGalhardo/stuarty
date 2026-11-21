package service_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
	"github.com/AlexGalhardo/stuarty/service/workspace"
)

// P4-SRV-001 (SDD-005 sections 7 and 8, section 18 checks 3, 4, 17 and 26).
//
// The package service may not import the web adapters (ADR-0001 rule 3), so the build under test is driven
// as the program it is: its command line opens the workspace through the real start-up sequence of SDD-005
// section 7 (database backup, migrations, migration of the documents).
//
// The workspaces come from released binaries: testdata/upgrade/build-workspace.sh runs the fixed scenario
// with each release and writes <dir>/<version>/. The upgrade job of P4-REL-002 produces that folder and the
// released binaries and sets the variables below; the workspaces are never committed. Without
// UpgradeWorkspacesEnv both tests skip.
//
// Releases that are not in the matrix, and why:
//   - v0.1.0 is the legacy Studio: another product, with no workspace and no project:create.
//   - v0.2.0 is the skeleton of the new product: it has no command that creates a project, so it cannot
//     write a workspace with a project in it.
//
// What the generated scenarios did (measured with the script on 2026-11-15):
//
//	v0.3.0   project, captions, trash and restore; no media import, so no clip, keyframes or export
//	v0.4.0   adds media import, clip and export; no keyframes, no templates
//	v0.5.0   adds keyframes; no templates
//	v0.6.0   adds saving a template; no instantiating a starter
//	v0.8.0+  the whole scenario
//
// The scenario.txt of each release is the authority: the test reads it and checks every step the release did.
const (
	// UpgradeWorkspacesEnv names the folder that holds one workspace per released version.
	UpgradeWorkspacesEnv = "VUKEM_UPGRADE_WORKSPACES"
	// UpgradeBinariesEnv names the folder that holds <version>/stuarty[.exe]; the released binaries
	// give the documents as they were before the upgrade and are the older binaries of the refusal test.
	UpgradeBinariesEnv = "VUKEM_UPGRADE_BINARIES"
	// UpgradeCandidateEnv names the binary of this build; without it the test builds ./web/cmd/app.
	UpgradeCandidateEnv = "VUKEM_UPGRADE_CANDIDATE"
)

const (
	mainProject     = "prj_1000000001"
	starterProject  = "prj_1000000002"
	restoredProject = "prj_1000000003"
	trashedProject  = "prj_1000000004"
	sourceProject   = "prj_1000000005"
	clipItem        = "itm_1000000001"
	// staleBackups are made before the upgrade to check that the 5 newest backups are kept.
	staleBackups = 5
	// backupsKept is SDD-005 section 7 step 3.
	backupsKept = 5
)

func upgradeVersions(t *testing.T) (root string, versions []string) {
	t.Helper()

	root = os.Getenv(UpgradeWorkspacesEnv)
	if root == "" {
		t.Skipf("%s is not set: the upgrade job of P4-REL-002 produces the workspaces", UpgradeWorkspacesEnv)
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}

	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "v") {
			versions = append(versions, e.Name())
		}
	}

	if len(versions) == 0 {
		t.Fatalf("%s holds no <version> folder", root)
	}

	slices.SortFunc(versions, compareVersions)

	return root, versions
}

// compareVersions orders vMAJOR.MINOR.PATCH numerically.
func compareVersions(a, b string) int {
	pa, pb := strings.Split(strings.TrimPrefix(a, "v"), "."), strings.Split(strings.TrimPrefix(b, "v"), ".")
	for i := range min(len(pa), len(pb)) {
		var x, y int

		_, _ = fmt.Sscan(pa[i], &x)
		_, _ = fmt.Sscan(pb[i], &y)

		if x != y {
			return x - y
		}
	}

	return len(pa) - len(pb)
}

// candidateBinary is the build under test.
func candidateBinary(t *testing.T) string {
	t.Helper()

	if p := os.Getenv(UpgradeCandidateEnv); p != "" {
		return p
	}

	exe := filepath.Join(t.TempDir(), "stuarty")
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}

	build := exec.CommandContext(context.Background(), "go", "build", "-C", "..", "-o", exe, "./web/cmd/app")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build the candidate: %v\n%s", err, out)
	}

	return exe
}

// releasedBinary finds the binary of a release; one before v0.30.0 has the former name (ADR-0044).

func releasedBinary(dir, version string) string {
	for _, name := range []string{"stuarty", "stuarty.exe", "vukem-video", "vukem-video.exe"} {
		p := filepath.Join(dir, version, name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	return ""
}

func copyTree(t *testing.T, from, to string) {
	t.Helper()

	err := filepath.WalkDir(from, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, _ := filepath.Rel(from, path)
		target := filepath.Join(to, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0o700)
		}

		if d.Name() == "lock" || d.Name() == "runtime.json" || d.Name() == "token" {
			return nil // process state and a secret the binary rewrites; a copy has the file permissions of the test, not the binary's
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		return os.WriteFile(target, data, 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// scenario reads scenario.txt: step name to "" when done, or to the reason it was skipped.
func scenario(t *testing.T, dir string) map[string]string {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(dir, "scenario.txt"))
	if err != nil {
		t.Fatal(err)
	}

	steps := map[string]string{}

	for line := range strings.SplitSeq(strings.TrimSpace(string(raw)), "\n") {
		name, rest, _ := strings.Cut(strings.TrimSpace(line), " ")
		status, reason, _ := strings.Cut(rest, " ")

		switch status {
		case "ok":
			steps[name] = ""
		case "skipped":
			steps[name] = reason
		default:
			t.Fatalf("scenario.txt: unreadable line %q", line)
		}
	}

	return steps
}

// done reports whether the release did the step; a step the script never reached counts as not done.
func done(steps map[string]string, name string) bool {
	reason, ok := steps[name]

	return ok && reason == ""
}

// upgradeEnv is what a binary under test needs on top of the process environment: the workspace and a
// loopback port of its own. A release before v0.30.0 (named vukem-video, ADR-0044) reads only the former
// names (SDD-000 section 13): given STUARTY_HOME alone it would open the person's own workspace, and its
// serve, should the refusal of a newer workspace regress, would take the default port 7878 (VV-295).
// HOST and PORT are read by every release.
func upgradeEnv(t *testing.T, exe, ws string) []string {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	addr := l.Addr().String()
	_ = l.Close()

	_, port, _ := net.SplitHostPort(addr)
	env := []string{workspace.EnvHome + "=" + ws, "STUARTY_ADDR=" + addr, "HOST=127.0.0.1", "PORT=" + port}

	if strings.HasPrefix(filepath.Base(exe), "vukem-video") {
		env = append(env, "VUKEM_VIDEO_HOME="+ws, "VUKEM_VIDEO_ADDR="+addr)
	}

	return env
}

func TestUpgradeEnvGivesEveryReleaseItsOwnWorkspaceAndPort(t *testing.T) {
	ws := t.TempDir()

	for _, exe := range []string{"stuarty", "stuarty.exe", "vukem-video", "vukem-video.exe"} {
		env := upgradeEnv(t, filepath.Join(t.TempDir(), "v0.29.0", exe), ws)
		former := strings.HasPrefix(exe, "vukem-video")

		want := []string{workspace.EnvHome, "STUARTY_ADDR", "HOST", "PORT"}
		if former {
			want = append(want, "VUKEM_VIDEO_HOME", "VUKEM_VIDEO_ADDR")
		}

		got := map[string]string{}

		for _, kv := range env {
			k, v, _ := strings.Cut(kv, "=")
			got[k] = v
		}

		for _, k := range want {
			v := got[k]
			if v == "" || strings.HasSuffix(v, ":7878") || v == "7878" {
				t.Errorf("%s: %s=%q, want a value of its own", exe, k, v)
			}
		}

		if got[workspace.EnvHome] != ws || former && got["VUKEM_VIDEO_HOME"] != ws {
			t.Errorf("%s: the workspace is not the one under test: %v", exe, env)
		}
	}
}

// run runs a command line of a binary on a workspace and returns its output and exit code.
func run(t *testing.T, exe, ws string, args ...string) (string, int) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Env = append(os.Environ(), upgradeEnv(t, exe, ws)...)

	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf

	err := cmd.Run()

	var exit *exec.ExitError

	switch {
	case err == nil:
		return buf.String(), 0
	case errors.As(err, &exit):
		return buf.String(), exit.ExitCode()
	default:
		t.Fatalf("%s %v: %v", filepath.Base(exe), args, err)

		return "", -1
	}
}

// jsonOf runs a command with --json and decodes the last line of its output (notes may precede it).
func jsonOf(t *testing.T, exe, ws string, args ...string) map[string]json.RawMessage {
	t.Helper()

	out, code := run(t, exe, ws, append(args, "--json")...)
	if code != 0 {
		t.Fatalf("%s %v = exit %d\n%s", filepath.Base(exe), args, code, out)
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")

	var doc map[string]json.RawMessage
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &doc); err != nil {
		t.Fatalf("%s %v: %v\n%s", filepath.Base(exe), args, err, out)
	}

	return doc
}

func projectStates(t *testing.T, exe, ws string) map[string]string {
	t.Helper()

	var projects []struct {
		ID    string `json:"id"`
		State string `json:"state"`
	}

	listed := jsonOf(t, exe, ws, "project:list", "--state", "all", "--limit", "200")
	if err := json.Unmarshal(listed["projects"], &projects); err != nil {
		t.Fatal(err)
	}

	states := map[string]string{}
	for _, p := range projects {
		states[p.ID] = p.State
	}

	return states
}

// documentOf is the stored document of a project as the binary shows it.
func documentOf(t *testing.T, exe, ws, project string) []byte {
	t.Helper()

	return jsonOf(t, exe, ws, "project:show", project, "--detail", "document")["document"]
}

func sha(b []byte) string {
	sum := sha256.Sum256(b)

	return hex.EncodeToString(sum[:])
}

func fileSHA(t *testing.T, path string) string {
	t.Helper()

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	return sha(b)
}

func backupFiles(t *testing.T, ws string) []string {
	t.Helper()

	names, err := filepath.Glob(filepath.Join(ws, "backups", "db-*-before-*.sqlite"))
	if err != nil {
		t.Fatal(err)
	}

	slices.Sort(names)

	return names
}

// wantedProjects is what the scenario left, project to state, for the steps the release did.
func wantedProjects(steps map[string]string) map[string]string {
	want := map[string]string{}

	if done(steps, "project.create") {
		want[mainProject] = "active"
	}

	if done(steps, "project.trash_restore") {
		want[restoredProject] = "active"
	}

	if done(steps, "project.trashed") {
		want[trashedProject] = "trashed"
	}

	if done(steps, "template.instantiate") {
		want[starterProject] = "active"
	}

	if done(steps, "edit.template_source") {
		want[sourceProject] = "active"
	}

	return want
}

// TestUpgradeMatrix opens the workspace written by each released version with this build.
func TestUpgradeMatrix(t *testing.T) {
	root, versions := upgradeVersions(t)
	candidate := candidateBinary(t)

	for _, version := range versions {
		t.Run(version, func(t *testing.T) {
			src := filepath.Join(root, version)
			steps := scenario(t, src)
			want := wantedProjects(steps)

			ws := filepath.Join(t.TempDir(), "ws")
			copyTree(t, src, ws)

			// The documents as the release shows them, read before the candidate touches the workspace.
			before := map[string]string{}

			if released := releasedBinary(os.Getenv(UpgradeBinariesEnv), version); released != "" {
				for id := range want {
					before[id] = identity(t, documentOf(t, released, ws, id))
				}
			} else {
				t.Logf("no binary of %s: the documents are compared with the second start only", version)
			}

			// A workspace that already holds backups: the 5 newest survive (check 4).
			if err := os.MkdirAll(filepath.Join(ws, "backups"), 0o700); err != nil {
				t.Fatal(err)
			}

			for i := range staleBackups {
				stale := filepath.Join(ws, "backups", fmt.Sprintf("db-2020010%dT000000-before-stale.sqlite", i+1))
				if err := os.WriteFile(stale, []byte("stale"), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			states := projectStates(t, candidate, ws)

			for id, state := range want {
				if states[id] != state {
					t.Errorf("project %s is %q after the upgrade, the scenario left it %q", id, states[id], state)
				}
			}

			if len(states) != len(want) {
				t.Errorf("%d projects after the upgrade, the scenario made %d", len(states), len(want))
			}

			checkBackups(t, candidate, ws, states)

			docs := map[string][]byte{}

			for id := range want {
				docs[id] = documentOf(t, candidate, ws, id)

				if was, ok := before[id]; ok && was != identity(t, docs[id]) {
					t.Errorf("the document of %s changed by the upgrade", id)
				}

				checkValid(t, id, docs[id])
			}

			checkScenario(t, candidate, ws, steps, docs)

			// A second start of the same workspace changes nothing: no backup, no document.
			backups := backupFiles(t, ws)
			_ = projectStates(t, candidate, ws)

			if again := backupFiles(t, ws); !slices.Equal(again, backups) {
				t.Errorf("the second start changed the backups: %v then %v", backups, again)
			}

			for id := range want {
				if !bytes.Equal(documentOf(t, candidate, ws, id), docs[id]) {
					t.Errorf("the second start changed the document of %s", id)
				}
			}
		})
	}
}

// checkBackups: the upgrade wrote one backup, openable, and no more than 5 are kept (check 4).
func checkBackups(t *testing.T, candidate, ws string, states map[string]string) {
	t.Helper()

	backups := backupFiles(t, ws)

	var fresh []string

	for _, b := range backups {
		if !strings.Contains(b, "-before-stale") {
			fresh = append(fresh, b)
		}
	}

	if len(backups) > backupsKept {
		t.Errorf("%d backups kept, at most %d", len(backups), backupsKept)
	}

	if len(fresh) == 0 {
		return // nothing was pending: the release is as new as this build
	}

	if len(fresh) != 1 || len(backups) != backupsKept {
		t.Errorf("one upgrade left %d fresh backups and %d in all, want 1 and %d",
			len(fresh), len(backups), backupsKept)
	}

	// Openable: the backup, put in place as the database of a fresh workspace, opens and lists the same projects.
	restored := filepath.Join(t.TempDir(), "restored")
	if err := os.MkdirAll(restored, 0o700); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(fresh[0])
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(restored, "db.sqlite"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	got := projectStates(t, candidate, restored)
	if len(got) != len(states) {
		t.Errorf("the backup opens with %d projects, the upgraded workspace holds %d", len(got), len(states))
	}

	for id, state := range states {
		if got[id] != state {
			t.Errorf("the backup holds %s as %q, want %q", id, got[id], state)
		}
	}
}

// identity is the hash of the canonical form of a document as a binary shows it: two binaries that print the
// same project in different key order or spacing still agree on it.
func identity(t *testing.T, shown []byte) string {
	t.Helper()

	doc, err := timeline.Decode(shown)
	if err != nil {
		t.Fatalf("a document does not decode: %v", err)
	}

	return sha(timeline.Canonical(doc))
}

// checkValid: the document of a project is valid for this build (SDD-005 section 8).
func checkValid(t *testing.T, id string, shown []byte) {
	t.Helper()

	doc, err := timeline.Decode(shown)
	if err != nil {
		t.Errorf("%s does not decode: %v", id, err)

		return
	}

	if violations := timeline.Validate(doc); len(violations) > 0 {
		t.Errorf("%s no longer validates: %v", id, violations)
	}
}

type exportJob struct {
	Kind   string `json:"kind"`
	State  string `json:"state"`
	Result struct {
		Export struct {
			Path   string `json:"path"`
			SHA256 string `json:"sha256"`
		} `json:"export"`
	} `json:"result"`
}

// checkScenario checks what the release did survives: edits, template, export.
func checkScenario(t *testing.T, candidate, ws string, steps map[string]string, docs map[string][]byte) {
	t.Helper()

	if done(steps, "edit.captions") {
		doc, err := timeline.Decode(docs[mainProject])
		if err != nil {
			t.Fatal(err)
		}

		var captions, texts int

		for id := range doc.Items {
			switch doc.Items[id].Type {
			case timeline.ItemCaption:
				captions++
			case timeline.ItemText:
				texts++
			default:
			}
		}

		if captions != 1 || texts != 1 {
			t.Errorf("the main project holds %d captions and %d texts, want 1 and 1", captions, texts)
		}

		if done(steps, "edit.clip") {
			if _, ok := doc.Items[clipItem]; !ok {
				t.Error("the clip of the main project was lost")
			}

			if len(doc.Assets) != 1 {
				t.Errorf("the main project holds %d assets, want 1", len(doc.Assets))
			}
		}

		if done(steps, "edit.keyframes") {
			if n := len(doc.Items[clipItem].Keyframes["scale"]); n != 2 {
				t.Errorf("the clip holds %d scale keyframes, want 2", n)
			}
		}
	}

	if done(steps, "template.save") && !slices.Contains(templateNames(t, candidate, ws), "Upgrade template") {
		t.Error("the template saved by the release is not in the library")
	}

	if done(steps, "export") {
		checkExport(t, candidate, ws)
	}
}

func templateNames(t *testing.T, candidate, ws string) []string {
	t.Helper()

	var templates []struct {
		Name string `json:"name"`
	}

	listed := jsonOf(t, candidate, ws, "template:list", "--limit", "200")
	if err := json.Unmarshal(listed["templates"], &templates); err != nil {
		t.Fatal(err)
	}

	names := make([]string, 0, len(templates))
	for _, tpl := range templates {
		names = append(names, tpl.Name)
	}

	return names
}

// checkExport: the export job of the release is still succeeded, and its file is still what the job recorded.
func checkExport(t *testing.T, candidate, ws string) {
	t.Helper()

	var jobs []exportJob
	listed := jsonOf(t, candidate, ws, "job:list", "--project", mainProject)
	if err := json.Unmarshal(listed["jobs"], &jobs); err != nil {
		t.Fatal(err)
	}

	var succeeded int

	for i := range jobs {
		job := &jobs[i]
		if job.Kind != "export" || job.State != "succeeded" {
			continue
		}

		succeeded++

		// The path was written for the workspace the release used; the file is in the copy under the same name.
		name := filepath.Base(strings.ReplaceAll(job.Result.Export.Path, `\`, "/"))
		file := filepath.Join(ws, "exports", mainProject, name)

		if err := ffmpeg.SniffFile(file); err != nil {
			t.Errorf("the export %s is no longer media: %v", name, err)

			continue
		}

		if want := job.Result.Export.SHA256; want != "" && fileSHA(t, file) != want {
			t.Errorf("the export %s no longer matches the hash its job recorded", name)
		}
	}

	if succeeded != 1 {
		t.Errorf("%d succeeded export jobs, want 1", succeeded)
	}
}

var migrationID = regexp.MustCompile(`d{14}_[a-z0-9_]+`)

// migrationsIn lists the migration ids written in a workspace database. The ids are plain text in the
// file; reading them this way keeps the test free of a SQLite driver.
func migrationsIn(t *testing.T, ws string) map[string]bool {
	t.Helper()

	ids := map[string]bool{}

	for _, name := range []string{"db.sqlite", "db.sqlite-wal"} {
		data, err := os.ReadFile(filepath.Join(ws, name))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}

		if err != nil {
			t.Fatal(err)
		}

		for _, id := range migrationID.FindAllString(string(data), -1) {
			ids[id] = true
		}
	}

	return ids
}

// TestUpgradeOlderBinaryRefusesCandidate (check 3): a workspace made by this build is refused by every
// older binary that does not know one of its migrations: exit 1, the newer-version message, and the
// database file untouched. A release that knows every migration of this build is not older for this purpose.
func TestUpgradeOlderBinaryRefusesCandidate(t *testing.T) {
	root, versions := upgradeVersions(t)

	binaries := os.Getenv(UpgradeBinariesEnv)
	if binaries == "" {
		t.Skipf( // verify:env-skip the upgrade matrix runs from scripts/upgrade-test.sh
			"%s is not set: it names the folder with <version>/stuarty[.exe]",
			UpgradeBinariesEnv,
		)
	}

	candidate := candidateBinary(t)

	made := filepath.Join(t.TempDir(), "candidate")
	if err := os.MkdirAll(made, 0o700); err != nil {
		t.Fatal(err)
	}

	_ = jsonOf(t, candidate, made, "project:create", "--name", "Candidate", "--id", "prj_2000000001",
		"--description", "A short video made by the upgrade test, to prove that an older binary refuses this workspace.",
		"--specific", "Create one project with this build", "--measurable", "The older binary exits with code one",
		"--achievable", "One project is enough for the check", "--relevant", "It protects a workspace from a downgrade",
		"--time-bound", "Done within the run of this test", "--due", "2999-12-31")
	ours := migrationsIn(t, made)

	for _, version := range versions {
		t.Run(version, func(t *testing.T) {
			exe := releasedBinary(binaries, version)
			if exe == "" {
				t.Skipf("no binary of %s in %s", version, binaries)
			}

			known := migrationsIn(t, filepath.Join(root, version))
			if knowsAll(known, ours) {
				t.Skipf("%s knows every migration of this build: it is not older for the database", version)
			}

			ws := filepath.Join(t.TempDir(), "ws")
			copyTree(t, made, ws)

			was := fileSHA(t, filepath.Join(ws, "db.sqlite"))

			// serve prints the refusal. The in-process commands of the released binaries answer a generic
			// E_INTERNAL and v0.3.0 has no serve: for it the exit code and the untouched file are what is checked.
			out, code := run(t, exe, ws, "serve")
			if code != 1 {
				t.Fatalf("%s serve on a workspace of this build = exit %d, want 1\n%s", version, code, out)
			}

			if strings.Contains(out, "APP_KEY is not set") {
				out, code = run(t, exe, ws, "project:list", "--json")
				if code != 1 {
					t.Fatalf("%s project:list on a workspace of this build = exit %d, want 1\n%s", version, code, out)
				}

				t.Logf("%s has no serve and its in-process commands hide the newer-version message: %s",
					version, strings.TrimSpace(out))
			} else if !strings.Contains(out, "newer version") {
				t.Errorf("the refusal does not name a newer version: %s", out)
			}

			if got := fileSHA(t, filepath.Join(ws, "db.sqlite")); got != was {
				t.Errorf("the database file of the candidate changed: %s then %s", was, got)
			}
		})
	}
}

// knowsAll reports whether every migration of ours is in known.
func knowsAll(known, ours map[string]bool) bool {
	for id := range ours {
		if !known[id] {
			return false
		}
	}

	return true
}
