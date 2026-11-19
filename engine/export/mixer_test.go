package export

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/audio"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg/ffargs"
)

const mixSamples = 24000 // 0.5 s

// synth is a decoded stream of n stereo samples made with integer arithmetic only: the same bytes on
// every machine.
func synth(seed, n int) []byte {
	b := make([]byte, n*4)

	for i := range n {
		l := ((i*(37+seed*11))%2000 - 1000) * (20 + seed*5)
		r := ((i*(53+seed*7)+seed*300)%2400 - 1200) * (20 + seed*5)
		binary.LittleEndian.PutUint16(b[i*4:], uint16(int16(l)))
		binary.LittleEndian.PutUint16(b[i*4+2:], uint16(int16(r)))
	}

	return b
}

func synthOpener(seeds map[string]int) audio.Opener {
	return audio.OpenerFunc(func(_ context.Context, src plan.AudioSource) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(synth(seeds[src.AssetID], mixSamples+int(src.SourceStart)))), nil
	})
}

func gp(pairs ...float64) []plan.GainPoint {
	var out []plan.GainPoint

	for i := 0; i < len(pairs); i += 2 {
		out = append(out, plan.GainPoint{Sample: int64(pairs[i]), Gain: pairs[i+1]})
	}

	return out
}

func src(item, asset string, start, from, n int64, g []plan.GainPoint) plan.AudioSource {
	return plan.AudioSource{
		ItemID: item, AssetID: asset, StartSample: start, SourceStart: from, Samples: n,
		Speed: timeline.Rational{Num: 1, Den: 1}, Gain: g,
	}
}

// mixCases are the mixes without a speed change of SDD-004 section 11.
func mixCases() map[string][]plan.AudioSource {
	return map[string][]plan.AudioSource{
		"volume-keyframes": {src("a", "a", 0, 0, mixSamples, gp(0, 1, 12000, 0.25, 24000, 1))},
		"fade-in":          {src("a", "a", 0, 0, mixSamples, gp(0, 0, 6000, 1))},
		"fade-out":         {src("a", "a", 0, 0, mixSamples, gp(0, 1, 18000, 1, 24000, 0))},
		"two-overlapping": {
			src("a", "a", 0, 0, 20000, gp(0, 0.6)),
			src("b", "b", 8000, 0, 16000, gp(0, 0.8)),
		},
		"audio-item-under-clip": {
			src("music", "b", 0, 0, mixSamples, gp(0, 0.3)),
			src("clip", "a", 4800, 4800, 14400, gp(0, 1, 4800, 1, 14400, 0)),
		},
	}
}

func mixWAV(t *testing.T, srcs []plan.AudioSource) []byte {
	t.Helper()

	var raw, out bytes.Buffer

	opener := synthOpener(map[string]int{"a": 1, "b": 2})
	if err := audio.MixToFloatWAV(context.Background(), srcs, mixSamples, opener, &raw); err != nil {
		t.Fatal(err)
	}

	if err := audio.GainToPCM16WAV(&raw, &out, mixSamples, 0); err != nil {
		t.Fatal(err)
	}

	return out.Bytes()
}

// The mixes without speed match their reference files sample for sample. Regenerate with -update.
func TestMixer(t *testing.T) {
	for name, srcs := range mixCases() {
		t.Run(name, func(t *testing.T) {
			got := mixWAV(t, srcs)
			path := filepath.Join("testdata", "audio", name+".wav")

			if *update {
				if err := os.WriteFile(path, got, 0o600); err != nil {
					t.Fatal(err)
				}
			}

			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}

			if !bytes.Equal(got, want) {
				t.Errorf("the mix differs from %s", path)
			}
		})
	}
}

