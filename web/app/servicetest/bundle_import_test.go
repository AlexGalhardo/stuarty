package servicetest

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service"
)

type bundleImporter interface {
	ProjectBundleImport(
		ctx context.Context,
		req service.ProjectBundleImportRequest,
	) (*service.ProjectCreateResult, error)
}

func (m *mediaEnv) importBundle(path string, asNew bool) (*service.ProjectCreateResult, error) {
	m.t.Helper()

	return m.svc.(bundleImporter).ProjectBundleImport(
		m.as(ui), service.ProjectBundleImportRequest{Path: path, AsNew: asNew},
	)
}

// bundleFile is one entry of a bundle under test.
type testEntry struct {
	name   string
	body   []byte
	method uint16
	mode   fs.FileMode
	flags  uint16
}

// bundleEntries reads a .vvbundle into editable entries, in file order.
func bundleEntries(t *testing.T, path string) []testEntry {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}

	var out []testEntry

	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}

		b, err := io.ReadAll(rc)
		_ = rc.Close()

		if err != nil {
			t.Fatal(err)
		}

		out = append(out, testEntry{name: f.Name, body: b, method: f.Method})
	}

	return out
}

func writeBundleEntries(t *testing.T, dir, name string, entries []testEntry) string {
	t.Helper()

	path := filepath.Join(dir, name)

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	zw := zip.NewWriter(f)

	for _, e := range entries {
		method := e.method
		if method == 0 && e.name != "" && !strings.HasPrefix(e.name, "media.") {
			method = zip.Deflate
		}

		h := &zip.FileHeader{Name: e.name, Method: method, Flags: e.flags}
		if e.mode != 0 {
			h.SetMode(e.mode)
		}

		w, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}

		if _, err := w.Write(e.body); err != nil {
			t.Fatal(err)
		}
	}

	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	return path
}

func entryIndex(entries []testEntry, name string) int {
	for i := range entries {
		if entries[i].name == name {
			return i
		}
	}

	return -1
}

// withManifest rewrites manifest.json after the edit.
func withManifest(t *testing.T, entries []testEntry, edit func(*service.BundleManifest)) {
	t.Helper()

	i := entryIndex(entries, "manifest.json")

	var man service.BundleManifest
	if err := json.Unmarshal(entries[i].body, &man); err != nil {
		t.Fatal(err)
	}

	edit(&man)

	b, err := json.Marshal(man)
	if err != nil {
		t.Fatal(err)
	}

	entries[i].body = b
}

func storeFiles(t *testing.T, ws string) []string {
	t.Helper()

	var out []string

	root := filepath.Join(ws, "store")

	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}

			return err
		}

		if !d.IsDir() {
			out = append(out, p)
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	return out
}

func listProjects(t *testing.T, m *mediaEnv) int {
	t.Helper()

	n, err := m.store.Projects().Count(context.Background(), service.ProjectQuery{IncludeTrashed: true})
	if err != nil {
		t.Fatal(err)
	}

	return n
}

