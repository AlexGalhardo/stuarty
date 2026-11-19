package servicetest

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
)

// exportBundleRequest runs the bundle export of the request to the end and returns the path of the file.
func (m *mediaEnv) exportBundleRequest(req service.ProjectBundleExportRequest) string {
	m.t.Helper()

	job, err := m.svc.(bundleExporter).ProjectBundleExport(m.as(ui), req)
	if err != nil {
		m.t.Fatal(err)
	}

	if job.State == service.JobQueued {
		if err := m.runner.RunJob(context.Background(), job.JobID); err != nil {
			m.t.Fatal(err)
		}
	}

	if done := m.job(job.JobID); done.State != service.JobSucceeded || done.Result == nil || done.Result.Export == nil {
		m.t.Fatalf("export = %+v", done)
	} else {
		return done.Result.Export.Path
	}

	return ""
}

// productionProject is a project with media, two timelines (each with history), a brief, a text and an output.
func (m *mediaEnv) productionProject() (project, second string) {
	m.t.Helper()

	project, _ = m.bundleProject()
	ctx := m.as(ui)
	p := m.prod()

	added, err := p.ProjectItemAdd(
		ctx,
		service.ProjectItemAddRequest{ProjectID: project, Kind: "timeline", Title: "Wide cut", Preset: "16:9"},
	)
	if err != nil {
		m.t.Fatal(err)
	}

	second = added.Item.TimelineID
	ops := []command.Operation{op("track.add", `{"kind":"visual"}`)}

	for range 2 {
		head := m.head(second)
		if _, err := m.svc.Edit(
			ctx,
			service.EditRequest{TimelineID: second, BaseRevision: head.Revision, Operations: ops},
		); err != nil {
			m.t.Fatal(err)
		}
	}

	if _, err := p.BriefSet(
		ctx,
		service.BriefSetRequest{ProjectID: project, Text: "# Brief\n\nFor makers."},
	); err != nil {
		m.t.Fatal(err)
	}

	if _, err := p.ProjectItemAdd(
		ctx,
		service.ProjectItemAddRequest{ProjectID: project, Kind: "text", Title: "Script", Text: "Scene one."},
	); err != nil {
		m.t.Fatal(err)
	}

	m.exportJob(project, "job_0000000777", "succeeded")

	if _, err := p.ProjectItemAdd(
		ctx,
		service.ProjectItemAddRequest{ProjectID: project, Kind: "output", Title: "Final", JobID: "job_0000000777"},
	); err != nil {
		m.t.Fatal(err)
	}

	return project, second
}

type itemShape struct {
	Kind, Title string
	Position    int
	Timeline    string
}

func shapeOf(items []service.ProjectItem) []itemShape {
	out := make([]itemShape, len(items))
	for i := range items {
		it := &items[i]
		out[i] = itemShape{it.Kind, it.Title, it.Position, it.TimelineID}
	}

	return out
}

