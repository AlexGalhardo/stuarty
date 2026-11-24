package owner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLocateCaptureHelper(t *testing.T) {
	dir := t.TempDir()
	helper := filepath.Join(dir, "stuarty-capture.exe")

	if got := locateCaptureHelper("", dir, "windows"); got != "" {
		t.Errorf("nothing there, got %q", got)
	}

	if err := os.WriteFile(helper, []byte("x"), 0o700); err != nil {
		t.Fatal(err)
	}

	if got := locateCaptureHelper("", dir, "windows"); got != helper {
		t.Errorf("next to the binary: got %q", got)
	}

	if got := locateCaptureHelper("", dir, "linux"); got != "" {
		t.Errorf("another system looks for another name: got %q", got)
	}

	if got := locateCaptureHelper(helper, "", "windows"); got != helper {
		t.Errorf("configured: got %q", got)
	}

	if got := locateCaptureHelper("relative/stuarty-capture", dir, "windows"); got != "" {
		t.Errorf("a relative configured path is refused: got %q", got)
	}

	if got := locateCaptureHelper("", "relative", "windows"); got != "" {
		t.Errorf("a relative exec dir is refused: got %q", got)
	}
}

func TestRecordingSeconds(t *testing.T) {
	for raw, want := range map[string]int{"": 0, "60": 60, "1": 1, "14400": 14400} {
		if got, err := recordingSeconds(raw); err != nil || got != want {
			t.Errorf("%q: %d %v", raw, got, err)
		}
	}

	for _, raw := range []string{"0", "-1", "14401", "abc", "1.5"} {
		if _, err := recordingSeconds(raw); err == nil {
			t.Errorf("%q must be refused", raw)
		}
	}
}
