package export

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/audio"
)

const chainTrack = "trk_chain00001"

// chainDoc is the clip-caption scene with its sound muted and a noisy tone (a second of white noise, then a tone over
// it, SDD-004 section 7) on an audio track of its own.
func chainDoc(t *testing.T, mods func(d *timeline.Document)) *timeline.Document {
	t.Helper()

	doc := loadScene(t, "clip-caption")

	for id := range doc.Items {
		if it := doc.Items[id]; it.Type == timeline.ItemClip {
			it.AudioMuted = true
			doc.Items[id] = it
		}
	}

	const asset = "ast_chaintone1"

	in := ingestClip(t, fixtureSpec{Name: "noisy-tone-chain.wav", NoisyTone: true})
	doc.Assets[asset] = timeline.Asset{ID: asset, Kind: timeline.AssetAudio, Name: "noisy-tone-chain.wav"}
	useAsset(doc, asset, in)

	doc.Tracks[chainTrack] = timeline.Track{ID: chainTrack, Kind: timeline.TrackAudio, Order: "z9"}

	it := timeline.NewItem("itm_chain00001", timeline.ItemAudio, chainTrack, 0, 60)
	it.Asset = asset
	doc.Items[it.ID] = it

	if mods != nil {
		mods(doc)
	}

	return doc
}

func withItemChain(chain ...timeline.Processor) func(*timeline.Document) {
	return func(d *timeline.Document) {
		it := d.Items["itm_chain00001"]
		it.Processors = chain
		d.Items[it.ID] = it
	}
}

