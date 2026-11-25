package recording

import (
	"errors"
	"testing"
)

func screenTrack() Track {
	return Track{Kind: TrackScreen, File: "capture.mp4", DurationMS: 1000}
}

func TestTrackFilesAndLabels(t *testing.T) {
	files := map[TrackKind]string{
		TrackScreen: "capture.mp4", TrackCamera: "camera.mp4",
		TrackMicrophone: "microphone.wav", TrackSystem: "system.wav",
	}

	for _, k := range TrackKinds {
		if !k.Valid() || k.File() != files[k] || k.Label() == "" {
			t.Errorf("%s: valid %v, file %q, label %q", k, k.Valid(), k.File(), k.Label())
		}

		if k.Audio() != (k == TrackMicrophone || k == TrackSystem) {
			t.Errorf("%s: Audio() = %v", k, k.Audio())
		}
	}

	if TrackKind("keyboard").Valid() || TrackKind("keyboard").File() != "" {
		t.Error("an unknown kind was accepted")
	}
}

func TestValidateTracks(t *testing.T) {
	good := []Track{
		screenTrack(),
		{Kind: TrackMicrophone, File: "microphone.wav", DurationMS: 990},
		{Kind: TrackSystem, File: "system.wav", StartMS: 5, DurationMS: 1000},
		{Kind: TrackCamera, File: "camera.mp4", DurationMS: 1010},
	}
	if err := ValidateTracks(good); err != nil {
		t.Fatalf("a valid list was refused: %v", err)
	}

	cases := map[string][]Track{
		"no screen":      {{Kind: TrackMicrophone, File: "microphone.wav"}},
		"empty":          nil,
		"unknown kind":   {screenTrack(), {Kind: "keys", File: "keys.log"}},
		"twice":          {screenTrack(), screenTrack()},
		"another file":   {{Kind: TrackScreen, File: "../capture.mp4"}},
		"negative start": {{Kind: TrackScreen, File: "capture.mp4", StartMS: -1}},
		"negative bytes": {{Kind: TrackScreen, File: "capture.mp4", Bytes: -1}},
	}

	for name, tracks := range cases {
		if err := ValidateTracks(tracks); !errors.Is(err, ErrTracks) {
			t.Errorf("%s: %v; want ErrTracks", name, err)
		}
	}
}

func TestPauses(t *testing.T) {
	pauses := []Pause{{FromMS: 1000, ToMS: 1500}, {FromMS: 3000, ToMS: 4000}}
	if err := ValidatePauses(pauses); err != nil {
		t.Fatal(err)
	}

	for name, bad := range map[string][]Pause{
		"empty":        {{FromMS: 5, ToMS: 5}},
		"backwards":    {{FromMS: 9, ToMS: 5}},
		"overlap":      {{FromMS: 0, ToMS: 10}, {FromMS: 9, ToMS: 20}},
		"out of order": {{FromMS: 100, ToMS: 200}, {FromMS: 0, ToMS: 50}},
	} {
		if err := ValidatePauses(bad); !errors.Is(err, ErrTracks) {
			t.Errorf("%s: %v", name, err)
		}
	}

	if got := PausedMS(pauses); got != 1500 {
		t.Errorf("PausedMS = %d", got)
	}

	for _, c := range []struct {
		wall   int64
		active int64
		ok     bool
	}{
		{0, 0, true},
		{999, 999, true},
		{1000, 0, false},
		{1499, 0, false},
		{1500, 1000, true},
		{2999, 2499, true},
		{3000, 0, false},
		{4000, 2500, true},
		{5000, 3500, true},
	} {
		got, ok := ActiveMS(c.wall, pauses)
		if ok != c.ok || (ok && got != c.active) {
			t.Errorf("ActiveMS(%d) = %d, %v; want %d, %v", c.wall, got, ok, c.active, c.ok)
		}
	}
}

func TestDriftIgnoresALostTrack(t *testing.T) {
	tracks := []Track{
		{Kind: TrackScreen, File: "capture.mp4", DurationMS: 60000},
		{Kind: TrackMicrophone, File: "microphone.wav", DurationMS: 60030},
		{Kind: TrackSystem, File: "system.wav", DurationMS: 12000, Lost: true},
	}

	if got := Drift(tracks); got != 30 || got > MaxDriftMS {
		t.Errorf("Drift = %d; want 30", got)
	}

	if Drift(tracks[:1]) != 0 || Drift(nil) != 0 {
		t.Error("a single track has no drift")
	}

	tracks[1].StartMS, tracks[1].DurationMS = 100, 59990
	if got := Drift(tracks); got != 90 {
		t.Errorf("Drift with a start offset = %d; want 90", got)
	}
}

func TestPlaceStartsAreRelativeToTheEarliest(t *testing.T) {
	got := PlaceStarts([]Track{
		{Kind: TrackScreen, StartMS: 120}, {Kind: TrackMicrophone, StartMS: 100}, {Kind: TrackCamera, StartMS: 130},
	})
	if got[TrackScreen] != 20 || got[TrackMicrophone] != 0 || got[TrackCamera] != 30 {
		t.Errorf("PlaceStarts = %v", got)
	}

	if len(PlaceStarts(nil)) != 0 {
		t.Error("no tracks, no starts")
	}
}

func TestTracksAskedAndDeviceValidation(t *testing.T) {
	spec := StartSpec{
		ProjectID: "prj_0123456789", SourceKind: SourceDisplay, SourceID: "d0",
		Microphone: DefaultMicrophone, SystemAudio: true, Camera: "c0123456789abcdef", Events: true,
	}
	if f := ValidateStart(spec); len(f) != 0 {
		t.Fatalf("a valid request was refused: %v", f)
	}

	got := TracksAsked(spec)
	want := []TrackKind{TrackScreen, TrackCamera, TrackMicrophone, TrackSystem}

	if len(got) != len(want) {
		t.Fatalf("TracksAsked = %v", got)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("TracksAsked = %v; want %v", got, want)
		}
	}

	spec.Microphone, spec.Camera, spec.SystemAudio = "", "", false
	if got := TracksAsked(spec); len(got) != 1 || got[0] != TrackScreen {
		t.Errorf("a screen only request asks for %v", got)
	}

	spec.Microphone, spec.Camera = "{0.0.1.00000000}", "a b"

	fields := map[string]bool{}
	for _, f := range ValidateStart(spec) {
		fields[f.Field] = true
	}

	if !fields["microphone"] || !fields["camera"] {
		t.Errorf("a device id with braces or a space was accepted: %v", fields)
	}
}

func TestTrackSoundKindDefaultsOnlyForTheMicrophone(t *testing.T) {
	t.Parallel()

	for _, k := range TrackKinds {
		want := ""
		if k == TrackMicrophone {
			want = "voice"
		}

		if got := k.SoundKind(); got != want {
			t.Errorf("%s: SoundKind() = %q, want %q", k, got, want)
		}
	}
}