// SDD-005 section 18 check 28: the round trip of an export through an empty workspace.
func TestBundleImportRoundTrip(t *testing.T) {
	src := newMediaEnv(t, mediaCfg{})
	project, hashes := src.bundleProject()
	head := src.head(project)

	job := src.exportBundle(project, nil)
	if job.State != service.JobSucceeded {
		t.Fatalf("export = %+v", job)
	}

	bundle := job.Result.Export.Path

	dst := newMediaEnv(t, mediaCfg{})

	res, err := dst.importBundle(bundle, false)
	if err != nil {
		t.Fatal(err)
	}

	if !res.Created || res.ProjectID != project || res.Revision != head.Revision {
		t.Fatalf("result = %+v", res)
	}

	if !bytes.Equal(dst.document(project), src.document(project)) {
		t.Error("the document bytes differ after the round trip")
	}

	ctx := context.Background()

	want, err := src.store.Operations().Since(ctx, project, 0, service.OperationFilter{})
	if err != nil {
		t.Fatal(err)
	}

	got, err := dst.store.Operations().Since(ctx, project, 0, service.OperationFilter{})
	if err != nil {
		t.Fatal(err)
	}

	for i := range want {
		want[i].IdempotencyKey = nil // a bundle carries no idempotency keys (SDD-005 section 16.1)

		// Foreign history is attributed to nobody who is live and cannot be undone (SDD-005 section 16.3 step 7).
		want[i].Actor = "imported:" + want[i].Actor

		if want[i].ClosedBy == nil {
			closed := want[i].Revision
			want[i].ClosedBy = &closed
		}
	}

	if len(got) != len(want) || len(got) == 0 || !reflect.DeepEqual(got, want) {
		t.Errorf("the log differs: %d rows against %d", len(got), len(want))
	}

	if h := dst.head(project); h.Revision != head.Revision || h.ResyncFloor != head.Revision {
		t.Errorf("head = %+v, want revision and resync floor %d", h, head.Revision)
	}

	active, err := dst.store.Assets().ListProjectAssets(ctx, project, service.ProjectAssetActive)
	if err != nil || len(active) != len(hashes) {
		t.Fatalf("%d active project assets (%v), want %d", len(active), err, len(hashes))
	}

	for _, h := range hashes {
		if !dst.mediaHas(h) {
			t.Errorf("%s is not in the store", h)
		}

		if _, err := dst.store.Assets().Get(ctx, h); err != nil {
			t.Errorf("assets row of %s: %v", h, err)
		}
	}

	if dst.ingests.Load() != int64(len(hashes)) {
		t.Errorf("%d ingest runs, want %d: the import creates one job per missing artifact set",
			dst.ingests.Load(), len(hashes))
	}

	// Importing again into the same workspace: the id exists.
	if _, err := dst.importBundle(bundle, false); !isCode(err, "E_ID_EXISTS") {
		t.Errorf("second import = %v, want E_ID_EXISTS", err)
	}

	// Even a trashed project holds its id.
	if _, err := dst.svc.ProjectTrash(dst.as(ui), service.ProjectTrashRequest{ProjectID: project}); err != nil {
		t.Fatal(err)
	}

	if _, err := dst.importBundle(bundle, false); !isCode(err, "E_ID_EXISTS") {
		t.Errorf("import over a trashed project = %v, want E_ID_EXISTS", err)
	}

	// as_new: another id, one import row, no log, and a backup of the original document.
	before := len(storeFiles(t, dst.ws))

	fresh, err := dst.importBundle(bundle, true)
	if err != nil {
		t.Fatal(err)
	}

	if fresh.ProjectID == project || fresh.Revision != 1 || !timeline.ValidID(timeline.PrefixProject, fresh.ProjectID) {
		t.Fatalf("as_new result = %+v", fresh)
	}

	rows, err := dst.store.Operations().Since(ctx, fresh.ProjectID, 0, service.OperationFilter{})
	if err != nil || len(rows) != 1 || rows[0].Kind != service.OpImport || rows[0].Revision != 1 {
		t.Errorf("as_new log = %+v, %v; want one import row", rows, err)
	}

	backups, err := dst.store.Backups().List(ctx, "project", fresh.ProjectID)
	if err != nil || len(backups) != 1 {
		t.Errorf("backups = %d, %v", len(backups), err)
	}

	row, err := dst.store.Projects().Get(ctx, fresh.ProjectID)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(row.Document), fresh.ProjectID) || strings.Contains(string(row.Document), project) {
		t.Error("the document of an as_new import still names the old id")
	}

	if after := len(storeFiles(t, dst.ws)); after != before {
		t.Errorf("as_new copied %d files again; the store already held them", after-before)
	}

	// A bundle without a log imports at its revision with no history.
	no := false

	bare := src.exportBundle(project, &no)
	if bare.State != service.JobSucceeded {
		t.Fatal(bare)
	}

	other := newMediaEnv(t, mediaCfg{})

	res, err = other.importBundle(bare.Result.Export.Path, false)
	if err != nil || res.Revision != head.Revision {
		t.Fatalf("import without a log = %+v, %v", res, err)
	}

	if n := other.operationCount(project); n != 0 {
		t.Errorf("%d log rows from a bundle without a log", n)
	}
}

