package servicetest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/core/timeline"
	"github.com/AlexGalhardo/vukem-video/service"
)

// templateProject is a project with a required text slot "title" and a required
// media slot "hero", ready to be saved as a template.
func (m *mediaEnv) templateProject() string {
	m.t.Helper()

	project, track := m.withTrack()
	rev := func() int64 { return m.head(project).Revision }

	m.mustEdit(ui, project, rev(), op("template.set_info", `{"description":"a template"}`))

	text, err := m.addText(ui, project, track, rev(), "0s", "headline")
	if err != nil {
		m.t.Fatal(err)
	}

	clip := m.mustEdit(ui, project, rev(), op(
		"clip.add", fmt.Sprintf(`{"track_id":%q,"asset_id":"ast_placeholder","start":"1s","duration":"1s"}`, track),
	))

	m.mustEdit(ui, project, rev(),
		op("slot.mark", fmt.Sprintf(`{"item_id":%q,"name":"title"}`, text.Created[0])),
		op("slot.mark", fmt.Sprintf(`{"item_id":%q,"name":"hero","description":"the main clip"}`, clip.Created[0])),
	)

	return project
}

func (m *mediaEnv) saveTemplate(project string) *service.TemplateResult {
	m.t.Helper()

	res, err := m.svc.TemplateSave(m.as(ui), service.TemplateSaveRequest{
		ProjectID: project, Name: "Launch", Description: "a launch teaser",
	})
	if err != nil {
		m.t.Fatal(err)
	}

	return res
}

func (m *mediaEnv) templateDoc(id string) *timeline.Document {
	m.t.Helper()

	row, err := m.store.Templates().Get(context.Background(), id)
	if err != nil {
		m.t.Fatal(err)
	}

	doc, err := timeline.Decode(row.Document)
	if err != nil {
		m.t.Fatal(err)
	}

	return doc
}

func (m *mediaEnv) projectIDs() []string {
	m.t.Helper()

	res, err := m.svc.ProjectList(m.as(ui), service.ProjectListRequest{State: "all", Limit: 100})
	if err != nil {
		m.t.Fatal(err)
	}

	ids := make([]string, 0, len(res.Projects))
	for i := range res.Projects {
		ids = append(ids, res.Projects[i].ID)
	}

	return ids
}

func serviceErr(err error) *service.Error {
	se, _ := errors.AsType[*service.Error](err)

	return se
}

// SDD-005 section 18 check 27 and ADR-0022: a template carries no media, and
// the project made from it is the template document with its own id and name.
func TestTemplateSaveAndInstantiateUnfilled(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{exporter: goodExporter()})
	source := m.templateProject()
	opsBefore := m.operationCount(source)

	saved := m.saveTemplate(source)
	if m.operationCount(source) != opsBefore {
		t.Error("saving a template wrote a log row")
	}

	if len(saved.Template.Slots) != 2 || saved.Template.Slots[0].Name != "hero" || !saved.Template.Slots[0].Required {
		t.Errorf("slots = %+v", saved.Template.Slots)
	}

	// TestTemplateHasNoRealAssets: every asset reference is the placeholder.
	doc := m.templateDoc(saved.Template.TemplateID)
	if len(doc.Assets) != 0 {
		t.Errorf("a template holds assets: %v", doc.Assets)
	}

	for id, it := range doc.Items {
		if (it.Type == timeline.ItemClip || it.Type == timeline.ItemAudio) && it.Asset != timeline.PlaceholderAsset {
			t.Errorf("item %s references %s", id, it.Asset)
		}
	}

	if again := m.saveTemplate(source); again.Template.TemplateID == saved.Template.TemplateID {
		t.Error("saving twice reused the template id")
	}

	res, err := m.svc.TemplateInstantiate(m.as(ui), service.TemplateInstantiateRequest{
		TemplateID: saved.Template.TemplateID, Name: "Spring teaser",
	})
	if err != nil || res.Revision != 0 || !slices.Equal(res.Unfilled, []string{"hero", "title"}) {
		t.Fatalf("instantiate = %+v, %v", res, err)
	}

	want := timeline.Clone(doc)
	want.ID, want.Name = res.ProjectID, "Spring teaser"
	want.Template.Instance = true

	if got := m.document(res.ProjectID); !bytes.Equal(got, timeline.Canonical(want)) {
		t.Errorf("the project is not the template document:\n got  %s\n want %s", got, timeline.Canonical(want))
	}

	// RF-051: the unfilled required slots are lint errors and block an export.
	lint, err := m.svc.Lint(m.as(ui), service.LintRequest{ProjectID: res.ProjectID})
	if err != nil {
		t.Fatal(err)
	}

	found := 0

	for _, f := range lint.Findings {
		if f.Code == "L_SLOT_UNFILLED" {
			found++
		}
	}

	if found != 2 || lint.Errors < 2 {
		t.Errorf("lint = %+v", lint)
	}

	_, err = m.svc.JobStart(m.as(ui), service.JobStartRequest{ProjectID: res.ProjectID, Kind: "export"})
	if code(err) != service.CodeLintBlocking {
		t.Errorf("export with unfilled slots = %v, want E_LINT_BLOCKING", err)
	}
}

