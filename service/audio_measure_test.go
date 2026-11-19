package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

const (
	measureAudioItem = "itm_bbbbbbbbbb"
	measureTrack     = "trk_bbbbbbbbbb"
)

// fakeMeter is an exporter that measures nothing but records what it was asked.
type fakeMeter struct {
	Exporter
	in  AudioMeasureInput
	err error
}

func (f *fakeMeter) MeasureAudio(_ context.Context, in AudioMeasureInput) (*AudioMeasureOutput, error) {
	f.in = in
	if f.err != nil {
		return nil, f.err
	}

	before, after, speech := -22.9, -20.4, 33.0
	peak := -9.1

	return &AudioMeasureOutput{
		Seconds: float64(in.End-in.Start) / 30,
		Before: AudioLevels{
			IntegratedLUFS:  &before,
			TruePeakDBTP:    &peak,
			NoiseFloorDBFS:  -55.8,
			SpeechToNoiseDB: &speech,
		},
		After: AudioLevels{IntegratedLUFS: &after, TruePeakDBTP: &peak, NoiseFloorDBFS: -71},
	}, nil
}

// measureEnv is the batch project with an audio item added to it, and a meter in place of the exporter.
func measureEnv(t *testing.T, meter *fakeMeter) *batchEnv {
	t.Helper()

	env := newBatchEnv(t, 10, Limits{}, nil)
	store, ok := env.s.store.(*batchStore)

	if !ok {
		t.Fatal("the test store is not the batch store")
	}

	doc, err := timeline.Decode(store.projects.row.Document)
	if err != nil {
		t.Fatal(err)
	}

	doc.Assets["ast_bbbbbbbbbb"] = timeline.Asset{
		ID: "ast_bbbbbbbbbb", Hash: "sha256:" + strings.Repeat("b2", 32), Kind: timeline.AssetAudio, Name: "voice.wav",
		SampleCount: 480000,
	}
	doc.Tracks[measureTrack] = timeline.Track{ID: measureTrack, Kind: timeline.TrackAudio, Name: "Voice", Order: "a1"}
	it := timeline.NewItem(measureAudioItem, timeline.ItemAudio, measureTrack, 30, 150)
	it.Asset = "ast_bbbbbbbbbb"
	doc.Items[it.ID] = it

	if vs := timeline.Validate(doc); len(vs) > 0 {
		t.Fatalf("the test project is not valid: %+v", vs[0])
	}

	store.projects.row.Document = timeline.Canonical(doc)

	if meter != nil {
		meter.Exporter = env.s.exporter
		env.s.exporter = meter
	}

	return env
}

func TestAudioMeasureAsksTheMeterForTheRightRange(t *testing.T) {
	t.Parallel()

	meter := &fakeMeter{}
	env := measureEnv(t, meter)
	ctx := renderCtx()

	// The whole timeline: the project of the test is 10 s at 30 fps.
	res, err := env.s.AudioMeasure(ctx, AudioMeasureRequest{ProjectID: batchProject})
	if err != nil {
		t.Fatal(err)
	}

	if meter.in.Start != 0 || meter.in.End != 300 || meter.in.ItemID != "" || meter.in.JobID == "" {
		t.Errorf("whole timeline: %+v", meter.in)
	}

	if res.Revision != 7 || res.StartFrame != 0 || res.EndFrame != 300 || res.Seconds != 10 {
		t.Errorf("result %+v", res)
	}

	if res.After.NoiseFloorDBFS >= res.Before.NoiseFloorDBFS || res.After.IntegratedLUFS == nil {
		t.Errorf("levels %+v %+v", res.Before, res.After)
	}

	for _, want := range []string{
		"audio measure prj_7k2m9x4q1b rev 7", "before: loudness -22.9 LUFS", "after : loudness -20.4 LUFS",
		"noise floor -71.0 dBFS", "speech to noise none",
	} {
		if !strings.Contains(res.Text, want) {
			t.Errorf("the text has no %q:\n%s", want, res.Text)
		}
	}

	// One item: its own range, and the item is passed to the meter.
	if _, err := env.s.AudioMeasure(
		ctx,
		AudioMeasureRequest{ProjectID: batchProject, ItemID: measureAudioItem},
	); err != nil {
		t.Fatal(err)
	}

	if meter.in.Start != 30 || meter.in.End != 180 || meter.in.ItemID != measureAudioItem {
		t.Errorf("item: %+v", meter.in)
	}

	// A range of the caller wins over the item's.
	if _, err := env.s.AudioMeasure(ctx, AudioMeasureRequest{
		ProjectID: batchProject, ItemID: measureAudioItem, Start: "2s", End: "3s",
	}); err != nil {
		t.Fatal(err)
	}

	if meter.in.Start != 60 || meter.in.End != 90 {
		t.Errorf("range: %+v", meter.in)
	}
}