func (m *mediaEnv) mediaHas(hash string) bool {
	_, err := os.Stat(filepath.Join(m.ws, "store", hash[len("sha256:"):len("sha256:")+2], hash[len("sha256:"):]))

	return err == nil
}

func isCode(err error, code string) bool {
	var e *service.Error

	return errors.As(err, &e) && e.Code == code
}

func TestBundleImportDryRunWritesNothing(t *testing.T) {
	src := newMediaEnv(t, mediaCfg{})
	project, _ := src.bundleProject()
	bundle := src.exportBundle(project, nil).Result.Export.Path

	dst := newMediaEnv(t, mediaCfg{})

	res, err := dst.svc.(bundleImporter).ProjectBundleImport(
		dst.as(ui), service.ProjectBundleImportRequest{Path: bundle, DryRun: true},
	)
	if err != nil || res.Created {
		t.Fatalf("dry run = %+v, %v", res, err)
	}

	if listProjects(t, dst) != 0 || len(storeFiles(t, dst.ws)) != 0 {
		t.Error("a dry run wrote a project or a file")
	}
}

// SDD-007 T-20, bundle half: each hostile or broken bundle is refused with its code and leaves the
// database and store/ as they were.
func TestSecurity_T20_BundleArchiveRules(t *testing.T) {
	src := newMediaEnv(t, mediaCfg{})
	project, _ := src.bundleProject()
	good := src.exportBundle(project, nil).Result.Export.Path

	var mediaName string

	for _, e := range bundleEntries(t, good) {
		if strings.HasPrefix(e.name, "media.") {
			mediaName = e.name

			break
		}
	}

	hex64 := strings.Repeat("ab", 32)

	cases := []struct {
		name string
		code string
		edit func(t *testing.T, e []testEntry) []testEntry
	}{
		{"dot dot entry", "E_VALIDATION", func(_ *testing.T, e []testEntry) []testEntry {
			return append(e, testEntry{name: "../" + mediaName, body: []byte("escape")})
		}},
		{"absolute entry", "E_VALIDATION", func(_ *testing.T, e []testEntry) []testEntry {
			return append(e, testEntry{name: "/etc/passwd", body: []byte("escape")})
		}},
		{"backslash entry", "E_VALIDATION", func(_ *testing.T, e []testEntry) []testEntry {
			return append(e, testEntry{name: `..\media.` + hex64, body: []byte("escape")})
		}},
		{"subdirectory entry", "E_VALIDATION", func(_ *testing.T, e []testEntry) []testEntry {
			return append(e, testEntry{name: "sub/" + mediaName, body: []byte("twice")})
		}},
		{"directory entry", "E_VALIDATION", func(_ *testing.T, e []testEntry) []testEntry {
			return append(e, testEntry{name: "media/", body: nil, mode: fs.ModeDir | 0o755})
		}},
		{"symlink entry", "E_VALIDATION", func(_ *testing.T, e []testEntry) []testEntry {
			return append(
				e,
				testEntry{name: "media." + hex64, body: []byte("/etc/passwd"), mode: fs.ModeSymlink | 0o777},
			)
		}},
		{"encrypted entry", "E_VALIDATION", func(_ *testing.T, e []testEntry) []testEntry {
			e[entryIndex(e, "document.json")].flags = 0x1

			return e
		}},
		{"two entries with one name", "E_VALIDATION", func(_ *testing.T, e []testEntry) []testEntry {
			return append(e, e[entryIndex(e, mediaName)])
		}},
		{"two names that collapse to one key", "E_VALIDATION", func(_ *testing.T, e []testEntry) []testEntry {
			return append(e, testEntry{name: strings.ToUpper(mediaName), body: []byte("collapsing")})
		}},
		{"unlisted entry", "E_VALIDATION", func(_ *testing.T, e []testEntry) []testEntry {
			return append(e, testEntry{name: "media." + hex64, body: []byte("unlisted media")})
		}},
		{"too many entries", "E_LIMIT_EXCEEDED", func(_ *testing.T, e []testEntry) []testEntry {
			for i := range 1100 {
				e = append(e, testEntry{name: "media." + strings.Repeat("0", 60) + string(rune('a'+i%6)) +
					string(rune('a'+i/6%6)) + string(rune('a'+i/36%6)) + string(rune('a'+i/216%6)), body: []byte("x")})
			}

			return e
		}},
		{"zip bomb over the ratio", "E_LIMIT_EXCEEDED", func(_ *testing.T, e []testEntry) []testEntry {
			e[entryIndex(e, mediaName)] = testEntry{
				name: mediaName, body: make([]byte, 8<<20), method: zip.Deflate,
			}

			return e
		}},
		{"entry larger than its cap", "E_LIMIT_EXCEEDED", func(_ *testing.T, e []testEntry) []testEntry {
			e[entryIndex(e, "manifest.json")].body = bytes.Repeat([]byte(" "), 2<<20)

			return e
		}},
		{"media bytes that do not match the hash", "E_VALIDATION", func(_ *testing.T, e []testEntry) []testEntry {
			i := entryIndex(e, mediaName)
			e[i].body = append([]byte("X"), e[i].body[1:]...)

			return e
		}},
		{"media of another size", "E_VALIDATION", func(_ *testing.T, e []testEntry) []testEntry {
			i := entryIndex(e, mediaName)
			e[i].body = append(e[i].body, 'x')

			return e
		}},
		{"missing media entry", "E_VALIDATION", func(_ *testing.T, e []testEntry) []testEntry {
			i := entryIndex(e, mediaName)

			return append(e[:i:i], e[i+1:]...)
		}},
		{"document that does not match its hash", "E_VALIDATION", func(t *testing.T, e []testEntry) []testEntry {
			t.Helper()

			withManifest(t, e, func(m *service.BundleManifest) { m.Document.SHA256 = hex64 })

			return e
		}},
		{"document that is not a document", "E_VALIDATION", func(t *testing.T, e []testEntry) []testEntry {
			t.Helper()

			body := []byte(`{"schema_version":1}`)
			e[entryIndex(e, "document.json")].body = body
			withManifest(t, e, func(m *service.BundleManifest) {
				m.Document.SHA256, m.Document.Bytes = hexSum(body), int64(len(body))
			})

			return e
		}},
		{"newer bundle version", "E_VALIDATION", func(t *testing.T, e []testEntry) []testEntry {
			t.Helper()

			withManifest(t, e, func(m *service.BundleManifest) { m.BundleVersion = service.BundleVersion + 1 })

			return e
		}},
		{"another format", "E_VALIDATION", func(t *testing.T, e []testEntry) []testEntry {
			t.Helper()

			withManifest(t, e, func(m *service.BundleManifest) { m.Format = "something-else" })

			return e
		}},
		{"log with a gap", "E_VALIDATION", func(t *testing.T, e []testEntry) []testEntry {
			t.Helper()

			i := entryIndex(e, "operations.ndjson")
			lines := strings.SplitAfter(string(e[i].body), "\n")
			e[i].body = []byte(lines[0] + strings.Join(lines[2:], ""))
			withManifest(t, e, func(m *service.BundleManifest) {
				m.Operations.SHA256, m.Operations.Count = hexSum(e[i].body), m.Operations.Count-1
			})

			return e
		}},
		{"log that disagrees with the manifest hash", "E_VALIDATION", func(t *testing.T, e []testEntry) []testEntry {
			t.Helper()

			withManifest(t, e, func(m *service.BundleManifest) { m.Operations.SHA256 = hex64 })

			return e
		}},
		{
			"log whose last document hash is not the document",
			"E_VALIDATION",
			func(t *testing.T, e []testEntry) []testEntry {
				t.Helper()

				i := entryIndex(e, "operations.ndjson")
				lines := strings.SplitAfter(strings.TrimSuffix(string(e[i].body), "\n"), "\n")

				var op map[string]any
				if err := json.Unmarshal([]byte(strings.TrimSpace(lines[len(lines)-1])), &op); err != nil {
					t.Fatal(err)
				}

				op["doc_sha256"] = hex64

				b, err := json.Marshal(op)
				if err != nil {
					t.Fatal(err)
				}

				lines[len(lines)-1] = string(b)
				e[i].body = []byte(strings.Join(lines[:len(lines)-1], "") + string(b) + "\n")
				withManifest(t, e, func(m *service.BundleManifest) { m.Operations.SHA256 = hexSum(e[i].body) })

				return e
			},
		},
		{"manifest of another project", "E_VALIDATION", func(t *testing.T, e []testEntry) []testEntry {
			t.Helper()

			withManifest(t, e, func(m *service.BundleManifest) { m.Project.ID = "prj_0000000000" })

			return e
		}},
	}

	dst := newMediaEnv(t, mediaCfg{})
	files := t.TempDir()

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := writeBundleEntries(t, files, "hostile.vvbundle", c.edit(t, bundleEntries(t, good)))

			_, err := dst.importBundle(path, false)
			if !isCode(err, c.code) {
				t.Fatalf("import = %v, want %s", err, c.code)
			}

			// Nothing leaks: not the entry name, which is attacker text.
			if strings.Contains(err.Error(), "passwd") {
				t.Errorf("the error quotes an entry name: %v", err)
			}

			if listProjects(t, dst) != 0 || len(storeFiles(t, dst.ws)) != 0 {
				t.Errorf("a refused bundle left a project or %d files", len(storeFiles(t, dst.ws)))
			}

			if _, serr := os.Stat(filepath.Join(dst.ws, "..", "passwd")); serr == nil {
				t.Error("a file was written outside the workspace")
			}
		})
	}

	// The good bundle still imports after all that.
	if _, err := dst.importBundle(good, false); err != nil {
		t.Fatalf("the unmodified bundle: %v", err)
	}
}

