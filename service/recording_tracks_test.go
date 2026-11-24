package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/recording"
	"github.com/AlexGalhardo/stuarty/engine/capturehost"
	"github.com/AlexGalhardo/stuarty/service/recordings"
)

func TestSettledTracksKeepsWhatTheHelperReported(t *testing.T) {
	dir := t.TempDir()
	fin := &capturehost.Finished{
		Tracks: []capturehost.TrackResult{
			{Kind: "screen", DurationMS: 1000, Bytes: 10},
			{Kind: "microphone", DurationMS: 990, Bytes: 5, Lost: true},
		},
		Pauses:        []capturehost.PauseSpan{{FromMS: 100, ToMS: 200}},
		PointerEvents: 7,
	}

	tracks, pauses, events := settledTracks(recordings.Row{}, dir, fin)
	got := decodeTracks(tracks)

	if len(got) != 2 || got[1].Kind != recording.TrackMicrophone || !got[1].Lost || got[1].File != "microphone.wav" {
		t.Errorf("tracks = %s", tracks)
	}

	if p := decodePauses(pauses); len(p) != 1 || p[0].ToMS != 200 || events != 7 {
		t.Errorf("pauses %s, events %d", pauses, events)
	}
}

// A message that does not hold together (an unknown kind, two screens, a path of its own) is not trusted: the
// tracks the request asked for, whose file is there, are kept instead.
func TestSettledTracksDistrustsAReportThatDoesNotHoldTogether(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "capture.mp4"), []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}

	row := recordings.Row{Tracks: `[{"kind":"screen","file":"capture.mp4"},{"kind":"camera","file":"camera.mp4"}]`}

	for name, fin := range map[string]*capturehost.Finished{
		"an unknown kind": {Tracks: []capturehost.TrackResult{{Kind: "screen"}, {Kind: "keyboard"}}},
		"no screen":       {Tracks: []capturehost.TrackResult{{Kind: "camera"}}},
		"two screens":     {Tracks: []capturehost.TrackResult{{Kind: "screen"}, {Kind: "screen"}}},
		"no report":       nil,
	} {
		tracks, _, _ := settledTracks(row, dir, fin)

		got := decodeTracks(tracks)
		if len(got) != 1 || got[0].Kind != recording.TrackScreen || got[0].Bytes != 10 {
			t.Errorf("%s: tracks = %s", name, tracks)
		}
	}
}

func TestSettledTracksCountsTheEventsOfAFileWithNoReport(t *testing.T) {
	dir := t.TempDir()
	body := `{"k":"header","v":1,"width":10,"height":10,"clock":"active_ms"}
{"k":"move","t":1,"x":1,"y":1}
{"k":"key","t":2}
{"k":"down","t":3,"b":"left","x":1,"y":1}
`

	if err := os.WriteFile(filepath.Join(dir, recording.EventsFile), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, _, n := settledTracks(recordings.Row{}, dir, nil); n != 2 {
		t.Errorf("events = %d, want the 2 that are pointer events", n)
	}

	if _, _, n := settledTracks(recordings.Row{}, t.TempDir(), nil); n != 0 {
		t.Errorf("a folder with no file has %d events", n)
	}
}
