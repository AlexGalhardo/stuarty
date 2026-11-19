package servicetest

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
)

// SDD-007 T-05.
func TestSecurity_T05_ServiceRejectsMissingCaller(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	ctx := context.Background()

	checks := map[string]func() error{
		"ProjectList":   func() error { _, err := e.svc.ProjectList(ctx, service.ProjectListRequest{}); return err },
		"ProjectCreate": func() error { _, err := e.svc.ProjectCreate(ctx, service.ProjectCreateRequest{Name: "x"}); return err },
		"ProjectGet": func() error {
			_, err := e.svc.ProjectGet(ctx, service.ProjectGetRequest{ProjectID: "prj_0000000001"})
			return err
		},
		"ProjectTrash": func() error {
			_, err := e.svc.ProjectTrash(ctx, service.ProjectTrashRequest{ProjectID: "prj_0000000001"})
			return err
		},
		"ProjectRestore": func() error {
			_, err := e.svc.ProjectRestore(ctx, service.ProjectTrashRequest{ProjectID: "prj_0000000001"})
			return err
		},
		"Edit": func() error { _, err := e.svc.Edit(ctx, service.EditRequest{ProjectID: "prj_0000000001"}); return err },
		"HistoryList": func() error {
			_, err := e.svc.HistoryList(ctx, service.HistoryListRequest{ProjectID: "prj_0000000001"})
			return err
		},
		"CommandHelp": func() error { _, err := e.svc.CommandHelp(ctx, service.CommandHelpRequest{}); return err },
		"Undo":        func() error { _, err := e.svc.Undo(ctx, service.UndoRequest{ProjectID: "prj_0000000001"}); return err },
		"Redo":        func() error { _, err := e.svc.Redo(ctx, service.UndoRequest{ProjectID: "prj_0000000001"}); return err },
		"Lint":        func() error { _, err := e.svc.Lint(ctx, service.LintRequest{ProjectID: "prj_0000000001"}); return err },
		"MediaImport": func() error {
			_, err := e.svc.MediaImport(ctx, service.MediaImportRequest{ProjectID: "prj_0000000001"})
			return err
		},
		"MediaInspect": func() error {
			_, err := e.svc.MediaInspect(ctx, service.MediaInspectRequest{ProjectID: "prj_0000000001"})
			return err
		},
		"MediaTrash": func() error {
			_, err := e.svc.MediaTrash(ctx, service.MediaTrashRequest{ProjectID: "prj_0000000001"})
			return err
		},
		"MediaRestore": func() error {
			_, err := e.svc.MediaRestore(ctx, service.MediaTrashRequest{ProjectID: "prj_0000000001"})
			return err
		},
		"MediaList": func() error {
			_, err := e.svc.MediaList(ctx, service.MediaListRequest{ProjectID: "prj_0000000001"})
			return err
		},
		"JobStart": func() error {
			_, err := e.svc.JobStart(ctx, service.JobStartRequest{ProjectID: "prj_0000000001"})
			return err
		},
		"JobGet": func() error { _, err := e.svc.JobGet(ctx, service.JobGetRequest{JobID: "job_0000000001"}); return err },
		"TemplateList": func() error {
			_, err := e.svc.TemplateList(ctx, service.TemplateListRequest{})
			return err
		},
		"TemplateSave": func() error {
			_, err := e.svc.TemplateSave(ctx, service.TemplateSaveRequest{ProjectID: "prj_0000000001"})
			return err
		},
		"TemplateInstantiate": func() error {
			_, err := e.svc.TemplateInstantiate(ctx, service.TemplateInstantiateRequest{TemplateID: "tpl_0000000001"})
			return err
		},
		"TemplateExport": func() error {
			_, err := e.svc.TemplateExport(ctx, service.TemplateRef{TemplateID: "tpl_0000000001"})
			return err
		},
		"ProjectBundleExport": func() error {
			_, err := e.svc.ProjectBundleExport(ctx, service.ProjectBundleExportRequest{ProjectID: "prj_0000000001"})
			return err
		},
		"ProjectBundleImport": func() error {
			_, err := e.svc.ProjectBundleImport(ctx, service.ProjectBundleImportRequest{Path: "/x"})
			return err
		},
		"ProjectOtioExport": func() error {
			_, err := e.svc.ProjectOtioExport(ctx, service.ProjectOtioExportRequest{ProjectID: "prj_0000000001"})
			return err
		},
		"ProjectOtioImport": func() error {
			_, err := e.svc.ProjectOtioImport(
				ctx,
				service.ProjectOtioImportRequest{ProjectID: "prj_0000000001", Path: "/x"},
			)
			return err
		},
		"ImageCreate": func() error {
			_, err := e.svc.ImageCreate(ctx, service.ImageCreateRequest{Width: 10, Height: 10})
			return err
		},
		"ImageList": func() error {
			_, err := e.svc.ImageList(ctx, service.ImageListRequest{})
			return err
		},
		"ImageDescribe": func() error {
			_, err := e.svc.ImageDescribe(ctx, service.ImageDescribeRequest{ImageID: "img_0000000001"})
			return err
		},
		"ImageEdit": func() error {
			_, err := e.svc.ImageEdit(ctx, service.ImageEditRequest{ImageID: "img_0000000001"})
			return err
		},
		"ImageUndo": func() error {
			_, err := e.svc.ImageUndo(ctx, service.ImageStepRequest{ImageID: "img_0000000001"})
			return err
		},
		"ImageRedo": func() error {
			_, err := e.svc.ImageRedo(ctx, service.ImageStepRequest{ImageID: "img_0000000001"})
			return err
		},
		"ImageExport": func() error {
			_, err := e.svc.ImageExport(ctx, service.ImageExportRequest{ImageID: "img_0000000001"})
			return err
		},
		"FontImport": func() error {
			_, err := e.svc.FontImport(ctx, service.FontImportRequest{Path: "/x"})
			return err
		},
		"BrandKitGet": func() error {
			_, err := e.svc.BrandKitGet(ctx, service.BrandKitGetRequest{})
			return err
		},
		"BrandKitSet": func() error {
			_, err := e.svc.BrandKitSet(ctx, service.BrandKitSetRequest{})
			return err
		},
		"TemplateImport": func() error {
			_, err := e.svc.TemplateImport(ctx, service.TemplateImportRequest{Path: "/x"})
			return err
		},
		"ToolsInstall": func() error {
			_, err := e.svc.ToolsInstall(ctx, service.ToolsInstallRequest{Name: "whisper"})
			return err
		},
		"ToolsStatus": func() error {
			_, err := e.svc.ToolsStatus(ctx, service.ToolsStatusRequest{})
			return err
		},
		"TrashEmpty": func() error {
			_, err := e.svc.TrashEmpty(ctx, service.TrashEmptyRequest{})
			return err
		},
		"JobCancel": func() error {
			_, err := e.svc.JobCancel(ctx, service.JobCancelRequest{JobID: "job_0000000001"})
			return err
		},
		"JobList": func() error {
			_, err := e.svc.JobList(ctx, service.JobListRequest{ProjectID: "prj_0000000001"})
			return err
		},
		"PlanWindow": func() error {
			_, err := e.svc.PlanWindow(ctx, service.PlanWindowRequest{ProjectID: "prj_0000000001", Until: 30})
			return err
		},
		"ViewFrame": func() error {
			_, err := e.svc.ViewFrame(ctx, service.ViewFrameRequest{ProjectID: "prj_0000000001", At: "0s"})
			return err
		},
		"ViewSheet": func() error {
			_, err := e.svc.ViewSheet(ctx, service.ViewSheetRequest{ProjectID: "prj_0000000001"})
			return err
		},
		"Doctor": func() error {
			_, err := e.svc.Doctor(ctx, service.DoctorRequest{Full: true})
			return err
		},
		"SnapPoints": func() error {
			_, err := e.svc.SnapPoints(ctx, service.SnapPointsRequest{ProjectID: "prj_0000000001"})
			return err
		},
		"FontList": func() error {
			_, err := e.svc.FontList(ctx, service.FontListRequest{})
			return err
		},
		"AssetList": func() error {
			_, err := e.svc.AssetList(ctx, service.AssetListRequest{})
			return err
		},
		"ExportFile": func() error {
			_, err := e.svc.ExportFile(ctx, service.ExportFileRequest{JobID: "job_0000000001"})
			return err
		},
		"AuditList": func() error {
			_, err := e.svc.AuditList(ctx, service.AuditListRequest{})
			return err
		},
		"KeyCreate": func() error {
			_, err := e.svc.KeyCreate(ctx, service.KeyCreateRequest{Name: "k", Scopes: []string{"read"}})
			return err
		},
		"KeyList": func() error {
			_, err := e.svc.KeyList(ctx, service.KeyListRequest{})
			return err
		},
		"KeyRevoke": func() error {
			_, err := e.svc.KeyRevoke(ctx, service.KeyRevokeRequest{KeyID: "key_0000000001"})
			return err
		},
		"MemoryList": func() error {
			_, err := e.svc.MemoryList(ctx, service.MemoryListRequest{})
			return err
		},
		"MemoryGet": func() error {
			_, err := e.svc.MemoryGet(ctx, service.LibraryRef{Name: "a"})
			return err
		},
		"MemoryWrite": func() error {
			_, err := e.svc.MemoryWrite(ctx, service.MemoryWriteRequest{Description: "d"})
			return err
		},
		"MemoryRemove": func() error {
			_, err := e.svc.MemoryRemove(ctx, service.LibraryRef{Name: "a"})
			return err
		},
		"SkillList": func() error {
			_, err := e.svc.SkillList(ctx, service.SkillListRequest{})
			return err
		},
		"SkillGet": func() error {
			_, err := e.svc.SkillGet(ctx, service.LibraryRef{Name: "a"})
			return err
		},
		"SkillWrite": func() error {
			_, err := e.svc.SkillWrite(ctx, service.SkillWriteRequest{Name: "a"})
			return err
		},
		"SkillImport": func() error {
			_, err := e.svc.SkillImport(ctx, service.SkillImportRequest{Path: "/x"})
			return err
		},
		"SkillMarkReviewed": func() error {
			_, err := e.svc.SkillMarkReviewed(ctx, service.LibraryRef{Name: "a"})
			return err
		},
		"SkillLint": func() error {
			_, err := e.svc.SkillLint(ctx, service.LibraryRef{Name: "a"})
			return err
		},
		"SkillExport": func() error {
			_, err := e.svc.SkillExport(ctx, service.SkillExportRequest{})
			return err
		},
		"PromptList": func() error {
			_, err := e.svc.PromptList(ctx, service.PromptListRequest{})
			return err
		},
		"PromptGet": func() error {
			_, err := e.svc.PromptGet(ctx, service.LibraryRef{Name: "a"})
			return err
		},
		"PromptWrite": func() error {
			_, err := e.svc.PromptWrite(ctx, service.PromptWriteRequest{Name: "a"})
			return err
		},
		"PromptRender": func() error {
			_, err := e.svc.PromptRender(ctx, service.PromptRenderRequest{Name: "a"})
			return err
		},
		"LibrarySearch": func() error {
			_, err := e.svc.LibrarySearch(ctx, service.LibrarySearchRequest{Query: "a"})
			return err
		},
		"LibraryReindex": func() error {
			_, err := e.svc.LibraryReindex(ctx, service.LibraryReindexRequest{})
			return err
		},
		"OpenFile": func() error {
			_, err := e.svc.OpenFile(
				ctx,
				service.FileRef{Kind: "proxy", ProjectID: "prj_0000000001", ID: "ast_0000000001"},
			)
			return err
		},
	}

	// Every method of the interface is checked here and has a scope entry.
	typ := reflect.TypeFor[service.Service]()
	for method := range typ.Methods() {
		name := method.Name
		if _, ok := checks[name]; !ok {
			t.Errorf("method %s has no missing-caller check", name)
		}

		if _, ok := service.Scopes[name]; !ok {
			t.Errorf("method %s has no entry in service.Scopes", name)
		}
	}

	for name, call := range checks {
		if got := code(call()); got != service.CodeUnauthorized {
			t.Errorf("%s without a caller = %q, want %s", name, got, service.CodeUnauthorized)
		}
	}

	if _, ok := service.CallerFrom(service.WithCaller(ctx, service.Caller{})); ok {
		t.Error("a caller with no actor must not count")
	}
}

