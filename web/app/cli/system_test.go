package cli

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
	"github.com/AlexGalhardo/stuarty/engine/transcribe"
	"github.com/AlexGalhardo/stuarty/service"
)

// sysService answers the commands of system.go and records what they sent.
type sysService struct {
	fakeService

	jobReq    service.JobStartRequest
	doctorReq service.DoctorRequest
	doctor    *service.DoctorResult
	installs  int
	installed service.ToolsInstallRequest
	status    []service.ToolsStatusResult
	polls     int
}

func (s *sysService) JobStart(ctx context.Context, r service.JobStartRequest) (*service.Job, error) {
	s.note(ctx)
	s.jobReq = r

	return &service.Job{JobID: "job_1", State: service.JobQueued, Text: "job_1 queued"}, s.err
}

func (s *sysService) JobGet(context.Context, service.JobGetRequest) (*service.Job, error) {
	return &service.Job{JobID: "job_1", State: service.JobRunning, Text: "job_1 running"}, nil
}

func (s *sysService) Doctor(ctx context.Context, r service.DoctorRequest) (*service.DoctorResult, error) {
	s.note(ctx)
	s.doctorReq = r

	if s.err != nil {
		return nil, s.err
	}

	return s.doctor, nil
}

func (s *sysService) FontList(context.Context, service.FontListRequest) (*service.FontListResult, error) {
	return &service.FontListResult{Fonts: []service.FontFamily{{Family: "Inter", Weights: []int{400, 700}}}}, s.err
}

func (s *sysService) AuditList(_ context.Context, r service.AuditListRequest) (*service.AuditListResult, error) {
	next := "c2"

	return &service.AuditListResult{
		Entries: []service.AuditEntry{
			{ID: 7, At: "2026-11-15T10:00:00Z", Actor: "local:cli", Action: "project.create", ProjectID: r.ProjectID},
		},
		NextCursor: &next,
	}, s.err
}

func (s *sysService) ToolsInstall(
	ctx context.Context,
	r service.ToolsInstallRequest,
) (*service.ToolsInstallResult, error) {
	s.note(ctx)
	s.installs++
	s.installed = r

	if s.err != nil {
		return nil, s.err
	}

	return &service.ToolsInstallResult{
		State: "running", Files: []service.ToolFile{{Name: "whisper-cli", Bytes: 10, Host: "example.test"}},
		DownloadBytes: 10, Text: "installing whisper",
	}, nil
}

func (s *sysService) ToolsStatus(context.Context, service.ToolsStatusRequest) (*service.ToolsStatusResult, error) {
	st := s.status[min(s.polls, len(s.status)-1)]
	s.polls++

	return &st, nil
}

func testManifest() (transcribe.Manifest, error) {
	return transcribe.Manifest{
		Hosts: []string{"example.test"},
		Entries: []transcribe.ManifestEntry{
			{
				Name: "whisper", Kind: transcribe.KindHelper, OS: runtime.GOOS, Arch: runtime.GOARCH,
				URL: "https://example.test/whisper-cli", Bytes: 2_000_000, File: "whisper-cli",
			},
			{
				Name: "whisper", Kind: transcribe.KindModel, Model: "small",
				URL: "https://example.test/small.bin", Bytes: 3_000_000, File: "small.bin",
			},
		},
	}, nil
}

func newSysHarness(t *testing.T) (*harness, *sysService) {
	t.Helper()

	h := newHarness(t)
	s := &sysService{status: []service.ToolsStatusResult{{State: "idle", Text: "idle"}}}
	h.res.svc = s
	h.rt.Manifest = testManifest
	h.rt.Installer = func(string) (service.ToolsInstaller, error) { return s, nil }

	return h, s
}

func decodeOut(t *testing.T, h *harness) map[string]any {
	t.Helper()

	var doc map[string]any
	if err := json.Unmarshal(h.stdout.Bytes(), &doc); err != nil {
		t.Fatalf("not JSON: %q", h.stdout.String())
	}

	return doc
}

func TestCLITranscribe(t *testing.T) {
	t.Parallel()

	h, s := newSysHarness(t)

	code := h.run(
		"transcribe",
		"--project",
		"prj_7k2m9x4q1b",
		"--asset",
		"ast_3c5v8n2w6d",
		"--language",
		"en",
		"--model",
		"base",
		"--no-wait",
		"--json",
	)
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, h.stdout.String())
	}

	if s.jobReq.Kind != service.JobTranscribe || s.jobReq.ProjectID != "prj_7k2m9x4q1b" {
		t.Fatalf("request: %+v", s.jobReq)
	}

	if got := string(s.jobReq.Args); got != `{"asset_id":"ast_3c5v8n2w6d","language":"en","model":"base"}` {
		t.Fatalf("args: %s", got)
	}

	if code = h.run("transcribe", "--project", "prj_7k2m9x4q1b", "--json"); code != ExitFailure {
		t.Fatalf("missing --asset: exit %d", code)
	}

	if doc := decodeOut(t, h); doc["code"] != command.CodeValidation || doc["path"] != "asset" {
		t.Fatalf("missing --asset: %v", doc)
	}
}

