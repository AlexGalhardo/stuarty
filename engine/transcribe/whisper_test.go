package transcribe

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"

	"github.com/AlexGalhardo/vukem/contrib/engine"
)

var (
	fakeOnce sync.Once
	fakePath string
	errFake  error
)

// fakeWhisper builds testdata/fakewhisper once per test binary and returns its path.
func fakeWhisper(t *testing.T) string {
	t.Helper()
	fakeOnce.Do(func() {
		dir, err := sharedDir()
		if err != nil {
			errFake = err
			return
		}
		name := "fakewhisper"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		fakePath = filepath.Join(dir, name)
		cmd := exec.Command("go", "build", "-o", fakePath, "./testdata/fakewhisper")
		if out, err := cmd.CombinedOutput(); err != nil {
			errFake = errors.New(string(out))
		}
	})
	if errFake != nil {
		t.Skipf("cannot build the fake helper: %v", errFake)
	}
	return fakePath
}

// sharedDir is made once for the whole binary, so it cannot be the TempDir of one test; TestMain removes it.
func sharedDir() (string, error) {
	return os.MkdirTemp("", "fakewhisper-")
}

func TestMain(m *testing.M) {
	code := m.Run()
	if fakePath != "" {
		_ = os.RemoveAll(filepath.Dir(fakePath))
	}
	os.Exit(code)
}

func fakeTool(t *testing.T, mode string) *engine.Tool {
	t.Helper()
	canned, err := filepath.Abs("testdata/pause.json")
	if err != nil {
		t.Fatal(err)
	}
	tool, err := engine.Find(context.Background(), engine.Config{
		Name: "fakewhisper", Path: fakeWhisper(t), VersionArgs: []string{},
		Env: map[string]string{"FAKEWHISPER_MODE": mode, "FAKEWHISPER_CANNED": canned},
	})
	if err != nil {
		t.Fatal(err)
	}
	return tool
}

func failureReason(t *testing.T, err error) string {
	t.Helper()
	fe, ok := errors.AsType[*ffmpeg.Error](err)
	if !ok || fe.Code != CodeFailed {
		t.Fatalf("err = %v, want E_TRANSCRIBE_FAILED", err)
	}
	return fe.Reason
}

