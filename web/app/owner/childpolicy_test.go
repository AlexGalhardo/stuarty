package owner

import (
	"context"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"

	"github.com/AlexGalhardo/vukem/contrib/engine"
)

func TestChildRunnersRefuseAnUnusableSandbox(t *testing.T) {
	in := &ffmpeg.Installation{CPUCount: 2}

	for _, mode := range []string{"docker", "chroot"} {
		if _, err := newChildRunners(context.Background(), in, t.TempDir(), mode, 1<<30); err == nil ||
			!strings.Contains(err.Error(), "sandbox requested but unavailable") {
			t.Errorf("STUARTY_SANDBOX=%s: err = %v", mode, err)
		}
	}

	if runtime.GOOS != "linux" {
		if _, err := newChildRunners(context.Background(), in, t.TempDir(), "bwrap", 1<<30); err == nil {
			t.Error("bwrap was accepted outside Linux")
		}
	}
}

// SDD-007 section 6.5: on Linux a missing prlimit is a boot error that names util-linux.
func TestChildRunnersNeedPrlimitOnLinux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("prlimit is required on Linux only")
	}

	t.Setenv("PATH", t.TempDir())

	_, err := newChildRunners(context.Background(), &ffmpeg.Installation{CPUCount: 2}, t.TempDir(), "", 1<<30)
	if err == nil || !strings.Contains(err.Error(), "util-linux") {
		t.Errorf("err = %v, want a boot error naming util-linux", err)
	}
}

// On macOS no limits are passed and the policy says why (SDD-007 section 6.5).
func TestChildRunnersWithoutLimitsOnMacOS(t *testing.T) {
	if runtime.GOOS == "linux" || runtime.GOOS == "windows" {
		t.Skip("Linux and Windows apply limits")
	}

	got, err := newChildRunners(context.Background(), &ffmpeg.Installation{CPUCount: 2}, t.TempDir(), "none", 1<<30)
	if err != nil {
		t.Fatal(err)
	}

	if got.Policy.Limits || got.Policy.LimitsWhy == "" || got.Policy.Sandbox != "" {
		t.Errorf("policy = %+v", got.Policy)
	}

	if got.Ingest == nil || got.Frame == nil || got.Export == nil || got.Ingest == got.Export {
		t.Errorf("runners = %+v", got)
	}
}

// On Windows the limits are applied through a Job Object, so the policy reports them (SDD-007 section 6.5).
func TestChildRunnersLimitedOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Job Objects exist on Windows only")
	}

	tool, err := engine.Find(context.Background(), engine.Config{
		Name: "self", Path: os.Args[0], VersionArgs: []string{"-test.list=^$"},
	})
	if err != nil {
		t.Fatal(err)
	}

	in := &ffmpeg.Installation{FFmpeg: tool, FFprobe: tool, CPUCount: 2}

	got, err := newChildRunners(context.Background(), in, t.TempDir(), "none", 1<<30)
	if err != nil {
		t.Fatal(err)
	}

	if !got.Policy.Limits || got.Policy.LimitsWhy != "" {
		t.Errorf("policy = %+v, want limits available", got.Policy)
	}
}