// The bundle of a project holds the whole project (SDD-005 section 16.1.1), and importing it in an empty
// workspace gives the project back: its items, its timelines with their documents and their history.
func TestBundleWholeProjectRoundTrip(t *testing.T) {
	src := newMediaEnv(t, mediaCfg{})
	project, second := src.productionProject()
	path := src.exportBundleRequest(service.ProjectBundleExportRequest{ProjectID: project})

	_, files := readZip(t, path)

	var man service.BundleManifest
	if err := json.Unmarshal(files["manifest.json"], &man); err != nil {
		t.Fatal(err)
	}

	if man.BundleVersion != 2 || man.Production == nil || len(man.Production.Items) != 5 || len(man.Timelines) != 1 ||
		man.Timelines[0].ID != second || man.Timelines[0].Key != "t1" {
		t.Fatalf("manifest = %+v", man)
	}

	for _, name := range []string{"document.json", "operations.ndjson", "timeline.t1.json", "timeline.t1.ndjson"} {
		if _, ok := files[name]; !ok {
			t.Errorf("the bundle lacks %s", name)
		}
	}

	if !bytes.Equal(files["timeline.t1.json"], src.document(second)) {
		t.Error("the second document is not the stored one")
	}

	dst := newMediaEnv(t, mediaCfg{})

	res, err := dst.importBundle(path, false)
	if err != nil || !res.Created || res.ProjectID != project {
		t.Fatalf("import = %+v, %v", res, err)
	}

	want := src.get(project, "", "")
	got := dst.get(project, "", "")

	if !slices.Equal(shapeOf(want.Items), shapeOf(got.Items)) || got.Production != want.Production {
		t.Fatalf(
			"items\n got %+v / %+v\nwant %+v / %+v",
			shapeOf(got.Items),
			got.Production,
			shapeOf(want.Items),
			want.Production,
		)
	}

	for _, id := range []string{project, second} {
		if !bytes.Equal(dst.document(id), src.document(id)) {
			t.Errorf("the document of %s differs", id)
		}

		if dst.head(id).Revision != src.head(id).Revision || dst.operationCount(id) != src.operationCount(id) {
			t.Errorf("%s: revision %d and %d rows, want %d and %d", id,
				dst.head(id).Revision, dst.operationCount(id), src.head(id).Revision, src.operationCount(id))
		}

		hist, err := dst.svc.HistoryList(dst.as(ui), service.HistoryListRequest{TimelineID: id})
		if err != nil || len(hist.Entries) != src.operationCount(id) {
			t.Errorf("history of %s: %v", id, err)
		}

		// The imported steps are history of nobody who is here.
		if _, err := dst.svc.Undo(
			dst.as(ui),
			service.UndoRequest{TimelineID: id},
		); code(
			err,
		) != service.CodeNothingToUndo {
			t.Errorf("undo of an imported step of %s: %v", id, err)
		}
	}

	if b, _ := dst.prod().
		BriefGet(dst.as(ui), service.BriefGetRequest{ProjectID: project}); b.Text != "# Brief\n\nFor makers." {
		t.Errorf("brief = %q", b.Text)
	}

	for _, it := range got.Items {
		if it.Kind == "output" && (it.Format != "mp4" || it.JobID != "") {
			t.Errorf("output item = %+v", it)
		}
	}

	// A copy gets new ids for the project and for every timeline, and no log.
	cp, err := src.importBundle(path, true)
	if err != nil || cp.ProjectID == project {
		t.Fatalf("copy = %+v, %v", cp, err)
	}

	copied := src.get(cp.ProjectID, "", "")
	if len(copied.Items) != 5 || copied.Production.Timelines != 2 || copied.Production != want.Production {
		t.Fatalf("copy = %+v / %+v", shapeOf(copied.Items), copied.Production)
	}

	for _, it := range copied.Items {
		if it.TimelineID == project || it.TimelineID == second {
			t.Errorf("the copy kept the timeline id %s", it.TimelineID)
		}

		if it.TimelineID != "" && src.head(it.TimelineID).Revision != 1 {
			t.Errorf("a copied timeline is at revision %d", src.head(it.TimelineID).Revision)
		}
	}

	// Importing again with the ids that exist is refused.
	if _, err := src.importBundle(path, false); code(err) != "E_ID_EXISTS" {
		t.Errorf("import over the same ids: %v", err)
	}
}

// A bundle of one timeline (a timeline_id) is the former format and imports as a project with one timeline.
func TestBundleOneTimelineIsTheFormerFormat(t *testing.T) {
	src := newMediaEnv(t, mediaCfg{})
	project, second := src.productionProject()

	path := src.exportBundleRequest(service.ProjectBundleExportRequest{ProjectID: project, TimelineID: second})

	_, files := readZip(t, path)

	var man service.BundleManifest
	if err := json.Unmarshal(files["manifest.json"], &man); err != nil {
		t.Fatal(err)
	}

	if man.BundleVersion != 1 || man.Production != nil || len(man.Timelines) != 0 || man.Project.ID != second {
		t.Fatalf("manifest = %+v", man)
	}

	dst := newMediaEnv(t, mediaCfg{})

	res, err := dst.importBundle(path, false)
	if err != nil || res.ProjectID != second {
		t.Fatalf("import = %+v, %v", res, err)
	}

	got := dst.get(second, "", "")
	if len(got.Items) != 1 || got.Items[0].Kind != "timeline" || got.Production.Timelines != 1 {
		t.Fatalf("items = %+v", got.Items)
	}
}