func TestScopesAreEnforcedPerMethod(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	project := e.newProject("p")

	only := func(scopes ...string) context.Context {
		return service.WithCaller(context.Background(), service.Caller{Actor: ui, Scopes: scopes})
	}

	if _, err := e.svc.ProjectGet(only(service.ScopeRead), service.ProjectGetRequest{ProjectID: project}); err != nil {
		t.Errorf("read must read: %v", err)
	}

	if _, err := e.svc.Edit(
		only(service.ScopeRead),
		service.EditRequest{ProjectID: project, Operations: []command.Operation{op("track.add", `{"kind":"visual"}`)}},
	); code(
		err,
	) != service.CodeForbidden {
		t.Errorf("a read key editing = %v, want E_FORBIDDEN", err)
	}

	if _, err := e.svc.ProjectGet(
		only(service.ScopeEdit),
		service.ProjectGetRequest{ProjectID: project},
	); code(
		err,
	) != service.CodeForbidden {
		t.Errorf("an edit key reading = %v, want E_FORBIDDEN", err)
	}

	// A key limited to one project cannot reach another.
	limited := service.WithCaller(
		context.Background(),
		service.Caller{Actor: "key:key_h8v2n5q7w3", Scopes: service.AllScopes, ProjectID: "prj_0000000099"},
	)
	if _, err := e.svc.ProjectGet(
		limited,
		service.ProjectGetRequest{ProjectID: project},
	); code(
		err,
	) != service.CodeForbidden {
		t.Errorf("a project-limited key = %v, want E_FORBIDDEN", err)
	}

	// A batch needs the scope of every command in it: asset.add is internal and unknown to callers.
	_, err := e.edit(ui, project, 0, op("asset.add", `{}`))
	se := serviceError(t, err)

	if se.Code != command.CodeUnknownCommand || se.Path != "operations[0].op" || se.OpIndex == nil || *se.OpIndex != 0 {
		t.Errorf("asset.add from a caller = %+v, want E_UNKNOWN_COMMAND at operations[0].op", se)
	}

	known, _ := se.Details["known"].([]string)
	if len(known) == 0 || slices.Contains(known, "asset.add") {
		t.Errorf("known = %v, want the public commands without asset.add", known)
	}
}

