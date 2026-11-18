//go:build linux

package ffmpeg

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"github.com/AlexGalhardo/vukem/contrib/engine"
)

// SandboxBwrap is the value of VUKEM_VIDEO_SANDBOX that wraps every tool in bubblewrap (SDD-007 6.7).
const SandboxBwrap = "bwrap"

// ResolveSandbox returns the path of bwrap for VUKEM_VIDEO_SANDBOX=bwrap, or "" for no sandbox.
// A bwrap that is missing or cannot create its namespaces is an error, so the owner process
// does not start with a sandbox that is not there.
func ResolveSandbox(ctx context.Context, mode string) (string, error) {
	switch mode {
	case "":
		return "", nil
	case SandboxBwrap:
	default:
		return "", fmt.Errorf("ffmpeg: VUKEM_VIDEO_SANDBOX=%q: the only value is %q", mode, SandboxBwrap)
	}

	path, err := exec.LookPath(SandboxBwrap)
	if err != nil {
		return "", fmt.Errorf("ffmpeg: VUKEM_VIDEO_SANDBOX=bwrap but bwrap is missing: install bubblewrap: %w", err)
	}

	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	probe := exec.CommandContext(cctx, path, "--unshare-all", "--ro-bind", "/", "/", "true")
	// Like every child: no application variable (SDD-007 6.4), and the probe has nothing to write to.
	probe.Dir = os.TempDir()
	probe.Env = childEnv(probe.Dir)
	if out, err := probe.CombinedOutput(); err != nil {
		return "", fmt.Errorf("ffmpeg: bwrap cannot create its namespaces (%s): %w", scrub(string(out), 200), err)
	}

	return path, nil
}

// BwrapArgs is the argv that runs tool with argv inside bubblewrap: every namespace unshared (so
// no network), the tool and its libraries and the store read-only, the run directory the only
// writable place, and the child dying with its parent.
func BwrapArgs(toolPath, storeDir, runDir string, inputDirs []string, argv []string) []string {
	args := []string{"--unshare-all", "--die-with-parent", "--new-session", "--proc", "/proc", "--dev", "/dev"}

	seen := map[string]bool{}
	ro := func(p string) {
		if p == "" || seen[p] {
			return
		}

		if _, err := os.Stat(p); err != nil {
			return
		}

		seen[p] = true

		args = append(args, "--ro-bind", p, p)
	}

	for _, p := range []string{"/usr", "/lib", "/lib64", "/bin", "/etc/ld.so.cache"} {
		ro(p)
	}

	ro(filepath.Dir(toolPath))
	ro(storeDir)

	for _, d := range inputDirs {
		ro(d)
	}

	args = append(args, "--bind", runDir, runDir, "--chdir", runDir, "--", toolPath)

	return append(args, argv...)
}

// prlimitArgs are the prlimit(1) options of the limits (the form contrib/engine uses).
func prlimitArgs(l engine.Limits) []string {
	var out []string

	if l.CPUTime > 0 {
		out = append(out, "--cpu="+strconv.FormatInt(int64((l.CPUTime+time.Second-1)/time.Second), 10))
	}

	if l.AddressSpace > 0 {
		out = append(out, "--as="+strconv.FormatInt(l.AddressSpace, 10))
	}

	if l.FileSize > 0 {
		out = append(out, "--fsize="+strconv.FormatInt(l.FileSize, 10))
	}

	return out
}

// command is the program and arguments of a run: the tool itself, or bwrap around it, with
// prlimit in front when the installation has limits.
func (r *Runner) command(s execSpec) (string, []string, error) {
	if r.Sandbox == "" {
		return s.tool.Path(), s.argv, nil
	}

	name, args := r.Sandbox, BwrapArgs(
		s.tool.Path(),
		r.StoreDir,
		s.dir,
		append(slices.Clone(s.inputDirs), r.In.OpenH264.Dir),
		s.argv,
	)

	if l := r.In.limits; l != (engine.Limits{}) {
		pl, err := exec.LookPath("prlimit")
		if err != nil {
			return "", nil, errors.Join(engine.ErrLimitsUnsupported, err)
		}

		name, args = pl, append(append(prlimitArgs(l), "--", name), args...)
	}

	return name, args, nil
}