func TestTemplateInstantiateFillsInOneRevision(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	template := m.saveTemplate(m.templateProject()).Template.TemplateID

	library := m.newProject("library")
	asset := m.importOne(library, m.write("a.mp4", clipBytes)).Assets[0].AssetID
	title := "Spring"

	res, err := m.svc.TemplateInstantiate(m.as(ui), service.TemplateInstantiateRequest{
		TemplateID: template, Name: "Filled",
		Fill: map[string]service.SlotValue{"title": {Text: &title}, "hero": {AssetID: asset}},
	})
	if err != nil || res.Revision != 1 || len(res.Unfilled) != 0 {
		t.Fatalf("instantiate = %+v, %v", res, err)
	}

	if n := m.operationCount(res.ProjectID); n != 1 {
		t.Errorf("the fill is %d log rows, want one revision", n)
	}

	doc, _ := timeline.Decode(m.document(res.ProjectID))
	if _, ok := doc.Assets[asset]; !ok {
		t.Error("the asset of the fill is not in the new project")
	}

	part, err := m.svc.TemplateInstantiate(m.as(ui), service.TemplateInstantiateRequest{
		TemplateID: template, Name: "Half", Fill: map[string]service.SlotValue{"title": {Text: &title}},
	})
	if err != nil || !slices.Equal(part.Unfilled, []string{"hero"}) {
		t.Errorf("partial = %+v, %v", part, err)
	}
}

func TestTemplateInstantiateRefusesWhatItCannotFillAndLeavesNoProject(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	template := m.saveTemplate(m.templateProject()).Template.TemplateID
	before := len(m.projectIDs())
	title := "x"

	for name, fill := range map[string]map[string]service.SlotValue{
		"unknown slot":    {"nope": {Text: &title}},
		"two members":     {"title": {Text: &title, AssetID: "ast_0000000001"}},
		"no member":       {"title": {}},
		"unknown asset":   {"hero": {AssetID: "ast_0000000001"}},
		"text in a media": {"hero": {Text: &title}},
		"asset in a text": {"title": {AssetID: "ast_0000000001"}},
	} {
		_, err := m.svc.TemplateInstantiate(m.as(ui), service.TemplateInstantiateRequest{
			TemplateID: template, Name: "Bad", Fill: fill,
		})
		if err == nil {
			t.Errorf("%s: accepted", name)
		}
	}

	if after := len(m.projectIDs()); after != before {
		t.Errorf("a refused instantiation left %d project(s)", after-before)
	}

	_, err := m.svc.TemplateInstantiate(m.as(ui), service.TemplateInstantiateRequest{
		TemplateID: "tpl_0000000000", Name: "x",
	})
	if code(err) != command.CodeNotFound {
		t.Errorf("unknown template = %v", err)
	}
}

