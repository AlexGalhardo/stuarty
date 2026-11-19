package servicetest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service"
)

// otioSource is a project with a track, an imported clip placed on it twice, and a fade.
func (m *mediaEnv) otioSource() (project string) {
	m.t.Helper()

	project, track := m.withTrack()
	asset := m.importOne(project, m.write("clip.mp4", clipBytes)).Assets[0].AssetID
	head := m.head(project).Revision

	m.mustEdit(ui, project, head,
		op("clip.add", fmt.Sprintf(`{"track_id":%q,"asset_id":%q,"start":"0s","duration":"1s"}`, track, asset)),
		op("clip.add", fmt.Sprintf(`{"track_id":%q,"asset_id":%q,"start":"2s","duration":"1s"}`, track, asset)),
	)

	return project
}

func (m *mediaEnv) otioExport(project string) *service.ProjectOtioExportResult {
	m.t.Helper()

	res, err := m.svc.ProjectOtioExport(m.as(ui), service.ProjectOtioExportRequest{ProjectID: project})
	if err != nil {
		m.t.Fatal(err)
	}

	return res
}

func (m *mediaEnv) otioImport(project string, base int64, path string) (*service.ProjectOtioImportResult, error) {
	return m.svc.ProjectOtioImport(m.as(ui), service.ProjectOtioImportRequest{
		ProjectID: project, BaseRevision: base, Path: path,
	})
}

func TestOtioExportIsTheFileOfTheHeadAndWritesNothing(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project := m.otioSource()

	res := m.otioExport(project)
	if res.Revision != m.head(project).Revision || res.FileName != "p.otio" || res.Lost == nil {
		t.Errorf("result = %+v", res)
	}

	var file struct {
		Schema string `json:"OTIO_SCHEMA"`
	}

	if err := json.Unmarshal([]byte(res.Content), &file); err != nil || !strings.HasPrefix(file.Schema, "Timeline.") {
		t.Errorf("content is not an OTIO timeline: %v %q", err, file.Schema)
	}

	if !strings.Contains(res.Content, "clip.mp4") || strings.Contains(res.Content, "sha256:") {
		t.Errorf("content names the asset, never its hash")
	}

	if again := m.otioExport(project); again.Content != res.Content {
		t.Errorf("export is not deterministic")
	}
}

// P3-SRV-005 assert: an import is one undo step and undo restores the document byte for byte.
func TestOtioImportIsOneUndoStepAndRoundTrips(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	source := m.otioSource()
	file := m.write("source.otio", m.otioExport(source).Content)

	target := m.newProject("target")
	m.importOne(target, m.write("clip.mp4", clipBytes))

	before := bytes.Clone(m.document(target))
	base := m.head(target).Revision

	res, err := m.otioImport(target, base, file)
	if err != nil {
		t.Fatal(err)
	}

	if !res.Applied || res.Revision != base+1 || res.Edit == nil || res.Edit.Actor != ui {
		t.Fatalf("result = %+v", res)
	}

	if len(res.UnresolvedMedia) != 0 {
		t.Errorf("unresolved = %+v", res.UnresolvedMedia)
	}

	if len(res.MatchedMedia) == 0 || res.MatchedMedia[0].Matched != "by_name" || res.MatchedMedia[0].AssetID == "" ||
		len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "by file name") {
		t.Errorf("matched = %+v, warnings = %v; a media matched by name must be reported (VV-025)",
			res.MatchedMedia, res.Warnings)
	}

	doc, derr := timeline.Decode(m.document(target))
	if derr != nil {
		t.Fatal(derr)
	}

	clips := 0

	for id := range doc.Items {
		if doc.Items[id].Type == "clip" {
			clips++
		}
	}

	if clips != 2 {
		t.Errorf("clips = %d, want 2", clips)
	}

	if _, err := m.svc.Undo(m.as(ui), service.UndoRequest{ProjectID: target}); err != nil {
		t.Fatal(err)
	}

	if after := m.document(target); !bytes.Equal(after, before) {
		t.Errorf("undo did not restore the document:\n%s\n%s", before, after)
	}
}

func TestOtioImportReportsUnresolvedMediaAndLostItems(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	source := m.otioSource()
	content := m.otioExport(source).Content

	// The target has no asset of that name: every clip is left out.
	target := m.newProject("target")

	// Without allow_partial nothing is applied and the missing media is listed (VV-133).
	file := m.write("a.otio", content)
	if _, err := m.otioImport(target, m.head(target).Revision, file); code(err) != command.CodeInvalidState ||
		!strings.Contains(err.Error(), "does not have") {
		t.Fatalf("a file whose media is missing = %v, want E_INVALID_STATE", err)
	}

	if m.head(target).Revision != 0 {
		t.Fatal("a refused import applied something")
	}

	// The tracks still import when the caller allows it; the clips are reported.
	res, err := m.svc.ProjectOtioImport(m.as(ui), service.ProjectOtioImportRequest{
		ProjectID: target, BaseRevision: m.head(target).Revision, Path: file, AllowPartial: true,
	})
	if err != nil {
		t.Fatalf("a file with tracks and no media imports the tracks: %v", err)
	}

	if len(res.UnresolvedMedia) != 1 || res.UnresolvedMedia[0].Ref != "clip.mp4" ||
		res.UnresolvedMedia[0].Reason == "" {
		t.Errorf("unresolved = %+v", res.UnresolvedMedia)
	}

	if len(res.Lost) == 0 {
		t.Errorf("the clips left out are not in lost: %+v", res)
	}

	// VV-025: an import that placed no clip says why and what to do.
	if !strings.Contains(res.Text, "no clip was placed") || !strings.Contains(res.Text, "media:import") ||
		!strings.HasPrefix(res.Text, "imported ") {
		t.Errorf("text = %q", res.Text)
	}
}

