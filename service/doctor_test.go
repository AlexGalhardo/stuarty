package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/internal/ffmpegpins"
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

// A second diagnostic run does not start while one is in progress: it is told
// to wait (E_QUEUE_FULL) and hashes nothing.
func TestDoctorRunsOneAtATime(t *testing.T) {
	t.Parallel()

	s := &svc{}
	ctx := WithCaller(context.Background(), Caller{Actor: "local:cli", Scopes: AllScopes, Operator: true})

	s.doctorMu.Lock()

	_, err := s.Doctor(ctx, DoctorRequest{Verify: true})
	if se, ok := errors.AsType[*Error](err); !ok || se.Code != CodeQueueFull {
		t.Fatalf("a second run = %v, want E_QUEUE_FULL", err)
	}

	s.doctorMu.Unlock()

	if _, err := s.Doctor(ctx, DoctorRequest{Verify: true}); err != nil {
		t.Errorf("a run after the first ended = %v", err)
	}
}

// The pins the release embeds are real digests: a file that is not the pinned binary is reported
// as a mismatch, and one whose digest equals the pin verifies.
func TestDoctorVerifyAgainstTheEmbeddedPins(t *testing.T) {
	t.Parallel()

	pins := ffmpegpins.For("linux", "amd64")
	if pins["ffmpeg"] == "" || pins["ffprobe"] == "" {
		t.Fatalf("the release embeds no pin for linux/amd64: %v", pins)
	}

	path := filepath.Join(t.TempDir(), "ffprobe")
	if err := os.WriteFile(path, []byte("not the pinned build"), 0o600); err != nil {
		t.Fatal(err)
	}

	s := &svc{doctor: DoctorOptions{FFmpegPins: pins}}
	if c := s.digestCheck("ffprobe", path, false); c.OK || c.Hint == "" {
		t.Errorf("a file that is not the pinned build passed: %+v", c)
	}

	sum, err := fileSHA256(path)
	if err != nil {
		t.Fatal(err)
	}

	s = &svc{doctor: DoctorOptions{FFmpegPins: map[string]string{"ffprobe": sum}}}
	if c := s.digestCheck("ffprobe", path, false); !c.OK {
		t.Errorf("a file equal to its pin did not verify: %+v", c)
	}
}

func TestDoctorSaysWhenTheGPUHostIsMissing(t *testing.T) {
	s := &svc{doctor: DoctorOptions{GPURenderer: GPURenderer{Requested: true}}}

	got := s.rendererChecks(true)
	if len(got) != 1 || got[0].Name != "export.renderer" || !got[0].OK ||
		got[0].Detail != "gpu requested, vukem-render not found, using cpu" {
		t.Fatalf("checks = %+v", got)
	}

	s.doctor.GPURenderer.Host = "/x/vukem-render"
	if got := s.rendererChecks(
		true,
	); len(got) != 1 ||
		got[0].Detail != "gpu requested, vukem-render at /x/vukem-render" {
		t.Fatalf("checks with host = %+v", got)
	}

	s.doctor.GPURenderer = GPURenderer{}
	if got := s.rendererChecks(true); len(got) != 0 {
		t.Fatalf("not requested: %+v", got)
	}
}

// The JSON status of a check is the word the text shows for it (VV-212).
func TestDoctorStatusAgreesWithTheText(t *testing.T) {
	t.Parallel()

	cases := []struct {
		check DoctorCheck
		want  string
		word  string
	}{
		{DoctorCheck{Name: "a", OK: true}, "ok", "ok a"},
		{DoctorCheck{Name: "b", OK: true, Hint: "h"}, "warn", "warn b"},
		{DoctorCheck{Name: "c"}, "fail", "FAIL c"},
		{DoctorCheck{Name: "d", Optional: true}, "missing", "missing d"},
	}

	for _, tc := range cases {
		if got := tc.check.status(); got != tc.want {
			t.Errorf("%s: status %q, want %q", tc.check.Name, got, tc.want)
		}

		if text := doctorText(&DoctorResult{Checks: []DoctorCheck{tc.check}}); !strings.Contains(text, tc.word) {
			t.Errorf("%s: text %q lacks %q", tc.check.Name, text, tc.word)
		}
	}
}
