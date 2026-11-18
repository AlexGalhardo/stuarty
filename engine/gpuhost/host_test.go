package gpuhost

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/vukem-video/core/plan"
	"github.com/AlexGalhardo/vukem-video/engine/bitmap"
	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg"
	"github.com/AlexGalhardo/vukem-video/engine/renderhost"
)

// A copy of the test binary named vukem-render answers as the GPU host does, in the mode its mode.txt names,
// so the process policy and the protocol are tested without a graphics adapter (the real child is tested by
// TestSecurity_T37_RealHostSurvivesHostileRequests and by the parity tests of engine/export).
func TestMain(m *testing.M) {
	if strings.TrimSuffix(strings.ToLower(filepath.Base(os.Args[0])), ".exe") == "vukem-render" {
		fakeChild()

		return
	}

	os.Exit(m.Run())
}

func fakeChild() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		_, _ = os.Stdout.WriteString("vukem-render fake\n")

		return
	}

	mode, _ := os.ReadFile(filepath.Join(filepath.Dir(os.Args[0]), "mode.txt"))

	switch string(mode) {
	case "noadapter":
		_, _ = os.Stderr.WriteString("no graphics adapter\n")
		os.Exit(3)
	case "garbage":
		_, _ = os.Stdout.Write([]byte{0, 0xff, 0xff, 0xff, 0xff})

		return
	}

	in := os.Stdin

	for {
		var head [5]byte
		if _, err := io.ReadFull(in, head[:]); err != nil {
			return
		}

		payload := make([]byte, binary.LittleEndian.Uint32(head[1:]))
		if _, err := io.ReadFull(in, payload); err != nil {
			return
		}

		switch kind := head[0]; {
		case kind == kindInit && string(mode) == "env":
			reply(0, []byte(strings.Join(os.Environ(), "\n")))
		case kind == kindInit:
			reply(0, []byte("fake adapter"))
		case kind != kindRender:
			reply(0, nil)
		default:
			switch string(mode) {
			case "hang":
				time.Sleep(time.Minute)
			case "huge":
				_, _ = os.Stdout.Write([]byte{0})
				_, _ = os.Stdout.Write(binary.LittleEndian.AppendUint32(nil, 0xffffffff))
			case "status":
				reply(77, []byte("a status nobody defined"))
			case "device":
				reply(8, []byte("device lost\nat C:\\secret\\path"))
			case "wrongsize":
				reply(0, []byte{1, 2, 3})
			case "missing":
				reply(4, []byte("clip-1"))
			default:
				reply(0, bytes.Repeat([]byte{9}, 2*2*4))
			}
		}
	}
}

func reply(status byte, body []byte) {
	_, _ = os.Stdout.Write(append(binary.LittleEndian.AppendUint32([]byte{status}, uint32(len(body))), body...))
}

type env struct {
	r    *ffmpeg.Runner
	dir  *ffmpeg.RunDir
	path string
}

// newEnv makes a runner, a run directory and a copy of the test binary named vukem-render in the given mode.
func newEnv(t *testing.T, mode string) env {
	t.Helper()

	r, err := ffmpeg.NewRunner(&ffmpeg.Installation{CPUCount: 2}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	dir, err := r.NewRunDir("gpu_1")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = dir.Remove() })

	self, err := os.Open(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = self.Close() }()

	bin := filepath.Join(t.TempDir(), "vukem-render")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}

	out, err := os.OpenFile(
		bin,
		os.O_CREATE|os.O_WRONLY,
		0o700,
	)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := io.Copy(out, self); err != nil {
		t.Fatal(err)
	}

	if err := out.Close(); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(filepath.Dir(bin), "mode.txt"), []byte(mode), 0o600); err != nil {
		t.Fatal(err)
	}

	return env{r, dir, bin}
}

func (e env) start(ctx context.Context, t *testing.T, deadline time.Duration) (*Host, error) {
	t.Helper()

	return Start(ctx, e.r, Config{Path: e.path, Dir: e.dir, Deadline: deadline})
}

func mustStart(t *testing.T, mode string) *Host {
	t.Helper()

	h, err := newEnv(t, mode).start(context.Background(), t, 2*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = h.Close() })

	return h
}

func frameInput() (renderhost.RenderInput, *image.RGBA) {
	return renderhost.RenderInput{
		Output: renderhost.Output{Width: 2, Height: 2, Scale: 1},
		Plan:   plan.FramePlan{Width: 2, Height: 2, Background: "#000000"},
	}, image.NewRGBA(image.Rect(0, 0, 2, 2))
}

