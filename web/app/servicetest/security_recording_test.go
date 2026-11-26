package servicetest

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/recording"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/testproject"
)

// SDD-007 T-06: in network mode, a member cannot list, read, create, mark, propose cuts on,
// or cancel/delete recordings belonging to another user's project.
func TestSecurity_T06_RecordingTenantIsolation(t *testing.T) {
	t.Parallel()

	e := newEnvWith(t, func(e *env, o *service.Options) {
		o.Workspace = string(e.dir)
	})

	alice := networkPerson("alice", memberScopes, true)
	bob := networkPerson("bob", memberScopes, true)
	local := e.browser("local:ui")

	proj, err := testproject.CreateWithTimeline(alice, e.svc, "Alice Recording Project")
	if err != nil {
		t.Fatal(err)
	}

	// Alice starts a browser upload recording and prepares a recording plan.
	up, err := e.svc.RecordingUploadBegin(alice, service.RecordingUploadBeginRequest{
		ProjectID: proj.ProjectID,
		Container: "webm",
		Label:     "Alice Upload",
	})
	if err != nil {
		t.Fatal(err)
	}

	chunkDir := filepath.Join(string(e.dir), "recordings", up.ID)
	if err := os.MkdirAll(chunkDir, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(chunkDir, "keep.txt")
	if err := os.WriteFile(marker, []byte("alice"), 0o600); err != nil {
		t.Fatal(err)
	}

	prep, err := e.svc.RecordingPrepare(alice, service.RecordingPrepareRequest{
		ProjectID: proj.ProjectID,
		Source:    service.RecordingSource{Kind: recording.SourceDisplay, ID: "disp_1", Name: "Main"},
		FPS:       30,
		Notes:     "private plan",
	})
	if err != nil {
		t.Fatal(err)
	}

	assertProjectNotFound := func(name string, gotErr error) {
		t.Helper()
		if code(gotErr) != command.CodeNotFound {
			t.Errorf("%s: got %v (code %s), want %s", name, gotErr, code(gotErr), command.CodeNotFound)
		}
	}
	assertRecordingNotFound := func(name string, gotErr error) {
		t.Helper()
		if code(gotErr) != service.CodeRecordingNotFound {
			t.Errorf("%s: got %v (code %s), want %s", name, gotErr, code(gotErr), service.CodeRecordingNotFound)
		}
	}

	// Bob must get not-found on every recording operation targeting Alice's project or recordings.
	_, err = e.svc.RecordingList(bob, service.RecordingListRequest{ProjectID: proj.ProjectID})
	assertProjectNotFound("RecordingList", err)

	_, err = e.svc.RecordingGet(bob, service.RecordingRef{ID: up.ID})
	assertRecordingNotFound("RecordingGet(upload)", err)

	_, err = e.svc.RecordingGet(bob, service.RecordingRef{ID: prep.ID})
	assertRecordingNotFound("RecordingGet(prepared)", err)

	_, err = e.svc.RecordingEvents(bob, service.RecordingEventsRequest{ID: up.ID})
	assertRecordingNotFound("RecordingEvents", err)

	_, err = e.svc.RecordingPrepare(bob, service.RecordingPrepareRequest{
		ProjectID: proj.ProjectID,
		Source:    service.RecordingSource{Kind: recording.SourceDisplay, ID: "disp_1", Name: "Main"},
		FPS:       30,
	})
	assertProjectNotFound("RecordingPrepare", err)

	_, err = e.svc.RecordingUploadBegin(bob, service.RecordingUploadBeginRequest{
		ProjectID: proj.ProjectID,
		Container: "webm",
	})
	assertProjectNotFound("RecordingUploadBegin", err)

	_, err = e.svc.RecordingUploadChunk(bob, service.RecordingUploadChunkRequest{
		ID: up.ID, Track: string(recording.TrackScreen), Index: 0, Body: bytes.NewReader([]byte("x")), Size: 1,
	})
	assertRecordingNotFound("RecordingUploadChunk", err)

	_, err = e.svc.RecordingUploadFinish(bob, service.RecordingUploadFinishRequest{
		ID: up.ID, Tracks: []service.RecordingUploadCount{{Kind: string(recording.TrackScreen), Chunks: 0}},
	})
	assertRecordingNotFound("RecordingUploadFinish", err)

	_, err = e.svc.RecordingMark(bob, service.RecordingMarkRequest{
		ID: up.ID, Kind: string(recording.MarkGood), AtMS: 100, Note: "intrusion",
	})
	assertRecordingNotFound("RecordingMark", err)

	_, err = e.svc.RecordingProposeCuts(bob, service.RecordingProposeCutsRequest{
		ID: up.ID, Ranges: []service.RecordingCutRange{{FromMS: 0, ToMS: 100, Reason: "x"}},
	})
	assertRecordingNotFound("RecordingProposeCuts", err)

	_, err = e.svc.RecordingProposalDecide(bob, service.RecordingProposalDecideRequest{
		ID: up.ID, ProposalID: "prop_0000000001", Accept: []bool{false},
	})
	assertRecordingNotFound("RecordingProposalDecide", err)

	_, err = e.svc.RecordingStop(bob, service.RecordingRef{ID: up.ID})
	assertRecordingNotFound("RecordingStop", err)

	_, err = e.svc.RecordingPause(bob, service.RecordingRef{ID: up.ID})
	assertRecordingNotFound("RecordingPause", err)

	_, err = e.svc.RecordingResume(bob, service.RecordingRef{ID: up.ID})
	assertRecordingNotFound("RecordingResume", err)

	_, err = e.svc.RecordingCancel(bob, service.RecordingRef{ID: up.ID})
	assertRecordingNotFound("RecordingCancel", err)

	_, err = e.svc.RecordingConfirm(bob, service.RecordingRef{ID: up.ID})
	assertRecordingNotFound("RecordingConfirm", err)

	_, err = e.svc.RecordingDismiss(bob, service.RecordingRef{ID: up.ID})
	assertRecordingNotFound("RecordingDismiss", err)

	// Bob's rejected RecordingCancel must not have removed Alice's recording folder.
	if _, statErr := os.Stat(marker); statErr != nil {
		t.Fatalf("alice's recording folder was touched by bob's rejected cancel: %v", statErr)
	}

	// Owner (alice) and local-mode caller still see both recordings.
	for name, ctx := range map[string]context.Context{"alice": alice, "local": local} {
		list, lerr := e.svc.RecordingList(ctx, service.RecordingListRequest{ProjectID: proj.ProjectID})
		if lerr != nil {
			t.Fatalf("%s RecordingList failed: %v", name, lerr)
		}
		if len(list.Recordings) != 2 {
			t.Fatalf("%s RecordingList len = %d, want 2", name, len(list.Recordings))
		}
	}
}