// TestAtempoChain: a speed outside 0.5 to 2 is chained in stages of at most 2 (SDD-004 section 7).
func TestAtempoChain(t *testing.T) {
	t.Parallel()

	cases := map[float64]string{
		0.25: "atempo=0.500000,atempo=0.500000",
		0.5:  "atempo=0.500000",
		1.5:  "atempo=1.500000",
		2:    "atempo=2.000000",
		4:    "atempo=2.000000,atempo=2.000000",
	}

	for tempo, want := range cases {
		var parts []string
		for _, f := range tempoChain(tempo) {
			parts = append(parts, string(f))
		}

		if got := strings.Join(parts, ","); got != want {
			t.Errorf("tempo %v: %s, want %s", tempo, got, want)
		}
	}

	if got := tempoChain(1); len(got) != 0 {
		t.Errorf("tempo 1 has stages %v", got)
	}
}

// windowRMS is the RMS in dB of each 100 ms window of the left channel of a 16-bit stereo WAV.
func windowRMS(wav []byte) []float64 {
	pcm := wav[44:]

	const win = audio.SampleRate / 10

	var out []float64

	for i := 0; (i+win)*4 <= len(pcm); i += win {
		var sum float64

		for j := i; j < i+win; j++ {
			v := float64(int16(binary.LittleEndian.Uint16(pcm[j*4:]))) / 32768
			sum += v * v
		}

		out = append(out, 10*math.Log10(sum/win+1e-12))
	}

	return out
}

// floatWAV wraps s16le samples as the float WAV that GainToPCM16WAV reads.
func floatWAV(t *testing.T, pcm []byte) io.Reader {
	t.Helper()

	var buf bytes.Buffer

	n := int64(len(pcm) / 4)
	op := audio.OpenerFunc(func(context.Context, plan.AudioSource) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(pcm)), nil
	})

	srcs := []plan.AudioSource{src("i", "x", 0, 0, n, gp(0, 1))}
	if err := audio.MixToFloatWAV(context.Background(), srcs, n, op, &buf); err != nil {
		t.Fatal(err)
	}

	return &buf
}

