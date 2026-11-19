package export

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"time"

	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/audio"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg/ffargs"
	"github.com/AlexGalhardo/stuarty/engine/ingest"
)

// Loudness is what the mix measured and the gain that was applied to it (SDD-004 section 7).
type Loudness struct {
	IntegratedLUFS *float64 `json:"integrated_lufs"` // before the gain; nil for a silent mix
	TruePeakDBTP   *float64 `json:"true_peak_dbtp"`  // before the gain
	GainDB         float64  `json:"gain_db"`
	TargetLUFS     float64  `json:"target_lufs"`
	// Limited is true when the gain took the mix over the true peak ceiling and the limiter held
	// the peaks. OutputLUFS is then the loudness of the file after the limiter, and ShortfallDB how
	// far it stays under the target (0 when it reached it).
	Limited     bool     `json:"limited"`
	OutputLUFS  *float64 `json:"output_lufs,omitempty"`
	ShortfallDB float64  `json:"shortfall_db"`
}

// shortfallTolerance is how far under the target a limited mix may land before the gain is raised
// once more to make up for the loudness that the limiter took.
const shortfallTolerance = 0.3

// samplesAt is the output sample of timeline frame t: t x 48000 x den / num, floored.
func samplesAt(fps timeline.Rational, t int) int64 {
	return int64(t) * audio.SampleRate * int64(fps.Den) / int64(fps.Num)
}

// mixAudio makes the audio of the whole timeline: the mix of section 7, one loudness gain, and the
// 16-bit WAV the encoder reads. It returns the path of that WAV.
func (s *state) mixAudio(ctx context.Context) (string, Loudness, error) {
	doc := s.job.Doc
	from := samplesAt(doc.Settings.FPS, s.offset)
	total := samplesAt(doc.Settings.FPS, s.offset+s.frames) - from
	target := doc.Settings.LoudnessTarget
	loud := Loudness{TargetLUFS: target}

	out, err := s.dir.Path("mix.wav")
	if err != nil {
		return "", loud, err
	}
	srcs := plan.Audio(doc, s.offset, s.offset+s.frames)
	for i := range srcs {
		srcs[i].StartSample -= from
	}

	if len(srcs) == 0 {
		return out, loud, writeFile(out, func(w io.Writer) error { return audio.SilentWAV(w, total) })
	}

	raw, err := s.dir.Path("mix-f32.wav")
	if err != nil {
		return "", loud, err
	}
	opener := &audioOpener{s: s}
	err = writeFile(raw, func(w io.Writer) error { return audio.MixToFloatWAV(ctx, srcs, total, opener, w) })
	if err != nil {
		return "", loud, wrapAudio(err, "")
	}

	l, stderr, err := s.measure(ctx, raw, total)
	if err != nil {
		return "", loud, wrapAudio(err, stderr)
	}
	loud.IntegratedLUFS, loud.TruePeakDBTP = l.IntegratedLUFS, l.TruePeakDBTP
	loud.GainDB = audio.GainDB(l.IntegratedLUFS, l.TruePeakDBTP, total, target)

	if err := s.writeGain(ctx, raw, out, total, &loud); err != nil {
		return "", loud, err
	}
	_ = os.Remove(raw)
	return out, loud, nil
}

// writeGain writes the 16-bit WAV out from the float mix raw with the gain of loud. A gain that takes
// the peaks over the ceiling goes through the limiter; the result is measured, and the gain is raised
// once when the limiter took more loudness than the tolerance (the peaks it cut were part of the
// measured level).
func (s *state) writeGain(ctx context.Context, raw, out string, total int64, loud *Loudness) error {
	ceilingGain := math.Inf(1)
	if loud.TruePeakDBTP != nil {
		ceilingGain = audio.PeakCeilingDBTP - *loud.TruePeakDBTP + audio.MaxLimitingDB
	}
	for pass := 0; ; pass++ {
		limited := audio.NeedsLimiter(loud.TruePeakDBTP, loud.GainDB)
		if err := writeGainOnce(raw, out, total, loud.GainDB, limited); err != nil {
			return wrapAudio(err, "")
		}
		loud.Limited = limited
		if !limited {
			return nil
		}

		l, stderr, err := s.measure(ctx, out, total)
		if err != nil {
			return wrapAudio(err, stderr)
		}
		loud.OutputLUFS = l.IntegratedLUFS
		loud.ShortfallDB = 0
		if l.IntegratedLUFS == nil {
			return nil
		}
		short := loud.TargetLUFS - *l.IntegratedLUFS
		loud.ShortfallDB = math.Max(0, short)
		next := math.Min(loud.GainDB+short, ceilingGain)
		if pass > 0 || short <= shortfallTolerance || next < loud.GainDB+0.05 {
			return nil
		}
		loud.GainDB = next
		if err := os.Remove(out); err != nil {
			return err
		}
	}
}

