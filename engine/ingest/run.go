package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg/ffargs"
)

// CodeIngestFailed is the E_INGEST_FAILED of SDD-011 5.3; details.reason is one of the codes below.
const CodeIngestFailed = "E_INGEST_FAILED"

// The reason codes of E_INGEST_FAILED.
const (
	ReasonDecodeFailed       = "decode_failed"
	ReasonAudioDecodeFailed  = "audio_decode_failed"
	ReasonVerificationFailed = "verification_failed"
	ReasonTruncated          = "truncated"
	ReasonTimeout            = "timeout"
)

// Step names, in the order of SDD-011 7.1.
const (
	StepProbe   = "probe"
	StepVideo   = "video"
	StepAudio   = "audio"
	StepMux     = "mux"
	StepVerify  = "verify"
	StepPublish = "publish"
)

// Progress is a report of the job: overall progress in [0, 1] and a message naming the step.
type Progress struct {
	Step     string
	Fraction float64
	Message  string
}

// Input is one ingest job. The original is read from the store and never modified.
type Input struct {
	Runner *ffmpeg.Runner
	// Caps is the capability list of ffmpeg.ReadCapabilities.
	Caps *ffmpeg.Capabilities
	// Hash is the store key of the original (lowercase hexadecimal SHA-256, no prefix).
	Hash string
	// ArtifactsDir is <workspace>/artifacts; the files go to <ArtifactsDir>/<Hash>/.
	ArtifactsDir string
	JobID        string
	Ceilings     ffmpeg.Ceilings
	// SoundMaps asks for the speech map and the beat grid of the audio (RF-043, RF-044). They are
	// artifacts of kinds the caller must be able to register, so a caller opts in.
	SoundMaps bool
	// OnProgress is called at most 4 times per second, and never with a smaller fraction.
	OnProgress func(Progress)
}

// Artifact is one published file, ready for the artifacts table (SDD-011 6.1).
type Artifact struct {
	Kind    string
	Profile int
	Variant string
	Ext     string
	RelPath string // slash path relative to ArtifactsDir's parent (the workspace)
	Size    int64
	SHA256  string
	Meta    any
}

// Facts are the measurements of SDD-011 4.9, taken from the produced files.
type Facts struct {
	FrameRate   Rational `json:"frame_rate"`
	FrameCount  int      `json:"frame_count"`
	SampleCount int64    `json:"sample_count"`
	Width       int      `json:"width"`
	Height      int      `json:"height"`
	HasAudio    bool     `json:"has_audio"`
	// HasAlpha is true for a video whose normalized file keeps an alpha channel (SDD-011 4.3.5).
	HasAlpha bool `json:"has_alpha,omitempty"`
	// AlphaOpaque is true when HasAlpha is and every sampled picture of the asset is fully opaque: the file
	// declares transparency and has none (SDD-011 4.3.6). It is a fact of the normalized artifact, not of the asset.
	AlphaOpaque bool `json:"alpha_opaque,omitempty"`
	// FrameDelays is the display time in milliseconds of each frame of an animated image, whose
	// FrameCount is its length (SDD-001 3.2). Empty for every other asset.
	FrameDelays []int `json:"frame_delays,omitempty"`
}

// Result is the outcome of a successful ingest.
type Result struct {
	Kind      Kind
	Summary   *Summary
	Plan      any // VideoPlan or ImagePlan, stored in the job result so a failure can be reproduced
	Facts     Facts
	Artifacts []Artifact
	Warnings  []string
	TimingsMS map[string]int64
}

// ingestErr is the typed E_INGEST_FAILED.
func ingestErr(reason, msg string, details map[string]any, cause error) *ffmpeg.Error {
	if details == nil {
		details = map[string]any{}
	}
	return &ffmpeg.Error{
		Code: CodeIngestFailed, Reason: reason, Message: msg, Details: details, Err: cause,
		Hint: "Try the file in another player; if it plays there, report it with the output of \"stuarty doctor\".",
	}
}

