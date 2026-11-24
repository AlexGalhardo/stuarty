package capturehost

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
)

// A copy of the test binary named stuarty-capture answers as the helper does, in the mode its mode.txt names, so
// the process policy and the protocol are tested without a screen (the real helper is tested by its own
// integration test and by the end to end run of epic 61).
func TestMain(m *testing.M) {
	if strings.TrimSuffix(strings.ToLower(filepath.Base(os.Args[0])), ".exe") == "stuarty-capture" {
		fakeChild()

		return
	}

	os.Exit(m.Run())
}

func say(s string) { _, _ = os.Stdout.WriteString(s + "\n") }

func fakeChild() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		say("stuarty-capture fake")

		return
	}

	modeBytes, _ := os.ReadFile(filepath.Join(filepath.Dir(os.Args[0]), "mode.txt"))
	mode := string(modeBytes)

	switch mode {
	case "noready":
		hang()

		return
	case "badversion":
		say(`{"type":"ready","v":1,"helper":"x","os":"windows","display":true,"window":true}`)
		hang()

		return
	case "garbageready":
		say("hello")
		hang()

		return
	}

	say(`{"type":"ready","v":2,"helper":"fake","os":"windows","display":true,"window":true,` +
		`"audio":true,"camera":false,"events":true}`)

	in := bufio.NewReader(os.Stdin)

	for {
		line, err := in.ReadString('\n')
		if err != nil {
			return
		}

		switch {
		case strings.Contains(line, `"sources"`) && mode == "env":
			say(fmt.Sprintf(`{"type":"sources","displays":[],"windows":[{"id":"w1","name":%q,"width":1,"height":1}]}`,
				strings.Join(os.Environ(), "|")))
		case strings.Contains(line, `"sources"`):
			say(`{"type":"sources","displays":[{"id":"d1","name":"Display 1","width":1920,"height":1080}],` +
				`"windows":[{"id":"w2","name":"Bad\u001b[31m title\nline","width":800,"height":600}],` +
				`"microphones":[{"id":"m0123456789abcdef","name":"Mic\u001b[31m one","default":true}],` +
				`"cameras":[],"system_audio":true}`)
		case strings.Contains(line, `"start"`):
			_ = os.WriteFile(filepath.Join(filepath.Dir(os.Args[0]), "start.json"), []byte(line), 0o600)

			start(mode, in)

			return
		}
	}
}

// hang blocks until the host kills the process: a signal handler keeps the runtime from reporting a deadlock.
func hang() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c
}

func start(mode string, in *bufio.Reader) {
	switch mode {
	case "helpererror":
		say(`{"type":"error","code":"unsupported","message":"no \u001b[31mdisplay"}`)

		return
	case "hang":
		hang()

		return
	}

	say(`{"type":"started","width":1920,"height":1080,"fps":30,"tracks":[{"kind":"screen","start_ms":0},` +
		`{"kind":"microphone","start_ms":2}]}`)

	switch mode {
	case "malformed":
		say("this is not json")
		hang()
	case "oversize":
		say(strings.Repeat("x", 70000))
		hang()
	case "unknown":
		say(`{"type":"surprise"}`)
		hang()
	case "silent":
		hang()
	case "exit":
		_, _ = os.Stderr.WriteString("boom: the encoder died\n")
		os.Exit(1)
	default:
		for i := 1; i <= 3; i++ {
			say(
				fmt.Sprintf(
					`{"type":"progress","elapsed_ms":%d,"frames":%d,"dropped":0,"bytes":%d}`,
					i*100,
					i*3,
					i*1000,
				),
			)
		}

		say(`{"type":"level","microphone":420}`)

		paused := false

		for {
			line, err := in.ReadString('\n')

			switch {
			case err != nil || strings.Contains(line, `"stop"`):
				say(`{"type":"finished","reason":"stopped","frames":9,"dropped":1,"duration_ms":300,"bytes":9000,` +
					`"tracks":[{"kind":"screen","start_ms":0,"duration_ms":300,"bytes":9000,"lost":false},` +
					`{"kind":"microphone","start_ms":2,"duration_ms":310,"bytes":3000,"lost":true}],` +
					`"pauses":[{"from_ms":100,"to_ms":250}],"pointer_events":4}`)

				return
			case strings.Contains(line, `"pause"`) && !paused:
				paused = true

				say(`{"type":"paused","elapsed_ms":120}`)
			case strings.Contains(line, `"resume"`) && paused:
				paused = false

				say(`{"type":"resumed","elapsed_ms":120}`)
			case strings.Contains(line, `"pause"`) || strings.Contains(line, `"resume"`):
				say(`{"type":"error","code":"bad_request","message":"the recording is not in that state"}`)
			}
		}
	}
}

