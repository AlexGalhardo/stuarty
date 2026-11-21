package servicetest

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/project"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/testproject"
)

func (e *env) prod() service.ProductionService {
	e.t.Helper()

	p, ok := e.svc.(service.ProductionService)
	if !ok {
		e.t.Fatal("the service has no production operations")
	}

	return p
}

// addTimeline adds a timeline to the project and returns its item and its timeline id.
func (e *env) addTimeline(projectID, title string) (itemID, timelineID string) {
	e.t.Helper()

	res, err := e.prod().ProjectItemAdd(e.as(ui), service.ProjectItemAddRequest{
		ProjectID: projectID, Kind: "timeline", Title: title, Preset: "16:9",
	})
	if err != nil {
		e.t.Fatalf("add a timeline: %v", err)
	}

	if res.Item.TimelineID == "" || res.Item.Kind != "timeline" {
		e.t.Fatalf("the item has no timeline: %+v", res.Item)
	}

	return res.Item.ID, res.Item.TimelineID
}

func (e *env) get(projectID, timelineID, detail string) *service.ProjectGetResult {
	e.t.Helper()

	res, err := e.svc.ProjectGet(e.as(ui), service.ProjectGetRequest{
		ProjectID: projectID, TimelineID: timelineID, Detail: detail,
	})
	if err != nil {
		e.t.Fatalf("project get: %v", err)
	}

	return res
}

// A project made the way every project was made before ADR-0039 is a project with one timeline that has its id.
func TestProjectHasOneTimelineItem(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	id := e.newProject("Teaser")

	got := e.get(id, "", "")
	if got.ProjectID != id || got.TimelineID != id || got.Name != "Teaser" {
		t.Fatalf("project %q timeline %q name %q", got.ProjectID, got.TimelineID, got.Name)
	}

	if len(got.Items) != 1 || got.Items[0].Kind != "timeline" || got.Items[0].TimelineID != id ||
		got.Items[0].Position != 0 {
		t.Fatalf("items = %+v", got.Items)
	}

	if want := (service.ProductionState{Timelines: 1}); got.Production != want {
		t.Fatalf("production = %+v, want %+v", got.Production, want)
	}

	// The commands of a project that has one timeline are addressed as they always were.
	e.mustEdit(ui, id, 0, op("track.add", `{"kind":"visual"}`))

	if h := e.head(id); h.Revision != 1 {
		t.Fatalf("revision = %d", h.Revision)
	}
}

// Acceptance criterion 2: a project holds two timelines, ProjectGet lists both and commands address each by
// timeline_id.
func TestProjectTwoTimelines(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	first := e.newProject("Show")
	_, second := e.addTimeline(first, "Vertical cut")

	if second == first {
		t.Fatal("the second timeline has the id of the first")
	}

	got := e.get(first, "", "")
	if len(got.Items) != 2 || got.Production.Timelines != 2 {
		t.Fatalf("items = %+v production = %+v", got.Items, got.Production)
	}

	ids := []string{got.Items[0].TimelineID, got.Items[1].TimelineID}
	if !slices.Equal(ids, []string{first, second}) {
		t.Fatalf("timelines = %v, want [%s %s]", ids, first, second)
	}

	if !strings.Contains(got.Text, first) || !strings.Contains(got.Text, second) {
		t.Fatalf("the overview does not name both timelines:\n%s", got.Text)
	}

	// The rule of SDD-003 section 1.7: no timeline_id and two timelines is an error that names the choices.
	_, err := e.svc.Edit(e.as(ui), service.EditRequest{
		ProjectID: first, Operations: []command.Operation{op("track.add", `{"kind":"visual"}`)},
	})

	se := serviceError(t, err)
	if se.Code != "E_VALIDATION" || se.Path != "timeline_id" || !strings.Contains(se.Hint, second) ||
		!strings.Contains(se.Hint, first) {
		t.Fatalf("ambiguous edit = %+v", se)
	}

	if listed, _ := se.Details["timelines"].([]string); !slices.Equal(listed, []string{first, second}) {
		t.Fatalf("details.timelines = %v", se.Details["timelines"])
	}

	if _, err := e.svc.ProjectGet(
		e.as(ui),
		service.ProjectGetRequest{ProjectID: first, Detail: "document"},
	); code(
		err,
	) != "E_VALIDATION" {
		t.Fatalf("a document of an ambiguous project: %v", err)
	}

	// Each timeline is edited by its id and has its own revision, history and undo.
	for _, tl := range []string{first, second} {
		res, err := e.svc.Edit(e.as(ui), service.EditRequest{
			ProjectID: first, TimelineID: tl, Operations: []command.Operation{op("track.add", `{"kind":"visual"}`)},
		})
		if err != nil || res.Revision != 1 || res.ProjectID != tl {
			t.Fatalf("edit of %s: %+v %v", tl, res, err)
		}
	}

	if _, err := e.svc.Edit(e.as(ui), service.EditRequest{
		TimelineID: second, BaseRevision: 1, Operations: []command.Operation{op("track.add", `{"kind":"audio"}`)},
	}); err != nil {
		t.Fatalf("edit by timeline_id alone: %v", err)
	}

	if e.head(first).Revision != 1 || e.head(second).Revision != 2 {
		t.Fatalf("revisions %d and %d", e.head(first).Revision, e.head(second).Revision)
	}

	hist, err := e.svc.HistoryList(e.as(ui), service.HistoryListRequest{ProjectID: first, TimelineID: second})
	if err != nil || len(hist.Entries) != 2 {
		t.Fatalf("history of the second timeline: %v %v", hist, err)
	}

	if _, err := e.svc.Undo(e.as(ui), service.UndoRequest{ProjectID: first, TimelineID: second}); err != nil {
		t.Fatalf("undo: %v", err)
	}

	if e.head(first).Revision != 1 || e.head(second).Revision != 3 {
		t.Fatalf("after the undo: %d and %d", e.head(first).Revision, e.head(second).Revision)
	}

	if _, err := e.svc.Lint(e.as(ui), service.LintRequest{ProjectID: first, TimelineID: second}); err != nil {
		t.Fatalf("lint: %v", err)
	}

	doc := e.get(first, second, "document")
	if doc.Document == nil || doc.Document.ID != second || doc.TimelineID != second || doc.Revision != 3 {
		t.Fatalf("document of the second timeline: %+v", doc)
	}

	// A timeline of another project, or one that does not exist, is not found.
	other := e.newProject("Other")

	for _, bad := range []service.ProjectGetRequest{
		{ProjectID: other, TimelineID: second},
		{ProjectID: first, TimelineID: "prj_9999999999"},
	} {
		if _, err := e.svc.ProjectGet(e.as(ui), bad); code(err) != "E_NOT_FOUND" {
			t.Errorf("%+v: %v", bad, err)
		}
	}

	// A timeline id in the place of the project id names that timeline.
	if got := e.get(second, "", ""); got.ProjectID != first || got.TimelineID != second {
		t.Errorf("project %q timeline %q", got.ProjectID, got.TimelineID)
	}

	list, err := e.svc.ProjectList(e.as(ui), service.ProjectListRequest{})
	if err != nil || len(list.Projects) != 2 {
		t.Fatalf("list = %v, %v", list, err)
	}

	for _, p := range list.Projects {
		if p.ID == first && p.Timelines != 2 {
			t.Errorf("timelines of the list entry = %d", p.Timelines)
		}
	}

	if !strings.Contains(list.Text, "timelines=2") {
		t.Errorf("the list text does not show the timelines:\n%s", list.Text)
	}
}