// ADR-0021: whisper-cli exits with 0 on corrupt audio; the run succeeds only if its JSON is there.
func TestSuccessJudgedByOutput(t *testing.T) {
	run := func(mode string) (Raw, error) {
		dir := t.TempDir()
		argv := Argv("model.bin", filepath.Join(dir, "in.wav"), "auto", 2, filepath.Join(dir, "out"))
		return runHelper(context.Background(), fakeTool(t, mode), dir, argv, 4*time.Second, nil)
	}

	raw, err := run("ok")
	if err != nil || len(raw.Words) != 5 || raw.Language != "en" {
		t.Fatalf("a good run: %v, %+v", err, raw)
	}

	for _, mode := range []string{"corrupt", "empty"} {
		_, err := run(mode)
		if got := failureReason(t, err); got != ReasonOutputMissing {
			t.Errorf("%s: reason %q, want %q", mode, got, ReasonOutputMissing)
		}
	}
	_, err = run("corrupt")
	if fe, _ := errors.AsType[*ffmpeg.Error](
		err,
	); fe == nil ||
		strings.ContainsAny(fe.Details["stderr"].(string), `/\`) ||
		!strings.Contains(fe.Details["stderr"].(string), "<path>") {
		t.Errorf("stderr detail %v has the path or is missing", fe.Details)
	}

	if _, err := run("fail"); failureReason(t, err) != ReasonExitCode {
		t.Errorf("a non-zero exit gave %v", err)
	}
}

func TestProgressFollowsTheTranscriptLines(t *testing.T) {
	var got []time.Duration
	dir := t.TempDir()
	argv := Argv("model.bin", filepath.Join(dir, "in.wav"), "auto", 2, filepath.Join(dir, "out"))
	_, err := runHelper(context.Background(), fakeTool(t, "ok"), dir, argv, 4*time.Second,
		func(done, total time.Duration) {
			if total != 4*time.Second {
				t.Errorf("total %s", total)
			}
			got = append(got, done)
		})
	if err != nil {
		t.Fatal(err)
	}
	if want := []time.Duration{time.Second, 3500 * time.Millisecond}; !slices.Equal(got, want) {
		t.Errorf("progress %v, want %v", got, want)
	}
}

func TestCancelEndsTheHelper(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	dir := t.TempDir()
	_, err := runHelper(
		ctx,
		fakeTool(t, "ok"),
		dir,
		Argv("m", filepath.Join(dir, "in.wav"), "auto", 1, filepath.Join(dir, "out")),
		time.Second,
		nil,
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want a cancellation", err)
	}
}

// SDD-007 section 6.6 item 2: the command line is the template, and no flag carries user text.
func TestSecurity_T15_WhisperArgvPolicy(t *testing.T) {
	tests := []struct {
		language string
		flag     string
	}{{"", "auto"}, {"auto", "auto"}, {"pt", "pt"}, {"pt-BR", "pt"}, {"EN_us", "en"}}
	for _, tt := range tests {
		lang, err := Language(tt.language)
		if err != nil || lang != tt.flag {
			t.Fatalf("Language(%q) = %q, %v; want %q", tt.language, lang, err, tt.flag)
		}
		want := []string{
			"-m", "m.bin", "-f", "in.wav", "-l", tt.flag, "-t", "4",
			"-ml", "1", "-sow", "-ojf", "-pp", "-of", "out",
		}
		if got := Argv("m.bin", "in.wav", lang, 4, "out"); !slices.Equal(got, want) {
			t.Errorf("argv %v, want %v", got, want)
		}
	}
	for _, bad := range []string{"xx", "klingon", "pt; rm -rf /", "-l", "../x", "p", strings.Repeat("a", 200)} {
		if _, err := Language(bad); !errors.Is(err, ErrLanguage) {
			t.Errorf("Language(%q) = %v, want a refusal", bad, err)
		}
	}
}

func TestThreads(t *testing.T) {
	if Threads(1) != 1 || Threads(0) != 1 {
		t.Error("at least one thread")
	}
	for _, n := range []int{1, 2, 4, 8, 16, 64} {
		if got := Threads(n); got < 1 || got > 8 || got > n {
			t.Errorf("Threads(%d) = %d", n, got)
		}
	}
}

func TestCheckReportsWhatIsMissing(t *testing.T) {
	w := &Whisper{
		Helper: filepath.Join(t.TempDir(), "nope"), ModelPath: filepath.Join(t.TempDir(), "nope.bin"),
		HelperItem: Item{Name: "whisper.cpp", URLHost: "github.com", Bytes: 8194445},
		ModelItem:  Item{Name: "small", URLHost: "huggingface.co", Bytes: 487601967},
	}
	m, ok := errors.AsType[*MissingError](w.Check(context.Background()))
	if !ok || len(m.Items) != 2 || m.DownloadBytes != 8194445+487601967 {
		t.Fatalf("Check = %+v", m)
	}
	w.Helper, w.ModelPath = fakeWhisper(t), fakeWhisper(t)
	if err := w.Check(context.Background()); err != nil {
		t.Errorf("Check with everything in place: %v", err)
	}
}

func TestParseWhisperJSONLimits(t *testing.T) {
	deep := strings.Repeat("[", 40) + strings.Repeat("]", 40)
	for name, in := range map[string]string{
		"too deep":         `{"result":{},"transcription":` + deep + `}`,
		"no transcription": `{"result":{"language":"pt"}}`,
		"not json":         `error: failed`,
	} {
		if _, err := ParseWhisperJSON([]byte(in), 1000); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	raw, err := ParseWhisperJSON([]byte(`{"result":{"language":"pt"},"transcription":[]}`), 1000)
	if err != nil || len(raw.Words) != 0 {
		t.Errorf("a transcript with no words is valid: %v, %+v", err, raw)
	}
}

func TestParseCleansUntrustedText(t *testing.T) {
	in := `{"result":{"language":"en"},"transcription":[
	 {"offsets":{"from":0,"to":500},"text":" he\u202Ello\u0007","tokens":[]},
	 {"offsets":{"from":500,"to":900},"text":" ,","tokens":[]}]}`
	raw, err := ParseWhisperJSON([]byte(in), 1000)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw.Words) != 1 || raw.Words[0].Text != "hello," {
		t.Errorf("words %+v, want one word %q (controls removed, punctuation attached)", raw.Words, "hello,")
	}
}

func TestArtifactKey(t *testing.T) {
	a := ArtifactKey("sha256:aa", ProviderWhisper, "small", "")
	if len(a) != 12 || a != ArtifactKey("sha256:aa", ProviderWhisper, "small", "auto") {
		t.Errorf("key %q: empty and auto must be one request", a)
	}
	for _, other := range []string{
		ArtifactKey("sha256:bb", ProviderWhisper, "small", ""),
		ArtifactKey("sha256:aa", "openai-compatible", "small", ""),
		ArtifactKey("sha256:aa", ProviderWhisper, "base", ""),
		ArtifactKey("sha256:aa", ProviderWhisper, "small", "pt"),
	} {
		if other == a {
			t.Error("a different request has the same key")
		}
	}
}

// SDD-007 section 6.8: a helper is started only from an absolute path; a relative one would be looked
// up on the PATH or in the working directory by the engine.
func TestSecurity_T15_WhisperRefusesRelativePaths(t *testing.T) {
	dir := t.TempDir()
	helper := filepath.Join(dir, "whisper-cli")
	model := filepath.Join(dir, "model.bin")
	for _, p := range []string{helper, model} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	relHelper, errH := filepath.Rel(wd, helper)
	relModel, errM := filepath.Rel(wd, model)
	if errH != nil || errM != nil {
		// skip-env: relative-path
		t.Skip("the temp directory has no relative path from the working directory")
	}

	for name, w := range map[string]*Whisper{
		"helper": {Helper: relHelper, ModelPath: model},
		"model":  {Helper: helper, ModelPath: relModel},
	} {
		_, err := w.Transcribe(context.Background(), Input{Language: "auto"}, nil)
		if got := failureReason(t, err); got != ReasonPrepare || !errors.Is(err, errNotAbsolute) {
			t.Errorf("relative %s: reason %q, err %v; want %q and errNotAbsolute", name, got, err, ReasonPrepare)
		}
	}
}