func TestCLITranscribeWaitTimeoutIsExit2(t *testing.T) {
	t.Parallel()

	h, _ := newSysHarness(t)

	if code := h.run(
		"transcribe",
		"--project",
		"prj_7k2m9x4q1b",
		"--asset",
		"ast_1",
		"--timeout",
		"1ms",
	); code != ExitTransient {
		t.Fatalf("exit %d", code)
	}
}

func TestCLIDoctor(t *testing.T) {
	t.Parallel()

	t.Run("ffmpeg missing is exit 1 with the code and a hint", func(t *testing.T) {
		t.Parallel()

		h, s := newSysHarness(t)
		s.doctor = &service.DoctorResult{Checks: []service.DoctorCheck{{
			Name: "ffmpeg.binary", Detail: "E_FFMPEG_MISSING: no usable FFmpeg was found", Hint: "install FFmpeg 9",
		}}}

		if code := h.run("doctor", "--full", "--json"); code != ExitFailure {
			t.Fatalf("exit %d", code)
		}

		doc := decodeOut(t, h)
		if doc["code"] != "E_FFMPEG_MISSING" || doc["hint"] != "install FFmpeg 9" {
			t.Fatalf("document: %v", doc)
		}

		if !s.doctorReq.Full {
			t.Fatal("--full was not sent")
		}
	})

	t.Run("a failed check is exit 1 and the report is printed", func(t *testing.T) {
		t.Parallel()

		h, s := newSysHarness(t)
		s.doctor = &service.DoctorResult{Text: "doctor", Checks: []service.DoctorCheck{{Name: "disk", Detail: "low"}}}

		if code := h.run("doctor", "--full"); code != ExitFailure || !strings.Contains(h.stdout.String(), "doctor") {
			t.Fatalf("exit %d: %s", code, h.stdout.String())
		}
	})

	t.Run("the person at the terminal of the owner is the operator", func(t *testing.T) {
		t.Parallel()

		h, s := newSysHarness(t)
		h.res.inProcess = true
		s.doctor = &service.DoctorResult{Text: "doctor"}

		if code := h.run("doctor", "--gc", "--bundle", "--verify"); code != ExitOK {
			t.Fatalf("exit %d", code)
		}

		if !s.caller.Operator || !s.doctorReq.GC || !s.doctorReq.Bundle || !s.doctorReq.Verify {
			t.Fatalf("caller %+v request %+v", s.caller, s.doctorReq)
		}

		h.res.inProcess = false

		h.run("doctor")

		if s.caller.Operator {
			t.Fatal("a call through a server is not the operator")
		}
	})
}

func TestCLIFontsAndAudit(t *testing.T) {
	t.Parallel()

	h, _ := newSysHarness(t)

	if code := h.run("fonts"); code != ExitOK || !strings.Contains(h.stdout.String(), "Inter: [400 700]") {
		t.Fatalf("fonts: %d %q", code, h.stdout.String())
	}

	if code := h.run("audit", "--project", "prj_1"); code != ExitOK ||
		!strings.Contains(
			h.stdout.String(),
			"project.create prj_1",
		) || !strings.Contains(h.stdout.String(), "--cursor c2") {
		t.Fatalf("audit: %d %q", code, h.stdout.String())
	}
}