type env struct {
	r    *ffmpeg.Runner
	dir  *ffmpeg.RunDir
	path string
}

func newEnv(t *testing.T, mode string) env {
	t.Helper()

	r, err := ffmpeg.NewRunner(&ffmpeg.Installation{CPUCount: 2}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	dir, err := r.NewRunDir("rec_1")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = dir.Remove() })

	self, err := os.Open(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = self.Close() }()

	bin := filepath.Join(t.TempDir(), "stuarty-capture")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}

	out, err := os.OpenFile(bin, os.O_CREATE|os.O_WRONLY, 0o700)
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

func (e env) start(t *testing.T, cfg Config) (*Helper, error) {
	t.Helper()

	cfg.Path, cfg.Dir = e.path, e.dir
	if cfg.Deadline == 0 {
		cfg.Deadline = 2 * time.Minute
	}

	return Start(context.Background(), e.r, cfg)
}

func mustStart(t *testing.T, mode string, cfg Config) *Helper {
	t.Helper()

	h, err := newEnv(t, mode).start(t, cfg)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = h.Close() })

	return h
}

func request(t *testing.T) StartRequest {
	t.Helper()

	return StartRequest{
		Recording: "rec_abc", Kind: "display", SourceID: "d1", FPS: 30, Encoder: "h264_nvenc",
		FFmpeg: filepath.Join(t.TempDir(), "ffmpeg.exe"), Output: filepath.Join(t.TempDir(), "capture.mp4"),
	}
}

// last reads events until the stream ends and returns the terminal one.
func last(t *testing.T, ev <-chan Event) (Event, int) {
	t.Helper()

	var (
		l        Event
		progress int
	)

	timeout := time.After(30 * time.Second)

	for {
		select {
		case e, ok := <-ev:
			if !ok {
				return l, progress
			}

			if _, isP := e.(Progress); isP {
				progress++
			}

			l = e
		case <-timeout:
			t.Fatal("the event stream did not end")
		}
	}
}

func TestStartAndStopRecordsAndFinishes(t *testing.T) {
	h := mustStart(t, "ok", Config{})

	if !h.Display || !h.Window {
		t.Errorf("capabilities %v %v", h.Display, h.Window)
	}

	st, ev, err := h.Begin(context.Background(), request(t))
	if err != nil {
		t.Fatal(err)
	}

	if st.Width != 1920 || st.Height != 1080 || st.FPS != 30 {
		t.Errorf("started = %+v", st)
	}

	if err := h.Stop(); err != nil {
		t.Fatal(err)
	}

	if err := h.Stop(); err != nil && !errors.Is(err, ErrLost) {
		t.Errorf("a second stop: %v", err)
	}

	end, progress := last(t, ev)

	fin, ok := end.(Finished)
	if !ok || fin.Reason != "stopped" || fin.Frames != 9 || fin.Dropped != 1 || fin.DurationMS != 300 {
		t.Fatalf("end = %#v", end)
	}

	if progress != 3 {
		t.Errorf("progress events = %d", progress)
	}

	if err := h.Close(); err != nil {
		t.Errorf("Close = %v", err)
	}
}