func TestProjectCreateListGetAndTrash(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	ctx := e.as(ui)

	res, err := e.svc.ProjectCreate(
		ctx,
		service.ProjectCreateRequest{Name: "Launch \"teaser\"", Preset: "16:9", FPS: "30000/1001"},
	)
	if err != nil {
		t.Fatal(err)
	}

	if res.Width != 1920 || res.Height != 1080 || res.FPS != "30000/1001" || res.Revision != 0 || !res.Created {
		t.Errorf("created = %+v", res)
	}

	if !strings.Contains(res.Text, `"Launch \"teaser\""`) || !strings.HasPrefix(res.Text, "note:") {
		t.Errorf("text = %s", res.Text)
	}

	// A dry run creates nothing.
	dry, err := e.svc.ProjectCreate(ctx, service.ProjectCreateRequest{Name: "dry", DryRun: true})
	if err != nil || dry.Created {
		t.Fatalf("dry run = %+v, %v", dry, err)
	}

	for name, req := range map[string]service.ProjectCreateRequest{
		"empty name":   {Name: ""},
		"long name":    {Name: strings.Repeat("n", 121)},
		"bad preset":   {Name: "x", Preset: "2:1"},
		"bad fps":      {Name: "x", FPS: "29.97"},
		"bad den":      {Name: "x", FPS: "30/2"},
		"bad id":       {Name: "x", ID: "prj_short"},
		"too fast fps": {Name: "x", FPS: "240"},
	} {
		if _, err := e.svc.ProjectCreate(ctx, req); code(err) != command.CodeValidation {
			t.Errorf("%s: err = %v, want E_VALIDATION", name, err)
		}
	}

	again, err := e.svc.ProjectCreate(ctx, service.ProjectCreateRequest{Name: "same id", ID: res.ProjectID})
	if code(err) != command.CodeIDExists || again != nil {
		t.Errorf("a taken id = %v, want E_ID_EXISTS", err)
	}

	list, err := e.svc.ProjectList(ctx, service.ProjectListRequest{})
	if err != nil || len(list.Projects) != 1 || list.Projects[0].ID != res.ProjectID ||
		list.Projects[0].FPS != "30000/1001" {
		t.Fatalf("list = %+v, %v", list, err)
	}

	if !strings.Contains(list.Text, "projects (1 of 1, active)") {
		t.Errorf("list text = %s", list.Text)
	}

	// Trash and restore are idempotent, touch neither revision nor document, and are audited.
	before := e.document(res.ProjectID)

	for range 2 {
		if st, err := e.svc.ProjectTrash(
			ctx,
			service.ProjectTrashRequest{ProjectID: res.ProjectID},
		); err != nil ||
			st.State != "trashed" {
			t.Fatalf("trash = %+v, %v", st, err)
		}
	}

	if l, _ := e.svc.ProjectList(ctx, service.ProjectListRequest{}); len(l.Projects) != 0 {
		t.Error("a trashed project is not in the active list")
	}

	for state, want := range map[string]int{"trashed": 1, "all": 1} {
		if l, _ := e.svc.ProjectList(ctx, service.ProjectListRequest{State: state}); len(l.Projects) != want {
			t.Errorf("list %s = %d, want %d", state, len(l.Projects), want)
		}
	}

	if _, err := e.edit(
		ui,
		res.ProjectID,
		0,
		op("track.add", `{"kind":"visual"}`),
	); code(
		err,
	) != command.CodeInvalidState {
		t.Errorf("edit of a trashed project = %v, want E_INVALID_STATE", err)
	}

	if _, err := e.svc.ProjectRestore(ctx, service.ProjectTrashRequest{ProjectID: res.ProjectID}); err != nil {
		t.Fatal(err)
	}

	if h := e.head(
		res.ProjectID,
	); h.Revision != 0 || h.State != service.ProjectActive ||
		!bytes.Equal(e.document(res.ProjectID), before) {
		t.Errorf("trash changed the revision or the document: %+v", h)
	}

	audit, err := e.store.Audit().List(context.Background(), res.ProjectID, 0, 10)
	if err != nil || len(audit) != 3 {
		t.Fatalf("audit = %+v, %v; want project.create, project.trash, project.restore", audit, err)
	}

	if _, err := e.svc.ProjectList(
		ctx,
		service.ProjectListRequest{State: "everything"},
	); code(
		err,
	) != command.CodeValidation {
		t.Errorf("unknown state = %v", err)
	}

	if _, err := e.svc.ProjectList(
		ctx,
		service.ProjectListRequest{Cursor: "!!"},
	); code(
		err,
	) != command.CodeValidation {
		t.Errorf("bad cursor = %v", err)
	}

	if _, err := e.svc.ProjectTrash(
		ctx,
		service.ProjectTrashRequest{ProjectID: "prj_0000000777"},
	); code(
		err,
	) != command.CodeNotFound {
		t.Errorf("trash of a missing project = %v", err)
	}
}