// The decoder with atempo (speeds 0.5, 2 and 4) is within 1 dB RMS per 100 ms window of its reference.
func TestMixerSpeedWithinOneDB(t *testing.T) {
	doc := loadScene(t, "clip-caption")
	job := newJob(t, doc, "mixer-speed", "unused.mp4")

	var asset string

	for id, a := range doc.Assets {
		if a.HasAudio {
			asset = id
		}
	}

	if asset == "" {
		t.Fatal("the scene has no audio asset")
	}

	for _, c := range []struct {
		name     string
		num, den int
	}{{"speed-half", 1, 2}, {"speed-2", 2, 1}, {"speed-4", 4, 1}} {
		t.Run(c.name, func(t *testing.T) {
			s := testState(t, job, "mixer-"+c.name)
			n := int64(mixSamples)

			rc, err := (&audioOpener{s: s}).Open(context.Background(), plan.AudioSource{
				ItemID: "i", AssetID: asset, Samples: n, Gain: gp(0, 1),
				Speed: timeline.Rational{Num: c.num, Den: c.den},
			})
			if err != nil {
				t.Fatal(err)
			}

			pcm, err := io.ReadAll(io.LimitReader(rc, n*4))
			_ = rc.Close()

			if err != nil {
				t.Fatal(err)
			}

			if int64(len(pcm)) != n*4 {
				t.Fatalf("%d samples decoded, want %d", len(pcm)/4, n)
			}

			var wav bytes.Buffer
			if err := audio.GainToPCM16WAV(floatWAV(t, pcm), &wav, n, 0); err != nil {
				t.Fatal(err)
			}

			path := filepath.Join("testdata", "audio", c.name+".wav")
			if *update {
				if err := os.WriteFile(path, wav.Bytes(), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			ref, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}

			got, want := windowRMS(wav.Bytes()), windowRMS(ref)
			if len(got) != len(want) || len(got) == 0 {
				t.Fatalf("%d windows against %d", len(got), len(want))
			}

			for i := range got {
				if d := math.Abs(got[i] - want[i]); d > 1 {
					t.Errorf("window %d: %.2f dB against %.2f dB", i, got[i], want[i])
				}
			}
		})
	}
}

type measured struct {
	lufs, peak *float64
	gain       float64
}

// normalize mixes bursts of tone with pauses (the level pattern of speech) of the given length, measures
// the mix, applies the gain and measures the result.
func normalize(t *testing.T, job Job, name string, seconds float64) (before, after measured) {
	t.Helper()

	ctx := context.Background()
	s := testState(t, job, "loud-"+name)
	n := int64(seconds * audio.SampleRate)

	op := audio.OpenerFunc(func(context.Context, plan.AudioSource) (io.ReadCloser, error) {
		b := synth(3, int(n))

		for i := range int(n) {
			if (i/9600)%3 == 2 {
				binary.LittleEndian.PutUint32(b[i*4:], 0)
			}
		}

		return io.NopCloser(bytes.NewReader(b)), nil
	})

	raw, err := s.dir.Path("raw.wav")
	if err != nil {
		t.Fatal(err)
	}

	srcs := []plan.AudioSource{src("i", "x", 0, 0, n, gp(0, 1))}
	if err := writeFile(raw, func(w io.Writer) error { return audio.MixToFloatWAV(ctx, srcs, n, op, w) }); err != nil {
		t.Fatal(err)
	}

	first, _, err := s.measure(ctx, raw, n)
	if err != nil {
		t.Fatal(err)
	}

	gain := audio.GainDB(first.IntegratedLUFS, first.TruePeakDBTP, n, -14)

	in, err := os.Open(raw)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = in.Close() }()

	out, err := s.dir.Path("out.wav")
	if err != nil {
		t.Fatal(err)
	}

	if err := writeFile(out, func(w io.Writer) error { return audio.GainToPCM16WAV(in, w, n, gain) }); err != nil {
		t.Fatal(err)
	}

	second, _, err := s.measure(ctx, out, n)
	if err != nil {
		t.Fatal(err)
	}

	before = measured{first.IntegratedLUFS, first.TruePeakDBTP, gain}
	after = measured{second.IntegratedLUFS, second.TruePeakDBTP, gain}

	return before, after
}