// The asset of a fill is looked up among the projects the caller can see: the
// asset of another owner answers E_NOT_FOUND, with the message of an asset that
// does not exist, so nothing is disclosed.
func TestTemplateFillRefusesAnotherOwnersAsset(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	alice := service.WithCaller(context.Background(), service.Caller{
		Actor: "user:alice", OwnerUserID: "alice", Scopes: service.AllScopes,
	})
	bob := service.WithCaller(context.Background(), service.Caller{
		Actor: "user:bob", OwnerUserID: "bob", Scopes: service.AllScopes,
	})

	mk := func(c context.Context, name string) string {
		res, err := m.svc.ProjectCreate(c, service.ProjectCreateRequest{Name: name})
		if err != nil {
			t.Fatal(err)
		}

		return res.ProjectID
	}

	// mkTemplate saves a one-slot template owned by c.
	mkTemplate := func(c context.Context) string {
		edit := func(project string, base int64, ops ...command.Operation) *service.EditResult {
			res, err := m.svc.Edit(c, service.EditRequest{ProjectID: project, BaseRevision: base, Operations: ops})
			if err != nil {
				t.Fatal(err)
			}

			return res
		}

		source := mk(c, "tpl")
		track := edit(source, 0, op("track.add", `{"kind":"visual","name":"V"}`)).Created[0]
		edit(source, 1, op("template.set_info", `{"description":"t"}`))
		clip := edit(source, 2, op(
			"clip.add", fmt.Sprintf(`{"track_id":%q,"asset_id":"ast_placeholder","start":"0s","duration":"1s"}`, track),
		))
		edit(source, 3, op("slot.mark", fmt.Sprintf(`{"item_id":%q,"name":"hero"}`, clip.Created[0])))

		tpl, err := m.svc.TemplateSave(c, service.TemplateSaveRequest{ProjectID: source, Name: "T"})
		if err != nil {
			t.Fatal(err)
		}

		return tpl.Template.TemplateID
	}

	aliceTemplate, bobTemplate := mkTemplate(alice), mkTemplate(bob)

	imp, err := m.svc.MediaImport(alice, service.MediaImportRequest{
		ProjectID: mk(alice, "lib"), Files: []service.ImportFile{{Path: m.write("a.mp4", clipBytes)}},
	})
	if err != nil {
		t.Fatal(err)
	}

	fill := map[string]service.SlotValue{"hero": {AssetID: imp.Assets[0].AssetID}}

	if _, err := m.svc.TemplateInstantiate(alice, service.TemplateInstantiateRequest{
		TemplateID: aliceTemplate, Name: "Mine", Fill: fill,
	}); err != nil {
		t.Fatalf("the owner of the asset: %v", err)
	}

	_, hidden := m.svc.TemplateInstantiate(bob, service.TemplateInstantiateRequest{
		TemplateID: bobTemplate, Name: "Theirs", Fill: fill,
	})
	if code(hidden) != command.CodeNotFound {
		t.Fatalf("another owner's asset = %v, want E_NOT_FOUND", hidden)
	}

	_, missing := m.svc.TemplateInstantiate(bob, service.TemplateInstantiateRequest{
		TemplateID: bobTemplate, Name: "Theirs",
		Fill: map[string]service.SlotValue{"hero": {AssetID: "ast_0000000001"}},
	})

	h, mi := serviceErr(hidden), serviceErr(missing)
	if h == nil || mi == nil || h.Code != mi.Code || h.Hint != mi.Hint ||
		h.Message != "there is no asset "+imp.Assets[0].AssetID+"." || mi.Message != "there is no asset ast_0000000001." {
		t.Errorf("a hidden asset must look like a missing one: %+v / %+v", h, mi)
	}
}

func TestTemplateOperationsRefuseAProjectLimitedKey(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project := m.templateProject()
	limited := service.WithCaller(context.Background(), service.Caller{
		Actor: "key:k", Scopes: service.AllScopes, ProjectID: project,
	})

	_, err := m.svc.TemplateSave(limited, service.TemplateSaveRequest{ProjectID: project, Name: "x"})
	if code(err) != service.CodeForbidden {
		t.Errorf("save = %v", err)
	}

	if _, err := m.svc.TemplateList(limited, service.TemplateListRequest{}); code(err) != service.CodeForbidden {
		t.Errorf("list = %v", err)
	}
}

func TestTemplateSaveRefusesMediaThatIsNotASlot(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project, track := m.withTrack()
	asset := m.importOne(project, m.write("a.mp4", clipBytes)).Assets[0].AssetID

	m.mustEdit(ui, project, m.head(project).Revision, op(
		"clip.add", fmt.Sprintf(`{"track_id":%q,"asset_id":%q,"start":"0s","duration":"1s"}`, track, asset),
	))

	_, err := m.svc.TemplateSave(m.as(ui), service.TemplateSaveRequest{ProjectID: project, Name: "x"})
	if code(err) != command.CodeSlot {
		t.Errorf("save with a plain clip = %v, want E_SLOT", err)
	}
}