func TestARoundTripFeedsTheChildAndReadsTheFrame(t *testing.T) {
	h := mustStart(t, "echo")

	if h.Adapter() != "fake adapter" {
		t.Errorf("adapter %q", h.Adapter())
	}

	if err := h.PutTexture("a", 1, 1, []byte{1, 2, 3, 4}, true); err != nil {
		t.Fatal(err)
	}

	if err := h.PutBitmap(
		"b",
		bitmap.Bitmap{Pix: []byte{1, 2, 3, 4}, Width: 1, Height: 1, NaturalW: 1, NaturalH: 1},
	); err != nil {
		t.Fatal(err)
	}

	if err := h.Release("a"); err != nil {
		t.Fatal(err)
	}

	in, dst := frameInput()
	if err := h.Render(in, dst); err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(dst.Pix, bytes.Repeat([]byte{9}, 16)) {
		t.Errorf("frame %v", dst.Pix)
	}

	if err := h.Close(); err != nil {
		t.Errorf("a clean end reported %v", err)
	}
}

func TestACallThatIsNotDrawableIsRefusedBeforeTheChild(t *testing.T) {
	h := mustStart(t, "echo")

	var re *renderhost.Error

	cases := map[string]error{
		"short pixels":  h.PutTexture("a", 4, 4, make([]byte, 8), false),
		"huge texture":  h.PutTexture("a", 9000, 1, nil, false),
		"long id":       h.Release(strings.Repeat("x", 65)),
		"negative size": h.PutTexture("a", -1, 1, nil, false),
	}
	for name, err := range cases {
		if !errors.As(err, &re) {
			t.Errorf("%s: error = %v, want a renderer error", name, err)
		}
	}

	in, _ := frameInput()
	if err := h.Render(in, image.NewRGBA(image.Rect(0, 0, 3, 3))); !errors.As(err, &re) {
		t.Errorf("a wrong destination: error = %v", err)
	}

	// The child was never asked, so it is still usable.
	in, dst := frameInput()
	if err := h.Render(in, dst); err != nil {
		t.Errorf("the child was lost: %v", err)
	}
}

func TestAReasonOfTheChildIsARendererErrorAndTheChildGoesOn(t *testing.T) {
	h := mustStart(t, "missing")
	in, dst := frameInput()

	err := h.Render(in, dst)

	re, ok := errors.AsType[*renderhost.Error](err)
	if !ok || re.Reason != renderhost.MissingTexture || re.Detail != "clip-1" {
		t.Fatalf("error = %v", err)
	}

	if err := h.PutTexture("a", 1, 1, []byte{1, 2, 3, 4}, true); err != nil {
		t.Errorf("a refused request ended the child: %v", err)
	}
}

func TestNoAdapterIsUnavailable(t *testing.T) {
	_, err := newEnv(t, "noadapter").start(context.Background(), t, time.Minute)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}
}

func TestAMissingOrRelativeHostIsUnavailable(t *testing.T) {
	e := newEnv(t, "echo")

	for name, path := range map[string]string{
		"missing":  filepath.Join(t.TempDir(), "vukem-render"),
		"relative": "vukem-render",
		"empty":    "",
	} {
		_, err := Start(context.Background(), e.r, Config{Path: path, Dir: e.dir, Deadline: time.Minute})
		if !errors.Is(err, ErrUnavailable) {
			t.Errorf("%s: error = %v, want ErrUnavailable", name, err)
		}
	}

	e.r.Sandbox = "/usr/bin/bwrap"
	if _, err := e.start(context.Background(), t, time.Minute); !errors.Is(err, ErrUnavailable) ||
		!errors.Is(err, ffmpeg.ErrHelperSandboxed) {
		t.Errorf("a sandboxed runner: error = %v", err)
	}
}

func TestSecurity_T37_ChildEnvironmentMinimal(t *testing.T) {
	for _, k := range []string{"VUKEM_VIDEO_TRANSCRIBE_KEY", "APP_KEY", "VUKEM_VIDEO_KEY", "OTEL_EXPORTER_OTLP_ENDPOINT", "VUKEM_VIDEO_TOKEN"} {
		t.Setenv(k, "canary-"+k)
	}

	h := mustStart(t, "env")

	if strings.Contains(h.Adapter(), "canary-") {
		t.Fatalf("a server variable reached the GPU host:\n%s", h.Adapter())
	}

	for _, k := range []string{"HOME=", "TMPDIR=", "TEMP=", "TMP="} {
		if !strings.Contains(h.Adapter(), k) {
			t.Errorf("%s is not set to the run directory", k)
		}
	}
}