// mapRunErr turns a runner error into the error of the step, passing cancellation through.
func mapRunErr(err error, step, stderr string) error {
	if err == nil {
		return nil
	}
	var fe *ffmpeg.Error
	switch {
	case errors.Is(err, context.Canceled):
		return err
	case errors.Is(err, ffmpeg.ErrDeadline):
		return ingestErr(
			ReasonTimeout,
			"Preparing the media took longer than the time allowed.",
			map[string]any{"step": step},
			err,
		)
	case errors.Is(err, ffmpeg.ErrStalled):
		return ingestErr(
			ReasonTimeout,
			"Preparing the media stopped making progress.",
			map[string]any{"step": step, "cause": "stalled"},
			err,
		)
	case errors.Is(err, ffmpeg.ErrOutputTooLarge):
		return ingestErr(
			ReasonVerificationFailed,
			"The media decoded to more data than it declares.",
			map[string]any{"step": step, "cause": "output_over_declared_size"},
			err,
		)
	case errors.As(err, &fe):
		return err
	}
	return ingestErr(ReasonDecodeFailed, "FFmpeg stopped during the "+step+" step: "+firstLine(stderr),
		map[string]any{"step": step, "stderr": ffmpeg.Scrub(stderr, 400)}, err)
}

func firstLine(s string) string {
	s = ffmpeg.Scrub(s, 400)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
}

// weights of SDD-011 7.4 for a video with audio; the weights of the steps that do not run
// are renormalized away.
var stepWeights = map[string]float64{
	StepProbe:   0.04,
	StepVideo:   0.66,
	StepAudio:   0.14,
	StepMux:     0.04,
	StepVerify:  0.10,
	StepPublish: 0.02,
}

// tracker computes the job progress: sum(weight_i * progress_i), never decreasing, reported at
// most 4 times per second.
type tracker struct {
	mu      sync.Mutex
	on      func(Progress)
	weights map[string]float64
	done    map[string]float64
	last    float64
	lastAt  time.Time
}

func newTracker(on func(Progress), steps ...string) *tracker {
	t := &tracker{on: on, weights: map[string]float64{}, done: map[string]float64{}}
	sum := 0.0
	for _, s := range steps {
		sum += stepWeights[s]
	}
	for _, s := range steps {
		t.weights[s] = stepWeights[s] / sum
	}
	return t
}

var stepText = map[string]string{
	StepProbe: "Reading the file", StepVideo: "Preparing video", StepAudio: "Preparing audio",
	StepMux: "Packaging", StepVerify: "Checking the result", StepPublish: "Saving",
}

// set records the progress of a step in [0, 1] and reports when due.
func (t *tracker) set(step string, frac float64) {
	if t.on == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.done[step] = max(t.done[step], min(1, max(0, frac)))
	total := 0.0
	for s, w := range t.weights {
		total += w * t.done[s]
	}
	total = max(total, t.last)
	if frac < 1 && time.Since(t.lastAt) < 250*time.Millisecond {
		return
	}
	t.last, t.lastAt = total, time.Now()
	msg := stepText[step]
	if frac > 0 && frac < 1 {
		msg += " (" + strconv.Itoa(int(frac*100)) + "%)"
	}
	t.on(Progress{Step: step, Fraction: total, Message: msg})
}

// job is the state of one running ingest.
type job struct {
	in       Input
	r        *ffmpeg.Runner
	wd       *ffmpeg.RunDir
	pol      ffargs.Policy
	original string
	res      *Result
	prog     *tracker
	started  time.Time
	mu       sync.Mutex
	// durations in milliseconds, for the deadlines
	durationMS int64
	decodeErrs int
	files      []workFile
	peaks      *Peaks
	layout     ThumbLayout
}

func (j *job) path(name string) string {
	p, err := j.wd.Path(name)
	if err != nil {
		panic("ingest: " + err.Error()) // names are constants of this package
	}
	return p
}

func (j *job) timed(step string, f func() error) error {
	start := time.Now()
	err := f()
	j.mu.Lock()
	j.res.TimingsMS[step] += time.Since(start).Milliseconds()
	j.mu.Unlock()
	return err
}