// A bundle made by release 0.29.0 (testdata/release-0.29.0.vvbundle, written by that release with
// bundle:export of a project that had four steps of history, one of them an undo) imports as a project with one
// timeline.
func TestBundleFromRelease029(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})

	raw, err := os.ReadFile(filepath.Join("testdata", "release-0.29.0.vvbundle"))
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(m.files, "release-0.29.0.vvbundle")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	for _, asNew := range []bool{false, true} {
		res, err := m.importBundle(path, asNew)
		if err != nil || !res.Created {
			t.Fatalf("import as_new=%v: %+v, %v", asNew, res, err)
		}

		got := m.get(res.ProjectID, "", "")
		if len(got.Items) != 1 || got.Items[0].Kind != "timeline" || got.Items[0].TimelineID != res.ProjectID ||
			got.Production.Timelines != 1 || got.Name != "Release 0.29 project" {
			t.Fatalf("as_new=%v: %+v", asNew, got)
		}

		if !asNew {
			if res.ProjectID != "prj_0290000001" || got.Revision != 4 || m.operationCount(res.ProjectID) != 4 {
				t.Errorf("revision %d with %d rows", got.Revision, m.operationCount(res.ProjectID))
			}

			hist, err := m.svc.HistoryList(m.as(ui), service.HistoryListRequest{ProjectID: res.ProjectID})
			if err != nil || len(hist.Entries) != 4 {
				t.Errorf("history = %+v, %v", hist, err)
			}
		}

		// The project edits as any other.
		if _, err := m.svc.Edit(m.as(ui), service.EditRequest{
			ProjectID:    res.ProjectID,
			BaseRevision: got.Revision,
			Operations:   []command.Operation{op("track.add", `{"kind":"audio"}`)},
		}); err != nil {
			t.Errorf("edit after the import: %v", err)
		}
	}
}

// A bundle of version 2 that lies about its production or its timelines is refused before anything is written.
func TestBundleProductionIsChecked(t *testing.T) {
	src := newMediaEnv(t, mediaCfg{})
	project, _ := src.productionProject()
	good := src.exportBundleRequest(service.ProjectBundleExportRequest{ProjectID: project})
	dir := t.TempDir()

	mutate := map[string]func(e []testEntry) []testEntry{
		"a timeline entry that is not listed": func(e []testEntry) []testEntry {
			return append(e, testEntry{name: "timeline.t9.json", body: []byte("{}")})
		},
		"a missing timeline document": func(e []testEntry) []testEntry {
			return slices.DeleteFunc(e, func(x testEntry) bool { return x.name == "timeline.t1.json" })
		},
		"a changed timeline document": func(e []testEntry) []testEntry {
			i := entryIndex(e, "timeline.t1.json")
			e[i].body = append(bytes.Clone(e[i].body), ' ')

			return e
		},
		"an item that names no timeline": func(e []testEntry) []testEntry {
			withManifest(t, e, func(m *service.BundleManifest) {
				for i := range m.Production.Items {
					if m.Production.Items[i].Timeline == "t1" {
						m.Production.Items[i].Timeline = "t7"
					}
				}
			})

			return e
		},
		"two briefs": func(e []testEntry) []testEntry {
			withManifest(t, e, func(m *service.BundleManifest) {
				m.Production.Items = append(
					m.Production.Items,
					service.BundleItem{Kind: "brief", Title: "Again", Position: 9},
				)
			})

			return e
		},
		"a kind that is reserved": func(e []testEntry) []testEntry {
			withManifest(t, e, func(m *service.BundleManifest) {
				m.Production.Items = append(
					m.Production.Items,
					service.BundleItem{Kind: "publication", Title: "Post", Position: 9},
				)
			})

			return e
		},
		"a timeline listed twice": func(e []testEntry) []testEntry {
			withManifest(t, e, func(m *service.BundleManifest) { m.Timelines = append(m.Timelines, m.Timelines[0]) })

			return e
		},
		"timelines without a production": func(e []testEntry) []testEntry {
			withManifest(t, e, func(m *service.BundleManifest) { m.Production = nil })

			return e
		},
		"a timeline with the id of the project": func(e []testEntry) []testEntry {
			withManifest(t, e, func(m *service.BundleManifest) { m.Timelines[0].ID = m.Project.ID })

			return e
		},
		"a title with a control character": func(e []testEntry) []testEntry {
			withManifest(t, e, func(m *service.BundleManifest) { m.Production.Items[0].Title = "a\x00b" })

			return e
		},
		"a text entry that is not listed": func(e []testEntry) []testEntry {
			return append(e, testEntry{name: "item.200.md", body: []byte("riding along")})
		},
		"a changed text": func(e []testEntry) []testEntry {
			i := entryIndex(e, textEntryOf(t, e, "brief"))
			e[i].body = append(bytes.Clone(e[i].body), '!')

			return e
		},
		"a missing text": func(e []testEntry) []testEntry {
			name := textEntryOf(t, e, "text")

			return slices.DeleteFunc(e, func(x testEntry) bool { return x.name == name })
		},
	}

	dst := newMediaEnv(t, mediaCfg{})
	before := listProjects(t, dst)

	for name, edit := range mutate {
		entries := edit(bundleEntries(t, good))
		bad := writeBundleEntries(t, dir, strings.ReplaceAll(name, " ", "-")+".vvbundle", entries)

		for _, asNew := range []bool{false, true} {
			if _, err := dst.importBundle(bad, asNew); code(err) != "E_VALIDATION" {
				t.Errorf("%s (as_new %v): %v", name, asNew, err)
			}
		}
	}

	if listProjects(t, dst) != before {
		t.Error("a refused bundle left a project behind")
	}

	// And the unmodified bundle still imports.
	if _, err := dst.importBundle(good, false); err != nil {
		t.Fatal(err)
	}
}