// A speech-like mix is normalized to within 1.0 LU of the target with the true peak at most -1.5 dBTP;
// a mix of 0.3 s gets no gain.
func TestLoudnessTarget(t *testing.T) {
	doc := loadScene(t, "contain-text")
	job := newJob(t, doc, "loudness-target", "unused.mp4")

	t.Run("speech", func(t *testing.T) {
		before, after := normalize(t, job, "speech", 6)
		if before.lufs == nil || after.lufs == nil || after.peak == nil {
			t.Fatal("a mix was measured as silent")
		}

		t.Logf("before %.2f LUFS %.2f dBTP, gain %.2f dB; after %.2f LUFS %.2f dBTP",
			*before.lufs, *before.peak, before.gain, *after.lufs, *after.peak)

		if math.Abs(*after.lufs+14) > 1 {
			t.Errorf("%.2f LUFS after normalization, want -14 within 1.0 LU", *after.lufs)
		}

		if *after.peak > -1.5 {
			t.Errorf("true peak %.2f dBTP, want at most -1.5", *after.peak)
		}
	})

	t.Run("limited", func(t *testing.T) {
		ctx := context.Background()
		s := testState(t, job, "loud-limited")
		const n = 6 * audio.SampleRate

		// Quiet noise-like bursts with a loud click each half second: the peak leaves no room for the
		// gain that the target needs, so the limiter takes the peaks.
		op := audio.OpenerFunc(func(context.Context, plan.AudioSource) (io.ReadCloser, error) {
			b := synth(1, n)
			for i := range n {
				v := int16(binary.LittleEndian.Uint16(b[i*4:])) / 4
				if (i/9600)%3 == 2 {
					v = 0
				}
				if i%24000 == 5000 {
					v = 30000
				}
				binary.LittleEndian.PutUint16(b[i*4:], uint16(v))
				binary.LittleEndian.PutUint16(b[i*4+2:], uint16(v))
			}
			return io.NopCloser(bytes.NewReader(b)), nil
		})
		raw, err := s.dir.Path("raw.wav")
		if err != nil {
			t.Fatal(err)
		}
		srcs := []plan.AudioSource{src("i", "x", 0, 0, n, gp(0, 1))}
		if err := writeFile(
			raw,
			func(w io.Writer) error { return audio.MixToFloatWAV(ctx, srcs, n, op, w) },
		); err != nil {
			t.Fatal(err)
		}
		first, _, err := s.measure(ctx, raw, n)
		if err != nil {
			t.Fatal(err)
		}
		loud := Loudness{
			IntegratedLUFS: first.IntegratedLUFS, TruePeakDBTP: first.TruePeakDBTP, TargetLUFS: -14,
			GainDB: audio.GainDB(first.IntegratedLUFS, first.TruePeakDBTP, n, -14),
		}
		out, err := s.dir.Path("out.wav")
		if err != nil {
			t.Fatal(err)
		}
		if err := s.writeGain(ctx, raw, out, n, &loud); err != nil {
			t.Fatal(err)
		}
		after, _, err := s.measure(ctx, out, n)
		if err != nil {
			t.Fatal(err)
		}

		t.Logf(
			"before %.2f LUFS %.2f dBTP; gain %.2f dB; after %.2f LUFS %.2f dBTP; shortfall %.2f dB",
			*first.IntegratedLUFS,
			*first.TruePeakDBTP,
			loud.GainDB,
			*after.IntegratedLUFS,
			*after.TruePeakDBTP,
			loud.ShortfallDB,
		)

		if !loud.Limited {
			t.Fatal("the mix was not limited, the test does not cover the limiter")
		}
		if *after.TruePeakDBTP > -1.5 {
			t.Errorf("true peak %.2f dBTP, want at most -1.5", *after.TruePeakDBTP)
		}
		if got := *after.IntegratedLUFS; math.Abs(got+14) > 1 && loud.ShortfallDB < 1 {
			t.Errorf(
				"%.2f LUFS with a shortfall of %.2f dB, want -14 within 1.0 LU or the shortfall reported",
				got,
				loud.ShortfallDB,
			)
		}
		if loud.OutputLUFS == nil || math.Abs(*loud.OutputLUFS-*after.IntegratedLUFS) > 0.3 {
			t.Errorf("output loudness %v, measured %.2f", loud.OutputLUFS, *after.IntegratedLUFS)
		}
	})

	t.Run("short", func(t *testing.T) {
		if before, _ := normalize(t, job, "short", 0.3); before.gain != 0 {
			t.Errorf("gain %.2f dB for a 0.3 s mix, want 0", before.gain)
		}
	})
}

// A project with no audio source exports a file with an AAC stream, and its loudness says silent.
func TestNoAudioExportsSilentTrack(t *testing.T) {
	doc := loadScene(t, "contain-text")

	for id, a := range doc.Assets {
		a.HasAudio = false
		doc.Assets[id] = a
	}

	job := newJob(t, doc, "no-audio", filepath.Join("noaudio", "out.mp4"))

	res, err := Run(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(res.Path) }()

	if res.Loudness.IntegratedLUFS != nil || res.Loudness.GainDB != 0 {
		t.Errorf("loudness %+v, want a silent mix", res.Loudness)
	}

	dir, err := shared.runner.NewRunDir("noaudio-probe")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dir.Remove() }()

	body, err := os.ReadFile(res.Path)
	if err != nil {
		t.Fatal(err)
	}

	probed, err := dir.Path("probe.mp4")
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(probed, body, 0o600); err != nil {
		t.Fatal(err)
	}

	argv, err := ffargs.ProbeJSON(shared.runner.Policy(dir), probed)
	if err != nil {
		t.Fatal(err)
	}

	raw, _, err := shared.runner.RunFFprobe(context.Background(), dir, argv, verifyTimeout)
	if err != nil {
		t.Fatal(err)
	}

	var pf probedFile
	if err := json.Unmarshal(raw, &pf); err != nil {
		t.Fatal(err)
	}

	var found bool

	for _, s := range pf.Streams {
		if s.CodecType == "audio" && s.CodecName == "aac" {
			found = true
		}
	}

	if !found {
		t.Errorf("no AAC stream in %s", res.Path)
	}
}