func TestProjectGetViews(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	project, track := e.withTrack()

	for i := range 3 {
		if _, err := e.addText(ui, project, track, int64(1+i), []string{"0s", "2s", "4s"}[i], "hello"); err != nil {
			t.Fatal(err)
		}
	}

	ctx := e.as(agent)

	got, err := e.svc.ProjectGet(ctx, service.ProjectGetRequest{ProjectID: project})
	if err != nil {
		t.Fatal(err)
	}

	if got.Revision != 4 || got.Counts.Items != 3 || got.Counts.Tracks != 1 || got.Duration.Frame != 150 ||
		got.Document != nil {
		t.Errorf("head = %+v", got)
	}

	for _, want := range []string{"project " + project, "rev 4", "tracks=1 items=3", `"hello"`} {
		if !strings.Contains(got.Text, want) {
			t.Errorf("concise text lacks %q:\n%s", want, got.Text)
		}
	}

	doc, err := e.svc.ProjectGet(ctx, service.ProjectGetRequest{ProjectID: project, Detail: "document"})
	if err != nil || doc.Document == nil || doc.Text != "" || len(doc.Document.Items) != 3 {
		t.Fatalf("document = %+v, %v", doc, err)
	}

	if _, err := e.svc.ProjectGet(
		ctx,
		service.ProjectGetRequest{ProjectID: project, Detail: "document", Limit: 5},
	); code(
		err,
	) != command.CodeValidation {
		t.Errorf("document with limit = %v", err)
	}

	if _, err := e.svc.ProjectGet(
		ctx,
		service.ProjectGetRequest{ProjectID: project, Detail: "weird"},
	); code(
		err,
	) != command.CodeValidation {
		t.Errorf("unknown detail = %v", err)
	}

	full, err := e.svc.ProjectGet(ctx, service.ProjectGetRequest{ProjectID: project, Detail: "full"})
	if err != nil || !strings.Contains(full.Text, "end=") {
		t.Errorf("full view = %v\n%s", err, full.Text)
	}

	// Paging by item, a cursor that goes stale after a write.
	page1, err := e.svc.ProjectGet(ctx, service.ProjectGetRequest{ProjectID: project, Limit: 1})
	if err != nil || page1.NextCursor == nil || !strings.Contains(page1.Text, "next_cursor: ") {
		t.Fatalf("page 1 = %+v, %v", page1, err)
	}

	page2, err := e.svc.ProjectGet(
		ctx,
		service.ProjectGetRequest{ProjectID: project, Cursor: *page1.NextCursor, Limit: 1},
	)
	if err != nil || !strings.Contains(page2.Text, "(page 2)") {
		t.Fatalf("page 2 = %v\n%s", err, page2.Text)
	}

	if _, err := e.addText(ui, project, track, 4, "6s", "late"); err != nil {
		t.Fatal(err)
	}

	_, err = e.svc.ProjectGet(
		ctx,
		service.ProjectGetRequest{ProjectID: project, Cursor: *page1.NextCursor},
	)
	se := serviceError(t, err)

	if se.Code != command.CodeValidation || se.Path != "cursor" || se.Details["reason"] != "stale" ||
		se.Details["revision"] != int64(5) {
		t.Errorf("stale cursor = %+v", se)
	}

	if _, err := e.svc.ProjectGet(
		ctx,
		service.ProjectGetRequest{ProjectID: "prj_0000000777"},
	); code(
		err,
	) != command.CodeNotFound {
		t.Errorf("missing project = %v", err)
	}
}

