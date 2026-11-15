package owner

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg"
	"github.com/AlexGalhardo/vukem-video/engine/transcribe"
	"github.com/AlexGalhardo/vukem-video/service/config"
)

const (
	// EnvTestTranscriber selects the test-only provider; the only value is "stub".
	EnvTestTranscriber = "VUKEM_VIDEO_TEST_TRANSCRIBER"
	// envAppEnv is vukem's environment name; the stub needs it to be "test".
	envAppEnv = "APP_ENV"
	// appEnvTesting is the only APP_ENV under which the stub runs.
	appEnvTesting = "test"
	stubName      = "stub"
)

// ErrTestTranscriber is the start-up refusal of VUKEM_VIDEO_TEST_TRANSCRIBER outside the test condition.
var ErrTestTranscriber = errors.New("owner: test transcriber refused")

// pickTranscriber chooses the transcription provider (SDD-011 section 10.1): the test stub when
// VUKEM_VIDEO_TEST_TRANSCRIBER=stub under APP_ENV=test in local mode; else the API provider when
// VUKEM_VIDEO_TRANSCRIBE_URL or _KEY is set; else the local helper (whisper may be nil). With
// nothing configured no API client is built, so nothing can dial (D-016).
func pickTranscriber(
	getenv func(string) string, mode string, whisper transcribe.Provider, r *ffmpeg.Runner,
) (transcribe.Provider, error) {
	stub, err := testTranscriber(getenv, mode)
	if err != nil {
		return nil, err
	}

	if stub != nil {
		return stub, nil
	}

	api, err := transcribe.APIFromEnv(getenv, r)
	if err != nil {
		return nil, err
	}

	if api != nil {
		return api, nil
	}

	return whisper, nil
}

// testTranscriber returns the stub provider, nil when the variable is unset, and an error when it is
// set where it must not work: any value but "stub", an APP_ENV other than "test", or network mode.
func testTranscriber(getenv func(string) string, mode string) (transcribe.Provider, error) {
	v := strings.TrimSpace(getenv(EnvTestTranscriber))
	if v == "" {
		return nil, nil //nolint:nilnil // unset is not an error, and there is no provider
	}

	if v != stubName {
		return nil, fmt.Errorf("%w: %s must be %q, got %q", ErrTestTranscriber, EnvTestTranscriber, stubName, v)
	}

	if mode == config.ModeNetwork || getenv(envAppEnv) != appEnvTesting {
		return nil, fmt.Errorf(
			"%w: %s=%s works only with %s=%s in local mode; unset it",
			ErrTestTranscriber, EnvTestTranscriber, stubName, envAppEnv, appEnvTesting,
		)
	}

	return &stubTranscriber{}, nil
}

// stubTranscriber answers every audio with the same four words and timings as
// web/contract/testenv.StubTranscriber, so an end-to-end suite can assert the captions that follow.
type stubTranscriber struct{}

func (*stubTranscriber) Name() string                { return stubName }
func (*stubTranscriber) Model() string               { return "tiny" }
func (*stubTranscriber) Check(context.Context) error { return nil }

func (*stubTranscriber) Transcribe(
	_ context.Context, _ transcribe.Input, progress func(done, total time.Duration),
) (transcribe.Raw, error) {
	if progress != nil {
		progress(time.Second, 2*time.Second)
	}

	return transcribe.Raw{Language: "en", Words: []transcribe.RawWord{
		{Text: "hello", StartMS: 100, EndMS: 400},
		{Text: "brave", StartMS: 400, EndMS: 700},
		{Text: "new", StartMS: 1100, EndMS: 1400},
		{Text: "world", StartMS: 1400, EndMS: 1900},
	}}, nil
}