// The disk check wants the declared total plus 10%.
func TestBundleImportDiskSpace(t *testing.T) {
	src := newMediaEnv(t, mediaCfg{})
	project, _ := src.bundleProject()
	bundle := src.exportBundle(project, nil).Result.Export.Path

	dst := newMediaEnv(t, mediaCfg{})
	dst.free.Store(10)

	if _, err := dst.importBundle(bundle, false); !isCode(err, "E_LIMIT_EXCEEDED") {
		t.Fatalf("import with no disk = %v, want E_LIMIT_EXCEEDED", err)
	}

	if listProjects(t, dst) != 0 || len(storeFiles(t, dst.ws)) != 0 {
		t.Error("a refused bundle left something behind")
	}
}

func TestBundleImportCallers(t *testing.T) {
	src := newMediaEnv(t, mediaCfg{})
	project, _ := src.bundleProject()
	bundle := src.exportBundle(project, nil).Result.Export.Path

	dst := newMediaEnv(t, mediaCfg{})
	imp := dst.svc.(bundleImporter)
	req := service.ProjectBundleImportRequest{Path: bundle}

	as := func(c service.Caller) error {
		_, err := imp.ProjectBundleImport(service.WithCaller(context.Background(), c), req)

		return err
	}

	if err := as(service.Caller{Actor: "key:ro", Scopes: []string{service.ScopeRead}}); !isCode(err, "E_FORBIDDEN") {
		t.Errorf("read only = %v, want E_FORBIDDEN", err)
	}

	if err := as(service.Caller{Actor: "key:edit", Scopes: []string{service.ScopeEdit}}); !isCode(err, "E_FORBIDDEN") {
		t.Errorf("edit without media = %v, want E_FORBIDDEN", err)
	}

	if err := as(
		service.Caller{Actor: "key:one", Scopes: service.AllScopes, ProjectID: project},
	); !isCode(
		err,
		"E_FORBIDDEN",
	) {
		t.Errorf("project-limited key = %v, want E_FORBIDDEN", err)
	}

	// Network mode: the server picks the id (no existence oracle), so keeping it is refused.
	member := service.Caller{
		Actor: "user:alice", Scopes: service.RoleScopes(service.RoleMember), OwnerUserID: "alice",
	}
	if err := as(member); !isCode(err, "E_VALIDATION") {
		t.Errorf("network import keeping the id = %v, want E_VALIDATION", err)
	}

	req.AsNew = true

	if err := as(member); err != nil {
		t.Errorf("network import as new: %v", err)
	}

	if listProjects(t, dst) != 1 {
		t.Error("the network import did not create its project")
	}
}

