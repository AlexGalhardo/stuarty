package owner

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/AlexGalhardo/vukem-video/engine/transcribe"
	"github.com/AlexGalhardo/vukem-video/service"
	"github.com/AlexGalhardo/vukem-video/service/config"
	"github.com/AlexGalhardo/vukem-video/service/workspace"
)

func TestTestTranscriberIsRefusedOutsideTheTestCondition(t *testing.T) {
	cases := map[string]struct {
		env  map[string]string
		mode string
	}{
		"no APP_ENV": {map[string]string{EnvTestTranscriber: "stub"}, config.ModeLocal},
		"production": {
			map[string]string{EnvTestTranscriber: "stub", "APP_ENV": "production"},
			config.ModeLocal,
		},
		"development": {
			map[string]string{EnvTestTranscriber: "stub", "APP_ENV": "development"},
			config.ModeLocal,
		},
		"network mode":       {map[string]string{EnvTestTranscriber: "stub", "APP_ENV": "test"}, config.ModeNetwork},
		"unknown value":      {map[string]string{EnvTestTranscriber: "real", "APP_ENV": "test"}, config.ModeLocal},
		"network, no APPENV": {map[string]string{EnvTestTranscriber: "stub"}, config.ModeNetwork},
	}

	for name, c := range cases {
		p, err := pickTranscriber(envOf(c.env), c.mode, nil, nil)
		if !errors.Is(err, ErrTestTranscriber) || p != nil {
			t.Errorf("%s: provider %v, error %v; want ErrTestTranscriber", name, p, err)
		}
	}
}

func TestTestTranscriberStubAnswersTheFixedTranscript(t *testing.T) {
	env := envOf(map[string]string{EnvTestTranscriber: "stub", "APP_ENV": "test"})

	p, err := pickTranscriber(env, config.ModeLocal, nil, nil)
	if err != nil || p == nil || p.Name() != "stub" {
		t.Fatalf("provider %v, error %v", p, err)
	}

	if err := p.Check(context.Background()); err != nil {
		t.Fatal(err)
	}

	raw, err := p.Transcribe(context.Background(), transcribe.Input{}, nil)
	if err != nil || raw.Language != "en" || len(raw.Words) != 4 ||
		raw.Words[0].Text != "hello" || raw.Words[0].StartMS != 100 ||
		raw.Words[3].Text != "world" || raw.Words[3].EndMS != 1900 {
		t.Errorf("transcript %+v, error %v", raw, err)
	}
}

type fakeWhisper struct{ transcribe.Provider }

func TestNothingDialsWhenTheAPIVariablesAreUnset(t *testing.T) {
	var hits atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	defer srv.Close()

	whisper := fakeWhisper{}

	p, err := pickTranscriber(envOf(nil), config.ModeLocal, whisper, nil)
	if err != nil || p != transcribe.Provider(whisper) {
		t.Errorf("provider %v, error %v; want the local helper", p, err)
	}

	// Configured, the provider exists but still makes no request until a job runs.
	p, err = pickTranscriber(envOf(map[string]string{
		transcribe.EnvAPIURL: srv.URL, transcribe.EnvAPIKey: "k",
	}), config.ModeLocal, whisper, nil)
	if err != nil || p == nil || p.Name() != transcribe.ProviderAPI {
		t.Errorf("provider %v, error %v; want the API provider", p, err)
	}

	if n := hits.Load(); n != 0 {
		t.Errorf("%d requests were made while building the providers", n)
	}
}

func TestAPIVariablesHalfSetAreABootError(t *testing.T) {
	for _, env := range []map[string]string{
		{transcribe.EnvAPIKey: "k"},
		{transcribe.EnvAPIURL: "https://example.com/v1"},
		{transcribe.EnvAPIURL: "http://example.com/v1", transcribe.EnvAPIKey: "k"},
	} {
		if _, err := pickTranscriber(envOf(env), config.ModeLocal, nil, nil); !errors.Is(err, transcribe.ErrAPIConfig) {
			t.Errorf("%v: error %v, want ErrAPIConfig", env, err)
		}
	}
}

func TestOpenRefusesTheTestTranscriberOutsideTest(t *testing.T) {
	ws := filepath.Join(t.TempDir(), "ws")
	ctx := service.WithCaller(context.Background(), service.Caller{Actor: "local:cli", Scopes: service.AllScopes})

	env := map[string]string{workspace.EnvHome: ws, EnvTestTranscriber: "stub"}

	if _, err := Open(ctx, Config{Getenv: envOf(env), Inline: true}); !errors.Is(err, ErrTestTranscriber) {
		t.Fatalf("Open with the stub and no APP_ENV: %v, want ErrTestTranscriber", err)
	}

	env["APP_ENV"] = "test"

	o, err := Open(ctx, Config{Getenv: envOf(env), Inline: true})
	if err != nil {
		t.Fatalf("Open with the stub under APP_ENV=test: %v", err)
	}

	if err := o.Close(); err != nil {
		t.Fatal(err)
	}
}
