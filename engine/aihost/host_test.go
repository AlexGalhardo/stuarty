package aihost

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/facetrack"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
	"github.com/AlexGalhardo/stuarty/engine/ingest/mediagen"
)

// TestMain makes the test binary the fake helper when it is started as one: the host runs the helper with a
// minimal environment, so the mode travels in the name of the model file (ok.onnx, fail.onnx, ...).
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "faces" || os.Args[1] == "probe") {
		os.Exit(fakeHelper(os.Args[1:]))
	}

	os.Exit(m.Run())
}

func flag(args []string, name string) string {
	for i := range len(args) - 1 {
		if args[i] == name {
			return args[i+1]
		}
	}

	return ""
}

const frameBytes = FrameSide * FrameSide * 3

func fakeHelper(args []string) int {
	out := bufio.NewWriter(os.Stdout)
	defer func() { _ = out.Flush() }()

	switch args[0] {
	case "--version":
		fmt.Fprintln(out, "stuarty-aihelper 0.0.0-fake")

		return 0
	case "probe":
		fmt.Fprintln(out, `{"type":"probe","protocol":1,"build":"fake","directml":true}`)

		return 0
	}

	mode := strings.TrimSuffix(filepath.Base(flag(args, "--model")), ".onnx")

	fmt.Fprintln(out, `{"type":"ready","protocol":1,"provider":"cpu","width":640,"height":640}`)
	_ = out.Flush()

	switch mode {
	case "fail":
		fmt.Fprintln(out, `{"type":"error","code":"model_load","message":"the model could not be loaded"}`)

		return 1
	case "garbage":
		fmt.Fprintln(out, "this is not json")

		return 0
	case "toomany":
		for i := range 100000 {
			fmt.Fprintf(out, `{"type":"frame","index":%d,"faces":[]}`+"\n", i)
		}

		return 0
	case "silent":
		// Block on a pipe nobody writes to: the host must stop the helper by its stall
		// timeout or its context, never by this process ending.
		pr, _, err := os.Pipe()
		if err != nil {
			return 2
		}

		var one [1]byte

		_, _ = pr.Read(one[:])

		return 0
	case "unknowncode":
		fmt.Fprintln(out, `{"type":"error","code":"/home/x/secret","message":"bad \u0007 path /home/x"}`)

		return 1
	}

	buf := make([]byte, frameBytes)
	n := 0

	for {
		if _, err := io.ReadFull(os.Stdin, buf); err != nil {
			break
		}

		switch mode {
		case "ok":
			// A face that moves 10 pixels to the right per frame, with the padding below the picture.
			fmt.Fprintf(
				out,
				`{"type":"frame","index":%d,"faces":[{"x":%d.4,"y":50.0,"w":80.0,"h":100.0,"score":0.912}]}`+"\n",
				n,
				100+10*n,
			)
		case "padding":
			fmt.Fprintf(
				out,
				`{"type":"frame","index":%d,"faces":[{"x":10.0,"y":630.0,"w":20.0,"h":10.0,"score":0.9}]}`+"\n",
				n,
			)
		}

		n++
	}

	fmt.Fprintf(out, `{"type":"done","frames":%d,"infer_ms":1}`+"\n", n)

	return 0
}

// pinned is a runner of the pinned FFmpeg, an artifacts folder holding a 3 s 320x180 clip at 10 fps, and a
// run directory. It skips when no pinned FFmpeg is installed (STUARTY_TEST_FFMPEG_DIR).
func pinned(t *testing.T) (r *ffmpeg.Runner, artifacts, clip string, run *ffmpeg.RunDir) {
	t.Helper()

	dir := os.Getenv("STUARTY_TEST_FFMPEG_DIR")
	if dir == "" {
		t.Skip("no pinned FFmpeg: set STUARTY_TEST_FFMPEG_DIR") // verify:env-skip no pinned FFmpeg on this machine
	}

	name := "ffmpeg"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}

	in, err := ffmpeg.Discover(context.Background(), ffmpeg.Config{
		FFmpegPath: filepath.Join(dir, name), CPUCount: runtime.NumCPU(),
	})
	if err != nil || in.Info.License.Name != "lgpl" {
		t.Skip("FFmpeg is not the pinned LGPL build") // verify:env-skip not the pinned build
	}

	ws := t.TempDir()

	if r, err = ffmpeg.NewRunner(in, ws); err != nil {
		t.Fatal(err)
	}

	artifacts = filepath.Join(ws, "artifacts", "aa")
	if err := os.MkdirAll(artifacts, 0o700); err != nil {
		t.Fatal(err)
	}

	gen := mediagen.Gen{
		Dir: artifacts,
		Run: func(ctx context.Context, dir string, args ...string) (string, error) {
			res, err := in.FFmpeg.Run(ctx, dir, args...)

			return res.Stderr, err
		},
	}

	if clip, err = gen.Clip(context.Background(), "clip.mp4", 320, 180, 10, 3); err != nil {
		t.Fatal(err)
	}

	if run, err = r.NewRunDir("aihost-test"); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = run.Remove() })

	return r, artifacts, clip, run
}