func writeGainOnce(raw, out string, total int64, gainDB float64, limit bool) error {
	in, err := os.Open(raw)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	return writeFile(out, func(w io.Writer) error {
		if limit {
			return audio.LimitToPCM16WAV(in, w, total, gainDB)
		}

		return audio.GainToPCM16WAV(in, w, total, gainDB)
	})
}

func wrapAudio(err error, stderr string) error {
	return wrapRun(err, ReasonAudio, "The audio of the project could not be mixed.", stderr)
}

// writeFile creates path (never over an existing file) and fills it.
func writeFile(path string, fill func(io.Writer) error) (err error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}()
	return fill(f)
}

// measure runs FFmpeg's ebur128 over the float mix.
func (s *state) measure(ctx context.Context, path string, samples int64) (*ingest.Loudness, string, error) {
	argv, err := ffargs.Cmd{
		Inputs: []ffargs.Input{ffargs.FileInput(path)},
		Outputs: []ffargs.Output{{
			Format: "null", Maps: []string{"0:a:0"}, AF: ffargs.Chain1(ffargs.Ebur128()),
		}},
	}.Argv(s.job.Runner.Policy(s.dir))
	if err != nil {
		return nil, "", exportErr(ReasonAudio, "The loudness command was refused.", err, "")
	}
	res, err := s.job.Runner.RunFFmpeg(ctx, ffmpeg.Request{
		Dir: s.dir, Argv: argv, Deadline: s.deadline, DeadlineCeiling: ffmpeg.ExportDeadlineCeiling,
	})
	if err != nil {
		return nil, res.Stderr, err
	}
	l, err := ingest.ParseLoudness(res.Stderr, samples)
	if err != nil {
		return nil, res.Stderr, err
	}
	return l, "", nil
}

// audioOpener decodes the audio of a source with FFmpeg: from the normalized media of its asset, at
// the sample the source starts at, with atempo for a speed other than 1 (SDD-004 section 7).
type audioOpener struct{ s *state }