// SDD-007 T-20 (bundle half), shared store: in network mode a bundle cannot link a file that it does
// not carry, so user B cannot attach user A's media by naming its hash, and cannot tell whether the
// hash is in the store: the refusal is the same as for a hash that was never seen.
func TestSecurity_T20_BundleCannotNameAnotherUsersMedia(t *testing.T) {
	src := newMediaEnv(t, mediaCfg{})
	project, _ := src.bundleProject()
	good := src.exportBundle(project, nil).Result.Export.Path

	entries := bundleEntries(t, good)

	var kept []testEntry

	for _, e := range entries {
		if !strings.HasPrefix(e.name, "media.") {
			kept = append(kept, e)
		}
	}

	withManifest(t, kept, func(m *service.BundleManifest) { m.Media = []service.BundleMedia{} })

	stripped := writeBundleEntries(t, t.TempDir(), "stripped.vvbundle", kept)
	bob := service.Caller{Actor: "user:bob", Scopes: service.RoleScopes(service.RoleMember), OwnerUserID: "bob"}
	req := service.ProjectBundleImportRequest{Path: stripped, AsNew: true}

	// The store of src holds the hashes (user A's files); a fresh workspace never saw them.
	known, errKnown := src.svc.(bundleImporter).ProjectBundleImport(service.WithCaller(context.Background(), bob), req)
	fresh := newMediaEnv(t, mediaCfg{})
	_, errUnknown := fresh.svc.(bundleImporter).ProjectBundleImport(service.WithCaller(context.Background(), bob), req)

	if known != nil || !isCode(errKnown, "E_VALIDATION") || !isCode(errUnknown, "E_VALIDATION") {
		t.Fatalf("known = %+v, %v; unknown = %v; want both refused", known, errKnown, errUnknown)
	}

	if errKnown.Error() != errUnknown.Error() {
		t.Errorf("the refusals differ, which tells whether a hash exists:\n%v\n%v", errKnown, errUnknown)
	}

	var e *service.Error
	if errors.As(errKnown, &e) && e.Details["hash"] != nil {
		t.Errorf("the refusal names a hash: %v", e.Details)
	}

	if n, _ := src.store.Projects().Count(context.Background(), service.ProjectQuery{IncludeTrashed: true}); n != 1 {
		t.Errorf("%d projects in the source workspace after the refused import, want 1", n)
	}

	if listProjects(t, fresh) != 0 || len(storeFiles(t, fresh.ws)) != 0 {
		t.Error("a refused bundle left something behind")
	}

	// The same bundle is fine for the local owner, whose store is their own.
	if _, err := src.importBundle(stripped, true); err != nil {
		t.Errorf("local import of a bundle that relies on the store: %v", err)
	}
}

