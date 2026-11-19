package servicetest

import (
	"context"
	"fmt"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
)

// SDD-003 section 1.7 rule 4, for the texts of SDD-019: a key limited to project A reaches the texts of A when the
// id it names is a timeline of A, and a timeline of another project, that project and an id of nothing get one
// answer, so the refusal says nothing about what exists.
func TestSecurity_T06_LimitedKeyReachesTextsByATimelineOfItsProject(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	a, b := e.newProject("A"), e.newProject("B")

	second := func(prj string) string {
		added, err := e.svc.ProjectItemAdd(e.as(ui), service.ProjectItemAddRequest{
			ProjectID: prj, Kind: "timeline", Title: "Wide", Preset: "16:9",
		})
		if err != nil {
			t.Fatal(err)
		}

		return added.Item.TimelineID
	}

	a2, b2 := second(a), second(b)
	script := e.newText(a, "Hook", "script", "VO: one two three")
	caption := e.newText(a, "Post", "caption", "## x\nhello")
	_ = e.newText(b, "Other", "copy", "of B")

	key := service.WithCaller(context.Background(), service.Caller{
		Actor: "key:limited", Scopes: service.AllScopes, ProjectID: a,
	})

	list, err := e.svc.TextList(key, service.TextListRequest{ProjectID: a2})
	if err != nil || list.ProjectID != a || len(list.Texts) != 2 {
		t.Fatalf("TextList by a timeline of A: %+v, %v", list, err)
	}

	calls := map[string]func(id string) error{
		"TextGet": func(id string) error {
			_, err := e.svc.TextGet(key, service.TextGetRequest{ProjectID: id, ItemID: script.ID})

			return err
		},
		"TextWrite": func(id string) error {
			_, err := e.svc.TextWrite(key, service.TextWriteRequest{
				ProjectID: id, ItemID: script.ID, Text: "VO: one two three four", DryRun: true,
			})

			return err
		},
		"TextRevisions": func(id string) error {
			_, err := e.svc.TextRevisions(key, service.TextRevisionsRequest{ProjectID: id, ItemID: script.ID})

			return err
		},
		"TextRevert": func(id string) error {
			_, err := e.svc.TextRevert(key, service.TextRevertRequest{
				ProjectID: id, ItemID: script.ID, Revision: 1, DryRun: true,
			})

			return err
		},
		"ScriptEstimate": func(id string) error {
			_, err := e.svc.ScriptEstimate(key, service.ScriptEstimateRequest{ProjectID: id, ItemID: script.ID})

			return err
		},
		"CaptionValidate": func(id string) error {
			_, err := e.svc.CaptionValidate(key, service.CaptionValidateRequest{ProjectID: id, ItemID: caption.ID})

			return err
		},
		"TextDraft": func(id string) error {
			_, err := e.svc.TextDraft(key, service.TextDraftRequest{
				ProjectID: id, ItemID: script.ID, ConnectionID: "con_0000000000", Model: "m", Instruction: "x",
				DryRun: true,
			})

			return err
		},
	}

	answer := func(err error) string {
		se := serviceError(t, err)

		return fmt.Sprintf("%s|%s|%s", se.Code, se.Message, se.Hint)
	}

	missing := "prj_0000000000zz"

	for name, call := range calls {
		// TextDraft goes on to the connection, which this test has not made: what matters is that the key
		// passed the project, so any refusal but E_FORBIDDEN is the connection's.
		if err := call(a2); err != nil && (name != "TextDraft" || code(err) == service.CodeForbidden) {
			t.Errorf("%s by a timeline of A: %v", name, err)
		}

		want := answer(call(missing))
		if code(call(missing)) != service.CodeForbidden {
			t.Errorf("%s of an id of nothing: %v, want E_FORBIDDEN", name, call(missing))
		}

		for what, id := range map[string]string{"timeline of B": b2, "project B": b} {
			if got := answer(call(id)); got != want {
				t.Errorf("%s of the %s: %q, want the answer of an id of nothing %q", name, what, got, want)
			}
		}
	}
}
