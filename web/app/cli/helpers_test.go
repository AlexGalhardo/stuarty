package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/service"
)

// fakeService records the last request of each method and answers with the
// configured result or error. A method a test does not use panics through the
// nil embedded interface.
type fakeService struct {
	service.Service

	createReq  service.ProjectCreateRequest
	getReq     service.ProjectGetRequest
	editReq    service.EditRequest
	lintReq    service.LintRequest
	undoReq    service.UndoRequest
	historyReq service.HistoryListRequest
	listReq    service.ProjectListRequest
	helpReq    service.CommandHelpRequest

	revision int64
	err      error
	lintRes  *service.LintResult
	undoRes  *service.UndoResult
	caller   service.Caller
}

func (f *fakeService) note(ctx context.Context) {
	f.caller, _ = service.CallerFrom(ctx)
}

func (f *fakeService) ProjectList(
	ctx context.Context,
	r service.ProjectListRequest,
) (*service.ProjectListResult, error) {
	f.note(ctx)
	f.listReq = r

	if f.err != nil {
		return nil, f.err
	}

	return &service.ProjectListResult{Projects: []service.ProjectSummary{}, Text: "projects (0 of 0, active)"}, nil
}

func (f *fakeService) ProjectCreate(
	ctx context.Context,
	r service.ProjectCreateRequest,
) (*service.ProjectCreateResult, error) {
	f.note(ctx)
	f.createReq = r

	if f.err != nil {
		return nil, f.err
	}

	return &service.ProjectCreateResult{
		ProjectID: "prj_7k2m9x4q1b",
		Name:      r.Name,
		Created:   !r.DryRun,
		Text:      "created prj_7k2m9x4q1b",
	}, nil
}

func (f *fakeService) ProjectGet(ctx context.Context, r service.ProjectGetRequest) (*service.ProjectGetResult, error) {
	f.note(ctx)
	f.getReq = r

	if f.err != nil {
		return nil, f.err
	}

	return &service.ProjectGetResult{ProjectID: r.ProjectID, Revision: f.revision, Text: "project " + r.ProjectID}, nil
}

func (f *fakeService) Edit(ctx context.Context, r service.EditRequest) (*service.EditResult, error) {
	f.note(ctx)
	f.editReq = r

	if f.err != nil {
		return nil, f.err
	}

	return &service.EditResult{
		ProjectID: r.ProjectID, Revision: r.BaseRevision + 1, BaseRevision: r.BaseRevision, Applied: !r.DryRun,
		DryRun: r.DryRun, Actor: f.caller.Actor, Created: []string{"itm_c3d4e5f6g7"},
		Description: "+ item itm_c3d4e5f6g7 clip",
	}, nil
}

func (f *fakeService) CommandHelp(_ context.Context, r service.CommandHelpRequest) (*service.CommandHelpResult, error) {
	f.helpReq = r

	if f.err != nil {
		return nil, f.err
	}

	return &service.CommandHelpResult{Text: "help " + r.Name}, nil
}

func (f *fakeService) HistoryList(_ context.Context, r service.HistoryListRequest) (*service.HistoryListResult, error) {
	f.historyReq = r

	return &service.HistoryListResult{ProjectID: r.ProjectID, Text: "history"}, f.err
}

func (f *fakeService) Undo(_ context.Context, r service.UndoRequest) (*service.UndoResult, error) {
	f.undoReq = r

	if f.err != nil {
		return nil, f.err
	}

	return f.undoRes, nil
}

func (f *fakeService) Lint(_ context.Context, r service.LintRequest) (*service.LintResult, error) {
	f.lintReq = r

	if f.err != nil {
		return nil, f.err
	}

	return f.lintRes, nil
}

type fakeResolver struct {
	svc       service.Service
	inProcess bool
	err       error
	target    Target
	resolved  int
	events    EventOpener
	download  Downloader
}

func (r *fakeResolver) Resolve(_ context.Context, t Target) (*Connection, error) {
	r.resolved++
	r.target = t

	if r.err != nil {
		return nil, r.err
	}

	return &Connection{
		Service: r.svc, InProcess: r.inProcess, Workspace: "/ws", Events: r.events, Download: r.download,
	}, nil
}

type harness struct {
	rt     *Runtime
	res    *fakeResolver
	svc    *fakeService
	stdout *bytes.Buffer
	stderr *bytes.Buffer
}

func newHarness(t *testing.T) *harness {
	t.Helper()

	h := &harness{svc: &fakeService{}, stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}
	h.res = &fakeResolver{svc: h.svc}
	h.rt = &Runtime{
		Stdin: strings.NewReader(""), Stdout: h.stdout, Stderr: h.stderr,
		Getenv:   func(string) string { return "" },
		Getwd:    func() (string, error) { return t.TempDir(), nil },
		Resolver: h.res,
	}

	return h
}

// run executes "command args..." and returns the exit code.
func (h *harness) run(argv ...string) int {
	h.stdout.Reset()
	h.stderr.Reset()

	return h.rt.Execute(context.Background(), argv[0], argv[1:])
}