func TestSourcesAreListedAndCleaned(t *testing.T) {
	h := mustStart(t, "ok", Config{})

	s, err := h.Sources(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if len(s.Displays) != 1 || s.Displays[0].Name != "Display 1" || s.Displays[0].Width != 1920 {
		t.Errorf("displays = %+v", s.Displays)
	}

	if len(s.Windows) != 1 || s.Windows[0].Name != "Bad?[31m title line" {
		t.Errorf("windows = %+v: control characters must not pass", s.Windows)
	}

	if len(s.Microphones) != 1 || s.Microphones[0].Name != "Mic?[31m one" || !s.Microphones[0].Default ||
		s.Microphones[0].ID != "m0123456789abcdef" {
		t.Errorf("microphones = %+v: control characters must not pass", s.Microphones)
	}

	if len(s.Cameras) != 0 || !s.SystemAudio {
		t.Errorf("cameras %+v, system audio %v", s.Cameras, s.SystemAudio)
	}

	if !h.Audio || h.Camera || !h.Events {
		t.Errorf("capabilities audio %v camera %v events %v", h.Audio, h.Camera, h.Events)
	}

	if err := h.Close(); err != nil {
		t.Errorf("Close = %v", err)
	}
}

// events reads a recording's events until the terminal one, keeping every one in order.
func events(t *testing.T, ev <-chan Event) []Event {
	t.Helper()

	var all []Event

	timeout := time.After(30 * time.Second)

	for {
		select {
		case e, ok := <-ev:
			if !ok {
				return all
			}

			all = append(all, e)
		case <-timeout:
			t.Fatal("the event stream did not end")
		}
	}
}

// next waits for the first event of the wanted type and returns it.
func next[T Event](t *testing.T, ev <-chan Event) T {
	t.Helper()

	timeout := time.After(30 * time.Second)

	for {
		select {
		case e, ok := <-ev:
			if !ok {
				t.Fatalf("the stream ended before a %T", *new(T))
			}

			if v, is := e.(T); is {
				return v
			}
		case <-timeout:
			t.Fatalf("no %T in 30 s", *new(T))
		}
	}
}

func TestPauseAndResumeAreConfirmedAndTheRecordingGoesOn(t *testing.T) {
	h := mustStart(t, "ok", Config{})

	st, ev, err := h.Begin(context.Background(), request(t))
	if err != nil {
		t.Fatal(err)
	}

	if len(st.Tracks) != 2 || st.Tracks[0].Kind != "screen" || st.Tracks[1].Kind != "microphone" ||
		st.Tracks[1].StartMS != 2 {
		t.Errorf("started tracks = %+v", st.Tracks)
	}

	if lv := next[Level](t, ev); lv.Microphone != 420 || lv.System != 0 {
		t.Errorf("level = %+v", lv)
	}

	if err := h.Pause(); err != nil {
		t.Fatal(err)
	}

	if p := next[Paused](t, ev); p.ElapsedMS != 120 {
		t.Errorf("paused = %+v", p)
	}

	// A second pause is refused by the helper; the refusal must not end the recording.
	if err := h.Pause(); err != nil {
		t.Fatal(err)
	}

	if r := next[Rejected](t, ev); !strings.Contains(r.Message, "not in that state") {
		t.Errorf("rejected = %+v", r)
	}

	if err := h.Resume(); err != nil {
		t.Fatal(err)
	}

	next[Resumed](t, ev)

	if err := h.Stop(); err != nil {
		t.Fatal(err)
	}

	all := events(t, ev)

	fin, ok := all[len(all)-1].(Finished)
	if !ok {
		t.Fatalf("the last event = %#v", all[len(all)-1])
	}

	if len(fin.Tracks) != 2 || fin.Tracks[1] != (TrackResult{
		Kind: "microphone", StartMS: 2, DurationMS: 310, Bytes: 3000, Lost: true,
	}) {
		t.Errorf("finished tracks = %+v", fin.Tracks)
	}

	if len(fin.Pauses) != 1 || fin.Pauses[0] != (PauseSpan{FromMS: 100, ToMS: 250}) || fin.PointerEvents != 4 {
		t.Errorf("pauses %+v, pointer events %d", fin.Pauses, fin.PointerEvents)
	}

	if err := h.Close(); err != nil {
		t.Errorf("Close = %v", err)
	}
}

func TestTheTracksAreAskedWithTheirFiles(t *testing.T) {
	e := newEnv(t, "ok")

	h, err := e.start(t, Config{})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = h.Close() })

	good := request(t)
	folder := filepath.Dir(good.Output)
	good.Microphone, good.MicrophoneOutput = "default", filepath.Join(folder, "microphone.wav")
	good.SystemAudio, good.SystemOutput = true, filepath.Join(folder, "system.wav")
	good.Camera, good.CameraOutput = "c0123456789abcdef", filepath.Join(folder, "camera.mp4")
	good.Events, good.EventsOutput = true, filepath.Join(folder, "events.jsonl")

	for name, mutate := range map[string]func(*StartRequest){
		"microphone without a file":  func(r *StartRequest) { r.MicrophoneOutput = "" },
		"system without a file":      func(r *StartRequest) { r.SystemOutput = "" },
		"a file for no track":        func(r *StartRequest) { r.Camera = "" },
		"events without a file":      func(r *StartRequest) { r.EventsOutput = "" },
		"another folder":             func(r *StartRequest) { r.CameraOutput = filepath.Join(t.TempDir(), "camera.mp4") },
		"a relative file":            func(r *StartRequest) { r.MicrophoneOutput = "microphone.wav" },
		"the wrong extension":        func(r *StartRequest) { r.SystemOutput = filepath.Join(folder, "system.mp3") },
		"a test microphone":          func(r *StartRequest) { r.Microphone = "fake" },
		"a microphone with a brace":  func(r *StartRequest) { r.Microphone = "{0.0.1}" },
		"a camera with a space":      func(r *StartRequest) { r.Camera = "a b" },
		"events with a media file":   func(r *StartRequest) { r.EventsOutput = filepath.Join(folder, "events.mp4") },
		"a camera named by the host": func(r *StartRequest) { r.Camera = "default-camera/../x" },
	} {
		r := good
		mutate(&r)

		if _, _, err := h.Begin(context.Background(), r); !errors.Is(err, ErrBadRequest) {
			t.Errorf("%s: err = %v", name, err)
		}
	}

	if _, ev, err := h.Begin(context.Background(), good); err != nil {
		t.Fatalf("a good request: %v", err)
	} else {
		_ = h.Stop()
		_ = events(t, ev)
	}

	sent, err := os.ReadFile(filepath.Join(filepath.Dir(e.path), "start.json"))
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		`"microphone":"default"`, `"system_audio":true`, `"camera":"c0123456789abcdef"`, `"events":true`,
		`"outputs":{`, `"system":`, `"events":`,
	} {
		if !strings.Contains(string(sent), want) {
			t.Errorf("the start message lacks %s: %s", want, sent)
		}
	}
}

