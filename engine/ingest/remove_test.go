package ingest

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// VV-387: on Windows a file that was just closed can stay locked for a while (a scanner or the indexer opens it), and
// the removal of the folder answers access denied or a sharing violation. RemoveArtifacts waits a bounded time.
func TestRemoveArtifactsWaitsForAFileThatIsStillHeld(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("verify:env-skip a file held open can be removed on this system, so there is nothing to wait for")
	}

	root := t.TempDir()
	hash := "cd34cd34cd34cd34cd34cd34cd34cd34cd34cd34cd34cd34cd34cd34cd34cd34"
	dir := filepath.Join(root, hash)

	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}

	held, err := os.Create(filepath.Join(dir, "proxy-1.mp4"))
	if err != nil {
		t.Fatal(err)
	}

	// The holder lets go while the removal is waiting, as a scanner does.
	release := time.AfterFunc(300*time.Millisecond, func() { _ = held.Close() })
	defer release.Stop()

	if err := RemoveArtifacts(root, hash); err != nil {
		t.Fatalf("the removal gave up while the file was held: %v", err)
	}

	if _, err := os.Stat(dir); err == nil {
		t.Error("the folder survived")
	}
}

func TestRemoveArtifactsStopsWaitingAndReturnsTheError(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("verify:env-skip a file held open can be removed on this system, so there is nothing to wait for")
	}

	root := t.TempDir()
	hash := "ef56ef56ef56ef56ef56ef56ef56ef56ef56ef56ef56ef56ef56ef56ef56ef56"
	dir := filepath.Join(root, hash)

	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}

	held, err := os.Create(filepath.Join(dir, "proxy-1.mp4"))
	if err != nil {
		t.Fatal(err)
	}

	defer held.Close()

	start := time.Now()
	err = RemoveArtifacts(root, hash)

	if err == nil {
		t.Fatal("a file that stays held must make the removal fail")
	}

	if waited := time.Since(start); waited > removeWait+2*time.Second {
		t.Errorf("waited %v, more than the bound of %v", waited, removeWait)
	}
}

func TestTransientRemoveErrorIsOnlyTheThreeCauses(t *testing.T) {
	if transientRemoveError(errors.New("something else")) {
		t.Error("an unknown error is not transient")
	}

	if transientRemoveError(nil) {
		t.Error("no error is not transient")
	}
}