// run executes an ffmpeg command built by the typed builders.
func (j *job) run(
	ctx context.Context,
	step string,
	c ffargs.Cmd,
	deadline time.Duration,
	onFrac func(float64),
) (ffmpeg.Result, error) {
	argv, err := c.Argv(j.pol)
	if err != nil {
		return ffmpeg.Result{}, &ffmpeg.Error{
			Code:    ffmpeg.CodeInternal,
			Reason:  "argv_policy",
			Message: "the FFmpeg arguments were refused before any start",
			Err:     err,
		}
	}
	req := ffmpeg.Request{
		Dir: j.wd, Argv: argv, Deadline: deadline, DeadlineCeiling: ffmpeg.IngestDeadlineCeiling,
	}
	if c.Progress && onFrac != nil && j.durationMS > 0 {
		total := float64(j.durationMS) * 1000
		req.OnProgress = func(p ffmpeg.Progress) { onFrac(float64(p.OutTimeUS) / total) }
	}
	res, err := j.r.RunFFmpeg(ctx, req)
	return res, mapRunErr(err, step, res.Stderr)
}

// runProbe runs an ffprobe query and returns its output.
// probeQuery is an ffprobe argv with the error of the builder that made it.
type probeQuery struct {
	argv []string
	err  error
}

func pq(argv []string, err error) probeQuery { return probeQuery{argv, err} }

func (j *job) runProbe(ctx context.Context, q probeQuery) ([]byte, error) {
	argv, err := q.argv, q.err
	if err != nil {
		return nil, &ffmpeg.Error{
			Code:    ffmpeg.CodeInternal,
			Reason:  "argv_policy",
			Message: "the ffprobe arguments were refused before any start",
			Err:     err,
		}
	}
	out, res, err := j.r.RunFFprobe(ctx, j.wd, argv, ffmpeg.ProbeTimeout)
	if err != nil {
		var fe *ffmpeg.Error
		if errors.As(err, &fe) || ctx.Err() != nil {
			return nil, err
		}
		return nil, ingestErr(
			ReasonDecodeFailed,
			"ffprobe could not read a file of the job: "+firstLine(res.Stderr),
			map[string]any{"step": "probe"},
			err,
		)
	}
	return out, nil
}

// Run ingests one stored original: probe, normalized media, proxy, thumbnails, peaks and the
// analyses, each verified by decoding it, then published as artifacts (SDD-011 7.1). It works in
// tmp/<job id> and removes it in every outcome. A failure publishes nothing. The caller owns the
// database: it registers the returned artifacts and the facts.
func Run(ctx context.Context, in Input) (*Result, error) {
	if !validHash(in.Hash) {
		return nil, ingestErr(ReasonDecodeFailed, "the store key is not a SHA-256", nil, nil)
	}
	wd, err := in.Runner.NewJobRunDir(in.JobID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = wd.Remove() }()

	j := &job{
		in: in, r: in.Runner, wd: wd, pol: in.Runner.Policy(wd),
		original: filepath.Join(in.Runner.StoreDir, in.Hash[:2], in.Hash),
		res:      &Result{TimingsMS: map[string]int64{}}, started: time.Now(),
	}
	if err := j.timed(StepProbe, func() error { return j.probe(ctx) }); err != nil {
		return nil, err
	}

	switch j.res.Kind {
	case KindVideo:
		err = j.video(ctx)
	case KindAudio:
		err = j.audioOnly(ctx)
	case KindImage:
		err = j.image(ctx)
	}
	if err != nil {
		return nil, err
	}
	if err := j.timed(StepPublish, func() error { return j.publish(ctx) }); err != nil {
		return nil, err
	}
	j.prog.set(StepPublish, 1)
	j.res.TimingsMS["total"] = time.Since(j.started).Milliseconds()
	return j.res, nil
}

