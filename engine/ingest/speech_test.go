package ingest

import (
	"context"
	"encoding/json"
	"testing"
)

func TestSpeechFromSilenceIsTheInverse(t *testing.T) {
	t.Parallel()

	s := &Silence{SampleRate: SampleRate, SampleCount: 100000, Sets: []SilenceSet{
		{ThresholdDB: -30},
		{ThresholdDB: -35, Ranges: []SilenceRange{{0, 5000}, {20000, 30000}, {90000, 100000}}},
	}}

	got, err := SpeechFromSilence(s, -35)
	if err != nil {
		t.Fatal(err)
	}

	want := []SpeechRange{{5000, 20000}, {30000, 90000}}
	if len(got.Ranges) != 2 || got.Ranges[0] != want[0] || got.Ranges[1] != want[1] {
		t.Errorf("ranges %v, want %v", got.Ranges, want)
	}

	if got.Source != SpeechFromEnvelope || got.ThresholdDB != -35 || got.SampleCount != 100000 {
		t.Errorf("speech %+v", got)
	}

	if all, _ := SpeechFromSilence(s, -30); len(all.Ranges) != 1 || all.Ranges[0] != (SpeechRange{0, 100000}) {
		t.Errorf("no silence: %v", all.Ranges)
	}

	if _, err := SpeechFromSilence(s, -40); err == nil {
		t.Error("a threshold the artifact does not have was accepted")
	}

	if _, err := SpeechFromSilence(nil, -35); err == nil {
		t.Error("no silence artifact was accepted")
	}
}

// The speech fixture has pauses of 0.5 s, 1.0 s and 0.2 s between four 1 s tones: the map of the
// envelope is the inverse of the -35 dB silences within one 20 ms window (P3-RND-005).
func TestSpeechMatchesSilenceInverse(t *testing.T) {
	e := newEnv(t)
	e.soundMaps = true
	p, err := e.gen.Speech3Pauses(context.Background())
	hash, res := e.mustIngest(mustGen(t, p, err))

	var s Silence
	if err := json.Unmarshal(readJSONFile(t, e.publishedPath(hash, res.find("silence", ""))), &s); err != nil {
		t.Fatal(err)
	}

	var sp Speech
	if err := json.Unmarshal(readJSONFile(t, e.publishedPath(hash, res.find("speech", ""))), &sp); err != nil {
		t.Fatal(err)
	}

	if sp.Source != SpeechFromEnvelope || len(sp.Ranges) != 4 {
		t.Fatalf("speech %+v, want 4 ranges from the envelope", sp)
	}

	// The tones last 1 s, the pauses 0.5, 1.0 and 0.2 s: speech at [0,1], [1.5,2.5], [3.5,4.5], [4.7,5.7].
	const window = SampleRate / 50

	set := s.Sets[1].Ranges
	for i, r := range sp.Ranges {
		if i > 0 && abs64(r.Start-set[i-1].End) > window {
			t.Errorf("speech %d starts at %d, the silence before it ends at %d", i, r.Start, set[i-1].End)
		}

		if i < len(set) && abs64(r.End-set[i].Start) > window {
			t.Errorf("speech %d ends at %d, the silence after it starts at %d", i, r.End, set[i].Start)
		}
	}

	if sp.Ranges[0].Start != 0 || sp.Ranges[3].End != sp.SampleCount {
		t.Errorf("the map does not reach the ends: %v of %d", sp.Ranges, sp.SampleCount)
	}
}

// A 120 BPM click track ingested through Run has a beat grid artifact, at least 90% of the beats
// within 40 ms of the true ones (P3-RND-005).
func TestBeatsArtifactOnClickTrack(t *testing.T) {
	e := newEnv(t)
	e.soundMaps = true
	p, err := e.gen.Click120(context.Background())
	hash, res := e.mustIngest(mustGen(t, p, err))

	var b Beats
	if err := json.Unmarshal(readJSONFile(t, e.publishedPath(hash, res.find("beats", ""))), &b); err != nil {
		t.Fatal(err)
	}

	var truth []int64
	for at := int64(SampleRate / 4); at < 19*SampleRate; at += SampleRate / 2 {
		truth = append(truth, at)
	}

	rate := hitRate(b.Beats, truth, 40)
	t.Logf("tempo %.1f, %d beats for %d, hit rate %.3f", b.TempoBPM, len(b.Beats), len(truth), rate)

	if rate < 0.9 || b.TempoBPM < 118 || b.TempoBPM > 122 {
		t.Errorf("tempo %.1f, hit rate %.3f", b.TempoBPM, rate)
	}
}
