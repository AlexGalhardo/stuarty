package ffmpeg

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// The GPU host stuarty-render is started by RunHelper, which is the same exec as FFmpeg: these tests run a
// helper through it and check the rules of SDD-007 6.4 to 6.6 that matter for a child that is not FFmpeg.

func helperRequest(
	t *testing.T,
	r *Runner,
	dir *RunDir,
	mode string,
	deadline time.Duration,
	out *bytes.Buffer,
) HelperRequest {
	t.Helper()

	tool, err := r.HelperTool(context.Background(), os.Args[0], deadline)
	if err != nil {
		t.Fatal(err)
	}

	return HelperRequest{Tool: tool, Dir: dir, Argv: []string{helperFlag + mode}, Deadline: deadline, Stdout: out}
}

func TestSecurity_T37_HelperEnvironmentMinimal(t *testing.T) {
	for _, k := range []string{"STUARTY_TRANSCRIBE_KEY", "APP_KEY", "STUARTY_KEY", "OTEL_EXPORTER_OTLP_ENDPOINT", "STUARTY_TOKEN"} {
		t.Setenv(k, "canary-"+k)
	}

	r, dir, _ := helperRunner(t)

	var out bytes.Buffer

	res, err := r.RunHelper(context.Background(), helperRequest(t, r, dir, "env", 30*time.Second, &out))
	if err != nil {
		t.Fatalf("%v (stderr %q)", err, res.Stderr)
	}

	if strings.Contains(out.String(), "canary-") {
		t.Fatalf("a server variable reached the helper:\n%s", out.String())
	}

	for _, k := range []string{"HOME=", "TMPDIR=", "TEMP=", "TMP="} {
		if !strings.Contains(out.String(), k) {
			t.Errorf("%s is not set to the run directory", k)
		}
	}

	if !strings.Contains(out.String(), "CWD=") {
		t.Error("the helper did not run in a directory")
	}
}

func TestSecurity_T37_HelperDeadlineKillsIt(t *testing.T) {
	r, dir, _ := helperRunner(t)

	start := time.Now()

	_, err := r.RunHelper(
		context.Background(),
		helperRequest(t, r, dir, "sleep", 500*time.Millisecond, &bytes.Buffer{}),
	)
	if !errors.Is(err, ErrDeadline) {
		t.Fatalf("error = %v, want ErrDeadline", err)
	}

	if took := time.Since(start); took > 30*time.Second {
		t.Errorf("the helper lived %s past a 500 ms deadline", took)
	}
}

func TestSecurity_T37_HelperCancelKillsIt(t *testing.T) {
	r, dir, _ := helperRunner(t)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() {
		_, err := r.RunHelper(ctx, helperRequest(t, r, dir, "sleep", time.Minute, &bytes.Buffer{}))
		done <- err
	}()

	time.Sleep(500 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a cancelled helper reported success")
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the helper outlived its cancel")
	}
}

func TestSecurity_T37_HelperRefusesRoot(t *testing.T) {
	r, dir, _ := helperRunner(t)
	req := helperRequest(t, r, dir, "sleep", 400*time.Millisecond, &bytes.Buffer{})

	old := geteuid
	t.Cleanup(func() { geteuid = old })

	geteuid = func() int { return 0 }

	t.Setenv(EnvAllowRoot, "")

	before := r.started.Load()

	if _, err := r.RunHelper(context.Background(), req); !errors.Is(err, ErrRefusedRoot) {
		t.Fatalf("error = %v, want ErrRefusedRoot", err)
	}

	if r.started.Load() != before {
		t.Error("a helper was started as root")
	}
}

func TestSecurity_T37_HelperRefusesRelativePathAndSandbox(t *testing.T) {
	r, dir, _ := helperRunner(t)

	var fe *Error
	if _, err := r.HelperTool(
		context.Background(),
		"stuarty-render",
		time.Minute,
	); !errors.As(err, &fe) ||
		fe.Reason != "helper_path" {
		t.Errorf("a relative helper path: error = %v, want reason helper_path", err)
	}

	req := helperRequest(t, r, dir, "sleep", 400*time.Millisecond, &bytes.Buffer{})
	r.Sandbox = "/usr/bin/bwrap"

	before := r.started.Load()

	if _, err := r.RunHelper(context.Background(), req); !errors.Is(err, ErrHelperSandboxed) {
		t.Errorf("a sandboxed runner: error = %v, want ErrHelperSandboxed", err)
	}

	if r.started.Load() != before {
		t.Error("a helper was started inside the sandbox wrapper")
	}
}
