package ffmpeg

import (
	"bytes"
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AlexGalhardo/stuarty/engine/ffmpeg/ffargs"

	"github.com/AlexGalhardo/vukem/contrib/engine"
)

// Runner starts FFmpeg and ffprobe for one workspace. Every argv goes through
// ffargs.Check first; a refused argv starts nothing.
type Runner struct {
	In *Installation
	// Workspace is the workspace root; run directories are Workspace/tmp/<job id>.
	Workspace string
	// StoreDir is the content store, Workspace/store unless it is a link elsewhere.
	StoreDir string

	// Sandbox is the path of bwrap (ResolveSandbox) or empty: the tools then run inside it (Linux).
	Sandbox string
	// Grace is how long a cancelled run may take to end; CancelGrace when zero.
	Grace time.Duration

	tmpDir  string
	started atomic.Int64

	encMu  sync.Mutex
	enc    *EncoderDetection
	encErr error

	capMu    sync.Mutex
	capCache map[sizeKey]error
}

// NewRunner returns a runner for the installation and the workspace root.
func NewRunner(in *Installation, workspace string) (*Runner, error) {
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return nil, err
	}
	store, err := canonicalDir(filepath.Join(abs, "store"))
	if err != nil {
		return nil, err
	}

	tmp, err := canonicalDir(filepath.Join(abs, "tmp"))
	if err != nil {
		return nil, err
	}

	return &Runner{In: in, Workspace: abs, StoreDir: store, tmpDir: tmp}, nil
}

// RunDir is the directory of one job: the child's working directory, HOME and temp
// directory, and the only place it writes (SDD-007 6.4).
type RunDir struct{ path string }

var jobIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// NewRunDir creates <workspace>/tmp/<job id> with mode 0700. The directory must not exist.
func (r *Runner) NewRunDir(jobID string) (*RunDir, error) {
	if !jobIDRe.MatchString(jobID) {
		return nil, internalErr("a job id is letters, digits, '_' and '-'", nil)
	}
	tmp := r.tmpDir
	if tmp == "" {
		tmp = filepath.Join(r.Workspace, "tmp")
	}
	if err := os.MkdirAll(tmp, 0o700); err != nil {
		return nil, internalErr("cannot create the tmp directory of the workspace", err)
	}
	p := filepath.Join(tmp, jobID)
	if err := os.Mkdir(p, 0o700); err != nil {
		return nil, internalErr("cannot create the run directory", err)
	}
	return &RunDir{path: p}, nil
}

// NewJobRunDir is NewRunDir for a job's own directory. A job id belongs to one job, so a
// directory that already exists is the leftover of an interrupted earlier attempt (the
// process was closed mid-encode and the job was requeued): it is removed first.
func (r *Runner) NewJobRunDir(jobID string) (*RunDir, error) {
	if !jobIDRe.MatchString(jobID) {
		return nil, internalErr("a job id is letters, digits, '_' and '-'", nil)
	}
	tmp := r.tmpDir
	if tmp == "" {
		tmp = filepath.Join(r.Workspace, "tmp")
	}
	stale := &RunDir{path: filepath.Join(tmp, jobID)}
	if _, err := os.Lstat(stale.path); err == nil {
		if err := stale.Remove(); err != nil {
			return nil, internalErr("cannot remove the leftover run directory of the job", err)
		}
	}
	return r.NewRunDir(jobID)
}

// newInternalRunDir creates a run directory for FFmpeg's own checks (capabilities, encoder
// detection and size probes) under a unique name, so a directory left behind by an
// interrupted run can never make a later one fail (VV-001).
func (r *Runner) newInternalRunDir(prefix string) (*RunDir, error) {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return nil, internalErr("cannot name the run directory", err)
	}
	return r.NewRunDir(prefix + "-" + hex.EncodeToString(b[:]))
}

// Dir is the directory path.
func (d *RunDir) Dir() string { return d.path }

// Path returns a generated file name inside the run directory (engine.Sanitized).
func (d *RunDir) Path(name string) (string, error) { return engine.Sanitized(d.path, name) }