// The timeline that trashed one of two is the only one left, and the compatibility rule addresses it again.
func TestProjectTrashedTimelineLeavesOneToAddress(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	first := e.newProject("Show")
	item, second := e.addTimeline(first, "Cut 2")
	ctx := e.as(ui)

	if _, err := e.prod().
		ProjectItemTrash(ctx, service.ProjectItemTrashRequest{ProjectID: first, ItemID: item}); err != nil {
		t.Fatal(err)
	}

	// The trashed timeline rejects edits until restored.
	_, err := e.svc.Edit(ctx, service.EditRequest{
		TimelineID: second, Operations: []command.Operation{op("track.add", `{"kind":"visual"}`)},
	})
	if code(err) != "E_INVALID_STATE" {
		t.Fatalf("edit of a trashed timeline: %v", err)
	}

	// With one active timeline, project_id alone addresses it.
	res, err := e.svc.Edit(
		ctx,
		service.EditRequest{ProjectID: first, Operations: []command.Operation{op("track.add", `{"kind":"visual"}`)}},
	)
	if err != nil || res.ProjectID != first {
		t.Fatalf("edit by project id: %+v %v", res, err)
	}

	if got := e.get(first, "", ""); got.Production.Timelines != 1 || got.TimelineID != first {
		t.Fatalf("after the trash: %+v", got.Production)
	}

	if _, err := e.prod().
		ProjectItemRestore(ctx, service.ProjectItemTrashRequest{ProjectID: first, ItemID: item}); err != nil {
		t.Fatal(err)
	}

	if _, err := e.svc.Edit(ctx, service.EditRequest{
		TimelineID: second, Operations: []command.Operation{op("track.add", `{"kind":"visual"}`)},
	}); err != nil {
		t.Fatalf("edit after the restore: %v", err)
	}
}

