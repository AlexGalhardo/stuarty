package workspace

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func env(home string) func(string) string {
	return func(k string) string {
		if k == EnvHome {
			return home
		}
		return ""
	}
}

func newDir(t *testing.T) Dir {
	t.Helper()
	d, err := Resolve(env(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestResolve(t *testing.T) {
	t.Parallel()

	t.Run("uses VUKEM_VIDEO_HOME and creates it", func(t *testing.T) {
		t.Parallel()
		home := filepath.Join(t.TempDir(), "ws")
		d, err := Resolve(env(home))
		if err != nil {
			t.Fatal(err)
		}
		if string(d) != home {
			t.Errorf("dir = %q, want %q", d, home)
		}
		if fi, err := os.Stat(home); err != nil || !fi.IsDir() {
			t.Errorf("workspace not created: %v", err)
		}
	})

	t.Run("defaults to the home directory", func(t *testing.T) {
		t.Parallel()
		home, err := os.UserHomeDir()
		if err != nil {
			t.Skip("no home directory")
		}
		// Resolve would create the real folder, so check only the path logic.
		want := filepath.Join(home, ".vukem-video")
		if _, err := os.Stat(want); err != nil {
			t.Skip("not creating the real default workspace in a test")
		}
		d, err := Resolve(func(string) string { return "" })
		if err != nil || string(d) != want {
			t.Errorf("Resolve = %q, %v; want %q", d, err, want)
		}
	})

	for _, bad := range []string{"a?b", "a#b", "a%b"} {
		t.Run("refuses "+bad, func(t *testing.T) {
			t.Parallel()
			home := filepath.Join(t.TempDir(), bad)
			if _, err := Resolve(env(home)); err == nil {
				t.Fatal("expected a refusal")
			}
			if _, err := os.Stat(home); err == nil {
				t.Error("a refused path must not be created")
			}
		})
	}
}

func TestEnsureLayout(t *testing.T) {
	t.Parallel()
	d := newDir(t)
	if err := d.EnsureLayout(); err != nil {
		t.Fatal(err)
	}
	for _, name := range Subdirectories {
		if fi, err := os.Stat(d.Path(name)); err != nil || !fi.IsDir() {
			t.Errorf("%s missing: %v", name, err)
		}
	}
}

// A workspace under a path with a space and a non-ASCII letter must work
// (the Windows profile of a user called "José Álvaro" is the real case).
func TestWorkspaceUnderSpaceAndNonASCIIPath(t *testing.T) {
	t.Parallel()
	home := filepath.Join(t.TempDir(), "José Álvaro", "vídeo workspace")
	d, err := Resolve(env(home))
	if err != nil {
		t.Fatal(err)
	}
	if err := d.EnsureLayout(); err != nil {
		t.Fatal(err)
	}
	l, err := d.TryLock()
	if err != nil {
		t.Fatalf("TryLock: %v", err)
	}
	defer l.Release()
	if _, err := d.TryLock(); !errors.Is(err, ErrLocked) {
		t.Errorf("second TryLock = %v, want ErrLocked", err)
	}
	if err := d.WriteRuntime(
		Runtime{PID: os.Getpid(), Addr: "127.0.0.1:1", Mode: ModeLocal, StartedAt: time.Now(), Version: "0.0.0"},
	); err != nil {
		t.Fatalf("WriteRuntime: %v", err)
	}
	token, err := d.EnsureToken()
	if err != nil {
		t.Fatalf("EnsureToken: %v", err)
	}
	if got, err := d.LoadToken(); err != nil || got != token {
		t.Errorf("LoadToken = %q, %v", got, err)
	}
}

func TestTryLockSameProcess(t *testing.T) {
	t.Parallel()
	d := newDir(t)

	l1, err := d.TryLock()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.TryLock(); !errors.Is(err, ErrLocked) {
		t.Fatalf("second TryLock = %v, want ErrLocked", err)
	}
	if err := l1.Release(); err != nil {
		t.Fatal(err)
	}
	if err := l1.Release(); err != nil {
		t.Errorf("second Release = %v, want nil", err)
	}
	l2, err := d.TryLock()
	if err != nil {
		t.Fatalf("TryLock after release: %v", err)
	}
	_ = l2.Release()

	if _, err := os.Stat(d.Path("lock")); err != nil {
		t.Errorf("the lock file is never deleted: %v", err)
	}
}

// SDD-005 section 18 check 22.
func TestTryLockTwoProcessesAndKill(t *testing.T) {
	if testing.Short() {
		t.Skip("starts child processes")
	}
	d := newDir(t)

	a := startLockChild(t, d)
	b := startLockChild(t, d)

	var winner, loser *child
	deadline := time.After(10 * time.Second)
	for winner == nil {
		select {
		case <-a.got:
			winner, loser = a, b
		case <-b.got:
			winner, loser = b, a
		case <-deadline:
			t.Fatal("no child acquired the lock")
		}
	}
	if loser.gotWithin(500 * time.Millisecond) {
		t.Fatal("both children hold the lock")
	}
	if _, err := d.TryLock(); !errors.Is(err, ErrLocked) {
		t.Fatalf("parent TryLock = %v, want ErrLocked", err)
	}

	// Kill, not a graceful stop: the OS must release the lock.
	if err := winner.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if !loser.gotWithin(time.Second) {
		t.Fatal("the other process did not acquire the lock within 1 s of the kill")
	}
}

// SDD-005 section 18 check 25: another process can open and read the lock
// file while it is held (antivirus, backup and indexers must not see a false
// "locked").
func TestLockFileReadableWhileHeld(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a child process")
	}
	d := newDir(t)
	c := startLockChild(t, d)
	if !c.gotWithin(10 * time.Second) {
		t.Fatal("child did not take the lock")
	}

	f, err := os.Open(d.Path("lock"))
	if err != nil {
		t.Fatalf("open while held: %v", err)
	}
	defer f.Close()
	buf := make([]byte, 16)
	if n, err := f.ReadAt(buf, 0); n != 0 || !errors.Is(err, io.EOF) {
		t.Errorf("ReadAt(0) = %d, %v; want 0, EOF", n, err)
	}
	if _, err := os.Stat(d.Path("lock")); err != nil {
		t.Errorf("stat while held: %v", err)
	}
	entries, err := os.ReadDir(string(d))
	if err != nil || !strings.Contains(strings.Join(names(entries), ","), "lock") {
		t.Errorf("lock not listed: %v", err)
	}
}

func names(es []os.DirEntry) []string {
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = e.Name()
	}
	return out
}
