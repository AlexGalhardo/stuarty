package ffmpeg

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/AlexGalhardo/vukem/contrib/engine"
)

func TestWatchSizeKillsPastTheCap(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "out.bin")
	killed := make(chan struct{})

	go watchSize(context.Background(), path, 100, 10*time.Millisecond, func() { close(killed) })

	select {
	case <-killed:
		t.Fatal("killed before the file existed")
	case <-time.After(50 * time.Millisecond):
	}

	if err := os.WriteFile(path, make([]byte, 100), 0o600); err != nil {
		t.Fatal(err)
	}

	select {
	case <-killed:
		t.Fatal("killed at exactly the cap")
	case <-time.After(50 * time.Millisecond):
	}

	if err := os.WriteFile(path, make([]byte, 101), 0o600); err != nil {
		t.Fatal(err)
	}

	select {
	case <-killed:
	case <-time.After(5 * time.Second):
		t.Fatal("not killed past the cap")
	}
}

func TestSecurity_T17_OutputSizeWatchdog(t *testing.T) {
	r, dir, tool := helperRunner(t)

	_, err := r.exec(context.Background(), execSpec{
		tool: tool, dir: dir.Dir(), argv: []string{helperFlag + "grow"}, deadline: time.Minute,
		stdout: &bytes.Buffer{}, outPath: filepath.Join(dir.Dir(), "out.bin"), maxBytes: 3 << 20,
	})
	if !errors.Is(err, ErrOutputTooLarge) {
		t.Fatalf("error = %v, want ErrOutputTooLarge", err)
	}
}

// blockedWriter never accepts a byte until released: a child whose output cannot be drained
// keeps the run from ending after it is killed. reached closes on the first Write, which proves
// the child is running and has written: before that, a killed child ends the copy with EOF.
type blockedWriter struct {
	once    sync.Once
	release chan struct{}
	reached chan struct{}
	first   sync.Once
}

func (w *blockedWriter) Write(p []byte) (int, error) {
	w.first.Do(func() { close(w.reached) })
	<-w.release

	return len(p), nil
}

func (w *blockedWriter) free() { w.once.Do(func() { close(w.release) }) }

func TestCancelTimeoutFailsTheJob(t *testing.T) {
	r, dir, tool := helperRunner(t)
	r.Grace = 100 * time.Millisecond

	w := &blockedWriter{release: make(chan struct{}), reached: make(chan struct{})}
	t.Cleanup(w.free)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() {
		_, err := r.exec(ctx, execSpec{
			tool: tool, dir: dir.Dir(), argv: []string{helperFlag + "tree"}, deadline: time.Minute, stdout: w,
		})
		done <- err
	}()

	select {
	case <-w.reached:
	case <-time.After(30 * time.Second):
		t.Fatal("the child never wrote")
	}

	cancel()

	select {
	case err := <-done:
		var e *Error
		if !errors.As(err, &e) || e.Reason != ReasonCancelTimeout || e.Details["reason"] != ReasonCancelTimeout {
			t.Fatalf("error = %v, want the reason %s", err, ReasonCancelTimeout)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the run never returned")
	}
}

func TestSymlinkInTheStoreIsRefused(t *testing.T) {
	t.Parallel()

	ws := t.TempDir()
	store := filepath.Join(ws, "store")

	if err := os.Mkdir(store, 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(store, "real"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	outside := filepath.Join(ws, "outside")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.Symlink(outside, filepath.Join(store, "link")); err != nil {
		t.Skipf("no symbolic links here: %v", err)
	}

	r, err := NewRunner(&Installation{}, ws)
	if err != nil {
		t.Fatal(err)
	}

	f, err := r.OpenStoreFile("real")
	if err != nil {
		t.Fatal(err)
	}

	_ = f.Close()

	if f, err := r.OpenStoreFile("link"); !errors.Is(err, ErrSymlinkInStore) {
		if f != nil {
			_ = f.Close()
		}

		t.Errorf("a link in the store: error = %v, want ErrSymlinkInStore", err)
	}

	if f, err := r.OpenStoreFile(filepath.Join("..", "outside")); err == nil {
		_ = f.Close()

		t.Error("a name that leaves the store was opened")
	}
}

func TestOperatorLinkedStoreIsResolvedOnce(t *testing.T) {
	t.Parallel()

	ws, elsewhere := t.TempDir(), t.TempDir()
	if err := os.Symlink(elsewhere, filepath.Join(ws, "store")); err != nil {
		t.Skipf("no symbolic links here: %v", err)
	}

	r, err := NewRunner(&Installation{}, ws)
	if err != nil {
		t.Fatal(err)
	}

	want, _ := filepath.EvalSymlinks(elsewhere)
	if r.StoreDir != want {
		t.Errorf("StoreDir = %s, want the resolved %s", r.StoreDir, want)
	}
}

func TestLimitsTable(t *testing.T) {
	t.Parallel()

	const gib = 1 << 30

	if l := limitsFor(KindProbe, 30*time.Second, 4, 0); l.AddressSpace != 2*gib || l.CPUTime != 2*time.Minute {
		t.Errorf("probe limits %+v", l)
	}

	for _, k := range []RunKind{KindIngest, KindFrame, KindExportSoftware} {
		if l := limitsFor(k, time.Minute, 2, 5*gib); l.AddressSpace != 8*gib || l.FileSize != 5*gib {
			t.Errorf("kind %d limits %+v", k, l)
		}
	}

	if l := limitsFor(KindExportHardware, time.Minute, 2, 0); l.AddressSpace != 0 {
		t.Errorf("a hardware export has RLIMIT_AS %d, want none", l.AddressSpace)
	}
}

func TestNoLimitsGiveTheSameInstallation(t *testing.T) {
	t.Parallel()

	in := &Installation{}

	got, err := in.WithLimits(context.Background(), engine.Limits{})
	if err != nil || got != in {
		t.Errorf("WithLimits(zero) = %p, %v, want the same installation", got, err)
	}
}

// TestSecurity_T17_OutputSizeWatchdogRealFFmpeg runs the pinned FFmpeg with a raw output far over
// the cap and no -frames:v, so only the watchdog can stop it: the run is killed and the partial
// file is gone.
func TestSecurity_T17_OutputSizeWatchdogRealFFmpeg(t *testing.T) {
	r, dir := pinnedRunner(t)
	out := filepath.Join(dir.Dir(), "big.raw")

	start := time.Now()
	_, err := r.exec(context.Background(), execSpec{
		tool: r.In.FFmpeg, dir: dir.Dir(), deadline: time.Minute,
		argv: []string{
			"-hide_banner", "-v", "error", "-nostdin", "-f", "lavfi", "-i", "testsrc2=size=640x480:rate=30",
			"-f", "rawvideo", "-pix_fmt", "rgb24", "big.raw",
		},
		outPath: out, maxBytes: 4 << 20, pollEvery: 20 * time.Millisecond,
	})
	if !errors.Is(err, ErrOutputTooLarge) {
		t.Fatalf("error = %v, want ErrOutputTooLarge", err)
	}
	if time.Since(start) > 30*time.Second {
		t.Fatalf("the kill took %s", time.Since(start))
	}
	if _, serr := os.Stat(out); !errors.Is(serr, os.ErrNotExist) {
		t.Fatalf("the oversized output is still there: %v", serr)
	}
}