func TestNetworkModeOwnersSeeOnlyTheirProjects(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	alice := service.WithCaller(
		context.Background(),
		service.Caller{Actor: "user:usr_a", Scopes: service.AllScopes, OwnerUserID: "usr_a"},
	)
	bob := service.WithCaller(
		context.Background(),
		service.Caller{Actor: "user:usr_b", Scopes: service.AllScopes, OwnerUserID: "usr_b"},
	)

	res, err := e.svc.ProjectCreate(alice, service.ProjectCreateRequest{Name: "alice's"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := e.svc.ProjectGet(
		bob,
		service.ProjectGetRequest{ProjectID: res.ProjectID},
	); code(
		err,
	) != command.CodeNotFound {
		t.Errorf("bob reading alice's project = %v, want E_NOT_FOUND (no disclosure)", err)
	}

	if _, err := e.svc.Edit(
		bob,
		service.EditRequest{
			ProjectID:  res.ProjectID,
			Operations: []command.Operation{op("track.add", `{"kind":"visual"}`)},
		},
	); code(
		err,
	) != command.CodeNotFound {
		t.Errorf("bob editing alice's project = %v", err)
	}

	if l, _ := e.svc.ProjectList(bob, service.ProjectListRequest{}); len(l.Projects) != 0 {
		t.Errorf("bob's list = %+v", l.Projects)
	}

	if l, _ := e.svc.ProjectList(alice, service.ProjectListRequest{}); len(l.Projects) != 1 {
		t.Errorf("alice's list = %+v", l.Projects)
	}
}

func TestErrorStatusAndExitCode(t *testing.T) {
	t.Parallel()

	for code, want := range map[string]int{
		command.CodeValidation: 400, service.CodeUnauthorized: 401, service.CodeForbidden: 403, command.CodeNotFound: 404,
		command.CodeRevisionConflict: 409, service.CodePayloadTooLarge: 413, command.CodeOverlap: 409, service.CodeLintBlocking: 422,
		service.CodeQueueFull: 429, service.CodeWorkspaceLocked: 503, command.CodeInternal: 500, "E_SOMETHING_ELSE": 500,
		service.CodeNothingToUndo: 409,
	} {
		if got := (&service.Error{Code: code}).HTTPStatus(); got != want {
			t.Errorf("HTTPStatus(%s) = %d, want %d", code, got, want)
		}
	}

	if (&service.Error{Retryable: true}).ExitCode() != 2 || (&service.Error{}).ExitCode() != 1 {
		t.Error("exit code is 2 when retryable and 1 otherwise")
	}
}

func network(actor, owner string) context.Context {
	return service.WithCaller(
		context.Background(),
		service.Caller{Actor: actor, Scopes: service.AllScopes, OwnerUserID: owner},
	)
}

func TestSecurity_T06_ProjectLimitedKeyConfined(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	owner := network("user:usr_a", "usr_a")

	var ids []string

	for _, n := range []string{"one", "two", "three"} {
		res, err := e.svc.ProjectCreate(owner, service.ProjectCreateRequest{Name: n})
		if err != nil {
			t.Fatal(err)
		}

		ids = append(ids, res.ProjectID)
	}

	key := service.WithCaller(context.Background(), service.Caller{
		Actor: "key:key_0000000001", Scopes: service.AllScopes, OwnerUserID: "usr_a", ProjectID: ids[1],
	})

	l, err := e.svc.ProjectList(key, service.ProjectListRequest{State: "all"})
	if err != nil {
		t.Fatal(err)
	}

	if len(l.Projects) != 1 || l.Projects[0].ID != ids[1] || !strings.Contains(l.Text, "(1 of 1") {
		t.Errorf("a project-limited key lists %+v\n%s", l.Projects, l.Text)
	}

	if _, err := e.svc.ProjectCreate(
		key,
		service.ProjectCreateRequest{Name: "more"},
	); code(
		err,
	) != service.CodeForbidden {
		t.Errorf("a project-limited key creating a project = %v, want E_FORBIDDEN", err)
	}

	if _, err := e.svc.ProjectGet(
		key,
		service.ProjectGetRequest{ProjectID: ids[0]},
	); code(
		err,
	) != service.CodeForbidden {
		t.Errorf("a project-limited key reading another project = %v, want E_FORBIDDEN", err)
	}
}

func TestSecurity_T06_ListAndCountAreScopedToTheOwner(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	alice, bob := network("user:usr_a", "usr_a"), network("user:usr_b", "usr_b")

	for i := range 5 {
		if _, err := e.svc.ProjectCreate(
			bob,
			service.ProjectCreateRequest{Name: fmt.Sprintf("bob %d", i)},
		); err != nil {
			t.Fatal(err)
		}

		e.clock.advance(time.Second)
	}

	mine, err := e.svc.ProjectCreate(alice, service.ProjectCreateRequest{Name: "alice"})
	if err != nil {
		t.Fatal(err)
	}

	// Alice's project is the oldest of the ones created after Bob's; with a
	// limit of 1 an in-memory filter would return an empty page with a cursor.
	l, err := e.svc.ProjectList(alice, service.ProjectListRequest{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}

	if len(l.Projects) != 1 || l.Projects[0].ID != mine.ProjectID || l.NextCursor != nil {
		t.Errorf("alice's page = %+v, next %v", l.Projects, l.NextCursor)
	}

	if strings.Contains(l.Text, "of 6") || strings.Contains(l.Text, "bob") {
		t.Errorf("the text leaks the total of other owners:\n%s", l.Text)
	}

	n, err := e.store.Projects().Count(context.Background(), service.ProjectQuery{OwnerUserID: "usr_a"})
	if err != nil || n != 1 {
		t.Errorf("Count for alice = %d, %v", n, err)
	}
}

func TestSecurity_T06_ProjectIDIsNotAnExistenceOracle(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	alice, bob := network("user:usr_a", "usr_a"), network("user:usr_b", "usr_b")

	theirs, err := e.svc.ProjectCreate(alice, service.ProjectCreateRequest{Name: "alice"})
	if err != nil {
		t.Fatal(err)
	}

	_, taken := e.svc.ProjectCreate(bob, service.ProjectCreateRequest{Name: "x", ID: theirs.ProjectID})
	_, free := e.svc.ProjectCreate(bob, service.ProjectCreateRequest{Name: "x", ID: "prj_00000zzzzz"})

	if code(taken) != command.CodeValidation || code(free) != command.CodeValidation {
		t.Fatalf("taken = %v, free = %v: network mode must refuse a client id the same way", taken, free)
	}

	if serviceError(t, taken).Message != serviceError(t, free).Message {
		t.Errorf("the answers differ: %q and %q", serviceError(t, taken).Message, serviceError(t, free).Message)
	}

	// Local mode keeps the client-chosen id (and E_ID_EXISTS), there is no other owner.
	if _, err := e.svc.ProjectCreate(
		e.as(ui),
		service.ProjectCreateRequest{Name: "l", ID: "prj_00000aaaaa"},
	); err != nil {
		t.Errorf("local create with an id: %v", err)
	}
}

func TestCommandHelp(t *testing.T) {
	t.Parallel()

	e := newEnv(t)

	list, err := e.svc.CommandHelp(e.as(ui), service.CommandHelpRequest{})
	if err != nil || len(list.Commands) == 0 || list.Text == "" {
		t.Fatalf("list = %+v, %v", list, err)
	}

	for _, c := range list.Commands {
		if c.Name == "asset.add" {
			t.Error("the list shows the internal command asset.add")
		}
	}

	one, err := e.svc.CommandHelp(e.as(ui), service.CommandHelpRequest{Name: "item.move"})
	if err != nil || one.Command == nil || one.Command.ArgsSchema["$schema"] == nil ||
		one.Command.Example["item_id"] == nil {
		t.Fatalf("detail = %+v, %v", one, err)
	}

	for _, name := range []string{"asset.add", "nope.nothing"} {
		_, err = e.svc.CommandHelp(e.as(ui), service.CommandHelpRequest{Name: name})
		if se := serviceError(t, err); se.Code != command.CodeUnknownCommand || se.Details["known"] == nil {
			t.Errorf("CommandHelp(%s) = %+v, want E_UNKNOWN_COMMAND with the known names", name, se)
		}
	}

	if _, err = e.svc.CommandHelp(
		context.Background(),
		service.CommandHelpRequest{},
	); code(
		err,
	) != service.CodeUnauthorized {
		t.Errorf("without a caller: %v", err)
	}
}