// textEntryOf is the name of the entry that holds the text of the first item of the kind in the manifest.
func textEntryOf(t *testing.T, entries []testEntry, kind string) string {
	t.Helper()

	var man service.BundleManifest
	if err := json.Unmarshal(entries[entryIndex(entries, "manifest.json")].body, &man); err != nil {
		t.Fatal(err)
	}

	for i, it := range man.Production.Items {
		if it.Kind == kind && it.Text != nil {
			return "item." + strconv.Itoa(i) + ".md"
		}
	}

	t.Fatalf("no %s with a text in the manifest", kind)

	return ""
}

// The texts of a project travel in entries of their own, not in the manifest, whose size is capped: a project
// with a brief and texts near their limit exports into a bundle that imports again with every text whole.
func TestBundleLongTextsRoundTrip(t *testing.T) {
	src := newMediaEnv(t, mediaCfg{})
	projectID, _ := src.bundleProject()
	ctx := src.as(ui)

	// Text that compresses as prose does, so the entries stay under the ratio check of an import.
	long := func(seed int) string {
		var b strings.Builder

		x := uint32(seed*2654435761 + 1)
		for b.Len() < 256<<10-64 { // just under the limit of a text
			x ^= x << 13
			x ^= x >> 17
			x ^= x << 5
			b.WriteByte(byte('a' + x%26))

			if x%7 == 0 {
				b.WriteByte(' ')
			}
		}

		return b.String()
	}

	want := map[string]string{}

	if _, err := src.prod().BriefSet(ctx, service.BriefSetRequest{ProjectID: projectID, Text: long(0)}); err != nil {
		t.Fatal(err)
	}

	want["brief"] = long(0)

	for i := 1; i <= 4; i++ {
		title := "Script " + strconv.Itoa(i)
		if _, err := src.prod().ProjectItemAdd(ctx, service.ProjectItemAddRequest{
			ProjectID: projectID, Kind: "text", Title: title, Text: long(i),
		}); err != nil {
			t.Fatal(err)
		}

		want[title] = long(i)
	}

	path := src.exportBundleRequest(service.ProjectBundleExportRequest{ProjectID: projectID})

	_, files := readZip(t, path)
	if n := len(files["manifest.json"]); n > 64<<10 {
		t.Errorf("the manifest holds %d bytes; the texts are in it", n)
	}

	dst := newMediaEnv(t, mediaCfg{})
	if _, err := dst.importBundle(path, false); err != nil {
		t.Fatalf("import: %v", err)
	}

	rows, err := dst.store.Productions().Items(context.Background(), projectID)
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]string{}

	for _, r := range rows {
		switch r.Kind {
		case "brief":
			got["brief"] = r.Body
		case "text":
			got[r.Title] = r.Body
		}
	}

	if len(got) != len(want) {
		t.Fatalf("imported %d texts, want %d", len(got), len(want))
	}

	for k, w := range want {
		if got[k] != w {
			t.Errorf("the text of %s differs: %d bytes, want %d", k, len(got[k]), len(w))
		}
	}
}