func TestProjectItems(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	id := e.newProject("Series")
	ctx := e.as(ui)
	p := e.prod()

	text, err := p.ProjectItemAdd(ctx, service.ProjectItemAddRequest{
		ProjectID: id, Kind: "text", Title: "Script", Text: "Scene one.\nScene two.",
	})
	if err != nil || text.Item.Position != 1 || !text.Production.HasScript {
		t.Fatalf("add a text: %+v %v", text, err)
	}

	at := 0

	notes, err := p.ProjectItemAdd(
		ctx,
		service.ProjectItemAddRequest{ProjectID: id, Kind: "text", Title: "Notes", Position: &at},
	)
	if err != nil || notes.Item.Position != 0 {
		t.Fatalf("add at the head: %+v %v", notes, err)
	}

	order := func() []string {
		var titles []string
		for _, it := range e.get(id, "", "").Items {
			titles = append(titles, it.Title)
		}

		return titles
	}

	if got := order(); !slices.Equal(got, []string{"Notes", "Series", "Script"}) {
		t.Fatalf("order = %v", got)
	}

	if _, err := p.ProjectItemMove(
		ctx,
		service.ProjectItemMoveRequest{ProjectID: id, ItemID: notes.Item.ID, Position: 99},
	); err != nil {
		t.Fatal(err)
	}

	if got := order(); !slices.Equal(got, []string{"Series", "Script", "Notes"}) {
		t.Fatalf("order after the move = %v", got)
	}

	for i, it := range e.get(id, "", "").Items {
		if it.Position != i {
			t.Errorf("item %q has position %d, want %d", it.Title, it.Position, i)
		}
	}

	renamed, err := p.ProjectItemRename(
		ctx,
		service.ProjectItemRenameRequest{ProjectID: id, ItemID: notes.Item.ID, Title: "Ideas"},
	)
	if err != nil || renamed.Item.Title != "Ideas" {
		t.Fatalf("rename: %+v %v", renamed, err)
	}

	// The timeline document is not touched by a rename of its item.
	if _, err := p.ProjectItemRename(
		ctx,
		service.ProjectItemRenameRequest{ProjectID: id, ItemID: e.get(id, "", "").Items[0].ID, Title: "Main cut"},
	); err != nil {
		t.Fatal(err)
	}

	if got := e.get(id, "", ""); got.Name != "Series" || got.Items[0].Title != "Main cut" {
		t.Fatalf("name %q, first item %q", got.Name, got.Items[0].Title)
	}

	trashed, err := p.ProjectItemTrash(ctx, service.ProjectItemTrashRequest{ProjectID: id, ItemID: text.Item.ID})
	if err != nil || trashed.Item.State != "trashed" || trashed.Production.HasScript {
		t.Fatalf("trash: %+v %v", trashed, err)
	}

	again, err := p.ProjectItemTrash(ctx, service.ProjectItemTrashRequest{ProjectID: id, ItemID: text.Item.ID})
	if err != nil || again.Item.State != "trashed" {
		t.Fatalf("trash again: %+v %v", again, err)
	}

	restored, err := p.ProjectItemRestore(ctx, service.ProjectItemTrashRequest{ProjectID: id, ItemID: text.Item.ID})
	if err != nil || restored.Item.State != "active" || !restored.Production.HasScript {
		t.Fatalf("restore: %+v %v", restored, err)
	}

	// The last timeline stays.
	_, err = p.ProjectItemTrash(
		ctx,
		service.ProjectItemTrashRequest{ProjectID: id, ItemID: e.get(id, "", "").Items[0].ID},
	)
	if se := serviceError(t, err); se.Code != "E_INVALID_STATE" {
		t.Fatalf("trash of the last timeline: %+v", se)
	}

	// Dry runs change nothing.
	if _, err := p.ProjectItemTrash(
		ctx,
		service.ProjectItemTrashRequest{ProjectID: id, ItemID: text.Item.ID, DryRun: true},
	); err != nil {
		t.Fatal(err)
	}

	if got := e.get(id, "", ""); got.Items[1].State != "active" {
		t.Fatalf("a dry run trashed the item: %+v", got.Items)
	}

	refused := []service.ProjectItemAddRequest{
		{ProjectID: id, Kind: "brief", Title: "Brief"},
		{ProjectID: id, Kind: "image", Title: "Thumb"},
		{ProjectID: id, Kind: "video", Title: "x"},
		{ProjectID: id, Kind: "text", Title: ""},
		{ProjectID: id, Kind: "text", Title: "a\x00b"},
		{ProjectID: id, Kind: "output", Title: "Final"},
		{ProjectID: id, Kind: "output", Title: "Final", JobID: "job_9999999999"},
		{ProjectID: id, Kind: "text", Title: "Big", Text: strings.Repeat("a", project.MaxText+1)},
		{ProjectID: id, Kind: "timeline", Title: "Odd", Preset: "3:2"},
	}
	for _, r := range refused {
		if _, err := p.ProjectItemAdd(ctx, r); err == nil {
			t.Errorf("%+v was accepted", r)
		}
	}

	if _, err := p.ProjectItemMove(
		ctx,
		service.ProjectItemMoveRequest{ProjectID: id, ItemID: "pit_9999999999", Position: 0},
	); code(
		err,
	) != "E_NOT_FOUND" {
		t.Errorf("move of an unknown item: %v", err)
	}

	if _, err := p.ProjectItemMove(
		ctx,
		service.ProjectItemMoveRequest{ProjectID: id, ItemID: text.Item.ID, Position: -1},
	); code(
		err,
	) != "E_VALIDATION" {
		t.Errorf("move to a negative position: %v", err)
	}
}

// An output item records an export that succeeded.
func TestProjectOutputItem(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	id := e.newProject("Ad")
	other := e.newProject("Other")
	ctx := e.as(ui)

	e.exportJob(id, "job_0000000901", "succeeded")
	e.exportJob(id, "job_0000000902", "running")
	e.exportJob(other, "job_0000000903", "succeeded")

	res, err := e.prod().ProjectItemAdd(ctx, service.ProjectItemAddRequest{
		ProjectID: id, Kind: "output", Title: "Final", JobID: "job_0000000901",
	})
	if err != nil || res.Item.JobID != "job_0000000901" || res.Item.Format != "mp4" || res.Production.Outputs != 1 {
		t.Fatalf("add an output: %+v %v", res, err)
	}

	if _, err := e.prod().ProjectItemAdd(ctx, service.ProjectItemAddRequest{
		ProjectID: id, Kind: "output", Title: "Pending", JobID: "job_0000000902",
	}); code(err) != "E_INVALID_STATE" {
		t.Errorf("a running export: %v", err)
	}

	if _, err := e.prod().ProjectItemAdd(ctx, service.ProjectItemAddRequest{
		ProjectID: id, Kind: "output", Title: "Foreign", JobID: "job_0000000903",
	}); code(err) != "E_NOT_FOUND" {
		t.Errorf("the job of another project: %v", err)
	}
}