func TestTemplateListPagesAndShowsSlots(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project := m.templateProject()

	for range 3 {
		m.saveTemplate(project)
	}

	var req service.TemplateListRequest

	req.Limit = 2

	first, err := m.svc.TemplateList(m.as(ui), req)
	if err != nil || len(first.Templates) != 2 || first.NextCursor == nil {
		t.Fatalf("first page = %+v, %v", first, err)
	}

	req.Cursor = *first.NextCursor

	rest, err := m.svc.TemplateList(m.as(ui), req)
	if err != nil || len(rest.Templates) != 1 || rest.NextCursor != nil {
		t.Fatalf("second page = %+v, %v", rest, err)
	}

	if s := first.Templates[0].Slots; len(s) != 2 || s[0].Kind != "media" || s[1].Kind != "text" {
		t.Errorf("slots = %+v", s)
	}
}

// A template belongs to the user who saved it: another owner cannot list it or
// make a project from it, and the refusal is the one of a missing template.
func TestTemplatesBelongToTheirOwner(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	alice := service.WithCaller(context.Background(), service.Caller{
		Actor: "user:alice", OwnerUserID: "alice", Scopes: service.AllScopes,
	})
	bob := service.WithCaller(context.Background(), service.Caller{
		Actor: "user:bob", OwnerUserID: "bob", Scopes: service.AllScopes,
	})

	res, err := m.svc.ProjectCreate(alice, service.ProjectCreateRequest{Name: "src"})
	if err != nil {
		t.Fatal(err)
	}

	saved, err := m.svc.TemplateSave(alice, service.TemplateSaveRequest{ProjectID: res.ProjectID, Name: "Mine"})
	if err != nil {
		t.Fatal(err)
	}

	id := saved.Template.TemplateID

	if list, err := m.svc.TemplateList(alice, service.TemplateListRequest{}); err != nil || len(list.Templates) != 1 {
		t.Errorf("the owner lists %+v, %v", list, err)
	}

	if list, err := m.svc.TemplateList(bob, service.TemplateListRequest{}); err != nil || len(list.Templates) != 0 {
		t.Errorf("another owner lists %+v, %v; want none", list, err)
	}

	var page service.TemplateListRequest

	page.Cursor = id
	if list, err := m.svc.TemplateList(bob, page); err != nil || len(list.Templates) != 0 {
		t.Errorf("a cursor on another owner's template: %+v, %v", list, err)
	}

	_, hidden := m.svc.TemplateInstantiate(bob, service.TemplateInstantiateRequest{TemplateID: id, Name: "x"})
	_, missing := m.svc.TemplateInstantiate(
		bob,
		service.TemplateInstantiateRequest{TemplateID: "tpl_0000000000", Name: "x"},
	)

	h, mi := serviceErr(hidden), serviceErr(missing)
	if h == nil || mi == nil || h.Code != command.CodeNotFound || h.Code != mi.Code || h.Hint != mi.Hint ||
		h.Message != "there is no template "+id+"." || mi.Message != "there is no template tpl_0000000000." {
		t.Errorf("a hidden template must look like a missing one: %+v / %+v", h, mi)
	}

	if _, err := m.svc.TemplateInstantiate(
		alice,
		service.TemplateInstantiateRequest{TemplateID: id, Name: "ok"},
	); err != nil {
		t.Errorf("the owner instantiates: %v", err)
	}
}

func TestTemplateDescriptionLimit(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project := m.templateProject()

	long := strings.Repeat("é", 501)
	if _, err := m.svc.TemplateSave(m.as(ui), service.TemplateSaveRequest{
		ProjectID: project, Name: "x", Description: long,
	}); code(err) != command.CodeValidation {
		t.Errorf("a 501-character description = %v, want E_VALIDATION", err)
	}

	if _, err := m.svc.TemplateSave(m.as(ui), service.TemplateSaveRequest{
		ProjectID: project, Name: "x", Description: long[:len(long)-2],
	}); err != nil {
		t.Errorf("a 500-character description: %v", err)
	}
}

func TestTemplateDryRunTextSaysWouldNotDone(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project := m.exportable()

	saved, err := m.svc.TemplateSave(m.as(ui), service.TemplateSaveRequest{ProjectID: project, Name: "t", DryRun: true})
	if err != nil || !strings.Contains(saved.Text, "would be saved") {
		t.Fatalf("dry-run save = %+v, %v", saved, err)
	}
}