func validHash(h string) bool {
	if len(h) != 64 {
		return false
	}
	for _, c := range h {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// probe is step 1: the checked ffprobe answer, the further runs of SDD-011 3.1, the summary and its
// classification, and probe-1.json.
func (j *job) probe(ctx context.Context) error {
	pr, err := j.r.Probe(ctx, j.wd, j.original, j.in.Ceilings)
	if err != nil {
		return err
	}
	doc, err := ParseDoc(pr.Raw)
	if err != nil {
		return err
	}
	sel, err := doc.Select()
	if err != nil {
		return err
	}
	var ex Extras
	switch sel.Kind {
	case KindAudio:
		// An audio file needs no further probe.
	case KindVideo:
		out, err := j.runProbe(ctx, pq(ffargs.ProbePacketTimes(j.pol, j.original, sel.Video.Index)))
		if err != nil {
			return err
		}
		ex.PacketTimes = parsePacketTimes(string(out))
	case KindImage:
		if out, err := j.runProbe(
			ctx,
			pq(ffargs.ProbeFirstFrameSideData(j.pol, j.original, sel.Video.Index)),
		); err == nil {
			ex.FrameSideData = out
		} else {
			return err
		}
		out, err := j.runProbe(ctx, pq(ffargs.ProbeAnimPackets(j.pol, j.original, sel.Video.Index)))
		if err != nil {
			return err
		}
		ex.FrameDelays = parseFrameDelays(string(out))
		ex.NbReadPackets = len(ex.FrameDelays)
	}
	st, err := os.Stat(j.original)
	if err != nil {
		return err
	}
	sum, err := Summarize(doc, sel, ex, j.in.Caps, st.Size())
	if err != nil {
		return err
	}
	j.res.Kind, j.res.Summary = sel.Kind, sum
	j.res.Warnings = append(j.res.Warnings, sum.Warnings...)
	j.durationMS = sum.DurationMS

	steps := []string{StepProbe, StepVideo, StepAudio, StepMux, StepVerify, StepPublish}
	switch {
	case sel.Kind == KindVideo && sel.Audio == nil:
		steps = []string{StepProbe, StepVideo, StepMux, StepVerify, StepPublish}
	case sel.Kind == KindAudio:
		steps = []string{StepProbe, StepAudio, StepVerify, StepPublish}
	case sel.Kind == KindImage:
		steps = []string{StepProbe, StepVideo, StepVerify, StepPublish}
	}
	j.prog = newTracker(j.in.OnProgress, steps...)
	j.prog.set(StepProbe, 1)
	return j.writeProbeJSON(pr.Raw, sum)
}

func parsePacketTimes(out string) []float64 {
	var ts []float64
	for line := range strings.SplitSeq(out, "\n") {
		first, _, _ := strings.Cut(strings.TrimSpace(line), ",")
		if f, err := strconv.ParseFloat(first, 64); err == nil {
			ts = append(ts, f)
		}
	}
	return ts
}

// writeProbeJSON stores the summary next to the raw ffprobe JSON with format.filename blanked:
// a server path never leaves the process.
func (j *job) writeProbeJSON(raw []byte, sum *Summary) error {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		return err
	}
	if f, ok := doc["format"]; ok {
		var format map[string]json.RawMessage
		if json.Unmarshal(f, &format) == nil {
			format["filename"] = json.RawMessage(`""`)
			if b, err := json.Marshal(format); err == nil {
				doc["format"] = b
			}
		}
	}
	rawClean, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	b, err := json.Marshal(struct {
		Summary *Summary        `json:"summary"`
		FFprobe json.RawMessage `json:"ffprobe"`
	}{sum, rawClean})
	if err != nil {
		return err
	}
	return os.WriteFile(j.path("probe.json"), b, 0o600)
}

func checkFail(name string, expected, actual any) map[string]any {
	return map[string]any{"name": name, "expected": expected, "actual": actual}
}

func verificationFailed(step string, checks []map[string]any) error {
	names := make([]string, len(checks))
	for i, c := range checks {
		names[i] = fmt.Sprint(c["name"])
	}
	return ingestErr(ReasonVerificationFailed, "The "+step+" failed verification: "+strings.Join(names, ", ")+".",
		map[string]any{"checks": checks}, nil)
}