func TestSecurity_T20_ImportedHistoryCannotBeUndone(t *testing.T) {
	src := newMediaEnv(t, mediaCfg{})
	project, _ := src.bundleProject()

	job := src.exportBundle(project, nil)
	if job.State != service.JobSucceeded {
		t.Fatalf("export = %+v", job)
	}

	dst := newMediaEnv(t, mediaCfg{})
	if _, err := dst.importBundle(job.Result.Export.Path, false); err != nil {
		t.Fatal(err)
	}

	rows, err := dst.store.Operations().Since(context.Background(), project, 0, service.OperationFilter{})
	if err != nil || len(rows) == 0 {
		t.Fatalf("log = %d rows, %v", len(rows), err)
	}

	for _, r := range rows {
		if !strings.HasPrefix(r.Actor, "imported:") || r.ClosedBy == nil {
			t.Errorf(
				"revision %d: actor %q, closed_by %v; want an imported actor and a closed row",
				r.Revision,
				r.Actor,
				r.ClosedBy,
			)
		}
	}

	// The bundle was made by "local:ui"; the person at the editor here did not make those steps.
	for _, actor := range []string{ui, "local:" + "agent"} {
		if _, err := dst.svc.Undo(
			dst.as(actor),
			service.UndoRequest{ProjectID: project},
		); !isCode(
			err,
			"E_NOTHING_TO_UNDO",
		) {
			t.Errorf("%s undo after import = %v, want E_NOTHING_TO_UNDO", actor, err)
		}
	}
}