// Acceptance criterion 4: BriefSet then BriefGet round-trips Markdown, and ProjectDescribe includes the brief
// within the token budget.
func TestBriefRoundTripAndDescribe(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	id := e.newProject("Launch")
	ctx := e.as(ui)
	p := e.prod()

	empty, err := p.BriefGet(ctx, service.BriefGetRequest{ProjectID: id})
	if err != nil || empty.Text != "" || empty.ItemID != "" || empty.UpdatedAt != nil {
		t.Fatalf("the brief of a new project: %+v %v", empty, err)
	}

	md := "# Brief\n\n- **Audience:** makers\n- Tone: warm, *direct*\n\n> \"Ship it\" — 日本語 ✓\n\n```go\nfmt.Println(\"<b>&</b>\")\n```\n"

	set, err := p.BriefSet(ctx, service.BriefSetRequest{ProjectID: id, Text: md})
	if err != nil || set.Text != md || set.ItemID == "" {
		t.Fatalf("set: %+v %v", set, err)
	}

	got, err := p.BriefGet(ctx, service.BriefGetRequest{ProjectID: id})
	if err != nil || got.Text != md || got.ItemID != set.ItemID {
		t.Fatalf("get: %+v %v", got, err)
	}

	if proj := e.get(
		id,
		"",
		"",
	); !proj.Production.HasBrief || proj.Items[0].Kind != "brief" ||
		proj.Items[0].Text != "" {
		t.Fatalf("project after the brief: %+v / %+v", proj.Production, proj.Items)
	}

	desc, err := p.ProjectDescribe(ctx, service.ProjectDescribeRequest{ProjectID: id})
	if err != nil {
		t.Fatal(err)
	}

	if !desc.Production.HasBrief || !strings.Contains(desc.Text, "Audience") ||
		!strings.Contains(desc.Text, "<untrusted_media_text") ||
		!strings.Contains(desc.Text, id) {
		t.Fatalf("the description lacks the brief:\n%s", desc.Text)
	}

	// An update keeps the one brief item; an empty text clears it.
	if _, err := p.BriefSet(ctx, service.BriefSetRequest{ProjectID: id, Text: "second"}); err != nil {
		t.Fatal(err)
	}

	if proj := e.get(id, "", ""); len(proj.Items) != 2 {
		t.Fatalf("a second BriefSet made another item: %+v", proj.Items)
	}

	cleared, err := p.BriefSet(ctx, service.BriefSetRequest{ProjectID: id, Text: ""})
	if err != nil || cleared.Text != "" || e.get(id, "", "").Production.HasBrief {
		t.Fatalf("clear: %+v %v", cleared, err)
	}

	// A dry run writes nothing; a text over the limit is refused.
	if _, err := p.BriefSet(ctx, service.BriefSetRequest{ProjectID: id, Text: "dry", DryRun: true}); err != nil {
		t.Fatal(err)
	}

	if got, _ := p.BriefGet(ctx, service.BriefGetRequest{ProjectID: id}); got.Text != "" {
		t.Fatalf("a dry run wrote the brief: %q", got.Text)
	}

	if _, err := p.BriefSet(
		ctx,
		service.BriefSetRequest{ProjectID: id, Text: strings.Repeat("a", project.MaxText+1)},
	); code(
		err,
	) != "E_LIMIT_EXCEEDED" {
		t.Fatalf("an oversized brief: %v", err)
	}

	// A very long brief and many items stay inside the budget, and the cut says so.
	long := strings.Repeat("The brief says something long. ", 4000)
	if _, err := p.BriefSet(ctx, service.BriefSetRequest{ProjectID: id, Text: long}); err != nil {
		t.Fatal(err)
	}

	for i := range 40 {
		if _, err := p.ProjectItemAdd(ctx, service.ProjectItemAddRequest{
			ProjectID: id, Kind: "text", Title: strings.Repeat("Title ", 5) + string(rune('A'+i%26)),
		}); err != nil {
			t.Fatal(err)
		}
	}

	for _, tokens := range []int{0, 200, 1500, 6000} {
		d, err := p.ProjectDescribe(ctx, service.ProjectDescribeRequest{ProjectID: id, MaxTokens: tokens})
		if err != nil {
			t.Fatal(err)
		}

		budget := tokens
		if budget == 0 {
			budget = 1500
		}

		// The brief keeps at least 200 characters and the head of the text is a few hundred more, so the text
		// may pass the budget by a small, fixed margin and no more.
		if limit := budget*3 + 700; len(d.Text) > limit {
			t.Errorf("max_tokens %d: the text has %d characters, at most %d", tokens, len(d.Text), limit)
		}

		if !strings.Contains(d.Text, "brief:") || !strings.Contains(d.Text, "something long") {
			t.Errorf("max_tokens %d: no brief in the text", tokens)
		}

		if tokens != 6000 && !strings.Contains(d.Text, `truncated="true"`) {
			t.Errorf("max_tokens %d: the cut brief is not marked", tokens)
		}
	}

	if _, err := p.ProjectDescribe(
		ctx,
		service.ProjectDescribeRequest{ProjectID: id, MaxTokens: 50},
	); code(
		err,
	) != "E_VALIDATION" {
		t.Errorf("a budget below the minimum: %v", err)
	}
}

