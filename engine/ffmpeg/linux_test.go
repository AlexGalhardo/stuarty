//go:build linux

package ffmpeg

import (
	"bytes"
	"context"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/vukem/contrib/engine"
)

// limitedRunner is a helper runner whose tool runs under the limits.
func limitedRunner(t *testing.T, l engine.Limits) (*Runner, *RunDir) {
	t.Helper()

	if ok, why := LimitsAvailable(); !ok {
		t.Skip(why)
	}

	tool, err := engine.Find(context.Background(), engine.Config{
		Name: "helper", Path: os.Args[0], VersionArgs: []string{helperFlag + "version"}, Limits: l,
	})
	if err != nil {
		t.Fatal(err)
	}

	r, err := NewRunner(&Installation{FFmpeg: tool, FFprobe: tool, CPUCount: 2, limits: l}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	dir, err := r.NewRunDir("job_1")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = dir.Remove() })

	return r, dir
}

func runHelper(t *testing.T, r *Runner, dir *RunDir, mode string, extra ...string) (string, error) {
	t.Helper()

	var out bytes.Buffer

	argv := append([]string{helperFlag + mode}, extra...)
	_, err := r.exec(context.Background(), execSpec{
		tool: r.In.FFmpeg, dir: dir.Dir(), argv: argv, deadline: time.Minute, stdout: &out, inputDirs: nil,
	})

	return out.String(), err
}

func TestLimitsAreAppliedToTheChild(t *testing.T) {
	r, dir := limitedRunner(t, LimitsFor(KindExportSoftware, 90*time.Second, 2, 1<<30))

	out, err := runHelper(t, r, dir, "limits")
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{"Max cpu time", "180", "Max file size", "1073741824", "Max address space", "8589934592"} {
		if !strings.Contains(out, want) {
			t.Errorf("/proc/self/limits has no %q:\n%s", want, out)
		}
	}
}

func TestNoAddressSpaceLimitForAHardwareEncoder(t *testing.T) {
	r, dir := limitedRunner(t, LimitsFor(KindExportHardware, time.Minute, 2, 1<<30))

	out, err := runHelper(t, r, dir, "limits")
	if err != nil {
		t.Fatal(err)
	}

	for line := range strings.SplitSeq(out, "\n") {
		if strings.HasPrefix(line, "Max address space") && !strings.Contains(line, "unlimited") {
			t.Errorf("RLIMIT_AS is set for a hardware encoder: %s", line)
		}
	}
}

func TestBwrapArgsMakeTheRunDirTheOnlyWritablePlace(t *testing.T) {
	t.Parallel()

	store, run, tool := t.TempDir(), t.TempDir(), t.TempDir()+"/ffmpeg"
	args := BwrapArgs(tool, store, run, nil, []string{"-i", "x"})
	joined := strings.Join(args, " ")

	for _, want := range []string{"--unshare-all", "--die-with-parent", "--new-session", "--ro-bind " + store + " " + store, "--bind " + run + " " + run} {
		if !strings.Contains(joined, want) {
			t.Errorf("no %q in %s", want, joined)
		}
	}

	if strings.Contains(joined, "--bind / ") || strings.Contains(joined, "--share-net") {
		t.Errorf("too much is shared: %s", joined)
	}

	if last := args[len(args)-2:]; last[0] != "-i" || last[1] != "x" {
		t.Errorf("the tool argv is not last: %v", args)
	}
}

func TestBwrapHasNoNetwork(t *testing.T) {
	path, err := ResolveSandbox(context.Background(), SandboxBwrap)
	if err != nil {
		t.Skipf("no usable bwrap: %v", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()

	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}

			_ = c.Close()
		}
	}()

	tool, err := engine.Find(context.Background(), engine.Config{
		Name: "helper", Path: os.Args[0], VersionArgs: []string{helperFlag + "version"},
	})
	if err != nil {
		t.Fatal(err)
	}

	r, err := NewRunner(&Installation{FFmpeg: tool, FFprobe: tool, CPUCount: 2}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	r.Sandbox = path

	dir, err := r.NewRunDir("job_1")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dir.Remove() }()

	// Outside the sandbox the loopback listener answers.
	plain := exec.Command(os.Args[0], helperFlag+"dial", ln.Addr().String())
	if out, err := plain.CombinedOutput(); err != nil {
		t.Fatalf("the control run could not dial: %v %s", err, out)
	}

	out, err := runHelper(t, r, dir, "dial", ln.Addr().String())
	if err == nil || strings.Contains(out, "dial ok") {
		t.Errorf("the sandboxed child reached a loopback listener: %v %q", err, out)
	}
}

func TestResolveSandboxRefusesAnUnknownMode(t *testing.T) {
	t.Parallel()

	if _, err := ResolveSandbox(context.Background(), "docker"); err == nil {
		t.Error("an unknown sandbox mode was accepted")
	}

	if p, err := ResolveSandbox(context.Background(), ""); p != "" || err != nil {
		t.Errorf("no mode: %q, %v", p, err)
	}
}
