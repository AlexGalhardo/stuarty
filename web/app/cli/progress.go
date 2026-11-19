package cli

import (
	"context"
	"fmt"
	"io"
	"sync/atomic"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
)

const (
	// progressStep is the smallest change of a percentage worth a new line.
	progressStep = 5
	// progressPoll is how often the progress of an inline job is read.
	progressPoll = 500 * time.Millisecond
	// progressSlice is the longest wait of one call while a person watches, so the line moves.
	progressSlice = 2 * time.Second
	// cleanupTimeout bounds what a timed out in-process command does to leave no job behind.
	cleanupTimeout = 10 * time.Second
)

// progress writes the progress lines of a long command to standard error. It is
// nil when nobody watches: --json mode, or a standard error that is not a terminal.
type progress struct {
	w    io.Writer
	last map[string]int
}

func (c *call) progress() *progress {
	if c.args.boolean("json") || !c.rt.StderrTTY || c.rt.Stderr == nil {
		return nil
	}

	return &progress{w: c.rt.Stderr, last: map[string]int{}}
}

// slice is the wait of one call of a --wait loop: shorter while a progress line is drawn.
func (p *progress) slice(deadline time.Time) int {
	if p == nil {
		return waitSlice(deadline)
	}

	left := min(time.Until(deadline), progressSlice)
	if left <= 0 {
		return 0
	}

	return int(left.Milliseconds())
}

// line prints text when the percentage of key moved by a step or the state changed.
func (p *progress) line(key string, pct int, text string) {
	if p == nil {
		return
	}

	if prev, ok := p.last[key]; ok && pct < 100 && pct-prev < progressStep {
		return
	}

	p.last[key] = pct

	fmt.Fprintln(p.w, text)
}

// job reports a job at its current state.
func (p *progress) job(j *service.Job) {
	if p == nil || j == nil || service.JobFinal(j.State) {
		return
	}

	pct := int(j.Progress * 100)
	text := fmt.Sprintf("%s %s %d%%", j.Kind, j.State, pct)

	if j.Message != "" {
		text += " " + j.Message
	}

	p.line(j.JobID+j.State, pct, text)
}

// download reports the bytes of a helper download.
func (p *progress) download(done, total int64) {
	if p == nil || total <= 0 {
		return
	}

	pct := int(done * 100 / total)
	p.line("download", pct, fmt.Sprintf("download %d of %d MiB, %d%%", done>>20, total>>20, pct))
}

// assets reports the ingest of the assets of an import.
func (p *progress) assets(done, total int, ratio float64) {
	if p == nil || total == 0 {
		return
	}

	pct := int((float64(done) + ratio) / float64(total) * 100)
	p.line("import", pct, fmt.Sprintf("import %d of %d files, %d%%", done, total, pct))
}

// noWaitInProcess refuses --no-wait when this process owns the workspace: a job
// started here runs inside the call and ends with it, so there is nothing to come back to.
func (c *call) noWaitInProcess() error {
	if !c.inProcess || !c.args.has("wait") || c.args.boolean("wait") {
		return nil
	}

	return &service.Error{
		Code: command.CodeValidation, Path: "wait",
		Message: "--no-wait needs a server: no server owns this workspace, so a job would end with this command.",
		Hint:    "start stuarty serve to run jobs in the background",
	}
}

// inline runs a call that blocks until its job is final (the in-process queue
// runs the job inside the call) while a goroutine draws the progress. When the
// deadline passes, stop is called if there is one (it should make the call
// return, as a cancelled job does); with no stop the context of the call ends.
// The second result is true when the deadline passed before the call returned.
func inline[T any](
	ctx context.Context, deadline time.Time, watch func(context.Context), stop func(),
	run func(context.Context) (T, error),
) (res T, expired bool, err error) {
	runCtx := ctx

	var (
		fired  atomic.Bool
		cancel context.CancelFunc = func() {}
	)

	if stop == nil {
		runCtx, cancel = context.WithDeadline(ctx, deadline)
	}

	defer cancel()

	timer := time.AfterFunc(time.Until(deadline), func() {
		fired.Store(true)

		if stop != nil {
			stop()
		}
	})
	defer timer.Stop()

	if watch != nil {
		done := make(chan struct{})
		stopped := make(chan struct{})

		go func() {
			defer close(stopped)

			tick := time.NewTicker(progressPoll)
			defer tick.Stop()

			for {
				select {
				case <-done:
					return
				case <-runCtx.Done():
					return
				case <-tick.C:
					watch(runCtx)
				}
			}
		}()

		defer func() {
			close(done)
			<-stopped
		}()
	}

	res, err = run(runCtx)

	// Without stop the context deadline and the timer are two clocks: the call can return on the
	// context a moment before the timer has set fired, so the context itself tells it expired.
	expired = fired.Load() || (stop == nil && runCtx.Err() != nil)

	return res, expired && ctx.Err() == nil, err
}