// Sub creates a subdirectory (for example the thumbnail tiles) and returns its path.
func (d *RunDir) Sub(name string) (string, error) {
	p, err := engine.Sanitized(d.path, name)
	if err != nil {
		return "", err
	}
	return p, os.Mkdir(p, 0o700)
}

// Remove deletes the directory after the process tree has ended. On Windows a handle that
// the dying process still holds can fail the first attempts, so it retries briefly.
func (d *RunDir) Remove() error {
	var err error
	for range 20 {
		if err = os.RemoveAll(d.path); err == nil {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return err
}

// Policy is the argv policy of a run: the store, the run directory, the extra input
// directories and the CPU budget.
func (r *Runner) Policy(d *RunDir, inputDirs ...string) ffargs.Policy {
	return ffargs.Policy{
		StoreDir: r.StoreDir, RunDir: d.path, InputDirs: inputDirs,
		MaxThreads: r.In.CPUCount,
	}
}

// Progress is one block of the -progress output.
type Progress struct {
	Frame     int64
	OutTimeUS int64
	End       bool
}

// Request is one ffmpeg run.
type Request struct {
	Dir  *RunDir
	Argv []string
	// InputDirs are artifact directories the argv may read from (see ffargs.Policy).
	InputDirs []string
	// Deadline bounds the run (SDD-007 6.5); required. While other runs are in flight it is
	// multiplied by that count, then capped at DeadlineCeiling.
	Deadline time.Duration
	// DeadlineCeiling caps a deadline that was multiplied because other runs share the process.
	// Zero uses ExportDeadlineCeiling when Deadline is within it, and IngestDeadlineCeiling when
	// Deadline is already longer (only an ingest deadline exceeds the export cap).
	DeadlineCeiling time.Duration
	// Stall overrides StallTimeout; used only when argv has -progress pipe:1.
	Stall time.Duration
	// OnProgress receives each progress block, from the goroutine that reads the pipe.
	OnProgress func(Progress)
	// Stdout receives the standard output of a run whose output is pipe:1 (raw frames).
	// An *os.File is handed to the child as it is, with no copy through this process.
	Stdout io.Writer
	// Stdin feeds a run whose input is pipe:0 (raw frames to the encoder). An *os.File is
	// handed to the child as it is. The runner never closes it: the caller owns both ends.
	Stdin io.Reader
	// OutputPath and MaxOutputBytes arm the size watchdog (STUARTY_MAX_EXPORT_BYTES): the file is
	// polled every OutputPollInterval and a run that grows it past the cap is killed with
	// ErrOutputTooLarge. The path must be inside the run directory.
	OutputPath     string
	MaxOutputBytes int64
}

// Result is what a run produced.
type Result struct {
	Stderr   string // tail, at most 4 MiB
	ExitCode int
	Duration time.Duration
}

// ErrStdinUnsupported is returned for an argv that reads pipe:0 when the request has no Stdin.
var ErrStdinUnsupported = errors.New("ffmpeg: an argv that reads pipe:0 needs a Stdin")

// RunFFmpeg checks the argv, then runs ffmpeg in the run directory with the deadline and
// the stall watchdog. A cancelled ctx kills the whole process tree and the error wraps ctx.Err().
func (r *Runner) RunFFmpeg(ctx context.Context, req Request) (Result, error) {
	if err := ffargs.Check(ffargs.FFmpeg, req.Argv, r.Policy(req.Dir, req.InputDirs...)); err != nil {
		return Result{}, &Error{
			Code:    CodeInternal,
			Reason:  "argv_policy",
			Message: "the FFmpeg arguments were refused before any start",
			Err:     err,
		}
	}
	if slices.Contains(req.Argv, "pipe:0") && req.Stdin == nil {
		return Result{}, &Error{
			Code:    CodeInternal,
			Reason:  "stdin_unsupported",
			Message: "this build cannot feed standard input to FFmpeg",
			Err:     ErrStdinUnsupported,
		}
	}
	progress := false
	for i, a := range req.Argv {
		if a == "-progress" && i+1 < len(req.Argv) && req.Argv[i+1] == "pipe:1" {
			progress = true
		}
	}
	s := execSpec{
		tool:       r.In.FFmpeg,
		inputDirs:  req.InputDirs,
		dir:        req.Dir.path,
		argv:       req.Argv,
		deadline:   req.Deadline,
		ceiling:    req.DeadlineCeiling,
		onProgress: req.OnProgress,
		stdout:     req.Stdout,
		stdin:      req.Stdin,
		outPath:    req.OutputPath,
		maxBytes:   req.MaxOutputBytes,
	}
	if progress {
		s.stall = req.Stall
		if s.stall <= 0 {
			s.stall = StallTimeout
		}
	}
	res, err := r.exec(ctx, s)
	return Result{Stderr: res.Stderr, ExitCode: res.ExitCode, Duration: res.Duration}, err
}

// RunFFprobe checks the argv and runs ffprobe. Its JSON is read through a writer capped at
// 4 MiB (FF-09); more than that is E_UNSUPPORTED_MEDIA.
func (r *Runner) RunFFprobe(
	ctx context.Context,
	dir *RunDir,
	argv []string,
	deadline time.Duration,
	inputDirs ...string,
) ([]byte, Result, error) {
	if err := ffargs.Check(ffargs.FFprobe, argv, r.Policy(dir, inputDirs...)); err != nil {
		return nil, Result{}, &Error{
			Code:    CodeInternal,
			Reason:  "argv_policy",
			Message: "the ffprobe arguments were refused before any start",
			Err:     err,
		}
	}
	w := &limitWriter{max: MaxProbeJSON}
	res, err := r.exec(
		ctx,
		execSpec{
			tool: r.In.FFprobe, dir: dir.path, argv: argv, deadline: deadline, stdout: w, onOverflow: w,
			inputDirs: inputDirs,
		},
	)
	out := Result{Stderr: res.Stderr, ExitCode: res.ExitCode, Duration: res.Duration}
	if w.over.Load() {
		return nil, out, &Error{
			Code:    CodeUnsupportedMedia,
			Reason:  "probe_output_too_large",
			Message: "the file declares more stream information than the product reads",
			Details: map[string]any{"max_bytes": MaxProbeJSON},
		}
	}
	return w.buf.Bytes(), out, err
}

type execSpec struct {
	tool       *engine.Tool
	dir        string
	argv       []string
	deadline   time.Duration
	ceiling    time.Duration
	stall      time.Duration
	onProgress func(Progress)
	stdout     io.Writer
	stdin      io.Reader
	onOverflow *limitWriter
	inputDirs  []string
	// outPath and maxBytes arm the output size watchdog.
	outPath  string
	maxBytes int64
	// pollEvery overrides OutputPollInterval; only tests set it.
	pollEvery time.Duration
}

// exec is the one place a child is started. Tests call it with a helper binary.
func (r *Runner) exec(ctx context.Context, s execSpec) (engine.Result, error) {
	if err := checkNotRoot(); err != nil {
		return engine.Result{}, err
	}

	r.started.Add(1)
	if s.deadline <= 0 {
		return engine.Result{}, internalErr("a run needs a deadline", nil)
	}

	// Count this run before the watchers start, so a peer that is already inside lengthens
	// this run, and a run that started alone hears the peer arrive.
	endFlight := beginFlight()
	defer endFlight()

	runCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	dctx, dcancel := context.WithCancelCause(runCtx)
	defer dcancel(nil)
	started := time.Now()
	go watchDeadline(dctx, started, s.deadline, deadlineCeiling(s.deadline, s.ceiling), func() {
		dcancel(ErrDeadline)
	})

	stdout := s.stdout
	var pw *progressWriter
	if s.stall > 0 {
		pw = &progressWriter{on: s.onProgress}
		pw.touch()
		stdout = pw
		go watchStall(dctx, pw, s.stall, func() { cancel(ErrStalled) })
	}
	if s.onOverflow != nil {
		s.onOverflow.cancel = func() { cancel(errOverflow) }
	}

	if s.outPath != "" && s.maxBytes > 0 {
		poll := cmp.Or(s.pollEvery, OutputPollInterval)
		go watchSize(dctx, s.outPath, s.maxBytes, poll, func() { cancel(ErrOutputTooLarge) })
	}

	type outcome struct {
		res engine.Result
		err error
	}
	ended := make(chan outcome, 1)
	go func() {
		var o outcome

		if r.Sandbox != "" {
			o.res, o.err = r.runSandboxed(dctx, s, stdout)
		} else {
			opts := engine.RunOptions{Dir: s.dir, Stdin: s.stdin, Stdout: stdout}
			o.res, o.err = s.tool.RunWith(dctx, opts, s.argv...)
		}
		ended <- o
	}()

	var res engine.Result
	var err error
	select {
	case o := <-ended:
		res, err = o.res, o.err
	case <-dctx.Done():
		grace := r.Grace
		if grace <= 0 {
			grace = CancelGrace
		}
		// contrib/engine kills the tree and waits at most 5 s for the pipes, so a run that is
		// still here after the grace plus a margin cannot be killed (SDD-007 6.6).
		select {
		case o := <-ended:
			res, err = o.res, o.err
		case <-time.After(grace + cancelMargin):
			return res, &Error{
				Code: CodeInternal, Reason: ReasonCancelTimeout,
				Message: "the process did not end after it was cancelled",
				Details: map[string]any{"reason": ReasonCancelTimeout},
				Err:     context.Cause(dctx),
			}
		}
	}
	if err == nil {
		return res, nil
	}
	if ctx.Err() != nil {
		return res, fmt.Errorf("ffmpeg: %w", ctx.Err())
	}
	switch cause := context.Cause(dctx); {
	case errors.Is(cause, ErrStalled):
		return res, fmt.Errorf("%w for %s", ErrStalled, s.stall)
	case errors.Is(cause, ErrDeadline):
		return res, fmt.Errorf("%w after %s", ErrDeadline, s.deadline)
	case errors.Is(cause, ErrOutputTooLarge):
		removeInside(s.dir, s.outPath)

		return res, fmt.Errorf("%w (limit %d bytes)", ErrOutputTooLarge, s.maxBytes)
	case errors.Is(cause, errOverflow):
		return res, nil
	}
	return res, err
}

var errOverflow = errors.New("output over the cap")

// flight is the number of runs inside this process. Export and ingest use different runners, so the
// count cannot live on one Runner: the run that started first is the one that would otherwise die.
type flightCounter struct {
	mu sync.Mutex
	n  int
	ch chan struct{}
}

var flights = flightCounter{ch: make(chan struct{})}

func beginFlight() (end func()) {
	flights.mu.Lock()
	flights.n++
	wakeFlightsLocked()
	flights.mu.Unlock()

	return func() {
		flights.mu.Lock()
		if flights.n > 0 {
			flights.n--
		}
		wakeFlightsLocked()
		flights.mu.Unlock()
	}
}

func wakeFlightsLocked() {
	close(flights.ch)
	flights.ch = make(chan struct{})
}

func flightNow() (n int, changed <-chan struct{}) {
	flights.mu.Lock()
	defer flights.mu.Unlock()

	return flights.n, flights.ch
}

// scaleAllowance multiplies base by the number of runs in flight. n <= 1 leaves base as it is.
// A positive ceiling caps the product but never below base, and a positive base never scales to zero.
func scaleAllowance(base time.Duration, n int, ceiling time.Duration) time.Duration {
	if base <= 0 {
		return base
	}
	if n < 1 {
		n = 1
	}

	scaled := base
	if n > 1 {
		const maxDuration = time.Duration(^uint64(0) >> 1)
		if base > maxDuration/time.Duration(n) {
			scaled = maxDuration
		} else {
			scaled = base * time.Duration(n)
		}
		if scaled <= 0 {
			scaled = base
		}
	}
	if ceiling > 0 {
		limit := max(ceiling, base)
		if scaled > limit {
			scaled = limit
		}
	}
	if scaled <= 0 {
		return base
	}

	return scaled
}

func deadlineCeiling(base, explicit time.Duration) time.Duration {
	if explicit > 0 {
		return explicit
	}
	if base > ExportDeadlineCeiling {
		return IngestDeadlineCeiling
	}

	return ExportDeadlineCeiling
}

// watchDeadline cancels the run once the wall clock passes the base deadline multiplied by the
// runs in flight. The start does not move: a peer lengthens the allowance, and a peer leaving
// shortens it back toward the base.
func watchDeadline(ctx context.Context, started time.Time, base, ceiling time.Duration, kill func()) {
	for {
		if ctx.Err() != nil {
			return
		}
		n, changed := flightNow()
		wait := time.Until(started.Add(scaleAllowance(base, n, ceiling)))
		if wait <= 0 {
			kill()

			return
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()

			return
		case <-timer.C:
		case <-changed:
			stopTimer(timer)
		}
	}
}

// watchStall cancels the run when -progress shows no advance for the stall window. The window is
// the base stall multiplied by the runs in flight. A change in that count starts a fresh window,
// so a run that began alone gains the longer stall when a peer appears, and a run left alone again
// is cancelled only after the base stall.
func watchStall(ctx context.Context, pw *progressWriter, stall time.Duration, kill func()) {
	if stall <= 0 {
		return
	}
	tick := max(min(stall/4, 5*time.Second), 10*time.Millisecond)
	t := time.NewTicker(tick)
	defer t.Stop()

	n, changed := flightNow()
	seen := n
	anchor := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-changed:
		}
		if ctx.Err() != nil {
			return
		}
		n, changed = flightNow()
		if n != seen {
			anchor = time.Now()
			seen = n
		}
		allowance := scaleAllowance(stall, seen, 0)
		last := time.Unix(0, pw.last.Load())
		if anchor.After(last) {
			last = anchor
		}
		if time.Since(last) >= allowance {
			kill()

			return
		}
	}
}