func TestCLIToolsInstall(t *testing.T) {
	t.Parallel()

	t.Run("answering n downloads nothing", func(t *testing.T) {
		t.Parallel()

		h, s := newSysHarness(t)
		h.rt.Interactive = true
		h.rt.Stdin = strings.NewReader("n\n")

		if code := h.run("tools:install", "whisper"); code != ExitOK || s.installs != 0 {
			t.Fatalf("exit %d, installs %d", code, s.installs)
		}

		if !strings.Contains(h.stderr.String(), "2 files, 5.0 MB from example.test") {
			t.Fatalf("the prompt does not state size and source: %q", h.stderr.String())
		}
	})

	t.Run("answering y installs the default model and waits for the end", func(t *testing.T) {
		t.Parallel()

		h, s := newSysHarness(t)
		h.res.inProcess = true
		h.rt.Interactive = true
		h.rt.Stdin = strings.NewReader("y\n")
		s.status = []service.ToolsStatusResult{
			{State: "running", Text: "running"},
			{State: "idle", Installed: []service.InstalledTool{{Name: "whisper", Kind: "helper"}}, Text: "installed"},
		}

		if code := h.run("tools:install", "whisper"); code != ExitOK {
			t.Fatalf("exit %d: %s", code, h.stdout.String())
		}

		if s.installs != 1 || s.installed.Model != "small" || !s.caller.Operator || s.polls != 2 {
			t.Fatalf("installs %d request %+v caller %+v polls %d", s.installs, s.installed, s.caller, s.polls)
		}
	})

	t.Run("the terminal installs while a server owns the workspace (VV-077)", func(t *testing.T) {
		t.Parallel()

		h, server := newSysHarness(t)
		here := &sysService{status: []service.ToolsStatusResult{{State: "idle", Text: "idle"}}}
		h.rt.Installer = func(dir string) (service.ToolsInstaller, error) {
			if dir != "/ws" {
				t.Errorf("installer built over %q", dir)
			}

			return here, nil
		}

		if code := h.run("tools:install", "whisper", "--yes", "--json"); code != ExitOK {
			t.Fatalf("exit %d: %s", code, h.stdout.String())
		}

		if here.installs != 1 || !here.caller.Operator || server.installs != 0 {
			t.Fatalf("terminal installs %d operator %v, server installs %d",
				here.installs, here.caller.Operator, server.installs)
		}
	})

	t.Run("--yes skips the question and --json prints one document", func(t *testing.T) {
		t.Parallel()

		h, s := newSysHarness(t)

		if code := h.run("tools:install", "whisper", "--model", "small", "--yes", "--json"); code != ExitOK {
			t.Fatalf("exit %d: %s", code, h.stdout.String())
		}

		if doc := decodeOut(t, h); doc["state"] != "idle" || doc["download_bytes"] != float64(10) || s.installs != 1 {
			t.Fatalf("document %v", doc)
		}
	})

	t.Run("openh264 is announced from the manifest of the own FFmpeg build", func(t *testing.T) {
		t.Parallel()

		manifest, err := ffmpeg.DefaultOpenH264Manifest()
		if err != nil {
			t.Fatal(err)
		}

		if entry, ok := manifest.For(runtime.GOOS, runtime.GOARCH); !ok || !entry.Published {
			t.Skipf("no published OpenH264 library for %s/%s", runtime.GOOS, runtime.GOARCH)
		}

		h, s := newSysHarness(t)

		if code := h.run("tools:install", "openh264", "--json"); code != ExitFailure || s.installs != 0 {
			t.Fatalf("exit %d, installs %d", code, s.installs)
		}

		doc := decodeOut(t, h)
		if msg, _ := doc["message"].(string); doc["path"] != "yes" ||
			!strings.Contains(msg, "ciscobinary.openh264.org") {
			t.Fatalf("document %v", doc)
		}

		if code := h.run(
			"tools:install",
			"openh264",
			"--yes",
			"--json",
		); code != ExitOK ||
			s.installed.Name != "openh264" {
			t.Fatalf("exit %d, request %+v", code, s.installed)
		}
	})

	t.Run("without a terminal and without --yes nothing is downloaded", func(t *testing.T) {
		t.Parallel()

		h, s := newSysHarness(t)

		if code := h.run("tools:install", "whisper", "--json"); code != ExitFailure || s.installs != 0 {
			t.Fatalf("exit %d, installs %d", code, s.installs)
		}

		if doc := decodeOut(t, h); doc["code"] != command.CodeValidation || doc["path"] != "yes" {
			t.Fatalf("document %v", doc)
		}
	})

	t.Run("--yes with an unknown helper is refused before any download", func(t *testing.T) {
		t.Parallel()

		h, s := newSysHarness(t)

		if code := h.run("tools:install", "no-such-helper", "--yes", "--json"); code != ExitFailure || s.installs != 0 {
			t.Fatalf("exit %d, installs %d", code, s.installs)
		}

		if doc := decodeOut(t, h); doc["code"] != command.CodeValidation || doc["path"] != "name" {
			t.Fatalf("document %v", doc)
		}
	})

	t.Run("an unknown model lists the ones this build can install", func(t *testing.T) {
		t.Parallel()

		h, s := newSysHarness(t)

		if code := h.run(
			"tools:install",
			"whisper",
			"--model",
			"huge",
			"--yes",
			"--json",
		); code != ExitFailure ||
			s.installs != 0 {
			t.Fatalf("exit %d, installs %d", code, s.installs)
		}

		if doc := decodeOut(
			t,
			h,
		); doc["code"] != command.CodeValidation ||
			!strings.Contains(doc["hint"].(string), "small") {
			t.Fatalf("document %v", doc)
		}
	})

	t.Run("a failed download is the error of the command", func(t *testing.T) {
		t.Parallel()

		h, s := newSysHarness(t)
		s.status = []service.ToolsStatusResult{
			{State: "failed", Error: &service.Error{Code: "E_TRANSCRIBE_UNAVAILABLE", Message: "digest mismatch."}},
		}

		if code := h.run("tools:install", "whisper", "--yes", "--json"); code != ExitFailure {
			t.Fatalf("exit %d", code)
		}

		if doc := decodeOut(t, h); doc["code"] != "E_TRANSCRIBE_UNAVAILABLE" {
			t.Fatalf("document %v", doc)
		}
	})

	t.Run("an install that outlives --timeout is exit 2", func(t *testing.T) {
		t.Parallel()

		h, s := newSysHarness(t)
		s.status = []service.ToolsStatusResult{{State: "running", Text: "running"}}

		if code := h.run("tools:install", "whisper", "--yes", "--timeout", "1ms"); code != ExitTransient {
			t.Fatalf("exit %d", code)
		}
	})
}
