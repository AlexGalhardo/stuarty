package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The addresses of upload sessions older than seven days are removed by the sweep (SDD-020 section 7.4);
// a younger one is kept, so a publication that failed this week still resumes.
func TestSweepUploadSessionsRemovesOnlyTheStaleOnes(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	uploads := filepath.Join(dir, "publish", "uploads")

	if err := os.MkdirAll(filepath.Join(uploads, "keep-dir"), 0o750); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)

	for name, age := range map[string]time.Duration{
		"stale.json":        8 * 24 * time.Hour,
		".session-123":      9 * 24 * time.Hour,
		"fresh.json":        6 * 24 * time.Hour,
		"just-in-time.json": 7 * 24 * time.Hour,
	} {
		path := filepath.Join(uploads, name)
		if err := os.WriteFile(path, []byte(`{"uri":"x"}`), 0o600); err != nil {
			t.Fatal(err)
		}

		if err := os.Chtimes(path, now.Add(-age), now.Add(-age)); err != nil {
			t.Fatal(err)
		}
	}

	s := &svc{workspace: dir}

	n, err := s.sweepUploadSessions(now)
	if err != nil || n != 2 {
		t.Fatalf("removed %d, %v; want the 2 stale files", n, err)
	}

	for name, want := range map[string]bool{
		"stale.json": false, ".session-123": false, "fresh.json": true, "just-in-time.json": true, "keep-dir": true,
	} {
		if _, err := os.Stat(filepath.Join(uploads, name)); (err == nil) != want {
			t.Errorf("%s exists = %v, want %v", name, err == nil, want)
		}
	}

	empty := &svc{workspace: t.TempDir()}
	if n, err := empty.sweepUploadSessions(now); n != 0 || err != nil {
		t.Errorf("a workspace with no uploads folder: %d, %v", n, err)
	}
}
