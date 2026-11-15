//go:build linux

package ffmpeg

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/AlexGalhardo/vukem/contrib/engine"
)

// maxStderrTail is the tail of stderr a run keeps (the cap of Result.Stderr).
const maxStderrTail = 4 << 20

// A run inside bubblewrap starts the wrapper (bwrap, behind prlimit when there are limits), not the
// tool, and contrib/engine's Tool.RunWith starts only the tool it resolved. The wrapped run is
// started here, with the same environment, process group, tree kill and pipe wait as contrib/engine.
// Every other run goes through Tool.RunWith.

// inheritedEnv are the only host variables a run sees (the list of contrib/engine).
var inheritedEnv = []string{"PATH", "SystemRoot", "windir", "LANG", "LC_ALL"}

// childEnv builds the environment of a run from nothing: HOME and the temp variables name the
// run directory, so the child scratches inside the directory that is about to be deleted.
func childEnv(dir string, extra ...string) []string {
	env := make([]string, 0, len(inheritedEnv)+5)
	for _, k := range inheritedEnv {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	return append(append(env, "HOME="+dir, "TMPDIR="+dir, "TEMP="+dir, "TMP="+dir), extra...)
}

// runSandboxed runs the wrapped command of a run in its directory with its stdin, copying stdout to
// stdout when it is not nil.
func (r *Runner) runSandboxed(ctx context.Context, s execSpec, stdout io.Writer) (engine.Result, error) {
	name, args, err := r.command(s)
	if err != nil {
		return engine.Result{}, err
	}

	return runWithStdin(ctx, name, args, s.dir, r.libraryEnv(), s.stdin, stdout)
}

func runWithStdin(
	ctx context.Context,
	name string,
	args []string,
	dir string,
	extraEnv []string,
	stdin io.Reader,
	stdout io.Writer,
) (engine.Result, error) {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return engine.Result{}, fmt.Errorf("%w: %q is not a run directory", engine.ErrDir, dir)
	}

	stderr := &tailWriter{max: maxStderrTail}
	// The argv went through ffargs.Check in RunFFmpeg and the program is the discovered FFmpeg,
	// alone or inside the wrapper of Runner.command.
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = childEnv(dir, extraEnv...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	setProcessGroup(cmd)
	cmd.Cancel = func() error { return killProcessGroup(cmd) }
	cmd.WaitDelay = 5 * time.Second

	start := time.Now()
	err = cmd.Run()
	code := -1
	if cmd.ProcessState != nil {
		code = cmd.ProcessState.ExitCode()
	}
	res := engine.Result{Stderr: stderr.String(), ExitCode: code, Duration: time.Since(start)}
	if err == nil {
		return res, nil
	}
	return res, fmt.Errorf("%w: ffmpeg exited %d after %s: %w",
		engine.ErrRun, code, res.Duration.Round(time.Millisecond), err)
}

// tailWriter keeps the last max bytes written to it.
type tailWriter struct {
	max  int
	buf  []byte
	over bool
}

func (w *tailWriter) Write(p []byte) (int, error) {
	n := len(p)
	if len(p) >= w.max {
		w.buf = append(w.buf[:0], p[len(p)-w.max:]...)
		w.over = true
		return n, nil
	}
	if drop := len(w.buf) + len(p) - w.max; drop > 0 {
		w.buf = w.buf[drop:]
		w.over = true
	}
	w.buf = append(w.buf, p...)
	return n, nil
}

func (w *tailWriter) String() string {
	if w.over {
		return "[earlier output dropped]\n" + string(w.buf)
	}
	return string(w.buf)
}

// libraryEnv is EnvOpenH264Dir for the own build whose Cisco library passed verification; the sandbox
// builds the environment itself, so the variable that Tool.Env carries elsewhere is added here.
func (r *Runner) libraryEnv() []string {
	if r.In.OpenH264.Installed && r.In.Flavour().Name == FlavourOwn {
		return []string{EnvOpenH264Dir + "=" + r.In.OpenH264.Dir}
	}
	return nil
}