func TestKillEndsTheStreamWithLost(t *testing.T) {
	h := mustStart(t, "silent", Config{Silence: time.Minute})

	_, ev, err := h.Begin(context.Background(), request(t))
	if err != nil {
		t.Fatal(err)
	}

	h.Kill()

	end, _ := last(t, ev)
	if _, ok := end.(Lost); !ok {
		t.Fatalf("end = %#v", end)
	}

	_ = h.Close()
}

func TestAHelperThatGoesSilentIsKilled(t *testing.T) {
	h := mustStart(t, "silent", Config{Silence: 300 * time.Millisecond})

	_, ev, err := h.Begin(context.Background(), request(t))
	if err != nil {
		t.Fatal(err)
	}

	end, _ := last(t, ev)

	lost, ok := end.(Lost)
	if !ok || !errors.Is(lost.Err, ErrLost) || !strings.Contains(lost.Err.Error(), "silent") {
		t.Fatalf("end = %#v", end)
	}
}

func TestAHelperThatExitsWithoutFinishingIsLost(t *testing.T) {
	h := mustStart(t, "exit", Config{})

	_, ev, err := h.Begin(context.Background(), request(t))
	if err != nil {
		t.Fatal(err)
	}

	end, _ := last(t, ev)

	lost, ok := end.(Lost)
	if !ok || !errors.Is(lost.Err, ErrLost) || !strings.Contains(lost.Err.Error(), "boom") {
		t.Fatalf("end = %#v", end)
	}
}

func TestMalformedMessagesDiscardTheHelper(t *testing.T) {
	for _, mode := range []string{"malformed", "oversize", "unknown"} {
		t.Run(mode, func(t *testing.T) {
			h := mustStart(t, mode, Config{Silence: time.Minute})

			_, ev, err := h.Begin(context.Background(), request(t))
			if err != nil {
				t.Fatal(err)
			}

			end, _ := last(t, ev)

			lost, ok := end.(Lost)
			if !ok || !errors.Is(lost.Err, ErrProtocol) {
				t.Fatalf("end = %#v", end)
			}

			select {
			case <-h.done:
			case <-time.After(10 * time.Second):
				t.Fatal("the helper was not killed")
			}
		})
	}
}

func TestTheDeadlineKillsTheHelper(t *testing.T) {
	h := mustStart(t, "silent", Config{Deadline: 1500 * time.Millisecond, Silence: time.Minute})

	_, ev, err := h.Begin(context.Background(), request(t))
	if err != nil {
		t.Fatal(err)
	}

	end, _ := last(t, ev)
	if _, ok := end.(Lost); !ok {
		t.Fatalf("end = %#v", end)
	}

	select {
	case <-h.done:
	case <-time.After(10 * time.Second):
		t.Fatal("the helper outlived its deadline")
	}
}

func TestAHelperErrorIsReturnedCleaned(t *testing.T) {
	h := mustStart(t, "helpererror", Config{})

	_, _, err := h.Begin(context.Background(), request(t))

	var he *HelperError
	if !errors.As(err, &he) || he.Code != "unsupported" || strings.ContainsRune(he.Message, 0x1b) {
		t.Fatalf("err = %v", err)
	}
}