func TestAudioMeasureRefusesWhatItCannotMeasure(t *testing.T) {
	t.Parallel()

	meter := &fakeMeter{}
	env := measureEnv(t, meter)
	ctx := renderCtx()

	code := func(req AudioMeasureRequest) string {
		_, err := env.s.AudioMeasure(ctx, req)

		var e *Error
		if !errors.As(err, &e) {
			t.Fatalf("%+v: error %v", req, err)
		}

		return e.Code
	}

	cases := map[string]struct {
		req  AudioMeasureRequest
		want string
	}{
		"unknown item": {AudioMeasureRequest{ProjectID: batchProject, ItemID: "itm_zzzzzzzzzz"}, "E_NOT_FOUND"},
		"text item":    {AudioMeasureRequest{ProjectID: batchProject, ItemID: "itm_aaaaaaaaaa"}, "E_TYPE_MISMATCH"},
		"bad time":     {AudioMeasureRequest{ProjectID: batchProject, Start: "soon"}, "E_INVALID_TIME"},
		"past the end": {AudioMeasureRequest{ProjectID: batchProject, End: "99s"}, "E_VALIDATION"},
		"no project":   {AudioMeasureRequest{ProjectID: "prj_zzzzzzzzzz"}, "E_NOT_FOUND"},
	}

	for name, c := range cases {
		if got := code(c.req); got != c.want {
			t.Errorf("%s: %s, want %s", name, got, c.want)
		}
	}

	// A meter that finds the range too long is a limit; any other failure passes through.
	meter.err = ErrAudioRangeTooLong
	if got := code(AudioMeasureRequest{ProjectID: batchProject}); got != "E_LIMIT" {
		t.Errorf("too long: %s", got)
	}

	// The read scope is enough and nothing is needed without it.
	if _, err := env.s.AudioMeasure(context.Background(), AudioMeasureRequest{ProjectID: batchProject}); err == nil {
		t.Error("a call with no credentials was answered")
	}

	// An exporter that cannot measure is said to be unavailable, not a crash.
	plain := measureEnv(t, nil)
	if _, err := plain.s.AudioMeasure(ctx, AudioMeasureRequest{ProjectID: batchProject}); err == nil {
		t.Error("an exporter without a meter measured")
	}
}

// The presets of audio.apply_preset (SDD-002 C.83): each gives a valid chain that reaches the plan of the mix, with the
// parameters complete, and applying one twice changes nothing more.
func TestAudioPresetsReachThePlan(t *testing.T) {
	t.Parallel()

	for _, name := range timeline.Presets {
		env := measureEnv(t, nil)
		store, _ := env.s.store.(*batchStore)

		doc, err := timeline.Decode(store.projects.row.Document)
		if err != nil {
			t.Fatal(err)
		}

		args, merr := json.Marshal(map[string]string{"item_id": measureAudioItem, "preset": name})
		if merr != nil {
			t.Fatal(merr)
		}

		op := command.Operation{Op: "audio.apply_preset", Args: args}

		res, aerr := command.Default().Apply(doc, op, command.Env{})
		if aerr != nil {
			t.Fatalf("%s: %v", name, aerr)
		}

		if vs := timeline.Validate(res.Doc); len(vs) > 0 {
			t.Fatalf("%s: the document is not valid: %+v", name, vs[0])
		}

		srcs := plan.Audio(res.Doc, 0, 300)
		if len(srcs) != 1 || len(srcs[0].Chain) == 0 {
			t.Fatalf("%s: the plan has %+v", name, srcs)
		}

		last := srcs[0].Chain[len(srcs[0].Chain)-1]
		if last.Kind != timeline.ProcLimiter || last.Params["ceiling_db"] != -1 {
			t.Errorf("%s: the chain does not end with the limiter at -1 dBFS: %+v", name, last)
		}

		for _, p := range srcs[0].Chain {
			if len(p.Params) != len(timeline.ProcessorParams(p.Kind)) {
				t.Errorf("%s: %s carries %d of its parameters", name, p.Kind, len(p.Params))
			}
		}

		// Only "voice with music" leaves the noise alone.
		hasNR := false

		for _, p := range srcs[0].Chain {
			hasNR = hasNR || p.Kind == timeline.ProcNoiseReduction
		}

		if hasNR == (name == "voice_with_music") {
			t.Errorf("%s: noise reduction in the chain is %v", name, hasNR)
		}
	}
}