// VV-108: the answer of the export, saved as it came, is accepted as the file.
func TestOtioImportAcceptsTheSavedAnswerOfTheExport(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	source := m.otioSource()
	exported := m.otioExport(source)

	raw, err := json.Marshal(exported)
	if err != nil {
		t.Fatal(err)
	}

	target := m.newProject("target")
	m.importOne(target, m.write("clip.mp4", clipBytes))

	res, err := m.otioImport(target, m.head(target).Revision, m.write("envelope.json", string(raw)))
	if err != nil || !res.Applied {
		t.Fatalf("the saved export answer was not accepted: %+v, %v", res, err)
	}
}

func TestOtioImportRefusals(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	source := m.otioSource()
	good := m.write("good.otio", m.otioExport(source).Content)
	target := m.newProject("target")
	base := m.head(target).Revision

	cases := map[string]struct {
		path string
		base int64
		code string
	}{
		"not an otio file": {m.write("not.otio", `{"hello":1}`), base, command.CodeValidation},
		"not json":         {m.write("bad.otio", "this is not json"), base, command.CodeValidation},
		"a url":            {"https://example.com/a.otio", base, "E_PATH_NOT_ALLOWED"},
		"a relative path":  {"a.otio", base, "E_PATH_NOT_ALLOWED"},
		"a missing file":   {m.files + "/nope.otio", base, "E_FILE_NOT_FOUND"},
		"a revision ahead": {good, base + 5, command.CodeValidation},
	}

	for name, c := range cases {
		_, err := m.otioImport(target, c.base, c.path)

		var se *service.Error
		if !errors.As(err, &se) || se.Code != c.code {
			t.Errorf("%s: err = %v, want %s", name, err, c.code)
		}
	}

	// A file at another frame rate than the project's.
	m.mustEdit(ui, target, base, op("settings.set_fps", `{"fps":"24"}`))

	_, err := m.otioImport(target, m.head(target).Revision, good)

	var se *service.Error
	if !errors.As(err, &se) || se.Code != command.CodeValidation || se.Path != "path" {
		t.Errorf("fps mismatch: %v", err)
	}
}

func TestOtioImportDryRunWritesNothing(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	source := m.otioSource()
	file := m.write("source.otio", m.otioExport(source).Content)
	target := m.newProject("target")
	m.importOne(target, m.write("clip.mp4", clipBytes))

	before, base := bytes.Clone(m.document(target)), m.head(target).Revision

	res, err := m.svc.ProjectOtioImport(m.as(ui), service.ProjectOtioImportRequest{
		ProjectID: target, BaseRevision: base, Path: file, DryRun: true,
	})
	if err != nil || res.Applied || res.Edit == nil || !res.Edit.DryRun {
		t.Fatalf("dry run = %+v, %v", res, err)
	}

	if !bytes.Equal(m.document(target), before) || m.head(target).Revision != base {
		t.Errorf("a dry run changed the project")
	}

	// VV-025: nothing was imported, so the text must not say it was.
	if !strings.HasPrefix(res.Text, "would import ") {
		t.Errorf("dry run text = %q", res.Text)
	}
}

func TestOtioOperationsNeedTheirScopes(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	source := m.otioSource()
	file := m.write("source.otio", m.otioExport(source).Content)
	target := m.newProject("target")

	readOnly := service.WithCaller(m.as(ui), service.Caller{Actor: "key:r", Scopes: []string{service.ScopeRead}})

	if _, err := m.svc.ProjectOtioExport(readOnly, service.ProjectOtioExportRequest{ProjectID: source}); err != nil {
		t.Errorf("read scope exports: %v", err)
	}

	_, err := m.svc.ProjectOtioImport(readOnly, service.ProjectOtioImportRequest{
		ProjectID: target, Path: file,
	})

	var se *service.Error
	if !errors.As(err, &se) || se.Code != service.CodeForbidden {
		t.Errorf("a read key imported: %v", err)
	}

	other := service.WithCaller(m.as(ui), service.Caller{
		Actor: "key:o", Scopes: service.AllScopes, ProjectID: target,
	})

	if _, err := m.svc.ProjectOtioExport(other, service.ProjectOtioExportRequest{ProjectID: source}); err == nil {
		t.Errorf("a key limited to another project exported")
	}
}
