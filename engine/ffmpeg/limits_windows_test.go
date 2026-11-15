//go:build windows

package ffmpeg

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/vukem/contrib/engine"
)

// jobRunner is a helper runner whose tool runs in a Job Object with these limits.
func jobRunner(t *testing.T, l engine.Limits) (*Runner, *RunDir) {
	t.Helper()

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

func runJobHelper(r *Runner, dir *RunDir, mode string) (string, error) {
	var out bytes.Buffer

	_, err := r.exec(context.Background(), execSpec{
		tool: r.In.FFmpeg, dir: dir.Dir(), argv: []string{helperFlag + mode}, deadline: time.Minute, stdout: &out,
	})

	return out.String(), err
}

func TestWindowsLimitsForIsTheLinuxTableWithoutFileSize(t *testing.T) {
	got := LimitsFor(KindExportSoftware, 90*time.Second, 2, 1<<30)
	want := engine.Limits{CPUTime: 180 * time.Second, AddressSpace: 8 << 30}

	if got != want {
		t.Errorf("LimitsFor(export) = %+v, want %+v (a Job Object has no file size limit)", got, want)
	}

	if LimitsFor(KindExportHardware, time.Minute, 2, 0).AddressSpace != 0 {
		t.Error("a hardware export must have no address space limit")
	}

	if ok, why := LimitsAvailable(); !ok {
		t.Errorf("LimitsAvailable() = false, %q", why)
	}
}

func TestWindowsJobObjectLimit(t *testing.T) {
	// 512 MiB: far below the 2 GiB the helper allocates, far above what a Go helper needs to start.
	l := engine.Limits{AddressSpace: 512 << 20}
	r, dir := jobRunner(t, l)

	out, err := runJobHelper(r, dir, "alloc")
	if !errors.Is(err, engine.ErrRun) {
		t.Fatalf("a helper over the limit ended with %v, want engine.ErrRun", err)
	}

	if strings.Contains(out, "allocated") {
		t.Errorf("the helper allocated past the limit:\n%s", out)
	}
}

func TestWindowsJobObjectLimitLetsAWellBehavedRunPass(t *testing.T) {
	r, dir := jobRunner(t, LimitsFor(KindExportSoftware, time.Minute, 2, 0))

	out, err := runJobHelper(r, dir, "alloc")
	if err != nil || !strings.Contains(out, "allocated 2048") {
		t.Fatalf("a helper under the 8 GiB limit: out %q, err %v", out, err)
	}
}

func TestWindowsJobObjectLimitLeavesNoProcess(t *testing.T) {
	r, dir := jobRunner(t, engine.Limits{AddressSpace: 512 << 20})

	if _, err := runJobHelper(r, dir, "alloctree"); !errors.Is(err, engine.ErrRun) {
		t.Fatalf("error = %v, want engine.ErrRun", err)
	}

	b, err := os.ReadFile(filepath.Join(dir.Dir(), "pids"))
	if err != nil {
		t.Fatal(err)
	}

	var parent, child int
	if n, _ := fmt.Sscanf(string(b), "%d %d", &parent, &child); n != 2 {
		t.Fatalf("pids file = %q", b)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && (processAlive(parent) || processAlive(child)) {
		time.Sleep(20 * time.Millisecond)
	}

	if processAlive(parent) || processAlive(child) {
		t.Errorf("the tree outlived the run: parent %v, grandchild %v", processAlive(parent), processAlive(child))
	}
}