func TestProjectTrashAndRestoreMirrorTheTimelines(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	first := e.newProject("Show")
	_, second := e.addTimeline(first, "Cut 2")
	ctx := e.as(ui)

	if _, err := e.svc.ProjectTrash(ctx, service.ProjectTrashRequest{ProjectID: first}); err != nil {
		t.Fatal(err)
	}

	for _, tl := range []string{first, second} {
		if h := e.head(tl); h.State != service.ProjectTrashed {
			t.Errorf("timeline %s is %s", tl, h.State)
		}

		_, err := e.svc.Edit(
			ctx,
			service.EditRequest{TimelineID: tl, Operations: []command.Operation{op("track.add", `{"kind":"visual"}`)}},
		)
		if code(err) != "E_INVALID_STATE" {
			t.Errorf("edit of %s in a trashed project: %v", tl, err)
		}
	}

	if _, err := e.prod().
		ProjectItemAdd(ctx, service.ProjectItemAddRequest{ProjectID: first, Kind: "text", Title: "x"}); code(
		err,
	) != "E_INVALID_STATE" {
		t.Errorf("an item added to a trashed project: %v", err)
	}

	if list, _ := e.svc.ProjectList(ctx, service.ProjectListRequest{}); len(list.Projects) != 0 {
		t.Errorf("the list shows a trashed project: %+v", list.Projects)
	}

	if _, err := e.svc.ProjectRestore(ctx, service.ProjectTrashRequest{ProjectID: first}); err != nil {
		t.Fatal(err)
	}

	for _, tl := range []string{first, second} {
		if h := e.head(tl); h.State != service.ProjectActive {
			t.Errorf("timeline %s is %s after the restore", tl, h.State)
		}
	}
}

// A timeline in the trash stays in the trash when its project is restored.
func TestProjectRestoreKeepsATrashedTimelineTrashed(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	first := e.newProject("Show")
	item, second := e.addTimeline(first, "Cut 2")
	ctx := e.as(ui)

	if _, err := e.prod().
		ProjectItemTrash(ctx, service.ProjectItemTrashRequest{ProjectID: first, ItemID: item}); err != nil {
		t.Fatal(err)
	}

	if _, err := e.svc.ProjectTrash(ctx, service.ProjectTrashRequest{ProjectID: first}); err != nil {
		t.Fatal(err)
	}

	if _, err := e.svc.ProjectRestore(ctx, service.ProjectTrashRequest{ProjectID: first}); err != nil {
		t.Fatal(err)
	}

	if h := e.head(second); h.State != service.ProjectTrashed {
		t.Fatalf("the trashed timeline came back: %s", h.State)
	}

	if h := e.head(first); h.State != service.ProjectActive {
		t.Fatalf("the first timeline is %s", h.State)
	}
}

