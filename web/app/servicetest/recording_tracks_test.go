package servicetest

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/engine/capturehost"
	"github.com/AlexGalhardo/stuarty/service"
)

// startAll starts a recording with every track and the pointer file (SDD-025 section 16).
func (r *recEnv) startAll(t *testing.T, project string) *service.Recording {
	t.Helper()

	rec, err := r.svc.RecordingStart(personCtx(), service.RecordingStartRequest{
		ProjectID: project, Source: service.RecordingSource{Kind: "display", ID: "d1"}, FPS: 30,
		Microphone: "default", SystemAudio: true, Camera: "c0123456789abcdef", Events: true,
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

	return rec
}

// allTracksFinish is the end the helper reports for a recording with every track.
func allTracksFinish() *capturehost.Finished {
	return &capturehost.Finished{
		Reason: "stopped", Frames: 60, Dropped: 1, DurationMS: 2000, Bytes: 44, PointerEvents: 5,
		Tracks: []capturehost.TrackResult{
			{Kind: "screen", DurationMS: 2000, Bytes: 20},
			{Kind: "camera", DurationMS: 2010, Bytes: 11},
			{Kind: "microphone", DurationMS: 1990, Bytes: 11},
			{Kind: "system", DurationMS: 2000, Bytes: 11, Lost: true},
		},
		Pauses: []capturehost.PauseSpan{{FromMS: 700, ToMS: 1200}},
	}
}

func TestRecordingSourcesListTheDevicesAndCleanTheirNames(t *testing.T) {
	r := newRecEnv(t)

	res, err := r.svc.RecordingSources(r.as(agent), service.RecordingSourcesRequest{})
	if err != nil {
		t.Fatal(err)
	}

	if len(res.Microphones) != 1 || !res.Microphones[0].Default || len(res.Cameras) != 1 || !res.SystemAudio {
		t.Fatalf("sources = %+v", res)
	}

	if strings.ContainsRune(res.Microphones[0].Name, 0x1b) {
		t.Errorf("a control character reached a caller: %q", res.Microphones[0].Name)
	}
}

func TestRecordingStartPassesTheTracksAndTheirFiles(t *testing.T) {
	r := newRecEnv(t)
	project := r.newProject("p")

	rec := r.startAll(t, project)
	if len(rec.Tracks) != 4 || rec.Tracks[0].Kind != "screen" || rec.Tracks[1].Kind != "camera" ||
		rec.Tracks[2].Kind != "microphone" || rec.Tracks[3].Kind != "system" {
		t.Fatalf("tracks asked = %+v", rec.Tracks)
	}

	folder := filepath.Join(r.ws, "recordings", rec.ID)
	for _, f := range []string{"capture.mp4", "camera.mp4", "microphone.wav", "system.wav", "events.jsonl"} {
		if _, err := os.Stat(filepath.Join(folder, f)); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}

	// a device id with a brace, and a test id, never reach the helper
	for _, bad := range []service.RecordingStartRequest{
		{ProjectID: project, Source: service.RecordingSource{Kind: "display", ID: "d1"}, Microphone: "{0.0.1}"},
		{ProjectID: project, Source: service.RecordingSource{Kind: "display", ID: "d1"}, Camera: "a b"},
	} {
		if _, err := r.svc.RecordingStart(personCtx(), bad); code(err) != "E_VALIDATION" {
			t.Errorf("%+v: %v", bad, err)
		}
	}
}

func TestRecordingPauseAndResume(t *testing.T) {
	r := newRecEnv(t)
	project := r.newProject("p")
	rec := r.start(t, project)
	agentCtx := r.as(agent)
	s := r.cap.last()

	// anyone with the media scope may pause: it only reduces what is captured
	paused, err := r.svc.RecordingPause(agentCtx, service.RecordingRef{ID: rec.ID})
	if err != nil || paused.State != "paused" || s.paused.Load() != 1 {
		t.Fatalf("pause by an agent: %+v %v", paused, err)
	}

	// pausing again is the same answer and tells the helper nothing
	if again, err := r.svc.RecordingPause(agentCtx, service.RecordingRef{ID: rec.ID}); err != nil ||
		again.State != "paused" || s.paused.Load() != 1 {
		t.Fatalf("a second pause: %+v %v", again, err)
	}

	// resuming restarts the capture of the screen: a person only, and before anything else is looked at
	if _, err := r.svc.RecordingResume(
		agentCtx,
		service.RecordingRef{ID: rec.ID},
	); code(
		err,
	) != "E_PRIVILEGED_SESSION" {
		t.Fatalf("resume by an agent: %v", err)
	}

	if _, err := r.svc.RecordingResume(
		agentCtx,
		service.RecordingRef{ID: "rec_nonexistent"},
	); code(
		err,
	) != "E_PRIVILEGED_SESSION" {
		t.Fatalf("resume of nothing by an agent must still be the privilege error: %v", err)
	}

	if s.resumed.Load() != 0 {
		t.Fatal("the helper was told to resume for an agent")
	}

	resumed, err := r.svc.RecordingResume(personCtx(), service.RecordingRef{ID: rec.ID})
	if err != nil || resumed.State != "recording" || s.resumed.Load() != 1 {
		t.Fatalf("resume by a person: %+v %v", resumed, err)
	}

	if again, err := r.svc.RecordingResume(personCtx(), service.RecordingRef{ID: rec.ID}); err != nil ||
		again.State != "recording" || s.resumed.Load() != 1 {
		t.Fatalf("a second resume: %+v %v", again, err)
	}

	// a recording that is not running cannot be paused
	if _, err := r.svc.RecordingStop(agentCtx, service.RecordingRef{ID: rec.ID}); err != nil {
		t.Fatal(err)
	}

	r.waitState(t, rec.ID, "ready")

	if _, err := r.svc.RecordingPause(agentCtx, service.RecordingRef{ID: rec.ID}); code(err) != "E_INVALID_STATE" {
		t.Errorf("pause of a finished recording: %v", err)
	}
}

func TestRecordingStopWhilePausedKeepsTheFile(t *testing.T) {
	r := newRecEnv(t)
	project := r.newProject("p")
	rec := r.start(t, project)

	if _, err := r.svc.RecordingPause(personCtx(), service.RecordingRef{ID: rec.ID}); err != nil {
		t.Fatal(err)
	}

	if _, err := r.svc.RecordingStop(r.as(agent), service.RecordingRef{ID: rec.ID}); err != nil {
		t.Fatalf("stop of a paused recording: %v", err)
	}

	if ready := r.waitState(t, rec.ID, "ready"); ready.Review != "pending" {
		t.Errorf("ready = %+v", ready)
	}
}

func TestRecordingKeepsTheTracksAndPausesTheHelperReported(t *testing.T) {
	r := newRecEnv(t)
	r.cap.finish = allTracksFinish()
	project := r.newProject("p")
	rec := r.startAll(t, project)

	if _, err := r.svc.RecordingStop(personCtx(), service.RecordingRef{ID: rec.ID}); err != nil {
		t.Fatal(err)
	}

	ready := r.waitState(t, rec.ID, "ready")
	if len(ready.Tracks) != 4 || !ready.Tracks[3].Lost || ready.Tracks[2].DurationMS != 1990 {
		t.Fatalf("tracks = %+v", ready.Tracks)
	}

	if len(ready.Pauses) != 1 || ready.Pauses[0].FromMS != 700 || ready.EventsCount != 5 {
		t.Errorf("pauses %+v, events %d", ready.Pauses, ready.EventsCount)
	}

	if ready.Bytes != 20+11+11+11 {
		t.Errorf("bytes = %d, the size of every file", ready.Bytes)
	}

	if ready.Levels != nil {
		t.Errorf("levels are for a recording that runs: %+v", ready.Levels)
	}
}

func TestRecordingLevelsAreShownWhileItRuns(t *testing.T) {
	r := newRecEnv(t)
	project := r.newProject("p")
	rec := r.startAll(t, project)
	s := r.cap.last()

	s.send(capturehost.Level{Microphone: 420, System: 15})

	var got *service.Recording

	pollUntil(5*time.Second, func() bool {
		got, _ = r.svc.RecordingGet(personCtx(), service.RecordingRef{ID: rec.ID})

		return got != nil && got.Levels != nil && got.Levels.Microphone == 420
	})

	if got == nil || got.Levels == nil || got.Levels.Microphone != 420 || got.Levels.System != 15 {
		t.Fatalf("levels = %+v", got)
	}
}

func TestRecordingConfirmImportsEveryTrackAndPlaceAlignsThem(t *testing.T) {
	r := newRecEnv(t)
	r.cap.finish = allTracksFinish()
	project := r.newProject("p")
	rec := r.startAll(t, project)

	if _, err := r.svc.RecordingStop(personCtx(), service.RecordingRef{ID: rec.ID}); err != nil {
		t.Fatal(err)
	}

	r.waitState(t, rec.ID, "ready")

	added, err := r.svc.RecordingConfirm(personCtx(), service.RecordingRef{ID: rec.ID})
	if err != nil {
		t.Fatal(err)
	}

	ids := map[string]bool{}

	for _, tr := range added.Tracks {
		if tr.AssetID == "" || ids[tr.AssetID] {
			t.Fatalf("every track is an asset of its own: %+v", added.Tracks)
		}

		ids[tr.AssetID] = true
	}

	if added.AssetID != added.Tracks[0].AssetID {
		t.Errorf("the screen asset is %s, the track says %s", added.AssetID, added.Tracks[0].AssetID)
	}

	folder := filepath.Join(r.ws, "recordings", rec.ID)
	for _, gone := range []string{"capture.mp4", "camera.mp4", "microphone.wav", "system.wav"} {
		if _, err := os.Stat(filepath.Join(folder, gone)); !os.IsNotExist(err) {
			t.Errorf("%s must go once it is in the content store: %v", gone, err)
		}
	}

	if _, err := os.Stat(filepath.Join(folder, "events.jsonl")); err != nil {
		t.Errorf("the pointer events stay with an added recording: %v", err)
	}

	// the text for an agent names the other tracks and the click count
	if !strings.Contains(added.Text, "microphone is asset") || !strings.Contains(added.Text, "5 pointer events") {
		t.Errorf("text = %q", added.Text)
	}

	agentCtx := r.as(agent)

	var placed *service.EditResult

	pollUntil(10*time.Second, func() bool {
		placed, err = r.svc.RecordingPlace(agentCtx, service.RecordingPlaceRequest{ID: rec.ID})

		return err == nil || code(err) != "E_ASSET_NOT_READY"
	})

	if err != nil || !placed.Applied {
		t.Fatalf("RecordingPlace: %+v %v", placed, err)
	}

	pg, err := r.svc.ProjectGet(agentCtx, service.ProjectGetRequest{ProjectID: project, Detail: "document"})
	if err != nil {
		t.Fatal(err)
	}

	doc := pg.Document
	if len(doc.Items) != 4 || len(doc.Tracks) != 4 {
		t.Fatalf("one clip on each of four tracks: %d items on %d tracks", len(doc.Items), len(doc.Tracks))
	}

	visual, audio, starts := 0, 0, map[int]bool{}

	for _, tr := range doc.Tracks {
		if tr.Kind == "visual" {
			visual++
		} else {
			audio++
		}
	}

	for _, it := range doc.Items {
		starts[it.Start] = true
	}

	if visual != 2 || audio != 2 || len(starts) != 1 {
		t.Errorf("%d visual and %d audio tracks, starts %v: the clips must start together", visual, audio, starts)
	}

	// one undo removes the whole batch
	if _, err := r.svc.Undo(agentCtx, service.UndoRequest{ProjectID: project}); err != nil {
		t.Fatal(err)
	}

	if pg, _ := r.svc.ProjectGet(
		agentCtx,
		service.ProjectGetRequest{ProjectID: project, Detail: "document"},
	); len(
		pg.Document.Items,
	) != 0 {
		t.Errorf("one undo must remove every clip: %d left", len(pg.Document.Items))
	}
}

func TestRecordingEventsAreReadByTheClicksOrAll(t *testing.T) {
	r := newRecEnv(t)
	project := r.newProject("p")
	rec := r.startAll(t, project)

	// not while it runs
	if _, err := r.svc.RecordingEvents(
		r.as(agent),
		service.RecordingEventsRequest{ID: rec.ID},
	); code(
		err,
	) != "E_INVALID_STATE" {
		t.Errorf("events of a recording that runs: %v", err)
	}

	if _, err := r.svc.RecordingStop(personCtx(), service.RecordingRef{ID: rec.ID}); err != nil {
		t.Fatal(err)
	}

	r.waitState(t, rec.ID, "ready")

	clicks, err := r.svc.RecordingEvents(r.as(agent), service.RecordingEventsRequest{ID: rec.ID})
	if err != nil {
		t.Fatal(err)
	}

	if clicks.Width != 1920 || clicks.Height != 1080 || clicks.Total != 4 || len(clicks.Events) != 4 ||
		clicks.Truncated {
		t.Fatalf("clicks = %+v", clicks)
	}

	first := clicks.Events[0]
	if first.Kind != "down" || first.Button != "left" || first.TMS != 500 || first.X != 300 || first.Y != 400 {
		t.Errorf("the first click = %+v", first)
	}

	all, err := r.svc.RecordingEvents(r.as(agent), service.RecordingEventsRequest{ID: rec.ID, Kind: "all", Limit: 2})
	if err != nil || all.Total != 5 || len(all.Events) != 2 || !all.Truncated {
		t.Fatalf("all, limit 2: %+v %v", all, err)
	}

	from, err := r.svc.RecordingEvents(r.as(agent), service.RecordingEventsRequest{ID: rec.ID, FromMS: 1000})
	if err != nil || from.Total != 2 || from.Events[0].Button != "right" {
		t.Fatalf("from 1000: %+v %v", from, err)
	}

	for _, bad := range []service.RecordingEventsRequest{
		{ID: rec.ID, Kind: "keys"}, {ID: rec.ID, Limit: 5001}, {ID: rec.ID, Limit: -1}, {ID: rec.ID, FromMS: -1},
	} {
		if _, err := r.svc.RecordingEvents(r.as(agent), bad); code(err) != "E_VALIDATION" {
			t.Errorf("%+v: %v", bad, err)
		}
	}

	// a dismissed recording is invisible to an agent, events included
	if _, err := r.svc.RecordingDismiss(personCtx(), service.RecordingRef{ID: rec.ID}); err != nil {
		t.Fatal(err)
	}

	if _, err := r.svc.RecordingEvents(
		r.as(agent),
		service.RecordingEventsRequest{ID: rec.ID},
	); code(
		err,
	) != "E_RECORDING_NOT_FOUND" {
		t.Errorf("events of a dismissed recording for an agent: %v", err)
	}

	if _, err := r.svc.RecordingEvents(personCtx(), service.RecordingEventsRequest{ID: rec.ID}); err != nil {
		t.Errorf("a person reads them: %v", err)
	}
}

func TestRecordingWithoutPointerEventsAnswersNone(t *testing.T) {
	r := newRecEnv(t)
	project := r.newProject("p")
	rec := r.start(t, project)

	if _, err := r.svc.RecordingStop(personCtx(), service.RecordingRef{ID: rec.ID}); err != nil {
		t.Fatal(err)
	}

	r.waitState(t, rec.ID, "ready")

	res, err := r.svc.RecordingEvents(r.as(agent), service.RecordingEventsRequest{ID: rec.ID})
	if err != nil || res.Total != 0 || len(res.Events) != 0 || !strings.Contains(res.Text, "no pointer events") {
		t.Fatalf("%+v %v", res, err)
	}
}

// The privacy rule of the pointer file (SDD-025 section 16.6): whatever a file holds, a caller gets a pointer event
// and nothing else. A line about a key, or a member the format does not have, is dropped.
func TestSecurity_T125_NoKeyDataReachesACallerOfRecordingEvents(t *testing.T) {
	r := newRecEnv(t)
	r.cap.eventsBody = `{"k":"header","v":1,"width":100,"height":100,"clock":"active_ms"}
{"k":"key","t":10,"code":65,"text":"a"}
{"k":"down","t":20,"b":"left","x":5,"y":6,"key":"Enter"}
{"k":"move","t":30,"x":7,"y":8,"text":"password"}
{"k":"down","t":40,"b":"left","x":9,"y":9}
{"k":"up","t":90,"b":"left","x":9,"y":9}
`
	project := r.newProject("p")
	rec := r.startAll(t, project)

	if _, err := r.svc.RecordingStop(personCtx(), service.RecordingRef{ID: rec.ID}); err != nil {
		t.Fatal(err)
	}

	r.waitState(t, rec.ID, "ready")

	res, err := r.svc.RecordingEvents(r.as(agent), service.RecordingEventsRequest{ID: rec.ID, Kind: "all"})
	if err != nil {
		t.Fatal(err)
	}

	if res.Total != 2 || len(res.Events) != 2 {
		t.Fatalf("only the two clean lines may come back: %+v", res)
	}

	for _, e := range res.Events {
		if e.Kind != "down" && e.Kind != "up" {
			t.Errorf("event %+v", e)
		}
	}

	for _, banned := range []string{"key", "Enter", "password", "text", "code"} {
		if strings.Contains(res.Text, banned) {
			t.Errorf("the text mentions %q", banned)
		}
	}
}

// The helper installs no hook and reads no key (SDD-025 section 16.6): no source of the helper names a function
// that could. The same list is held by capture/tests/privacy.rs, which also pins the one call that asks about the
// three mouse buttons; this is the check of the Go test run.
func TestSecurity_T125_TheHelperSourceNamesNoFunctionThatReadsAKey(t *testing.T) {
	forbidden := []string{
		"SetWindowsHookEx", "GetKeyboardState", "GetKeyState", "GetKeyNameText", "RegisterRawInputDevices",
		"GetRawInputData", "RegisterHotKey", "MapVirtualKey", "ToUnicode", "keybd_event", "WH_KEYBOARD", "RIDEV_",
	}

	dir := filepath.Join("..", "..", "..", "capture", "src")

	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) < 10 {
		t.Fatalf("the helper source was not found: %v (%d files)", err, len(entries))
	}

	for _, e := range entries {
		body, rerr := os.ReadFile(filepath.Join(dir, e.Name()))
		if rerr != nil {
			t.Fatal(rerr)
		}

		for _, f := range forbidden {
			if strings.Contains(string(body), f) {
				t.Errorf("%s names %s: the helper must not read a key", e.Name(), f)
			}
		}
	}
}
