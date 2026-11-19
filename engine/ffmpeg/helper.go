package ffmpeg

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"time"

	"github.com/AlexGalhardo/vukem/contrib/engine"
)

// ErrHelperSandboxed means a helper that needs the graphics devices was asked for while the runner
// wraps its tools in bubblewrap: the wrapper hides the devices, so the caller uses its fallback.
var ErrHelperSandboxed = errors.New("ffmpeg: a graphics helper cannot run inside the sandbox")

// HelperTool resolves a helper of this product (the GPU host stuarty-render) as a tool that runs under
// the limits of an export with a hardware encoder: a CPU backstop and no address space ceiling, because
// graphics drivers map very large ranges (SDD-007 6.5). The path must be absolute; the helper answers
// --version, which the discovery reads.
func (r *Runner) HelperTool(ctx context.Context, path string, deadline time.Duration) (*engine.Tool, error) {
	if !filepath.IsAbs(path) {
		return nil, &Error{
			Code: CodeInternal, Reason: "helper_path",
			Message: "a helper is started only from an absolute path",
			Err:     errors.New("relative helper path"),
		}
	}

	cpus := r.In.CPUCount

	return engine.Find(ctx, engine.Config{
		Path:        path,
		Limits:      LimitsFor(KindExportHardware, deadline, cpus, 0),
		CPUCount:    cpus,
		VersionArgs: []string{"--version"},
	})
}

// HelperRequest is one run of a helper.
type HelperRequest struct {
	Tool *engine.Tool
	Dir  *RunDir
	Argv []string
	// Deadline bounds the run; there is no stall watchdog, the helper answers requests and is idle
	// between them.
	Deadline time.Duration
	// Stdin and Stdout carry the protocol of the helper. A Stdin that blocks in Read must return
	// when the run ends (contrib/engine).
	Stdin  io.Reader
	Stdout io.Writer
}

// RunHelper runs a helper under the process policy of FFmpeg: no root, the minimal environment, its
// own run directory, a deadline, the cancel grace and the kill of the whole process tree (SDD-007
// 6.4 to 6.6). The argv is the caller's own constant, never user text.
func (r *Runner) RunHelper(ctx context.Context, req HelperRequest) (Result, error) {
	if r.Sandbox != "" {
		return Result{}, ErrHelperSandboxed
	}

	res, err := r.exec(ctx, execSpec{
		tool:     req.Tool,
		dir:      req.Dir.path,
		argv:     req.Argv,
		deadline: req.Deadline,
		stdin:    req.Stdin,
		stdout:   req.Stdout,
	})

	return Result{Stderr: res.Stderr, ExitCode: res.ExitCode, Duration: res.Duration}, err
}