func readMix(t *testing.T, doc *timeline.Document, id string) []byte {
	t.Helper()

	job := newJob(t, doc, id, "unused.mp4")
	s := testState(t, job, id)

	path, _, err := s.mixAudio(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	return b
}

// The audio of an export is the audio of its preview: the mix that goes to the encoder and the file of an audio-only
// render of the same revision are the same bytes, with the processors of a clip and of a track in the chain
// (SDD-004 section 7.1).
func TestAudioChainExportEqualsPreview(t *testing.T) {
	doc := chainDoc(t, func(d *timeline.Document) {
		withItemChain(timeline.PresetChain("voice")...)(d)

		tr := d.Tracks[chainTrack]
		tr.Processors = []timeline.Processor{{Kind: timeline.ProcLimiter, Params: map[string]float64{"ceiling_db": -3}}}
		d.Tracks[chainTrack] = tr
	})

	exported := readMix(t, doc, "chain-export")

	job := newJob(t, doc, "chain-preview", "chain-preview.wav")
	job.Format = FormatWAV

	res, err := Run(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}

	preview, err := os.ReadFile(res.Path)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(exported, preview) {
		t.Fatalf("the preview render (%d bytes) is not the mix of the export (%d bytes)", len(preview), len(exported))
	}

	if !bytes.Equal(exported, readMix(t, doc, "chain-export-2")) {
		t.Error("two renders of the same revision differ")
	}

	// Without the chain the sound is not the same: the processors are in the render.
	if bytes.Equal(exported, readMix(t, chainDoc(t, nil), "chain-none")) {
		t.Error("the processors changed nothing")
	}
}

// A chain on a clip and a chain on a track both act, and the track chain sits on the sum of its items: a limiter on the
// track holds the sum under its ceiling, which no limiter on one item of two does.
func TestAudioChainClipAndTrack(t *testing.T) {
	// A second audio item on the same track, overlapping the first, so that the sum is louder than either.
	loud := func(d *timeline.Document) {
		second := timeline.NewItem("itm_chain00002", timeline.ItemAudio, "trk_chain00002", 0, 60)
		second.Asset = d.Items["itm_chain00001"].Asset
		d.Items[second.ID] = second
		d.Tracks["trk_chain00002"] = timeline.Track{ID: "trk_chain00002", Kind: timeline.TrackAudio, Order: "z8"}
	}

	onTrack := func(d *timeline.Document) {
		loud(d)

		tr := d.Tracks[chainTrack]
		tr.Processors = []timeline.Processor{
			{Kind: timeline.ProcLimiter, Params: map[string]float64{"ceiling_db": -30}},
		}
		d.Tracks[chainTrack] = tr
	}

	// The chain of a track acts on that track's sum only: the other track is not limited, and the measure with the
	// chain is not the measure without it.
	m, err := MeasureAudio(
		context.Background(),
		newJob(t, chainDoc(t, onTrack), "chain-track", "unused.mp4"),
		"",
		0,
		60,
	)
	if err != nil {
		t.Fatal(err)
	}

	if m.After.TruePeakDBTP == nil || m.Before.TruePeakDBTP == nil {
		t.Fatalf("no peak: %+v", m)
	}

	if *m.After.TruePeakDBTP >= *m.Before.TruePeakDBTP-1 {
		t.Errorf("a limiter at -30 dBFS on one of two tracks left the peak at %.2f from %.2f dBTP",
			*m.After.TruePeakDBTP, *m.Before.TruePeakDBTP)
	}

	// With the item measured alone, its track chain is in it and the other track's source is out.
	only, err := MeasureAudio(context.Background(), newJob(t, chainDoc(t, onTrack), "chain-track-item", "unused.mp4"),
		"itm_chain00001", 0, 60)
	if err != nil {
		t.Fatal(err)
	}

	if *only.After.TruePeakDBTP > -29 {
		t.Errorf("the item alone passes its track limiter: %.2f dBTP", *only.After.TruePeakDBTP)
	}

	// A gate on the clip changes the sound of the render, and nothing else does.
	gate := withItemChain(
		timeline.Processor{Kind: timeline.ProcGate, Params: map[string]float64{"threshold_db": -20, "range_db": -60}},
	)
	if bytes.Equal(readMix(t, chainDoc(t, gate), "chain-item-gate"), readMix(t, chainDoc(t, nil), "chain-item-plain")) {
		t.Error("a gate on the item changed nothing")
	}
}

// wavMeasure measures the 16-bit stereo WAV of an export with the meter of the engine.
func wavMeasure(wav []byte, from, to int) audio.Measurement {
	pcm := wav[44:]
	block := make([]float32, 0, (to-from)*2)

	for i := from; i < to; i++ {
		l := int16(uint16(pcm[i*4]) | uint16(pcm[i*4+1])<<8)
		r := int16(uint16(pcm[i*4+2]) | uint16(pcm[i*4+3])<<8)
		block = append(block, float32(l)/32768, float32(r)/32768)
	}

	m := audio.NewMeter()
	m.Write(block)

	return m.Result()
}

// The "voice" preset on the noisy fixture lowers the noise floor and the export still brings the loudness to the project
// target (SDD-004 section 7, epic 34 criterion 2): both numbers are of the file that the export writes.
func TestAudioChainVoicePresetKeepsTheTarget(t *testing.T) {
	plain := readMix(t, chainDoc(t, nil), "chain-voice-plain")
	voice := readMix(t, chainDoc(t, withItemChain(timeline.PresetChain("voice")...)), "chain-voice")
	n := 2 * audio.SampleRate // the item: a second of noise, then the tone over it

	const target = -14.0 // the loudness_target of a new project

	p, v := wavMeasure(plain, 0, n), wavMeasure(voice, 0, n)
	t.Logf("plain: %.2f LUFS, noise floor %.2f dBFS; voice: %.2f LUFS, noise floor %.2f dBFS",
		*p.IntegratedLUFS, p.NoiseFloorDBFS, *v.IntegratedLUFS, v.NoiseFloorDBFS)

	for name, m := range map[string]audio.Measurement{"plain": p, "voice": v} {
		if m.IntegratedLUFS == nil || *m.IntegratedLUFS < target-1 || *m.IntegratedLUFS > target+1 {
			t.Errorf("%s: loudness %v, want %.1f within 1 LU", name, m.IntegratedLUFS, target)
		}
	}

	if v.NoiseFloorDBFS > p.NoiseFloorDBFS-6 {
		t.Errorf(
			"the noise floor went from %.2f to %.2f dBFS, want at least 6 dB lower",
			p.NoiseFloorDBFS,
			v.NoiseFloorDBFS,
		)
	}
}

// A noise reduction with a marked profile reads that range of the asset through the decoder (the first second of the
// fixture is the noise alone) and cleans the whole clip with it.
func TestAudioChainMarkedProfile(t *testing.T) {
	marked := timeline.Processor{
		Kind:   timeline.ProcNoiseReduction,
		Params: map[string]float64{"strength": 0.6, "profile_start_ms": 100, "profile_end_ms": 900},
	}
	doc := chainDoc(t, withItemChain(marked))

	m, err := MeasureAudio(context.Background(), newJob(t, doc, "chain-marked", "unused.mp4"), "itm_chain00001", 0, 60)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("noise floor %.1f dBFS before, %.1f dBFS after", m.Before.NoiseFloorDBFS, m.After.NoiseFloorDBFS)

	if m.After.NoiseFloorDBFS > m.Before.NoiseFloorDBFS-8 {
		t.Errorf("the noise floor went from %.1f to %.1f dBFS, want at least 8 dB lower",
			m.Before.NoiseFloorDBFS, m.After.NoiseFloorDBFS)
	}
}

// AudioMeasure: the voice preset lowers the noise floor of the noisy tone, and the numbers come from the same mixer.
func TestAudioChainMeasure(t *testing.T) {
	doc := chainDoc(t, withItemChain(timeline.PresetChain("voice")...))
	job := newJob(t, doc, "chain-measure", "unused.mp4")

	m, err := MeasureAudio(context.Background(), job, "itm_chain00001", 0, 60)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("noise floor %.1f dBFS before, %.1f dBFS after; loudness %v before, %v after",
		m.Before.NoiseFloorDBFS, m.After.NoiseFloorDBFS, *m.Before.IntegratedLUFS, *m.After.IntegratedLUFS)

	if m.Seconds < 1.9 || m.Seconds > 2.1 {
		t.Errorf("measured %.2f s", m.Seconds)
	}

	if m.Before.IntegratedLUFS == nil || m.After.IntegratedLUFS == nil || m.Before.TruePeakDBTP == nil {
		t.Fatalf("no loudness: %+v %+v", m.Before, m.After)
	}

	if m.After.NoiseFloorDBFS >= m.Before.NoiseFloorDBFS-6 {
		t.Errorf("the noise floor went from %.1f to %.1f dBFS, want at least 6 dB lower",
			m.Before.NoiseFloorDBFS, m.After.NoiseFloorDBFS)
	}

	// The tone is the part that stays: the loudness of the whole falls by far less than the noise floor.
	if d := *m.Before.IntegratedLUFS - *m.After.IntegratedLUFS; d > 8 {
		t.Errorf("the loudness fell by %.1f LU", d)
	}

	// A range over 600 s is refused.
	long := chainDoc(t, nil)
	long.Settings.FPS = timeline.Rational{Num: 30, Den: 1}

	it := long.Items["itm_chain00001"]
	it.Duration = 30 * (MaxMeasureSeconds + 1)
	long.Items[it.ID] = it

	if _, err := MeasureAudio(
		context.Background(),
		newJob(t, long, "chain-measure-long", "unused.mp4"),
		"",
		0,
		0,
	); err == nil {
		t.Error("a range of 601 s was measured")
	}

	_ = audio.SampleRate
}
