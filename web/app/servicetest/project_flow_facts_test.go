package servicetest

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/project"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/approval"
)

// Epic 99: the facts the project flow screens draw come with the project: the kind, the words and the reading
// time of its text, the step it stands at, the skill and the settled cost of each step.
func TestProjectFlowFactsComeWithTheProject(t *testing.T) {
	t.Parallel()

	w := newActivityWorld(t)
	projectID := w.newProject("Launch video")
	w.reviewedSkill(t, "short-script")
	w.reviewedSkill(t, "brief-a-production")

	body := strings.TrimSpace(strings.Repeat("word ", 450))
	if _, err := w.svc.ProjectItemAdd(w.as(ui), service.ProjectItemAddRequest{
		ProjectID: projectID, Kind: "text", Title: "Script", Text: body, TextKind: "script",
	}); err != nil {
		t.Fatal(err)
	}

	day := approval.Day(w.clock.now())

	for i, l := range []struct {
		kind, state string
		micros      int64
	}{
		{"text.generate", "settled", 12_000}, {"text.generate", "settled", 3_000},
		{"image.generate", "settled", 70_000}, {"text.generate", "reserved", 500_000},
	} {
		if _, err := w.store.DB().ExecContext(t.Context(), `INSERT INTO cost_ledger (id, request_id, job_id,
 connection_id, project_id, kind, currency, estimate_micros, actual_micros, state, day, created_ms, settled_ms)
 VALUES (?, 'apr_f', ?, '', ?, ?, 'USD', ?, ?, ?, ?, 1, 1)`,
			fmt.Sprintf("led_flow0000%d", i), fmt.Sprintf("job_f%d", i), projectID, l.kind, l.micros, l.micros,
			l.state, day); err != nil {
			t.Fatal(err)
		}
	}

	step := func(wf project.Workflow, id project.StepID) project.Step {
		t.Helper()

		st, ok := wf.Step(id)
		if !ok {
			t.Fatalf("no step %q in %+v", id, wf.Steps)
		}

		return st
	}

	check := func(name string, wf project.Workflow, sum *service.TextSummary, next any) {
		t.Helper()

		if sum == nil || sum.TextKind != "script" || sum.Words != 450 || sum.ReadingMinutes != 3 {
			t.Errorf("%s: text summary = %+v, want a script of 450 words read in 3 minutes", name, sum)
		}

		if next != nil {
			t.Errorf("%s: next post = %v, want none", name, next)
		}

		script := step(wf, project.StepScript)
		if script.Skill != "short-script" || !slices.Equal(script.Spent, []project.Cost{{Micros: 15_000, Currency: "USD"}}) {
			t.Errorf("%s: script step = %+v", name, script)
		}

		if got := step(wf, project.StepBriefing).Skill; got != "brief-a-production" {
			t.Errorf("%s: briefing skill = %q", name, got)
		}

		editing := step(wf, project.StepEditing)
		if editing.Skill != "" || !slices.Equal(editing.Spent, []project.Cost{{Micros: 70_000, Currency: "USD"}}) {
			t.Errorf("%s: editing step = %+v (a skill the library lacks is not named; a reservation is not spent)", name, editing)
		}
	}

	got, err := w.svc.ProjectGet(w.as(ui), service.ProjectGetRequest{ProjectID: projectID})
	if err != nil {
		t.Fatal(err)
	}

	var next any
	if got.NextPostAt != nil {
		next = *got.NextPostAt
	}

	check("ProjectGet", got.Workflow, got.TextSummary, next)

	desc, err := w.svc.ProjectDescribe(w.as(ui), service.ProjectDescribeRequest{ProjectID: projectID})
	if err != nil {
		t.Fatal(err)
	}

	next = nil
	if desc.NextPostAt != nil {
		next = *desc.NextPostAt
	}

	check("ProjectDescribe", desc.Workflow, desc.TextSummary, next)

	list, err := w.svc.ProjectList(w.as(ui), service.ProjectListRequest{})
	if err != nil || len(list.Projects) != 1 {
		t.Fatalf("ProjectList = %+v, %v", list, err)
	}

	if p := list.Projects[0]; p.CurrentStep == "" || p.CurrentStep != string(got.Workflow.Current) || p.NextPostAt != nil {
		t.Errorf("list: current step %q (project says %q), next post %v", p.CurrentStep, got.Workflow.Current, p.NextPostAt)
	}
}
