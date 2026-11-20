package servicetest

import (
	"context"
	"fmt"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
)

// Epic 36 under SDD-003 section 1.7 rules 2 and 4: a key limited to project A reaches the revert, the diff and the
// checkpoints of every timeline of A, named by the timeline id in the place of the project id; a timeline of B, B
// itself and an id of nothing get the one refusal a project outside the key gets.
func TestSecurity_T06_LimitedKeyReachesTheHistoryOfItsSecondTimeline(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{exporter: goodExporter()})
	a := m.exportable()
	a2 := m.exportableSecond(a)
	b := m.exportable()
	b2 := m.exportableSecond(b)

	key := service.WithCaller(context.Background(), service.Caller{
		Actor: "key:limited", Scopes: service.AllScopes, ProjectID: a,
	})

	// In order: create, list, rename, diff from it, revert to it (a dry run), remove.
	calls := []struct {
		name string
		call func(id string) error
	}{
		{"CheckpointCreate", func(id string) error {
			_, err := m.svc.CheckpointCreate(
				key,
				service.CheckpointCreateRequest{ProjectID: id, Name: "mark", Revision: 1},
			)
			return err
		}},
		{"CheckpointList", func(id string) error {
			_, err := m.svc.CheckpointList(key, service.CheckpointListRequest{ProjectID: id})
			return err
		}},
		{"CheckpointRename", func(id string) error {
			_, err := m.svc.CheckpointRename(
				key,
				service.CheckpointRenameRequest{ProjectID: id, Name: "mark", NewName: "kept"},
			)
			return err
		}},
		{"HistoryDiff", func(id string) error {
			_, err := m.svc.HistoryDiff(key, service.HistoryDiffRequest{ProjectID: id, FromCheckpoint: "kept"})
			return err
		}},
		{"HistoryRevert", func(id string) error {
			_, err := m.svc.HistoryRevert(
				key,
				service.HistoryRevertRequest{ProjectID: id, Checkpoint: "kept", DryRun: true},
			)
			return err
		}},
		{"CheckpointRemove", func(id string) error {
			_, err := m.svc.CheckpointRemove(key, service.CheckpointRemoveRequest{ProjectID: id, Name: "kept"})
			return err
		}},
	}

	for _, c := range calls {
		if err := c.call(a2); err != nil {
			t.Errorf("%s of the second timeline of A by its id: %v", c.name, err)
		}
	}

	answer := func(err error) string {
		se := serviceError(t, err)

		return fmt.Sprintf("%s|%s|%s", se.Code, se.Message, se.Hint)
	}

	for _, c := range calls {
		want := answer(c.call("prj_0000000000zz"))

		for what, id := range map[string]string{"timeline of B": b2, "project B": b} {
			if got := answer(c.call(id)); got != want || code(c.call(id)) != service.CodeForbidden {
				t.Errorf("%s of the %s = %q, want the refusal of a missing id %q", c.name, what, got, want)
			}
		}
	}
}