// rmsDB is the RMS level, in dB relative to full scale, of the left channel of the samples from to to
// (exclusive) of 16-bit stereo PCM, or of their first difference (a crude high-pass that leaves white
// noise and drops a low tone by about 25 dB) when diff is set.
func rmsDB(pcm []byte, from, to int, diff bool) float64 {
	var sum float64

	prev := 0.0

	for i := from; i < to; i++ {
		v := float64(int16(binary.LittleEndian.Uint16(pcm[i*4:]))) / 32768
		if diff {
			v, prev = v-prev, v
		}

		sum += v * v
	}

	return 10 * math.Log10(sum/float64(to-from)+1e-12)
}

// A source with denoise asks the decoder for afftdn, and a tone under white noise gets quieter as the
// strength grows, while a denoise of 0 decodes the same bytes as no denoise at all (SDD-004 section 7).
func TestAudioDenoiseQuietsANoisyTone(t *testing.T) {
	doc := loadScene(t, "clip-caption")
	job := newJob(t, doc, "mixer-denoise", "unused.mp4")

	const id = "ast_noisytone"

	in := ingestClip(t, fixtureSpec{Name: "noisy-tone-quiet.wav", NoisyTone: true})
	doc.Assets[id] = timeline.Asset{Kind: timeline.AssetAudio}
	useAsset(doc, id, in)

	calls := 0
	decode := func(denoise float64) []byte {
		calls++
		s := testState(t, job, fmt.Sprintf("mixer-denoise-%d", calls))
		n := int64(2 * audio.SampleRate)

		rc, err := (&audioOpener{s: s}).Open(context.Background(), plan.AudioSource{
			ItemID: "i", AssetID: id, Samples: n, Gain: gp(0, 1), Denoise: denoise,
			Speed: timeline.Rational{Num: 1, Den: 1},
		})
		if err != nil {
			t.Fatal(err)
		}

		pcm, err := io.ReadAll(io.LimitReader(rc, n*4))
		_ = rc.Close()

		if err != nil || int64(len(pcm)) != n*4 {
			t.Fatalf("%d bytes decoded (%v), want %d", len(pcm), err, n*4)
		}

		return pcm
	}

	plain, off, mid, full := decode(0), decode(0.0001), decode(0.4), decode(1)
	if !bytes.Equal(plain, off) {
		t.Error("a denoise too small to matter changed the audio")
	}

	// The first second is the noise alone (what the first difference measures); the tone joins at 1 s.
	const sec = audio.SampleRate

	na, nb, nc := rmsDB(plain, 0, sec*9/10, true), rmsDB(mid, 0, sec*9/10, true), rmsDB(full, 0, sec*9/10, true)
	ta, tc := rmsDB(plain, sec*11/10, 2*sec, false), rmsDB(full, sec*11/10, 2*sec, false)
	t.Logf(
		"noise: plain %.2f dB, denoise 0.4 %.2f dB, denoise 1 %.2f dB; tone: %.2f dB then %.2f dB",
		na,
		nb,
		nc,
		ta,
		tc,
	)

	if !(na-nb >= 3 && nb-nc >= 0.5 && math.Abs(ta-tc) < 1) {
		t.Errorf(
			"the noise was not reduced in proportion: %.2f, %.2f, %.2f dB (tone %.2f, %.2f dB)",
			na,
			nb,
			nc,
			ta,
			tc,
		)
	}
}
