package workspace

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRuntimeRoundTripAndSixFields(t *testing.T) {
	t.Parallel()
	d := newDir(t)

	if _, err := d.ReadRuntime(); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("ReadRuntime on absent file = %v, want ErrNotExist", err)
	}

	want := Runtime{
		PID:       18344,
		Addr:      "127.0.0.1:7878",
		Mode:      ModeLocal,
		StartedAt: time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC),
		Version:   "0.5.0",
	}
	if err := d.WriteRuntime(want); err != nil {
		t.Fatal(err)
	}
	got, err := d.ReadRuntime()
	if err != nil {
		t.Fatal(err)
	}
	want.Schema = RuntimeSchema
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}

	raw, err := os.ReadFile(d.Path("runtime.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"schema", "pid", "addr", "mode", "started_at", "version"} {
		if _, ok := fields[k]; !ok {
			t.Errorf("field %q missing", k)
		}
	}
	if len(fields) != 6 {
		t.Errorf("runtime.json has %d fields, want exactly 6", len(fields))
	}

	entries, _ := os.ReadDir(string(d))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("temporary file left behind: %s", e.Name())
		}
	}

	if err := d.RemoveRuntime(); err != nil {
		t.Fatal(err)
	}
	if err := d.RemoveRuntime(); err != nil {
		t.Errorf("RemoveRuntime on absent file = %v, want nil", err)
	}
	if _, err := d.ReadRuntime(); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("after remove: %v", err)
	}
}

func TestReadRuntimeRetriesParseError(t *testing.T) {
	t.Parallel()
	d := newDir(t)
	if err := os.WriteFile(d.Path("runtime.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The writer replaces the file when the reader first retries: no sleeps, so
	// a loaded machine cannot move the write outside the retry window.
	var once sync.Once
	write := func() {
		once.Do(func() {
			_ = d.WriteRuntime(Runtime{
				PID:       1,
				Addr:      "127.0.0.1:1",
				Mode:      ModeLocal,
				StartedAt: time.Now(),
				Version:   "x",
			})
		})
	}
	if _, err := d.readRuntime(write); err != nil {
		t.Fatalf("ReadRuntime should have waited for the writer: %v", err)
	}

	if err := os.WriteFile(d.Path("runtime.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	retries := 0
	if _, err := d.readRuntime(func() { retries++ }); err == nil {
		t.Fatal("a file that stays broken must be an error")
	}
	// The loop ends at the deadline; the upper bound is left out on purpose,
	// a loaded machine only makes it longer.
	if el := time.Since(start); el < readRetryFor || retries < 2 {
		t.Errorf("retried %d times for %v, want at least 2 times and %v", retries, el, readRetryFor)
	}
}

func TestProcessAlive(t *testing.T) {
	t.Parallel()
	if !ProcessAlive(os.Getpid()) {
		t.Error("the current process must be alive")
	}
	if ProcessAlive(0) || ProcessAlive(-1) {
		t.Error("pid <= 0 is never alive")
	}
	if ProcessAlive(deadPID(t)) {
		t.Error("an exited process must not be alive")
	}
}

// deadPID returns the pid of a process that has exited.
func deadPID(t *testing.T) int {
	t.Helper()
	c := startLockChild(t, newDir(t))
	if !c.gotWithin(10 * time.Second) {
		t.Fatal("child did not start")
	}
	pid := c.cmd.Process.Pid
	c.stop()
	return pid
}