// Open starts the decoder. The stream is 48 kHz stereo signed 16-bit samples, ready for the mixer.
func (o *audioOpener) Open(ctx context.Context, src plan.AudioSource) (io.ReadCloser, error) {
	if src.Reverse || src.Curve != nil {
		return o.openShaped(ctx, src)
	}

	s := o.s
	a, ok := s.asset(src.AssetID)
	if !ok {
		return nil, fmt.Errorf("the asset %s is not in the document", src.AssetID)
	}
	ext := "mov"
	if a.Kind == timeline.AssetAudio {
		ext = "wav"
	}
	path, err := s.artifact(&a, ext)
	if err != nil {
		return nil, err
	}

	// The input seeks to a whole millisecond (48 samples), which FFmpeg does to the sample; at speed 1
	// the few samples before the wanted one are dropped here. At another speed the tempo change
	// makes sample accuracy meaningless (the check there is within 1 dB, SDD-004 section 11).
	const perMS = audio.SampleRate / 1000
	ms := src.SourceStart / perMS
	drop := int64(0)
	speed1 := src.Speed.Num == src.Speed.Den
	if speed1 {
		drop = src.SourceStart - ms*perMS
	}

	// The noise reduction cleans the decoded source before the tempo change (SDD-004 section 7).
	var chain []ffargs.Filter
	if nr, nf, ok := audio.DenoiseParams(src.Denoise); ok {
		chain = append(chain, ffargs.AFFTDN(nr, nf))
	}
	if !speed1 && src.Speed.Den > 0 && src.Speed.Num > 0 {
		chain = append(chain, tempoChain(float64(src.Speed.Num)/float64(src.Speed.Den))...)
	}
	// A little more than needed: the mixer truncates, and a stream that ends early is padded.
	need := src.Samples + drop
	dur := float64(need)/audio.SampleRate + 0.05

	dir, err := s.childDir()
	if err != nil {
		return nil, err
	}
	out := ffargs.Output{
		Format: "s16le", Pipe: true, Maps: []string{"0:a:0"}, NoVideo: true,
		Opts: []ffargs.Opt{
			ffargs.Int("-ar", audio.SampleRate), ffargs.Int("-ac", audio.Channels),
			ffargs.Enum("-t", fmt.Sprintf("%.6f", dur)),
		},
	}
	if len(chain) > 0 {
		out.AF = ffargs.Chain1(chain...)
	}
	argv, err := ffargs.Cmd{
		Inputs:  []ffargs.Input{ffargs.FileInput(path).At(ms)},
		Outputs: []ffargs.Output{out},
	}.Argv(s.job.Runner.Policy(dir, s.inputDirs...))
	if err != nil {
		_ = dir.Remove()
		return nil, exportErr(ReasonAudio, "The audio decoder command was refused.", err, "")
	}
	pr, pw, err := os.Pipe()
	if err != nil {
		_ = dir.Remove()
		return nil, err
	}

	rctx, cancel := context.WithCancel(ctx)
	st := &audioStream{pr: pr, ctx: rctx, cancel: cancel, dir: dir, done: make(chan struct{})}
	go func() {
		res, err := s.job.Runner.RunFFmpeg(rctx, ffmpeg.Request{
			Dir: dir, Argv: argv, InputDirs: s.inputDirs, Deadline: s.deadline,
			DeadlineCeiling: ffmpeg.ExportDeadlineCeiling, Stdout: pw,
		})
		_ = pw.Close()
		st.err, st.stderr = err, res.Stderr
		close(st.done)
	}()
	if drop > 0 {
		if _, err := io.CopyN(io.Discard, pr, drop*audio.BytesPerSample); err != nil && !errors.Is(err, io.EOF) {
			_ = st.Close()
			return nil, err
		}
	}
	return st, nil
}

// tempoChain is the atempo filters that give a tempo (each takes 0.5 to 2 here, so a larger change is
// chained).
func tempoChain(tempo float64) []ffargs.Filter {
	var out []ffargs.Filter
	for tempo > 2 {
		out = append(out, ffargs.ATempo(2))
		tempo /= 2
	}
	for tempo < 0.5 {
		out = append(out, ffargs.ATempo(0.5))
		tempo *= 2
	}
	if math.Abs(tempo-1) > 1e-6 {
		out = append(out, ffargs.ATempo(tempo))
	}
	return out
}

// audioStream is the pipe of one audio decoder; Close ends the process and waits for it.
type audioStream struct {
	pr     *os.File
	ctx    context.Context
	cancel context.CancelFunc
	dir    *ffmpeg.RunDir
	done   chan struct{}
	err    error
	stderr string
}

// Read returns the decoded bytes. At the end of the pipe it reports a decoder that failed, so that a
// broken source is an error and not a silent stretch.
func (a *audioStream) Read(p []byte) (int, error) {
	n, err := a.pr.Read(p)
	if err == io.EOF {
		<-a.done
		if a.err != nil && a.ctx.Err() == nil {
			return n, wrapRun(a.err, ReasonAudio, "A source of the audio could not be decoded.", a.stderr)
		}
	}
	return n, err
}

func (a *audioStream) Close() error {
	a.cancel()
	select {
	case <-a.done:
	case <-time.After(30 * time.Second):
	}
	_ = a.pr.Close()
	return a.dir.Remove()
}
