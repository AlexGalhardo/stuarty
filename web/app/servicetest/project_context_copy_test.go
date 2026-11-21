package servicetest

import (
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/testproject"
)

// A copy of a project with a context is such a project: a name given to it obeys the rule of the name.
func TestDuplicateHoldsAGivenNameToTheRule(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	id := e.newProject("Source project")

	_, err := e.prod().ProjectDuplicate(e.as(ui), service.ProjectDuplicateRequest{ProjectID: id, Name: "x"})
	if code(err) != command.CodeValidation || serviceError(t, err).Path != "name" {
		t.Fatalf("err = %v, want E_VALIDATION on name", err)
	}

	cp, err := e.prod().ProjectDuplicate(e.as(ui), service.ProjectDuplicateRequest{ProjectID: id, Name: "Good copy"})
	if err != nil || cp.Name != "Good copy" {
		t.Fatalf("a valid name: %+v, %v", cp, err)
	}
}

// The longest context there is comes whole in the text an agent reads first, with the default budget.
func TestDescribeKeepsTheLongestContextWhole(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	req := testproject.Request("Long context")
	req.Description = strings.Repeat("d", 1024)
	part := strings.Repeat("g", 279) + "!"
	req.Goal.Specific, req.Goal.Measurable, req.Goal.Achievable, req.Goal.Relevant, req.Goal.TimeBound =
		part, part, part, part, part

	made, err := e.svc.ProjectCreate(e.as(ui), req)
	if err != nil {
		t.Fatal(err)
	}

	d, err := e.prod().ProjectDescribe(e.as(ui), service.ProjectDescribeRequest{ProjectID: made.ProjectID})
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(d.Text, `truncated="true"`) || strings.Count(d.Text, part) != 5 ||
		!strings.Contains(d.Text, req.Description) || !strings.Contains(d.Text, "goal due: "+req.Goal.Due) {
		t.Fatalf("the context was cut:\n%s", d.Text)
	}
}
