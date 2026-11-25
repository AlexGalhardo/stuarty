package servicetest

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/capturehost"
	"github.com/AlexGalhardo/stuarty/service"
)

// The commands that finish a screen take (SDD-002 C.93 to C.95, SDD-025 section 17) through the Service: the service
// finds the recording of the clip's asset, reads its pointer events and fills resolved.

// takePlaced makes a recording with the pointer events (or without), has a person add it and places it on a timeline.
// It returns the project and the id of the screen clip.
func (r *recEnv) takePlaced(t *testing.T, events bool) (project, clip string) {
	t.Helper()

	project = r.newProject("p")

	rec, err := r.svc.RecordingStart(personCtx(), service.RecordingStartRequest{
		ProjectID: project, Source: service.RecordingSource{Kind: "display", ID: "d1"}, FPS: 30, Events: events,
	})
	if err != nil {
		t.Fatalf("RecordingStart: %v", err)
	}

	s := r.cap.last()

	t.Cleanup(func() {
		s.end(capturehost.Lost{Err: capturehost.ErrLost})

		pollUntil(5*time.Second, func() bool {
			open, oerr := r.store.Recordings().Open(context.Background())

			return oerr == nil && len(open) == 0
		})
	})

	if _, err = r.svc.RecordingStop(personCtx(), service.RecordingRef{ID: rec.ID}); err != nil {
		t.Fatal(err)
	}

	r.waitState(t, rec.ID, "ready")

	if _, err = r.svc.RecordingConfirm(personCtx(), service.RecordingRef{ID: rec.ID}); err != nil {
		t.Fatal(err)
	}

	pollUntil(10*time.Second, func() bool {
		_, err = r.svc.RecordingPlace(r.as(agent), service.RecordingPlaceRequest{ID: rec.ID})

		return err == nil || code(err) != "E_ASSET_NOT_READY"
	})

	if err != nil {
		t.Fatalf("RecordingPlace: %v", err)
	}

	for id, it := range r.document2(t, project).Items {
		if it.Type == timeline.ItemClip {
			return project, id
		}
	}

	t.Fatal("no clip was placed")

	return "", ""
}

func (r *recEnv) document2(t *testing.T, project string) *timeline.Document {
	t.Helper()

	pg, err := r.svc.ProjectGet(r.as(agent), service.ProjectGetRequest{ProjectID: project, Detail: "document"})
	if err != nil {
		t.Fatal(err)
	}

	return pg.Document
}

func (r *recEnv) revision(t *testing.T, project string) int64 {
	t.Helper()

	pg, err := r.svc.ProjectGet(r.as(agent), service.ProjectGetRequest{ProjectID: project, Detail: "document"})
	if err != nil {
		t.Fatal(err)
	}

	return pg.Revision
}

func TestFinishingATakeReadsThePointerEventsOfItsRecording(t *testing.T) {
	r := newRecEnv(t)
	project, clip := r.takePlaced(t, true)
	base := r.revision(t, project)

	res := r.mustEdit(agent, project, base,
		op("recording.auto_zoom", `{"item_id":"`+clip+`"}`),
		op("recording.set_cursor", `{"item_id":"`+clip+`"}`),
		op("recording.set_click_highlight", `{"item_id":"`+clip+`","style":"wave"}`))
	if !res.Applied {
		t.Fatalf("not applied: %+v", res)
	}

	doc := r.document2(t, project)
	it := doc.Items[clip]

	if len(it.Keyframes["scale"]) < 4 || len(it.Keyframes["x"]) != len(it.Keyframes["scale"]) {
		t.Errorf("the zoom left %v", it.Keyframes)
	}

	cursor, clicks := 0, 0

	for _, o := range doc.Items {
		switch {
		case o.Type == timeline.ItemShape && o.Name == "Cursor "+clip:
			cursor++
		case o.Type == timeline.ItemShape && len(o.Name) > 6 && o.Name[:6] == "Click ":
			clicks++
		}
	}

	// the recording holds two presses (a left and a right one) in its 2 s
	if cursor < 1 || clicks != 2 {
		t.Errorf("%d cursor items and %d highlights", cursor, clicks)
	}

	// one undo of the batch gives back the timeline as it was
	before := r.document2(t, project)

	if _, err := r.svc.Undo(r.as(agent), service.UndoRequest{ProjectID: project}); err != nil {
		t.Fatal(err)
	}

	after := r.document2(t, project)
	if len(after.Items) >= len(before.Items) || len(after.Items[clip].Keyframes) != 0 {
		t.Errorf("undo left %d items and keyframes %v", len(after.Items), after.Items[clip].Keyframes)
	}
}

func TestFinishingATakeWithoutPointerEventsChangesNothing(t *testing.T) {
	r := newRecEnv(t)
	project, clip := r.takePlaced(t, false)
	base := r.revision(t, project)
	before := r.document(project)

	for _, name := range []string{"recording.auto_zoom", "recording.set_cursor", "recording.set_click_highlight"} {
		_, err := r.edit(agent, project, base, op(name, `{"item_id":"`+clip+`"}`))
		if code(err) != "E_INVALID_STATE" {
			t.Errorf("%s: %v", name, err)
		}
	}

	if !bytes.Equal(before, r.document(project)) || r.revision(t, project) != base {
		t.Error("a refused edit changed the project")
	}
}

func TestFinishingATakeRefusesWhatIsNotAScreenRecording(t *testing.T) {
	r := newRecEnv(t)
	project, clip := r.takePlaced(t, true)
	base := r.revision(t, project)

	// the caller cannot send the pointer events: the service reads them
	_, err := r.edit(agent, project, base, op("recording.auto_zoom",
		`{"item_id":"`+clip+`","resolved":{"width":10,"height":10,"events":[]}}`))
	if code(err) != "E_VALIDATION" {
		t.Errorf("resolved from a caller: %v", err)
	}

	// a clip of any other video has no recording to read
	other := r.newProject("other")
	if _, err = r.edit(
		agent,
		other,
		0,
		op("recording.auto_zoom", `{"item_id":"itm_zzzzzzzzzz"}`),
	); code(
		err,
	) != "E_NOT_FOUND" {
		t.Errorf("an item that is not there: %v", err)
	}
}
