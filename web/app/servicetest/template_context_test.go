package servicetest

import (
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/testproject"
)

// A project made from a template is born with no description and no goal, and with a name of a title (VV-359,
// SDD-005 section 5.12.2): the page offers to complete it, and completing it holds the name to the rule of a project
// name and renames the editor as well.
func TestProjectFromATemplateCanBeCompleted(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{exporter: goodExporter()})
	ctx := m.as(ui)

	saved := m.saveTemplate(m.templateProject())
	long := strings.Repeat("Spring teaser ", 7) + "end"

	res, err := m.svc.TemplateInstantiate(ctx, service.TemplateInstantiateRequest{
		TemplateID: saved.Template.TemplateID, Name: long,
	})
	if err != nil {
		t.Fatalf("a name of a title: %v", err)
	}

	got, err := m.prod().ProjectDescribe(ctx, service.ProjectDescribeRequest{ProjectID: res.ProjectID})
	if err != nil || got.Complete || got.Name != long {
		t.Fatalf("describe = %+v, %v; want an incomplete project named %q", got, err, long)
	}

	name, goal, desc := "Spring teaser", testproject.Goal, testproject.Description

	if _, err := m.svc.ProjectUpdate(ctx, service.ProjectUpdateRequest{
		ProjectID: res.ProjectID, Name: &long, Description: &desc, Goal: &goal,
	}); !isCode(err, "E_VALIDATION") {
		t.Errorf("completing it with the long name: %v, want E_VALIDATION", err)
	}

	if _, err := m.svc.ProjectUpdate(ctx, service.ProjectUpdateRequest{
		ProjectID: res.ProjectID, Name: &name, Description: &desc, Goal: &goal,
	}); err != nil {
		t.Fatalf("completing it: %v", err)
	}

	got, err = m.prod().ProjectDescribe(ctx, service.ProjectDescribeRequest{ProjectID: res.ProjectID})
	if err != nil || !got.Complete || got.Name != name {
		t.Fatalf("describe = %+v, %v; want a complete project named %q", got, err, name)
	}

	doc, err := timeline.Decode(m.document(res.ProjectID))
	if err != nil || doc.Name != name {
		t.Errorf("the editor shows %q, want %q (%v)", doc.Name, name, err)
	}
}
