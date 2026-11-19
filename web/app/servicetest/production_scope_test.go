package servicetest

import (
	"context"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
)

// productionCalls is one call of every operation that loads a production, with id as its project_id.
func productionCalls(m *mediaEnv, id string) map[string]func(ctx context.Context) error {
	p := m.prod()

	return map[string]func(ctx context.Context) error{
		"ProjectGet": func(ctx context.Context) error {
			_, err := m.svc.ProjectGet(ctx, service.ProjectGetRequest{ProjectID: id})
			return err
		},
		"ProjectDescribe": func(ctx context.Context) error {
			_, err := p.ProjectDescribe(ctx, service.ProjectDescribeRequest{ProjectID: id})
			return err
		},
		"ProjectDuplicate": func(ctx context.Context) error {
			_, err := p.ProjectDuplicate(ctx, service.ProjectDuplicateRequest{ProjectID: id, DryRun: true})
			return err
		},
		"ProjectItemAdd": func(ctx context.Context) error {
			_, err := p.ProjectItemAdd(
				ctx, service.ProjectItemAddRequest{ProjectID: id, Kind: "text", Title: "t", Text: "x", DryRun: true},
			)

			return err
		},
		"ProjectItemTrash": func(ctx context.Context) error {
			_, err := p.ProjectItemTrash(
				ctx, service.ProjectItemTrashRequest{ProjectID: id, ItemID: "itm_0000000001", DryRun: true},
			)

			return err
		},
		"BriefGet": func(ctx context.Context) error {
			_, err := p.BriefGet(ctx, service.BriefGetRequest{ProjectID: id})
			return err
		},
		"BriefSet": func(ctx context.Context) error {
			_, err := p.BriefSet(ctx, service.BriefSetRequest{ProjectID: id, Text: "x", DryRun: true})
			return err
		},
		"ProjectTrash": func(ctx context.Context) error {
			_, err := m.svc.ProjectTrash(ctx, service.ProjectTrashRequest{ProjectID: id})
			return err
		},
	}
}

// VV-246: a key limited to one id never reaches the production of another project through the id of one of
// its timelines, and learns no more than for an id that does not exist.
func TestSecurity_VV246_LimitedKeyDoesNotReachAnotherProductionByTimeline(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{noQueue: true})
	a, _ := m.productionProject()
	b := m.newProject("other")

	added, err := m.prod().ProjectItemAdd(
		m.as(ui), service.ProjectItemAddRequest{ProjectID: b, Kind: "timeline", Title: "Wide", Preset: "16:9"},
	)
	if err != nil {
		t.Fatal(err)
	}

	timelineOfB := added.Item.TimelineID

	keyOf := func(id string) context.Context {
		return service.WithCaller(context.Background(), service.Caller{
			Actor: "key:limited", Scopes: service.AllScopes, ProjectID: id,
		})
	}

	// The key of project A names a timeline of B: refused, as an id of nothing is.
	for name, call := range productionCalls(m, timelineOfB) {
		if err := call(keyOf(a)); code(err) != service.CodeForbidden {
			t.Errorf("%s: key of A, timeline of B = %v, want E_FORBIDDEN", name, err)
		}
	}

	// A key limited to the id of a timeline of B (what KeyCreate used to accept) names that timeline: the
	// project it resolves to is not the one the key is limited to.
	for name, call := range productionCalls(m, timelineOfB) {
		if err := call(keyOf(timelineOfB)); code(err) != service.CodeForbidden {
			t.Errorf("%s: key limited to a timeline of B = %v, want E_FORBIDDEN", name, err)
		}
	}

	// Its own project still works.
	for _, name := range []string{"ProjectGet", "ProjectDescribe", "BriefGet", "BriefSet"} {
		if err := productionCalls(m, a)[name](keyOf(a)); err != nil {
			t.Errorf("%s: key of A, project A = %v", name, err)
		}
	}

	// A duplicate creates a project, which a limited key never does.
	if err := productionCalls(m, a)["ProjectDuplicate"](keyOf(a)); code(err) != service.CodeForbidden {
		t.Errorf("ProjectDuplicate: key of A = %v, want E_FORBIDDEN", err)
	}
}