func stopTimer(t *time.Timer) {
	if !t.Stop() {
		select {
		case <-t.C:
		default:
		}
	}
}

// progressWriter parses the key=value lines of -progress pipe:1. A line longer than 4 KiB
// is discarded (SDD-007 6.5).
type progressWriter struct {
	on   func(Progress)
	mu   sync.Mutex
	line []byte
	skip bool
	cur  Progress
	max  Progress
	last atomic.Int64
}

func (w *progressWriter) touch() { w.last.Store(time.Now().UnixNano()) }

func (w *progressWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, c := range p {
		if c != '\n' {
			if w.skip {
				continue
			}
			if len(w.line) >= maxProgressLine {
				w.line, w.skip = w.line[:0], true
				continue
			}
			w.line = append(w.line, c)
			continue
		}
		if !w.skip {
			w.handle(strings.TrimRight(string(w.line), "\r"))
		}
		w.line, w.skip = w.line[:0], false
	}
	return len(p), nil
}

func (w *progressWriter) handle(line string) {
	k, v, ok := strings.Cut(line, "=")
	if !ok {
		return
	}
	switch k {
	case "frame":
		w.cur.Frame, _ = strconv.ParseInt(v, 10, 64)
	case "out_time_us":
		w.cur.OutTimeUS, _ = strconv.ParseInt(v, 10, 64)
	case "progress":
		w.cur.End = v == "end"
		if w.cur.Frame > w.max.Frame || w.cur.OutTimeUS > w.max.OutTimeUS || w.cur.End {
			w.touch()
			w.max.Frame = max(w.max.Frame, w.cur.Frame)
			w.max.OutTimeUS = max(w.max.OutTimeUS, w.cur.OutTimeUS)
		}
		if w.on != nil {
			w.on(w.cur)
		}
	}
}

// limitWriter keeps at most max bytes and asks for the run to be killed when a stream
// goes over it.
type limitWriter struct {
	max    int
	buf    bytes.Buffer
	over   atomic.Bool
	cancel func()
}

func (w *limitWriter) Write(p []byte) (int, error) {
	if w.over.Load() {
		return len(p), nil
	}
	if w.buf.Len()+len(p) > w.max {
		w.over.Store(true)
		w.buf.Reset()
		if w.cancel != nil {
			w.cancel()
		}
		return len(p), nil
	}
	return w.buf.Write(p)
}
