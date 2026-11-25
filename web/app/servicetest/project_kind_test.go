package servicetest

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/approval"
	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/project"
	"github.com/AlexGalhardo/stuarty/service"
	appr "github.com/AlexGalhardo/stuarty/service/approval"
	"github.com/AlexGalhardo/stuarty/service/testproject"
)

func stepIDs(w *project.Workflow) []string {
	out := make([]string, len(w.Steps))
	for i := range w.Steps {
		out[i] = string(w.Steps[i].ID)
	}

	return out
}

func stepState(t *testing.T, w *project.Workflow, id project.StepID) project.StepState {
	t.Helper()

	st, ok := w.Step(id)
	if !ok {
		t.Fatalf("the workflow has no step %s: %v", id, stepIDs(w))
	}

	return st.State
}

func createKind(t *testing.T, e *env, name, kind string) string {
	t.Helper()

	req := testproject.Request(name)
	req.Kind = kind

	res, err := e.svc.ProjectCreate(e.as(ui), req)
	if err != nil {
		t.Fatalf("create %s: %v", kind, err)
	}

	return res.ProjectID
}

// Acceptance 1 of epic 78: a project created with kind text has no timeline, no format and no media, and its
// workflow has the five text steps.
func TestProjectKindTextCreatesNoMedia(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	id := createKind(t, e, "Blog post", "text")

	got, err := e.svc.ProjectGet(e.as(ui), service.ProjectGetRequest{ProjectID: id})
	if err != nil {
		t.Fatal(err)
	}

	if got.Kind != "text" || got.Workflow.Kind != project.KindText {
		t.Errorf("kind = %q, workflow kind = %q", got.Kind, got.Workflow.Kind)
	}

	want := []string{"briefing", "writing", "review", "approval", "publication"}
	if g := stepIDs(&got.Workflow); !slices.Equal(g, want) {
		t.Errorf("steps = %v, want %v", g, want)
	}

	if got.TimelineID != "" || got.Width != 0 || got.Height != 0 || got.FPS != "" || got.Counts.Assets != 0 ||
		len(got.Items) != 0 || got.Production.Timelines != 0 {
		t.Errorf("a text project holds media: %+v", got)
	}

	if got.Workflow.Current != project.StepBriefing || got.Workflow.Next != project.StepWriting {
		t.Errorf("current, next = %q, %q", got.Workflow.Current, got.Workflow.Next)
	}

	for _, kind := range []string{"video", "image", "undecided", ""} {
		id := createKind(t, e, "Kind "+strings.ToUpper(kind[:min(len(kind), 1)])+"x", kind)

		g, err := e.svc.ProjectGet(e.as(ui), service.ProjectGetRequest{ProjectID: id})
		if err != nil {
			t.Fatal(err)
		}

		if kind == "" {
			kind = "undecided"
		}

		if g.Kind != kind || g.TimelineID != "" {
			t.Errorf("kind %q: got %q, timeline %q", kind, g.Kind, g.TimelineID)
		}
	}
}

func TestProjectKindRefusesAnUnknownKind(t *testing.T) {
	t.Parallel()

	e := newEnv(t)

	req := testproject.Request("Odd kind")
	req.Kind = "audio"

	_, err := e.svc.ProjectCreate(e.as(ui), req)
	if se := serviceError(t, err); se.Code != command.CodeValidation || se.Path != "kind" || se.Hint == "" {
		t.Fatalf("err = %+v, want E_VALIDATION on kind with a hint", se)
	}

	// A dry run says the kind and writes nothing.
	req.Kind, req.DryRun = "image", true

	res, err := e.svc.ProjectCreate(e.as(ui), req)
	if err != nil || res.Created || res.Kind != "image" {
		t.Fatalf("dry run = %+v, %v", res, err)
	}

	if l, _ := e.svc.ProjectList(e.as(ui), service.ProjectListRequest{}); len(l.Projects) != 0 {
		t.Fatal("a dry run created a project")
	}
}