// cancelStarted cancels the jobs of the project that this command created, so a
// job interrupted by --timeout does not come back at the next serve.
func (c *call) cancelStarted(ctx context.Context, since time.Time) *service.Job {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
	defer cancel()

	list, err := c.svc.JobList(ctx, service.JobListRequest{
		ProjectID: c.projectID, State: "active",
	})
	if err != nil {
		return nil
	}

	var last *service.Job

	for i := range list.Jobs {
		j := &list.Jobs[i]
		if j.CreatedAt.Before(since.Add(-time.Second)) {
			continue
		}

		cancelled, cerr := c.svc.JobCancel(ctx, service.JobCancelRequest{JobID: j.JobID})
		if cerr == nil {
			last = cancelled
		} else {
			last = j
		}
	}

	return last
}

// watchJobs draws the progress of the active jobs of the project.
func (c *call) watchJobs(p *progress, since time.Time) func(context.Context) {
	if p == nil {
		return nil
	}

	return func(ctx context.Context) {
		list, err := c.svc.JobList(ctx, service.JobListRequest{
			ProjectID: c.projectID, State: "active",
		})
		if err != nil {
			return
		}

		for i := range list.Jobs {
			if !list.Jobs[i].CreatedAt.Before(since.Add(-time.Second)) {
				p.job(&list.Jobs[i])
			}
		}
	}
}

// watchAssets draws the progress of the ingest of the assets of the project.
func (c *call) watchAssets(p *progress, total int) func(context.Context) {
	if p == nil {
		return nil
	}

	return func(ctx context.Context) {
		list, err := c.svc.MediaList(ctx, service.MediaListRequest{ProjectID: c.projectID})
		if err != nil {
			return
		}

		done, ratio := 0, 0.0

		for i := range list.Assets {
			switch a := &list.Assets[i]; a.State {
			case "ready":
				done++
			case "pending", "ingesting":
				ratio += a.Progress
			}
		}

		p.assets(min(done, total), total, ratio)
	}
}

// startJob starts a job and, when asked to wait, returns it final or timed out.
// In process the Service runs the job inside the start call: the call is bounded
// by the deadline, and a job it interrupts is cancelled. start receives the
// wait the Service should honour (0 in process).
func (c *call) startJob(
	ctx context.Context, p *progress, wait bool, deadline time.Time,
	start func(ctx context.Context, waitMS int) (*service.Job, error),
) (job *service.Job, expired bool, err error) {
	if !c.inProcess || !wait {
		waitMS := 0
		if wait {
			waitMS = p.slice(deadline)
		}

		job, err = start(ctx, waitMS)

		return job, false, err
	}

	since := time.Now()

	job, expired, err = inline(ctx, deadline, c.watchJobs(p, since), func() { c.cancelStarted(ctx, since) },
		func(ctx context.Context) (*service.Job, error) { return start(ctx, 0) })
	if expired && (job == nil || job.State != service.JobCancelled) {
		// It finished, or failed, on its own just as the time ran out: report it as it is.
		expired = false
	}

	return job, expired, err
}

// runJob starts a job with start and reports it, the same way on every command
// that starts one (export, bundle:export, transcribe).
func (c *call) runJob(
	ctx context.Context, wait bool, deadline time.Time,
	start func(ctx context.Context, waitMS int) (*service.Job, error),
) (*output, error) {
	if err := c.noWaitInProcess(); err != nil {
		return nil, err
	}

	p := c.progress()

	job, expired, err := c.startJob(ctx, p, wait, deadline, start)
	if err != nil {
		return nil, err
	}

	if expired {
		return c.expiredOutput(job), nil
	}

	return jobOutput(ctx, c, job, wait, deadline, p)
}

// expiredText is what a command that ran out of --timeout in process says.
func (c *call) expiredText() string {
	return fmt.Sprintf("timed out after %s: no server owns this workspace, so the work was stopped with the "+
		"command. Run \"stuarty serve\" to keep it running, or raise --timeout.", c.args.timeout())
}

func (c *call) expiredOutput(job *service.Job) *output {
	if job == nil {
		job = &service.Job{State: service.JobCancelled}
	}

	job.Text = c.expiredText()

	return &output{value: job, text: job.Text, exit: ExitTransient}
}

func importDone(r *service.MediaImportResult) int {
	n := 0

	for i := range r.Assets {
		if a := &r.Assets[i]; a.State == "ready" || a.Error != nil {
			n++
		}
	}

	return n
}

// importExpired answers an in-process import that ran out of --timeout: the files
// that finished stay imported, and the same command again continues (import is idempotent).
func (c *call) importExpired(ctx context.Context) (*output, error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
	defer cancel()

	list, err := c.svc.MediaList(ctx, service.MediaListRequest{ProjectID: c.projectID})
	if err != nil {
		return nil, err
	}

	list.Text = c.expiredText() + " Run the same command again to continue: files already imported are kept."

	return &output{value: list, text: list.Text, exit: ExitTransient}, nil
}