// VV-225: a project can be duplicated.
func TestProjectDuplicate(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	src := e.newProject("Teaser")
	_, second := e.addTimeline(src, "Wide")
	ctx := e.as(ui)

	if _, err := e.svc.Edit(ctx, service.EditRequest{
		TimelineID: src, Operations: []command.Operation{op("track.add", `{"kind":"visual"}`)},
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := e.svc.Edit(ctx, service.EditRequest{
		TimelineID: second, Operations: []command.Operation{op("track.add", `{"kind":"audio"}`)},
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := e.prod().BriefSet(ctx, service.BriefSetRequest{ProjectID: src, Text: "the brief"}); err != nil {
		t.Fatal(err)
	}

	if _, err := e.prod().
		ProjectItemAdd(ctx, service.ProjectItemAddRequest{ProjectID: src, Kind: "text", Title: "Script", Text: "words"}); err != nil {
		t.Fatal(err)
	}

	dry, err := e.prod().ProjectDuplicate(ctx, service.ProjectDuplicateRequest{ProjectID: src, DryRun: true})
	if err != nil || dry.Created || dry.ProjectID == "" {
		t.Fatalf("dry run: %+v %v", dry, err)
	}

	if list, _ := e.svc.ProjectList(ctx, service.ProjectListRequest{}); len(list.Projects) != 1 {
		t.Fatalf("a dry run made a project: %d", len(list.Projects))
	}

	cp, err := e.prod().ProjectDuplicate(ctx, service.ProjectDuplicateRequest{ProjectID: src})
	if err != nil || !cp.Created || cp.ProjectID == src || cp.Name != "Teaser copy" {
		t.Fatalf("duplicate: %+v %v", cp, err)
	}

	got := e.get(cp.ProjectID, "", "")
	if len(got.Items) != 4 || got.Production.Timelines != 2 || !got.Production.HasBrief || !got.Production.HasScript {
		t.Fatalf("the copy: %+v / %+v", got.Items, got.Production)
	}

	var timelines []string

	for _, it := range got.Items {
		if it.TimelineID != "" {
			timelines = append(timelines, it.TimelineID)
		}
	}

	if timelines[0] != cp.ProjectID || slices.Contains(timelines, src) || slices.Contains(timelines, second) {
		t.Fatalf("timelines of the copy = %v", timelines)
	}

	// The documents were copied at revision 0 with no log, and the originals are untouched.
	for i, tl := range timelines {
		h := e.head(tl)
		if h.Revision != 0 || e.operationCount(tl) != 0 || h.Summary.Tracks != 1 {
			t.Errorf("copied timeline %d: revision %d, tracks %d", i, h.Revision, h.Summary.Tracks)
		}
	}

	if e.head(src).Revision != 1 || e.head(second).Revision != 1 || e.get(src, "", "").Production.Timelines != 2 {
		t.Fatal("the original changed")
	}

	if b, _ := e.prod().BriefGet(ctx, service.BriefGetRequest{ProjectID: cp.ProjectID}); b.Text != "the brief" {
		t.Fatalf("the brief of the copy: %q", b.Text)
	}

	named, err := e.prod().ProjectDuplicate(ctx, service.ProjectDuplicateRequest{ProjectID: src, Name: "Alt cut"})
	if err != nil || named.Name != "Alt cut" {
		t.Fatalf("named copy: %+v %v", named, err)
	}

	if _, err := e.prod().
		ProjectDuplicate(ctx, service.ProjectDuplicateRequest{ProjectID: src, Name: "  "}); code(
		err,
	) != "E_VALIDATION" {
		t.Errorf("a blank name: %v", err)
	}

	// A copy of a long name is still a valid name (at most 32 characters for a project with a complete context).
	long := strings.Repeat("n", 120)
	big := e.newProject(long)

	if cp, err := e.prod().
		ProjectDuplicate(ctx, service.ProjectDuplicateRequest{ProjectID: big}); err != nil ||
		len([]rune(cp.Name)) > 32 || !strings.HasSuffix(cp.Name, " copy") {
		t.Fatalf("copy of a long name: %+v %v", cp, err)
	}

	// A key limited to a project cannot make another.
	limited := service.WithCaller(
		context.Background(),
		service.Caller{Actor: "key:k", Scopes: service.AllScopes, ProjectID: src},
	)
	if _, err := e.prod().
		ProjectDuplicate(limited, service.ProjectDuplicateRequest{ProjectID: src}); code(
		err,
	) != service.CodeForbidden {
		t.Errorf("a limited key duplicated: %v", err)
	}
}

// Emptying the trash deletes every timeline of a project, and the project with its items.
func TestProjectTrashEmptyDeletesEveryTimeline(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	first := e.newProject("Gone")
	_, second := e.addTimeline(first, "Cut 2")
	keep := e.newProject("Kept")
	ctx := e.as(ui)

	if _, err := e.svc.Edit(ctx, service.EditRequest{
		TimelineID: second, Operations: []command.Operation{op("track.add", `{"kind":"visual"}`)},
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := e.svc.ProjectTrash(ctx, service.ProjectTrashRequest{ProjectID: first}); err != nil {
		t.Fatal(err)
	}

	person := service.WithCaller(
		context.Background(),
		service.Caller{Actor: "local:ui", Scopes: service.AllScopes, Browser: true},
	)

	res, err := e.svc.TrashEmpty(person, service.TrashEmptyRequest{})
	if err != nil || res.Projects != 1 {
		t.Fatalf("empty: %+v %v", res, err)
	}

	for _, tl := range []string{first, second} {
		if _, err := e.store.Projects().Head(context.Background(), tl); !errors.Is(err, service.ErrNotFound) {
			t.Errorf("timeline %s is still there: %v", tl, err)
		}
	}

	if _, err := e.store.Productions().Get(context.Background(), first); !errors.Is(err, service.ErrNotFound) {
		t.Errorf("the project is still there: %v", err)
	}

	if items, err := e.store.Productions().Items(context.Background(), first); err != nil || len(items) != 0 {
		t.Errorf("items left: %v %v", items, err)
	}

	if e.operationCount(second) != 0 {
		t.Error("the log of the second timeline stayed")
	}

	if _, err := e.svc.ProjectGet(ctx, service.ProjectGetRequest{ProjectID: keep}); err != nil {
		t.Fatalf("the other project: %v", err)
	}
}

// Another user's project is not found, and its timelines are not listed to someone who may not see them.
func TestProjectTimelinesRespectOwnership(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	alice := service.WithCaller(
		context.Background(),
		service.Caller{Actor: "user:alice", OwnerUserID: "alice", Scopes: service.AllScopes},
	)
	bob := service.WithCaller(
		context.Background(),
		service.Caller{Actor: "user:bob", OwnerUserID: "bob", Scopes: service.AllScopes},
	)

	made, err := testproject.CreateWithTimeline(alice, e.svc, "Alice")
	if err != nil {
		t.Fatal(err)
	}

	added, err := e.prod().
		ProjectItemAdd(alice, service.ProjectItemAddRequest{ProjectID: made.ProjectID, Kind: "timeline", Title: "Two"})
	if err != nil {
		t.Fatal(err)
	}

	two := added.Item.TimelineID
	ops := []command.Operation{op("track.add", `{"kind":"visual"}`)}

	// Two timelines and no timeline_id: alice is told the choices, bob is told the project does not exist.
	_, err = e.svc.Edit(alice, service.EditRequest{ProjectID: made.ProjectID, Operations: ops})
	if se := serviceError(t, err); se.Code != "E_VALIDATION" || !strings.Contains(se.Hint, two) {
		t.Fatalf("alice: %+v", se)
	}

	_, err = e.svc.Edit(bob, service.EditRequest{ProjectID: made.ProjectID, Operations: ops})
	if se := serviceError(t, err); se.Code != "E_NOT_FOUND" || strings.Contains(se.Hint+se.Message, two) {
		t.Fatalf("bob: %+v", se)
	}

	for name, req := range map[string]service.EditRequest{
		"by timeline": {TimelineID: two, Operations: ops},
		"both":        {ProjectID: made.ProjectID, TimelineID: two, Operations: ops},
	} {
		if _, err := e.svc.Edit(bob, req); code(err) != "E_NOT_FOUND" {
			t.Errorf("bob %s: %v", name, err)
		}
	}

	if _, err := e.prod().
		ProjectItemAdd(bob, service.ProjectItemAddRequest{ProjectID: made.ProjectID, Kind: "text", Title: "x"}); code(
		err,
	) != "E_NOT_FOUND" {
		t.Errorf("bob added an item: %v", err)
	}

	if _, err := e.prod().
		BriefGet(bob, service.BriefGetRequest{ProjectID: made.ProjectID}); code(
		err,
	) != "E_NOT_FOUND" {
		t.Errorf("bob read the brief: %v", err)
	}

	if _, err := e.prod().
		ProjectDescribe(bob, service.ProjectDescribeRequest{ProjectID: made.ProjectID}); code(
		err,
	) != "E_NOT_FOUND" {
		t.Errorf("bob described: %v", err)
	}

	if _, err := e.prod().
		ProjectDuplicate(bob, service.ProjectDuplicateRequest{ProjectID: made.ProjectID}); code(
		err,
	) != "E_NOT_FOUND" {
		t.Errorf("bob duplicated: %v", err)
	}

	if list, _ := e.svc.ProjectList(bob, service.ProjectListRequest{}); len(list.Projects) != 0 {
		t.Errorf("bob lists %d projects", len(list.Projects))
	}

	if cp, err := e.prod().
		ProjectDuplicate(alice, service.ProjectDuplicateRequest{ProjectID: made.ProjectID}); err != nil {
		t.Errorf("alice duplicated: %v", err)
	} else if h, _ := e.store.Productions().
		Get(context.Background(), cp.ProjectID); h.OwnerUserID == nil ||
		*h.OwnerUserID != "alice" {
		t.Errorf("the owner of the copy: %v", h.OwnerUserID)
	}
}

// A key limited to one project reaches every timeline of that project and no other.
func TestProjectLimitedKeyAndTimelines(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	mine := e.newProject("Mine")
	_, two := e.addTimeline(mine, "Two")
	other := e.newProject("Other")
	otherTwoItem, otherTwo := e.addTimeline(other, "Two")
	_ = otherTwoItem

	key := service.WithCaller(
		context.Background(),
		service.Caller{Actor: "key:k", Scopes: service.AllScopes, ProjectID: mine},
	)
	ops := []command.Operation{op("track.add", `{"kind":"visual"}`)}

	if res, err := e.svc.Edit(
		key,
		service.EditRequest{TimelineID: two, Operations: ops},
	); err != nil ||
		res.ProjectID != two {
		t.Fatalf("own second timeline: %+v %v", res, err)
	}

	for _, tl := range []string{other, otherTwo} {
		if _, err := e.svc.Edit(
			key,
			service.EditRequest{TimelineID: tl, Operations: ops},
		); code(
			err,
		) != service.CodeForbidden {
			t.Errorf("timeline %s of another project: %v", tl, err)
		}
	}

	if _, err := e.svc.Edit(
		key,
		service.EditRequest{ProjectID: other, TimelineID: otherTwo, Operations: ops},
	); code(
		err,
	) != service.CodeForbidden {
		t.Errorf("another project: %v", err)
	}
}

// A timeline that is renamed through the command renames the project that has its id, while the two carry one name
// (SDD-005 section 5.12); a later timeline is a label of its own.
func TestProjectNameFollowsTheFirstTimeline(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	id := e.newProject("Before")
	_, second := e.addTimeline(id, "Second")
	ctx := e.as(ui)

	if _, err := e.svc.Edit(
		ctx,
		service.EditRequest{TimelineID: id, Operations: []command.Operation{op("project.rename", `{"name":"After"}`)}},
	); err != nil {
		t.Fatal(err)
	}

	if _, err := e.svc.Edit(
		ctx,
		service.EditRequest{
			TimelineID: second,
			Operations: []command.Operation{op("project.rename", `{"name":"Other name"}`)},
		},
	); err != nil {
		t.Fatal(err)
	}

	got := e.get(id, "", "")
	if got.Name != "After" {
		t.Fatalf("the project is named %q", got.Name)
	}

	list, _ := e.svc.ProjectList(ctx, service.ProjectListRequest{})
	if len(list.Projects) != 1 || list.Projects[0].Name != "After" {
		t.Fatalf("list = %+v", list.Projects)
	}
}

// The name of a project and that of its first timeline stay one: a rename on either side reaches the other, an
// undo brings both back, and a rename the rule of a project name refuses changes nothing.
func TestProjectNameAndFirstTimelineStayInStep(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	id := e.newProject("Before")
	ctx := e.as(ui)

	name := "Renamed by update"
	if _, err := e.svc.ProjectUpdate(ctx, service.ProjectUpdateRequest{ProjectID: id, Name: &name}); err != nil {
		t.Fatal(err)
	}

	doc := e.get(id, id, "document")
	if doc.Name != name || doc.Document.Name != name {
		t.Fatalf("project %q, timeline %q, want both %q", doc.Name, doc.Document.Name, name)
	}

	if _, err := e.svc.Undo(ctx, service.UndoRequest{ProjectID: id}); err != nil {
		t.Fatal(err)
	}

	if got := e.get(id, id, "document"); got.Name != "Before" || got.Document.Name != "Before" {
		t.Fatalf("after undo: project %q, timeline %q", got.Name, got.Document.Name)
	}

	_, err := e.svc.Edit(ctx, service.EditRequest{
		ProjectID: id, Operations: []command.Operation{op("project.rename", `{"name":"no"}`)},
	})
	if code(err) != command.CodeValidation || serviceError(t, err).Path != "name" {
		t.Fatalf("a name the rule refuses: %v", err)
	}

	if got := e.get(id, id, "document"); got.Name != "Before" || got.Document.Name != "Before" {
		t.Fatalf("the refused rename changed something: %q, %q", got.Name, got.Document.Name)
	}

	// A first timeline with a title of its own is a label: renaming it leaves the project alone.
	made, err := testproject.Create(ctx, e.svc, "Own title")
	if err != nil {
		t.Fatal(err)
	}

	e.addTimeline(made.ProjectID, "Main cut")

	if _, err := e.svc.Edit(ctx, service.EditRequest{
		ProjectID: made.ProjectID, Operations: []command.Operation{op("project.rename", `{"name":"x"}`)},
	}); err != nil {
		t.Fatal(err)
	}

	if got := e.get(made.ProjectID, made.ProjectID, "document"); got.Name != "Own title" || got.Document.Name != "x" {
		t.Fatalf("project %q, timeline %q", got.Name, got.Document.Name)
	}
}

// The brief is cleared, never trashed: a trashed brief would stand beside the next one, and restoring it broke
// the rule of one brief with an internal error.
func TestProjectBriefIsNotTrashed(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	ctx := e.as(ui)
	p := e.newProject("P")

	b, err := e.prod().BriefSet(ctx, service.BriefSetRequest{ProjectID: p, Text: "one"})
	if err != nil {
		t.Fatal(err)
	}

	_, err = e.prod().ProjectItemTrash(ctx, service.ProjectItemTrashRequest{ProjectID: p, ItemID: b.ItemID})
	if se := serviceError(t, err); se.Code != "E_INVALID_STATE" || se.Path != "item_id" {
		t.Fatalf("trash of the brief: %+v", se)
	}

	got, err := e.prod().BriefGet(ctx, service.BriefGetRequest{ProjectID: p})
	if err != nil || got.ItemID != b.ItemID || got.Text != "one" {
		t.Fatalf("the brief after the refusal: %+v, %v", got, err)
	}
}

// A project holds at most as many items and timelines as a bundle of the whole project may carry, so that its
// bundle imports again; a restore counts as an add.
func TestProjectItemsHaveRoom(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	ctx := e.as(ui)
	p := e.newProject("P")

	var last string

	for range project.MaxItems - 1 { // the first timeline is one of them
		res, err := e.prod().ProjectItemAdd(ctx, service.ProjectItemAddRequest{ProjectID: p, Kind: "text", Title: "T"})
		if err != nil {
			t.Fatal(err)
		}

		last = res.Item.ID
	}

	_, err := e.prod().ProjectItemAdd(ctx, service.ProjectItemAddRequest{ProjectID: p, Kind: "text", Title: "T"})
	if se := serviceError(t, err); se.Code != "E_LIMIT_EXCEEDED" {
		t.Fatalf("one item too many: %+v", se)
	}

	_, err = e.prod().BriefSet(ctx, service.BriefSetRequest{ProjectID: p, Text: "x"})
	if code(err) != "E_LIMIT_EXCEEDED" {
		t.Fatalf("a brief beyond the limit: %v", err)
	}

	trash := service.ProjectItemTrashRequest{ProjectID: p, ItemID: last}
	if _, err := e.prod().ProjectItemTrash(ctx, trash); err != nil {
		t.Fatal(err)
	}

	text := service.ProjectItemAddRequest{ProjectID: p, Kind: "text", Title: "T"}
	if _, err := e.prod().ProjectItemAdd(ctx, text); err != nil {
		t.Fatalf("a trashed item still counted: %v", err)
	}

	_, err = e.prod().ProjectItemRestore(ctx, service.ProjectItemTrashRequest{ProjectID: p, ItemID: last})
	if se := serviceError(t, err); se.Code != "E_LIMIT_EXCEEDED" || se.Path != "item_id" {
		t.Fatalf("a restore beyond the limit: %+v", se)
	}

	q := e.newProject("Q")
	for range project.MaxTimelines - 1 {
		e.addTimeline(q, "T")
	}

	_, err = e.prod().ProjectItemAdd(ctx, service.ProjectItemAddRequest{ProjectID: q, Kind: "timeline", Title: "T"})
	if se := serviceError(t, err); se.Code != "E_LIMIT_EXCEEDED" {
		t.Fatalf("one timeline too many: %+v", se)
	}
}