// Acceptance 2 of epic 78: a project made before the kind existed reads its kind from its items, and nothing is
// written to it.
func TestProjectKindOfAnExistingProjectComesFromItsItems(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	ctx := context.Background()

	e.oldProject("prj_0000000101", "Only a brief")
	e.oldProject("prj_0000000102", "Some copy")
	e.oldProject("prj_0000000103", "A video")

	err := e.store.Write(ctx, func(ctx context.Context, tx service.Tx) error {
		items := []service.ItemRow{
			{ID: "pit_0000000101", ProjectID: "prj_0000000101", Kind: "brief", Title: "Brief", Body: "A brief."},
			{
				ID: "pit_0000000102", ProjectID: "prj_0000000102", Kind: "text", Title: "Copy", TextKind: "copy",
				Body: strings.Repeat("word ", 842),
			},
			{ID: "pit_0000000103", ProjectID: "prj_0000000103", Kind: "timeline", Title: "Cut", Ref: "prj_0000000103"},
		}

		for i := range items {
			items[i].State, items[i].CreatedMs, items[i].UpdatedMs, items[i].CreatedBy = service.ProjectActive, 1, 1, ui
			if err := tx.Productions().CreateItem(ctx, items[i]); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	list, err := e.svc.ProjectList(e.as(ui), service.ProjectListRequest{})
	if err != nil {
		t.Fatal(err)
	}

	kinds := map[string]service.ProjectSummary{}
	for _, p := range list.Projects {
		kinds[p.ProjectID] = p
	}

	for id, want := range map[string]string{
		"prj_0000000101": "undecided", "prj_0000000102": "text", "prj_0000000103": "video",
	} {
		if kinds[id].Kind != want {
			t.Errorf("%s reads %q, want %q", id, kinds[id].Kind, want)
		}
	}

	// Acceptance 5: a post of 842 words is listed with 5 minutes.
	sum := kinds["prj_0000000102"].TextSummary
	if sum == nil || sum.Words != 842 || sum.ReadingMinutes != 5 || sum.TextKind != "copy" ||
		!strings.HasPrefix(sum.Excerpt, "word word") || len(sum.Excerpt) > 160 {
		t.Errorf("text summary = %+v", sum)
	}

	if kinds["prj_0000000101"].TextSummary != nil || kinds["prj_0000000103"].TextSummary != nil {
		t.Error("a project with no text has a text summary")
	}

	// Nothing was written: every row still stores the default.
	for _, id := range []string{"prj_0000000101", "prj_0000000102", "prj_0000000103"} {
		head, err := e.store.Productions().Get(ctx, id)
		if err != nil || head.Kind != project.KindUndecided || len(head.Skipped) != 0 || head.UpdatedMs != 1 {
			t.Errorf("%s: %+v, %v", id, head, err)
		}
	}
}

// Acceptance 3 of epic 78: a timeline added to a text project brings the video steps, and the steps that were done
// stay done.
func TestProjectKindAddingATimelineKeepsTheFinishedSteps(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	id := createKind(t, e, "Copy then video", "text")

	if _, err := e.svc.BriefSet(
		e.as(ui),
		service.BriefSetRequest{ProjectID: id, Text: "# Brief\nwhat it is"},
	); err != nil {
		t.Fatal(err)
	}

	if _, err := e.svc.ProjectItemAdd(e.as(ui), service.ProjectItemAddRequest{
		ProjectID: id, Kind: "text", Title: "Post", Text: "The words of the post.", TextKind: "post",
	}); err != nil {
		t.Fatal(err)
	}

	before, err := e.svc.ProjectGet(e.as(ui), service.ProjectGetRequest{ProjectID: id})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := e.svc.ProjectItemAdd(e.as(ui), service.ProjectItemAddRequest{
		ProjectID: id, Kind: "timeline", Title: "Teaser", Preset: "9:16",
	}); err != nil {
		t.Fatal(err)
	}

	after, err := e.svc.ProjectGet(e.as(ui), service.ProjectGetRequest{ProjectID: id})
	if err != nil {
		t.Fatal(err)
	}

	for _, step := range []project.StepID{
		project.StepScript, project.StepRecording, project.StepEditing, project.StepFinishing,
	} {
		if _, ok := before.Workflow.Step(step); ok {
			t.Errorf("a text project had the step %s", step)
		}

		if _, ok := after.Workflow.Step(step); !ok {
			t.Errorf("the step %s did not appear: %v", step, stepIDs(&after.Workflow))
		}
	}

	for _, step := range []project.StepID{project.StepBriefing, project.StepWriting} {
		if stepState(t, &before.Workflow, step) != project.StepDone ||
			stepState(t, &after.Workflow, step) != project.StepDone {
			t.Errorf("%s did not stay done", step)
		}
	}

	if after.Kind != "text" {
		t.Errorf("the kind became %q", after.Kind)
	}
}

// A project that has not chosen takes the kind it reads as when something else joins, so that a text that gets a
// video does not turn into a video.
func TestProjectKindIsStoredByTheFirstItem(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	ctx := context.Background()

	stored := func(id string) project.Intent {
		t.Helper()

		head, err := e.store.Productions().Get(ctx, id)
		if err != nil {
			t.Fatal(err)
		}

		return head.Kind
	}

	add := func(id, kind string) {
		t.Helper()

		if _, err := e.svc.ProjectItemAdd(e.as(ui), service.ProjectItemAddRequest{
			ProjectID: id, Kind: kind, Title: "Item", Text: "words", Preset: "9:16",
		}); err != nil {
			t.Fatalf("add %s: %v", kind, err)
		}
	}

	undecided := createKind(t, e, "Undecided one", "")
	if stored(undecided) != project.KindUndecided {
		t.Fatal("a new project chose")
	}

	add(undecided, "text")
	add(undecided, "timeline")

	if stored(undecided) != project.KindText {
		t.Errorf("a text, then a timeline: stored %q, want text", stored(undecided))
	}

	video := createKind(t, e, "Undecided two", "")
	add(video, "timeline")

	if stored(video) != project.KindVideo {
		t.Errorf("a timeline: stored %q, want video", stored(video))
	}

	chosen := createKind(t, e, "Chosen one", "image")
	add(chosen, "text")

	if stored(chosen) != project.KindImage {
		t.Errorf("a stored kind changed to %q", stored(chosen))
	}

	// VV-394, D-139: a script decides nothing, and the timeline after it makes the project a video.
	scripted := createKind(t, e, "Script first", "")

	if _, err := e.svc.ProjectItemAdd(e.as(ui), service.ProjectItemAddRequest{
		ProjectID: scripted, Kind: "text", Title: "Script", Text: "words", TextKind: "script",
	}); err != nil {
		t.Fatal(err)
	}

	if got, _ := e.svc.ProjectGet(e.as(ui), service.ProjectGetRequest{ProjectID: scripted}); stored(scripted) !=
		project.KindUndecided || got.Kind != "undecided" {
		t.Errorf("a script chose: stored %q, reads %q", stored(scripted), got.Kind)
	}

	add(scripted, "timeline")

	if stored(scripted) != project.KindVideo {
		t.Errorf("a script, then a timeline: stored %q, want video", stored(scripted))
	}

	// A dry run decides nothing.
	dry := createKind(t, e, "Dry run one", "")

	if _, err := e.svc.ProjectItemAdd(e.as(ui), service.ProjectItemAddRequest{
		ProjectID: dry, Kind: "text", Title: "Item", Text: "words", DryRun: true,
	}); err != nil || stored(dry) != project.KindUndecided {
		t.Errorf("a dry run stored %q (%v)", stored(dry), err)
	}
}

// Acceptance 4 of epic 78 and the skip: a required step is refused with E_VALIDATION, an optional one is skipped
// and restored, and a duplicate keeps what was skipped.
func TestWorkflowStepSkip(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	id := createKind(t, e, "Teaser video", "video")
	skip := func(step string, restore, dry bool) (*service.ProjectStepSkipResult, error) {
		return e.svc.ProjectStepSkip(e.as(ui), service.ProjectStepSkipRequest{
			ProjectID: id, Step: step, Restore: restore, DryRun: dry,
		})
	}

	for step, want := range map[string]string{"editing": "E_VALIDATION", "briefing": "E_VALIDATION", "dancing": "E_VALIDATION"} {
		_, err := skip(step, false, false)
		if se := serviceError(t, err); se.Code != want || se.Path != "step" || se.Hint == "" {
			t.Errorf("%s: %+v, want %s on step with a hint", step, se, want)
		}
	}

	_, err := skip("writing", false, false)
	if code(err) != command.CodeValidation {
		t.Errorf("a step of another kind = %v", err)
	}

	res, err := skip("recording", false, true)
	if err != nil || res.Changed || stepState(t, &res.Workflow, project.StepRecording) != project.StepSkipped {
		t.Fatalf("dry run = %+v, %v", res, err)
	}

	if got, _ := e.svc.ProjectGet(
		e.as(ui),
		service.ProjectGetRequest{ProjectID: id},
	); stepState(
		t,
		&got.Workflow,
		project.StepRecording,
	) == project.StepSkipped {
		t.Fatal("a dry run skipped the step")
	}

	res, err = skip("recording", false, false)
	if err != nil || !res.Changed || stepState(t, &res.Workflow, project.StepRecording) != project.StepSkipped {
		t.Fatalf("skip = %+v, %v", res, err)
	}

	// Skipping again changes nothing and succeeds.
	if res, err = skip("recording", false, false); err != nil || res.Changed {
		t.Fatalf("second skip = %+v, %v", res, err)
	}

	got, err := e.svc.ProjectGet(e.as(ui), service.ProjectGetRequest{ProjectID: id})
	if err != nil || stepState(t, &got.Workflow, project.StepRecording) != project.StepSkipped {
		t.Fatalf("get after skip: %v", err)
	}

	dup, err := e.svc.ProjectDuplicate(e.as(ui), service.ProjectDuplicateRequest{ProjectID: id})
	if err != nil || dup.Kind != "video" {
		t.Fatalf("duplicate = %+v, %v", dup, err)
	}

	copied, err := e.svc.ProjectGet(e.as(ui), service.ProjectGetRequest{ProjectID: dup.ProjectID})
	if err != nil || stepState(t, &copied.Workflow, project.StepRecording) != project.StepSkipped {
		t.Fatalf("a duplicate lost the skipped step: %v", err)
	}

	if res, err = skip("recording", true, false); err != nil || !res.Changed ||
		stepState(t, &res.Workflow, project.StepRecording) == project.StepSkipped {
		t.Fatalf("restore = %+v, %v", res, err)
	}

	if res, err = skip("recording", true, false); err != nil || res.Changed {
		t.Fatalf("restore of a step that is not skipped = %+v, %v", res, err)
	}

	// A trashed project refuses the change.
	if _, err := e.svc.ProjectTrash(e.as(ui), service.ProjectTrashRequest{ProjectID: id}); err != nil {
		t.Fatal(err)
	}

	if _, err := skip("recording", false, false); code(err) != command.CodeInvalidState {
		t.Errorf("trashed = %v, want E_INVALID_STATE", err)
	}

	var events []string

	for _, ev := range e.events {
		events = append(events, ev.Type)
	}

	if !slices.Contains(events, "project.updated") {
		t.Errorf("no project.updated event: %v", events)
	}

	rows, err := e.store.Audit().List(context.Background(), id, 0, 50)
	if err != nil {
		t.Fatal(err)
	}

	var actions []string

	for i := range rows {
		if rows[i].Action != "project.create" && rows[i].SubjectID != nil && *rows[i].SubjectID == "recording" {
			actions = append(actions, rows[i].Action)
		}
	}

	slices.Sort(actions)

	if want := []string{"project.step.restore", "project.step.skip"}; !slices.Equal(actions, want) {
		t.Errorf("audit rows of the step = %v, want %v (a no-op writes none)", actions, want)
	}
}

// The publication of a text project is optional while it holds no post: a copy ends in copy or export (D-137).
func TestWorkflowCopyEndsWithoutPublication(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	id := createKind(t, e, "Only copy", "text")

	res, err := e.svc.ProjectStepSkip(e.as(ui), service.ProjectStepSkipRequest{ProjectID: id, Step: "publication"})
	if err != nil || stepState(t, &res.Workflow, project.StepPublication) != project.StepSkipped {
		t.Fatalf("a copy cannot skip its publication: %+v, %v", res, err)
	}

	if _, err := e.svc.ProjectItemAdd(e.as(ui), service.ProjectItemAddRequest{
		ProjectID: id, Kind: "text", Title: "Post", Text: "words", TextKind: "post",
	}); err != nil {
		t.Fatal(err)
	}

	got, err := e.svc.ProjectGet(e.as(ui), service.ProjectGetRequest{ProjectID: id})
	if err != nil {
		t.Fatal(err)
	}

	if st, _ := got.Workflow.Step(project.StepPublication); st.Optional || st.State == project.StepSkipped {
		t.Errorf("a blog post is published: %+v", st)
	}
}

// The approval step carries what the approvals of the project estimate or cost; a rejected one costs nothing.
func TestWorkflowCostIsTheApprovalsOfTheProject(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	id := createKind(t, e, "Costly one", "text")
	other := createKind(t, e, "Other cost", "text")

	add := func(project, state, currency string, micros int64) {
		t.Helper()

		now := e.clock.now().UnixMilli()
		row := appr.Row{
			ID: e.newID("apr"), Actor: token, Kind: "publish.post", Summary: "x", ProjectID: project, PayloadRef: "d",
			Estimate: approval.Money{Micros: micros, Currency: currency}, State: appr.StatePending, CreatedMs: now,
			ExpiresMs: now + appr.ExpireAfter.Milliseconds(),
		}

		if err := e.store.Approvals().InsertRequest(context.Background(), &row); err != nil {
			t.Fatal(err)
		}

		if state != appr.StatePending {
			if _, err := e.store.Approvals().Decide(context.Background(), row.ID, state, ui, "", "", now); err != nil {
				t.Fatal(err)
			}
		}
	}

	add(id, appr.StatePending, "USD", 1_000_000)
	add(id, appr.StateApproved, "USD", 250_000)
	add(id, appr.StateApproved, "EUR", 300_000)
	add(id, appr.StateRejected, "USD", 9_000_000)
	add(other, appr.StatePending, "USD", 5_000_000)

	got, err := e.svc.ProjectGet(e.as(ui), service.ProjectGetRequest{ProjectID: id})
	if err != nil {
		t.Fatal(err)
	}

	step, _ := got.Workflow.Step(project.StepApproval)
	want := []project.Cost{{Micros: 300_000, Currency: "EUR"}, {Micros: 1_250_000, Currency: "USD"}}

	if !slices.Equal(step.Cost, want) {
		t.Errorf("cost = %v, want %v", step.Cost, want)
	}

	if st, _ := got.Workflow.Step(project.StepPublication); len(st.Cost) != 0 {
		t.Errorf("the publication step has a cost: %v", st.Cost)
	}

	none := createKind(t, e, "No cost", "text")

	g, err := e.svc.ProjectGet(e.as(ui), service.ProjectGetRequest{ProjectID: none})
	if err != nil {
		t.Fatal(err)
	}

	if st, _ := g.Workflow.Step(project.StepApproval); st.Cost != nil {
		t.Errorf("a project with no approvals has a cost: %v", st.Cost)
	}
}

// A member sees the kind, the workflow and the summary of their own projects only, and cannot skip a step of
// another member's project, which answers as a project that does not exist.
func TestProjectKindMemberIsolation(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	alice, bob := network("user:alice", "alice"), network("user:bob", "bob")

	mk := func(c context.Context, name string) string {
		req := testproject.Request(name)
		req.Kind = "text"

		res, err := e.svc.ProjectCreate(c, req)
		if err != nil {
			t.Fatal(err)
		}

		return res.ProjectID
	}

	theirs := mk(bob, "Bob copy")
	mine := mk(alice, "Alice copy")

	if _, err := e.svc.ProjectItemAdd(bob, service.ProjectItemAddRequest{
		ProjectID: theirs, Kind: "text", Title: "Secret", Text: "secret words that are bobs",
	}); err != nil {
		t.Fatal(err)
	}

	calls := map[string]func() error{
		"ProjectGet": func() error {
			_, err := e.svc.ProjectGet(alice, service.ProjectGetRequest{ProjectID: theirs})
			return err
		},
		"ProjectDescribe": func() error {
			_, err := e.svc.ProjectDescribe(alice, service.ProjectDescribeRequest{ProjectID: theirs})
			return err
		},
		"ProjectStepSkip": func() error {
			_, err := e.svc.ProjectStepSkip(
				alice,
				service.ProjectStepSkipRequest{ProjectID: theirs, Step: "publication"},
			)
			return err
		},
	}

	missing := e.svc.ProjectGet
	_, ghost := missing(alice, service.ProjectGetRequest{ProjectID: "prj_0000009999"})

	for name, call := range calls {
		err := call()
		if code(err) != command.CodeNotFound {
			t.Errorf("%s on another member's project = %v, want E_NOT_FOUND", name, err)
		} else if serviceError(t, err).Message != strings.ReplaceAll(serviceError(t, ghost).Message, "prj_0000009999", theirs) {
			t.Errorf("%s tells a project that exists from one that does not: %q", name, serviceError(t, err).Message)
		}
	}

	list, err := e.svc.ProjectList(alice, service.ProjectListRequest{})
	if err != nil {
		t.Fatal(err)
	}

	if len(list.Projects) != 1 || list.Projects[0].ProjectID != mine || list.Projects[0].Kind != "text" ||
		list.Projects[0].TextSummary != nil {
		t.Fatalf("alice lists %+v", list.Projects)
	}

	if _, err := e.svc.ProjectStepSkip(
		bob,
		service.ProjectStepSkipRequest{ProjectID: theirs, Step: "publication"},
	); err != nil {
		t.Errorf("the owner cannot skip: %v", err)
	}

	// The text of another member is in none of what alice reads.
	got, err := e.svc.ProjectList(bob, service.ProjectListRequest{})
	if err != nil || len(got.Projects) != 1 || got.Projects[0].TextSummary == nil ||
		!strings.Contains(got.Projects[0].TextSummary.Excerpt, "secret words") {
		t.Fatalf("bob lists %+v, %v", got.Projects, err)
	}
}

// A key limited to one project reaches that project's workflow and no other.
func TestWorkflowLimitedKeyIsConfined(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	a, b := createKind(t, e, "Project A", "video"), createKind(t, e, "Project B", "video")

	key := service.WithCaller(context.Background(), service.Caller{
		Actor: "key:limited", Scopes: service.AllScopes, ProjectID: a,
	})

	if _, err := e.svc.ProjectStepSkip(
		key,
		service.ProjectStepSkipRequest{ProjectID: a, Step: "recording"},
	); err != nil {
		t.Errorf("own project: %v", err)
	}

	if _, err := e.svc.ProjectStepSkip(
		key,
		service.ProjectStepSkipRequest{ProjectID: b, Step: "recording"},
	); code(
		err,
	) != service.CodeForbidden {
		t.Errorf("another project = %v, want E_FORBIDDEN", err)
	}

	readOnly := service.WithCaller(context.Background(), service.Caller{
		Actor: "key:reader", Scopes: []string{service.ScopeRead},
	})

	if _, err := e.svc.ProjectStepSkip(
		readOnly,
		service.ProjectStepSkipRequest{ProjectID: a, Step: "finishing"},
	); code(
		err,
	) != service.CodeForbidden {
		t.Errorf("a read key = %v, want E_FORBIDDEN", err)
	}
}

// VV-436: the line that answers a created text-only project does not tell its author to add a timeline.
func TestProjectCreateTextAnswersWithoutATimelineHint(t *testing.T) {
	t.Parallel()

	e := newEnv(t)

	req := testproject.Request("Blog post two")
	req.Kind = "text"

	text, err := e.svc.ProjectCreate(e.as(ui), req)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(text.Text, "timeline") || !strings.Contains(text.Text, "kind text") {
		t.Errorf("text project answer = %q", text.Text)
	}

	req.Name, req.Kind = "Video two", "video"

	video, err := e.svc.ProjectCreate(e.as(ui), req)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(video.Text, "kind timeline") {
		t.Errorf("video project answer = %q", video.Text)
	}
}
