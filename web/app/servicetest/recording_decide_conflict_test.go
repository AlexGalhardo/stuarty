package servicetest

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service"
)

// interposeStore runs a function once, at the next write of a project, before that write: it puts an edit of a
// person between what a decision read and the batch it sends.
type interposeStore struct {
	service.Store
	next *atomic.Pointer[func()]
}

func (s interposeStore) WriteProject(
	ctx context.Context, id string, fn func(ctx context.Context, tx service.ProjectTx) error,
) error {
	if f := s.next.Swap(nil); f != nil {
		(*f)()
	}

	return s.Store.WriteProject(ctx, id, fn)
}

// SDD-025 section 19.5: the batch of a decision is planned on the revision read just before it, so an edit made
// in between, even by the same person, is a conflict: nothing of the decision is applied and the proposal stays
// pending.
func TestRecordingProposalDecideConflictsWithAnEditMadeMeanwhile(t *testing.T) {
	var next atomic.Pointer[func()]

	r := &recEnv{cap: &fakeCapture{}}
	r.mediaEnv = newMediaEnv(t, mediaCfg{
		wrapStore: func(s service.Store) service.Store { return interposeStore{Store: s, next: &next} },
		with: func(_ *env, o *service.Options) {
			o.Capture = r.cap
			o.Limits.MinFreeBytes = 1 << 20
		},
	})

	project, id := r.placedTake(t)

	prop, err := r.svc.RecordingProposeCuts(r.as(agent), service.RecordingProposeCutsRequest{
		ID: id, Ranges: ranges(200, 400),
	})
	if err != nil {
		t.Fatal(err)
	}

	var clip string

	for itemID, it := range r.document2(t, project).Items {
		if it.Type == timeline.ItemClip {
			clip = itemID
		}
	}

	if clip == "" {
		t.Fatal("no clip of the take on the timeline")
	}

	rev := r.revision(t, project)

	// the person nudges the clip of the take by one frame while the decision is on its way: planned on the old
	// revision, the cut would take the wrong frames
	var moved *service.EditResult

	move := func() {
		args, merr := json.Marshal(map[string]string{"item_id": clip, "start": "1f"})
		if merr != nil {
			t.Error(merr)

			return
		}

		var eerr error

		moved, eerr = r.svc.Edit(personCtx(), service.EditRequest{
			ProjectID: project, BaseRevision: rev, Operations: []command.Operation{{Op: "item.move", Args: args}},
		})
		if eerr != nil {
			t.Errorf("the edit of the person: %v", eerr)
		}
	}
	next.Store(&move)

	_, err = r.svc.RecordingProposalDecide(personCtx(), service.RecordingProposalDecideRequest{
		ID: id, ProposalID: prop.Proposals[0].ID, Accept: []bool{true},
	})
	if next.Load() != nil || moved == nil {
		t.Fatal("the edit of the person did not run between the read and the batch")
	}

	if code(err) != "E_REVISION_CONFLICT" {
		t.Fatalf("a decision over a stale revision = %v, want E_REVISION_CONFLICT", err)
	}

	if got := r.revision(t, project); got != moved.Revision {
		t.Errorf("revision = %d, want %d: only the edit of the person", got, moved.Revision)
	}

	if it := r.document2(t, project).Items[clip]; it.Start != 1 {
		t.Errorf("the clip the person moved is at %d, want 1: %s", it.Start, fmt.Sprint(it))
	}

	read, err := r.svc.RecordingGet(r.as(agent), service.RecordingRef{ID: id})
	if err != nil || read.Proposals[0].State != "pending" {
		t.Fatalf("after the conflict: %+v %v", read, err)
	}
}