func host(t *testing.T, r *ffmpeg.Runner, mode string) *Host {
	t.Helper()

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	h, err := New(Config{
		Runner: r, Helper: exe, Runtime: filepath.Join(t.TempDir(), "onnxruntime.dll"),
		Model: filepath.Join(t.TempDir(), mode+".onnx"), Provider: ProviderCPU, StallTimeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}

	return h
}

func input(artifacts, clip string, run *ffmpeg.RunDir) *Input {
	return &Input{
		Source: clip, InputDirs: []string{artifacts}, WorkDir: run, Width: 320, Height: 180,
		StartMS: 0, EndMS: 3000, FPS: 5,
	}
}

func TestDetectMapsTheBoxesOfEveryFrame(t *testing.T) {
	r, artifacts, clip, run := pinned(t)

	var last, total int

	track, err := host(t, r, "ok").Detect(context.Background(), input(artifacts, clip, run), func(done, all int) {
		last, total = done, all
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := track.Validate(); err != nil {
		t.Fatal(err)
	}

	if len(track.Samples) != 15 || track.FPS != 5 || track.Provider != "cpu" || track.Model != ModelName {
		t.Fatalf(
			"track = %d samples, fps %d, provider %q, model %q",
			len(track.Samples),
			track.FPS,
			track.Provider,
			track.Model,
		)
	}

	if last != 15 || total != 15 {
		t.Errorf("progress %d of %d, want 15 of 15", last, total)
	}

	// The 320x180 source is scaled by 2 to 640x360: the box x of frame 0 is 100.4 px, so 100.4/640 of the width,
	// and its y is 50 of 360.
	f := track.Samples[0].Faces[0]
	if f.X != 1569 || f.Y != 1389 || f.W != 1250 || f.H != 2778 || f.Score != 912 {
		t.Errorf("frame 0 face = %+v", f)
	}

	if track.Samples[1].T != 200 || track.Samples[14].T != 2800 {
		t.Errorf("times %d and %d", track.Samples[1].T, track.Samples[14].T)
	}

	if track.Samples[1].Faces[0].X <= f.X {
		t.Error("the face does not move to the right")
	}
}

func TestDetectDropsABoxInThePadding(t *testing.T) {
	r, artifacts, clip, run := pinned(t)

	track, err := host(t, r, "padding").Detect(context.Background(), input(artifacts, clip, run), nil)
	if err != nil {
		t.Fatal(err)
	}

	for i, s := range track.Samples {
		if len(s.Faces) != 0 {
			t.Fatalf("sample %d kept a box that lies in the padding: %+v", i, s.Faces)
		}
	}
}

func TestDetectRefusesWhatTheHelperGetsWrong(t *testing.T) {
	r, artifacts, clip, run := pinned(t)

	for mode, reason := range map[string]string{
		"fail": "model_load", "garbage": "protocol", "toomany": "protocol", "unknowncode": "protocol",
	} {
		_, err := host(t, r, mode).Detect(context.Background(), input(artifacts, clip, run), nil)

		var e *Error
		if !errors.As(err, &e) || e.Reason != reason {
			t.Errorf("%s: error = %v, want reason %s", mode, err, reason)

			continue
		}

		if strings.ContainsAny(e.Message, "\x07") || strings.Contains(e.Message, "/home/x") && mode != "unknowncode" {
			t.Errorf("%s: the message is not printable text: %q", mode, e.Message)
		}
	}
}

func TestDetectKillsAHelperThatGoesSilent(t *testing.T) {
	r, artifacts, clip, run := pinned(t)

	start := time.Now()

	_, err := host(t, r, "silent").Detect(context.Background(), input(artifacts, clip, run), nil)

	var e *Error
	if !errors.As(err, &e) || e.Reason != "timeout" {
		t.Fatalf("error = %v, want the timeout", err)
	}

	if took := time.Since(start); took > 30*time.Second {
		t.Errorf("the stalled helper lived %s", took)
	}
}

func TestDetectStopsWhenCancelled(t *testing.T) {
	r, artifacts, clip, run := pinned(t)

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(300*time.Millisecond, cancel)

	_, err := host(t, r, "silent").Detect(ctx, input(artifacts, clip, run), nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestProbeReadsTheAnswer(t *testing.T) {
	r, _, _, run := pinned(t)

	dml, err := host(t, r, "ok").Probe(context.Background(), run)
	if err != nil || !dml {
		t.Fatalf("probe = %v, %v", dml, err)
	}
}

func TestInputIsValidated(t *testing.T) {
	ok := Input{Source: "/a", WorkDir: &ffmpeg.RunDir{}, Width: 10, Height: 10, StartMS: 0, EndMS: 1000, FPS: 5}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}

	for name, mut := range map[string]func(*Input){
		"no source": func(i *Input) { i.Source = "" },
		"fps":       func(i *Input) { i.FPS = 11 },
		"empty":     func(i *Input) { i.EndMS = 0 },
		"size":      func(i *Input) { i.Width = 0 },
		"huge":      func(i *Input) { i.Height = facetrack.MaxSide + 1 },
	} {
		in := ok
		mut(&in)

		if in.Validate() == nil {
			t.Errorf("%s: accepted", name)
		}
	}

	if got := (&Input{StartMS: 500, EndMS: 3000, FPS: 5}).Frames(); got != 13 {
		t.Errorf("Frames = %d, want 13", got)
	}

	if d := Deadline(&Input{StartMS: 0, EndMS: int64(10 * time.Hour / time.Millisecond)}); d != 2*time.Hour {
		t.Errorf("Deadline = %s, want 2h", d)
	}
}

func TestNewRefusesRelativePathsAndUnknownProviders(t *testing.T) {
	if _, err := New(Config{Runner: &ffmpeg.Runner{}, Helper: "helper", Runtime: "/a", Model: "/b"}); err == nil {
		t.Error("a relative helper path was accepted")
	}

	abs, _ := filepath.Abs("x")

	if _, err := New(
		Config{Runner: &ffmpeg.Runner{}, Helper: abs, Runtime: abs, Model: abs, Provider: "cuda"},
	); err == nil {
		t.Error("an unknown provider was accepted")
	}

	if _, err := New(Config{Helper: abs, Runtime: abs, Model: abs}); err == nil {
		t.Error("a missing runner was accepted")
	}
}

func TestTenthsAndScoresAreExactIntegers(t *testing.T) {
	for in, want := range map[string]int{"79.4": 794, "3": 30, "0": 0, "-1.0": -10, "5.55": 56, "100.04": 1000, "0.0": 0} {
		got, err := tenths(jsonNumber(in))
		if err != nil || got != want {
			t.Errorf("tenths(%s) = %d, %v; want %d", in, got, err, want)
		}
	}

	for _, bad := range []string{"", "1e3", "+1", "a", "1.2.3", "--1", "12345678901234567"} {
		if _, err := tenths(jsonNumber(bad)); err == nil {
			t.Errorf("tenths(%q) was accepted", bad)
		}
	}

	for in, want := range map[string]int{"0.912": 912, "1": 1000, "0": 0, "0.9": 900, "0.9995": 1000} {
		got, err := score(jsonNumber(in))
		if err != nil || got != want {
			t.Errorf("score(%s) = %d, %v; want %d", in, got, err, want)
		}
	}

	for _, bad := range []string{"1.001", "-0.1", "2"} {
		if _, err := score(jsonNumber(bad)); err == nil {
			t.Errorf("score(%q) was accepted", bad)
		}
	}
}

func TestParseLineLimits(t *testing.T) {
	faces := strings.Repeat(`{"x":1.0,"y":1.0,"w":1.0,"h":1.0,"score":0.5},`, MaxFaces+1)
	line := `{"type":"frame","index":0,"faces":[` + strings.TrimSuffix(faces, ",") + `]}`

	if _, _, err := parseLine([]byte(line)); err == nil {
		t.Error("a frame with too many faces was accepted")
	}

	if _, _, err := parseLine([]byte(`{"type":"frame","faces":[]}`)); err == nil {
		t.Error("a frame with no index was accepted")
	}

	if _, _, err := parseLine(
		[]byte(`{"type":"frame","index":0,"faces":[{"x":1.0,"y":1.0,"w":0.0,"h":1.0,"score":0.5}]}`),
	); err == nil {
		t.Error("an empty box was accepted")
	}
}

func jsonNumber(s string) json.Number { return json.Number(s) }