func TestStartFailures(t *testing.T) {
	for mode, want := range map[string]string{
		"noready": "no ready message", "badversion": "protocol version 1", "garbageready": "no ready message",
	} {
		_, err := newEnv(t, mode).start(t, Config{ReadyTimeout: 700 * time.Millisecond})
		if !errors.Is(err, ErrUnavailable) || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v", mode, err)
		}

		// Only a helper of another version is ErrProtocolVersion, which the person is told by its own reason.
		if errors.Is(err, ErrProtocolVersion) != (mode == "badversion") {
			t.Errorf("%s: ErrProtocolVersion = %v", mode, errors.Is(err, ErrProtocolVersion))
		}
	}

	e := newEnv(t, "ok")

	if _, err := Start(
		context.Background(),
		e.r,
		Config{Path: "relative/stuarty-capture", Dir: e.dir, Deadline: time.Minute},
	); !errors.Is(
		err,
		ErrUnavailable,
	) {
		t.Errorf("a relative path: %v", err)
	}

	if _, err := Start(
		context.Background(),
		e.r,
		Config{Path: filepath.Join(t.TempDir(), "missing.exe"), Dir: e.dir, Deadline: time.Minute},
	); !errors.Is(
		err,
		ErrUnavailable,
	) {
		t.Errorf("a missing binary: %v", err)
	}
}

func TestBadRequestsNeverReachTheHelper(t *testing.T) {
	h := mustStart(t, "ok", Config{})

	good := request(t)

	for name, mutate := range map[string]func(*StartRequest){
		"recording": func(r *StartRequest) { r.Recording = "" },
		"kind":      func(r *StartRequest) { r.Kind = "camera" },
		"source":    func(r *StartRequest) { r.SourceID = "" },
		"fps":       func(r *StartRequest) { r.FPS = 59 },
		"encoder":   func(r *StartRequest) { r.Encoder = "libx264" },
		"ffmpeg":    func(r *StartRequest) { r.FFmpeg = "ffmpeg" },
		"output":    func(r *StartRequest) { r.Output = "capture.mp4" },
		"extension": func(r *StartRequest) { r.Output = filepath.Join(t.TempDir(), "capture.avi") },
	} {
		r := good
		mutate(&r)

		if _, _, err := h.Begin(context.Background(), r); !errors.Is(err, ErrBadRequest) {
			t.Errorf("%s: err = %v", name, err)
		}
	}

	// the helper is still usable after every refusal
	if _, _, err := h.Begin(context.Background(), good); err != nil {
		t.Errorf("a good request after the refusals: %v", err)
	}
}

// The helper runs under the process policy of FFmpeg: no variable of the server reaches it.
func TestSecurity_T120_HelperEnvironmentMinimal(t *testing.T) {
	t.Setenv("STUARTY_SECRET_PROBE", "must-not-leak")
	t.Setenv("STUARTY_MASTER_KEY", "must-not-leak")

	h := mustStart(t, "env", Config{})

	s, err := h.Sources(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if len(s.Windows) != 1 {
		t.Fatalf("windows = %+v", s.Windows)
	}

	env := s.Windows[0].Name
	if strings.Contains(env, "must-not-leak") || strings.Contains(env, "STUARTY_") {
		t.Errorf("the helper saw the environment of the server: %s", env)
	}
}

func TestSecurity_T120_HelperDeadlineKillsIt(t *testing.T) {
	h := mustStart(t, "hang", Config{Deadline: time.Second, StartedTimeout: time.Minute, Silence: time.Minute})

	_, _, err := h.Begin(context.Background(), request(t))
	if err == nil {
		t.Fatal("a helper that never answers must end Begin")
	}

	select {
	case <-h.done:
	case <-time.After(10 * time.Second):
		t.Fatal("the helper outlived its deadline")
	}
}

func TestSecurity_T120_MalformedMessageDiscardsIt(t *testing.T) {
	h := mustStart(t, "malformed", Config{Silence: time.Minute})

	_, ev, err := h.Begin(context.Background(), request(t))
	if err != nil {
		t.Fatal(err)
	}

	end, _ := last(t, ev)
	if lost, ok := end.(Lost); !ok || !errors.Is(lost.Err, ErrProtocol) {
		t.Fatalf("end = %#v", end)
	}

	select {
	case <-h.done:
	case <-time.After(10 * time.Second):
		t.Fatal("the helper survived a malformed message")
	}
}
