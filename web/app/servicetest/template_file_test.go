package servicetest

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/core/timeline"
	"github.com/AlexGalhardo/vukem-video/service"
)

func readZipMember(t *testing.T, data []byte, name string) []byte {
	t.Helper()

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}

	for _, f := range zr.File {
		if f.Name != name {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}

		b, err := io.ReadAll(rc)
		_ = rc.Close()

		if err != nil {
			t.Fatal(err)
		}

		return b
	}

	t.Fatalf("no member %s", name)

	return nil
}

func zipMembers(t *testing.T, members map[string][]byte) []byte {
	t.Helper()

	var buf bytes.Buffer

	zw := zip.NewWriter(&buf)

	for _, name := range []string{"manifest.json", "template.json"} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}

		_, _ = w.Write(members[name])
	}

	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	return buf.Bytes()
}

func (m *mediaEnv) exportTemplate(id string) []byte {
	m.t.Helper()

	res, err := m.svc.TemplateExport(m.as(ui), service.TemplateRef{TemplateID: id})
	if err != nil || res.Path == "" || res.Bytes == 0 {
		m.t.Fatalf("export = %+v, %v", res, err)
	}

	data, err := os.ReadFile(res.Path)
	if err != nil {
		m.t.Fatal(err)
	}

	return data
}

// SDD-005 section 18 check 27: export then import on a fresh workspace yields
// the same template; a newer schema_version is refused.
func TestTemplateFileRoundTripAndNewerSchema(t *testing.T) {
	a := newMediaEnv(t, mediaCfg{})
	saved := a.saveTemplate(a.templateProject())
	file := a.exportTemplate(saved.Template.TemplateID)

	b := newMediaEnv(t, mediaCfg{})

	imported, err := b.svc.TemplateImport(b.as(ui), service.TemplateImportRequest{Path: b.write("t.zip", string(file))})
	if err != nil {
		t.Fatal(err)
	}

	want, got := a.templateDoc(saved.Template.TemplateID), b.templateDoc(imported.Template.TemplateID)
	if !bytes.Equal(timeline.Canonical(want), timeline.Canonical(got)) {
		t.Errorf("the imported template differs:\n%s\n%s", timeline.Canonical(want), timeline.Canonical(got))
	}

	if len(imported.Template.Slots) != len(saved.Template.Slots) {
		t.Errorf("slots %+v vs %+v", imported.Template.Slots, saved.Template.Slots)
	}

	// A newer schema_version.
	var doc map[string]any
	if err := json.Unmarshal(readZipMember(t, file, "template.json"), &doc); err != nil {
		t.Fatal(err)
	}

	doc["schema_version"] = 999

	newer, merr := json.Marshal(doc)
	if merr != nil {
		t.Fatal(merr)
	}
	bad := zipMembers(
		t,
		map[string][]byte{"manifest.json": readZipMember(t, file, "manifest.json"), "template.json": newer},
	)

	_, err = b.svc.TemplateImport(b.as(ui), service.TemplateImportRequest{Path: b.write("newer.zip", string(bad))})

	se := serviceErr(err)
	if se == nil || se.Code != command.CodeValidation || se.Details["code"] != "V_SCHEMA_VERSION" {
		t.Errorf("a newer schema_version = %v", err)
	}
}

func TestTemplateImportWritesNothingForARefusedFile(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})

	var buf bytes.Buffer

	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("../x")
	_, _ = w.Write([]byte("x"))
	_ = zw.Close()

	_, err := m.svc.TemplateImport(m.as(ui), service.TemplateImportRequest{Path: m.write("evil.zip", buf.String())})
	if code(err) != command.CodeValidation {
		t.Fatalf("import of a hostile archive = %v", err)
	}

	list, err := m.svc.TemplateList(m.as(ui), service.TemplateListRequest{})
	if err != nil || len(list.Templates) != 0 {
		t.Errorf("a refused import left %+v, %v", list, err)
	}

	if entries, _ := os.ReadDir(m.ws + "/templates"); len(entries) != 0 {
		t.Errorf("a refused import wrote %v", entries)
	}
}

func TestTemplateExportIsPerOwner(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	saved := m.saveTemplate(m.templateProject())
	bob := service.WithCaller(
		m.as(ui),
		service.Caller{Actor: "user:bob", OwnerUserID: "bob", Scopes: service.AllScopes},
	)

	// The template has no owner (local save): another user cannot see it.
	if _, err := m.svc.TemplateExport(
		bob,
		service.TemplateRef{TemplateID: saved.Template.TemplateID},
	); code(
		err,
	) != command.CodeNotFound {
		t.Errorf("export by another owner = %v", err)
	}

	if _, err := m.svc.OpenFile(
		bob,
		service.FileRef{Kind: "template", ID: saved.Template.TemplateID},
	); code(
		err,
	) != command.CodeNotFound {
		t.Errorf("download by another owner = %v", err)
	}
}

// VV-128: importing the same template file twice answers the existing template and creates no copy.
func TestTemplateImportDeduplicatesByContent(t *testing.T) {
	a := newMediaEnv(t, mediaCfg{})
	saved := a.saveTemplate(a.templateProject())
	file := string(a.exportTemplate(saved.Template.TemplateID))

	b := newMediaEnv(t, mediaCfg{})
	path := b.write("t.zip", file)

	first, err := b.svc.TemplateImport(b.as(ui), service.TemplateImportRequest{Path: path})
	if err != nil || first.Deduplicated {
		t.Fatalf("first = %+v, %v", first, err)
	}

	second, err := b.svc.TemplateImport(b.as(ui), service.TemplateImportRequest{Path: path})
	if err != nil || !second.Deduplicated || second.Template.TemplateID != first.Template.TemplateID ||
		!strings.Contains(second.Text, "already imported") {
		t.Fatalf("second = %+v, %v", second, err)
	}

	list, err := b.svc.TemplateList(b.as(ui), service.TemplateListRequest{})
	if err != nil {
		t.Fatal(err)
	}

	n := 0

	for i := range list.Templates {
		if list.Templates[i].Name == first.Template.Name {
			n++
		}
	}

	if n != 1 {
		t.Errorf("%d templates named %q after two imports, want 1", n, first.Template.Name)
	}
}