// A child that answers with what the protocol does not allow fails the call, then every later call with the
// same error, and ends without a hang; nothing it says reaches an error unbounded or with a control character.
func TestSecurity_T37_HostileRepliesAreContained(t *testing.T) {
	for _, mode := range []string{"huge", "status", "device", "wrongsize"} {
		t.Run(mode, func(t *testing.T) {
			h := mustStart(t, mode)
			in, dst := frameInput()

			err := h.Render(in, dst)
			if err == nil {
				t.Fatal("a hostile reply was accepted")
			}

			if again := h.Render(in, dst); again == nil || again.Error() != err.Error() {
				t.Errorf("a later call: %v, want the same failure as %v", again, err)
			}

			if strings.ContainsAny(err.Error(), "\n\r") {
				t.Errorf("a control character reached the error: %q", err)
			}

			done := make(chan struct{})

			go func() { _ = h.Close(); close(done) }()

			select {
			case <-done:
			case <-time.After(30 * time.Second):
				t.Fatal("Close hung")
			}
		})
	}
}

func TestSecurity_T37_AGarbageAnswerToTheInitIsUnavailable(t *testing.T) {
	_, err := newEnv(t, "garbage").start(context.Background(), t, time.Minute)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}
}

func TestSecurity_T37_DeadlineKillsTheChild(t *testing.T) {
	e := newEnv(t, "hang")

	h, err := e.start(context.Background(), t, time.Second)
	if err != nil {
		t.Fatal(err)
	}

	in, dst := frameInput()
	start := time.Now()

	err = h.Render(in, dst)
	if !errors.Is(err, ffmpeg.ErrDeadline) {
		t.Fatalf("error = %v, want ErrDeadline", err)
	}

	if took := time.Since(start); took > 30*time.Second {
		t.Errorf("the child lived %s past a 1 s deadline", took)
	}

	_ = h.Close()
}

func TestSecurity_T37_CancelKillsTheChild(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h, err := newEnv(t, "hang").start(ctx, t, time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	in, dst := frameInput()
	done := make(chan error, 1)

	go func() { done <- h.Render(in, dst) }()

	time.Sleep(500 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("error = %v, want context.Canceled", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the child outlived its cancel")
	}

	if err := h.Close(); err != nil {
		t.Errorf("Close after a cancel: %v", err)
	}
}

// The real child, when it is built: a request the protocol does not allow ends it with one answer and a
// non-zero exit, never a hang and never a frame.
func TestSecurity_T37_RealHostSurvivesHostileRequests(t *testing.T) {
	bin := os.Getenv("VUKEM_VIDEO_TEST_GPU_HOST")
	if bin == "" {
		name := "vukem-render"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}

		bin = filepath.Join("..", "..", "renderer", "target", "release", name)
	}

	if _, err := os.Stat(bin); err != nil {
		// skip-env: gpu-adapter
		t.Skip("vukem-render is not built: set VUKEM_VIDEO_TEST_GPU_HOST or run bash scripts/build-renderer.sh native")
	}

	if out, err := exec.CommandContext(t.Context(), bin, "--probe").Output(); err != nil {
		// skip-env: gpu-adapter
		t.Skipf("no graphics adapter for the real host: %v (%s)", err, out)
	}

	init := append([]byte{0}, 8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	over := append([]byte{1}, binary.LittleEndian.AppendUint32(nil, 8192*8192*4+4097)...)

	for name, req := range map[string][]byte{
		"a request before the init": {4, 0, 0, 0, 0},
		"an unknown kind":           append(bytes.Clone(init), 200, 0, 0, 0, 0),
		"a length over the largest": append(bytes.Clone(init), over...),
	} {
		cmd := exec.CommandContext(t.Context(), bin)
		cmd.Stdin = bytes.NewReader(req)

		var stdout bytes.Buffer
		cmd.Stdout = &stdout

		done := make(chan error, 1)

		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}

		go func() { done <- cmd.Wait() }()

		select {
		case err := <-done:
			if err == nil {
				t.Errorf("%s: the host ended cleanly", name)
			}
		case <-time.After(60 * time.Second):
			_ = cmd.Process.Kill()

			t.Fatalf("%s: the host hung", name)
		}

		if last := lastStatus(stdout.Bytes()); last != statusProtocolFailure {
			t.Errorf("%s: the last answer has status %d, want the protocol failure", name, last)
		}
	}
}

// statusProtocolFailure is the status the real host answers a request the protocol does not allow with.
const statusProtocolFailure = 9

// lastStatus is the status of the last reply of a stream of replies, or -1 when it does not parse.
func lastStatus(b []byte) int {
	last := -1

	for len(b) > 0 {
		if len(b) < 5 {
			return -1
		}

		n := int(binary.LittleEndian.Uint32(b[1:5]))
		if len(b) < 5+n {
			return -1
		}

		last, b = int(b[0]), b[5+n:]
	}

	return last
}
