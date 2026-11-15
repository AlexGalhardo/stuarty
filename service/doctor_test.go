package service

import (
	"os"
	"path/filepath"
	"testing"
)

// doctor --verify on a bundled FFmpeg that was modified prints a mismatch; a
// user's own build is fingerprinted but never compared.
func TestDoctorVerifyMismatch(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "ffmpeg")
	if err := os.WriteFile(path, []byte("the shipped bytes"), 0o600); err != nil {
		t.Fatal(err)
	}

	sum, err := fileSHA256(path)
	if err != nil {
		t.Fatal(err)
	}

	s := &svc{doctor: DoctorOptions{FFmpegPins: map[string]string{"ffmpeg": sum}}}

	if c := s.digestCheck("ffmpeg", path, false); !c.OK {
		t.Errorf("the shipped file does not match its pin: %+v", c)
	}

	if err := os.WriteFile(path, []byte("modified"), 0o600); err != nil {
		t.Fatal(err)
	}

	if c := s.digestCheck("ffmpeg", path, false); c.OK || c.Hint == "" {
		t.Errorf("a modified file passed: %+v", c)
	}

	if c := s.digestCheck("ffmpeg", path, true); !c.OK {
		t.Errorf("a user's own build must not fail the comparison: %+v", c)
	}

	unpinned := &svc{}
	if c := unpinned.digestCheck("ffmpeg", path, false); c.OK {
		t.Errorf("a bundled file with no pin passed: %+v", c)
	}
}

func TestSecretVariablesAreRecognisedByName(t *testing.T) {
	t.Parallel()

	for _, n := range []string{"APP_KEY", "VUKEM_VIDEO_TRANSCRIBE_KEY", "VUKEM_VIDEO_ADMIN_PASSWORD", "SOME_NEW_TOKEN", "X_SECRET"} {
		if !isSecretVar(n) {
			t.Errorf("%s is not treated as a secret", n)
		}
	}

	for _, n := range []string{"VUKEM_VIDEO_MAX_EXPORTS", "VUKEM_VIDEO_HOME"} {
		if isSecretVar(n) {
			t.Errorf("%s is treated as a secret", n)
		}
	}
}
