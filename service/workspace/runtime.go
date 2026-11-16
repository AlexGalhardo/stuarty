package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"runtime"
	"time"
)

// RuntimeSchema is the value of the schema field of runtime.json.
const RuntimeSchema = 1

// Modes of an owner process (SDD-007 section 4).
const (
	ModeLocal   = "local"
	ModeNetwork = "network"
)

// Runtime is runtime.json: exactly six fields and no secret (SDD-005 section 6.1).
type Runtime struct {
	Schema    int       `json:"schema"`
	PID       int       `json:"pid"`
	Addr      string    `json:"addr"`
	Mode      string    `json:"mode"`
	StartedAt time.Time `json:"started_at"`
	Version   string    `json:"version"`
}

const (
	readRetryFor   = 200 * time.Millisecond
	readRetryEvery = 20 * time.Millisecond
	renameTries    = 10
	renameEvery    = 100 * time.Millisecond
)

// ReadRuntime reads runtime.json. It returns an error wrapping fs.ErrNotExist
// when the file is absent. A parse error is retried for 200 ms because the
// writer may be replacing the file (SDD-005 section 6.2).
func (d Dir) ReadRuntime() (Runtime, error) {
	return d.readRuntime(nil)
}

// readRuntime is ReadRuntime with a test seam: onRetry runs before each pause.
func (d Dir) readRuntime(onRetry func()) (Runtime, error) {
	deadline := time.Now().Add(readRetryFor)
	for {
		rt, err := d.readRuntimeOnce()
		if err == nil || errors.Is(err, fs.ErrNotExist) || !time.Now().Before(deadline) {
			return rt, err
		}
		if onRetry != nil {
			onRetry()
		}
		time.Sleep(readRetryEvery)
	}
}

func (d Dir) readRuntimeOnce() (Runtime, error) {
	data, err := os.ReadFile(d.Path("runtime.json"))
	if err != nil {
		return Runtime{}, err
	}
	var rt Runtime
	if err := json.Unmarshal(data, &rt); err != nil {
		return Runtime{}, fmt.Errorf("workspace: runtime.json: %w", err)
	}
	if rt.Schema != RuntimeSchema {
		return Runtime{}, fmt.Errorf("workspace: runtime.json has schema %d, expected %d", rt.Schema, RuntimeSchema)
	}
	return rt, nil
}

// WriteRuntime writes runtime.json atomically: a temporary file in the same
// directory, then a rename. Call it only after the listener is bound.
func (d Dir) WriteRuntime(r Runtime) error {
	r.Schema = RuntimeSchema
	data, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("workspace: %w", err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(string(d), "runtime-*.tmp")
	if err != nil {
		return fmt.Errorf("workspace: %w", err)
	}
	tmpName := tmp.Name()
	_, werr := tmp.Write(data)
	serr := tmp.Sync()
	cerr := tmp.Close()
	if err := errors.Join(werr, serr, cerr); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("workspace: %w", err)
	}

	if err := retryOnWindows(func() error { return os.Rename(tmpName, d.Path("runtime.json")) }); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("workspace: %w", err)
	}
	return nil
}

// RemoveRuntime deletes runtime.json and the per-run secret beside it; an absent file is not an
// error.
func (d Dir) RemoveRuntime() error {
	err := retryOnWindows(func() error {
		err := os.Remove(d.Path("runtime.json"))
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	})
	if err != nil {
		return fmt.Errorf("workspace: %w", err)
	}
	return d.removeRuntimeSecret()
}

// RemoveStaleRuntime deletes runtime.json unless it names a live process. Call
// it where the workspace lock is not held: the file may belong to another owner
// that is still serving, and removing it would hide that owner from the CLI.
func (d Dir) RemoveStaleRuntime() error {
	rt, err := d.ReadRuntime()
	if err == nil && ProcessAlive(rt.PID) {
		return nil
	}

	return d.RemoveRuntime()
}

// retryOnWindows retries up to 10 times over 1 second: a rename or delete
// fails there while another process has the file open without delete sharing.
func retryOnWindows(op func() error) error {
	err := op()
	if runtime.GOOS != "windows" {
		return err
	}
	for i := 1; err != nil && i < renameTries; i++ {
		time.Sleep(renameEvery)
		err = op()
	}
	return err
}
