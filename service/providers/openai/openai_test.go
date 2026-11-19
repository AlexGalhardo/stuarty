package openai_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/provider"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/connections/connectionstest"
	"github.com/AlexGalhardo/stuarty/service/providers"
	"github.com/AlexGalhardo/stuarty/service/providers/openai"
	"github.com/AlexGalhardo/stuarty/service/providers/providertest"
)

const secret = "sk-test-SECRET-0123456789"

var auth = map[string]string{"Authorization": "Bearer " + secret}

// run starts the fake server with the contract, connects to it and runs fn with the credential.
func run(
	t *testing.T,
	fn func(ctx context.Context, c connections.Credential),
	xs ...providertest.Exchange,
) *providertest.Recorder {
	t.Helper()

	rec := providertest.Serve(t, "testdata", xs...)
	connectionstest.New(t, openai.ID, rec.Endpoint("/v1"), secret).Do(t, fn)

	return rec
}

func TestDescriptor(t *testing.T) {
	t.Parallel()

	d := openai.New().Descriptor()
	if d.ID != "openai" || !provider.ValidProviderID(d.ID) || !strings.HasPrefix(d.DefaultEndpoint, "https://") {
		t.Fatalf("%+v", d)
	}

	if len(d.Capabilities) != 1 || d.Capabilities[0] != provider.CapabilityText ||
		len(d.CredentialKinds) != 1 || d.CredentialKinds[0] != provider.CredentialAPIKey {
		t.Fatalf("epic 09 is the text capability with an API key: %+v", d)
	}
}

func TestModels(t *testing.T) {
	t.Parallel()

	var got []providers.Model

	run(t, func(ctx context.Context, c connections.Credential) {
		var err error

		got, err = openai.New().Models(ctx, c)
		if err != nil {
			t.Fatal(err)
		}
	}, providertest.Exchange{Method: "GET", Path: "/v1/models", Header: auth, Response: "models.json"})

	// The adapter returns what the provider lists; the registry drops what is not a valid id.
	if len(got) != 4 || got[0].ID != "gpt-4o" || got[1].ID != "gpt-4o-mini" {
		t.Fatalf("got %+v", got)
	}

	if got[0].Price != nil || got[0].ContextTokens != 0 {
		t.Errorf("the catalogue of OpenAI has no price and no limit: %+v", got[0])
	}
}

func TestGenerateText(t *testing.T) {
	t.Parallel()

	temp := 0.7
	req := provider.TextRequest{
		Model: "gpt-4o", System: "You write short captions.", Prompt: "Caption a video about sourdough.",
		MaxTokens: 200, Temperature: &temp,
	}

	var res providers.TextResult

	rec := run(t, func(ctx context.Context, c connections.Credential) {
		var err error

		res, err = openai.New().GenerateText(ctx, c, req)
		if err != nil {
			t.Fatal(err)
		}
	}, providertest.Exchange{
		Method: "POST", Path: "/v1/chat/completions", Header: auth, WantBody: "chat_request.json",
		Response: "chat_ok.json",
	})

	if res.Text != "Sourdough, from starter to slice." || res.FinishReason != providers.FinishStop ||
		res.Usage.InputTokens != 24 || res.Usage.OutputTokens != 9 || res.Model != "gpt-4o-2024-08-06" {
		t.Fatalf("got %+v", res)
	}

	if ct := rec.Last().Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("content type %q", ct)
	}
}

func TestGenerateTextWithNoSystemAndNoTemperature(t *testing.T) {
	t.Parallel()

	run(t, func(ctx context.Context, c connections.Credential) {
		_, err := openai.New().GenerateText(ctx, c, provider.TextRequest{
			Model: "gpt-4o", Prompt: "Caption a video about sourdough.", MaxTokens: 1024,
		})
		if err != nil {
			t.Fatal(err)
		}
	}, providertest.Exchange{
		Method: "POST", Path: "/v1/chat/completions", Header: auth, WantBody: "chat_request_plain.json",
		Response: "chat_ok.json",
	})
}

func TestGenerateTextMapsTheFinishReason(t *testing.T) {
	t.Parallel()

	for fixture, want := range map[string]string{"chat_length.json": providers.FinishLength, "chat_refusal.json": providers.FinishStop} {
		run(t, func(ctx context.Context, c connections.Credential) {
			res, err := openai.New().
				GenerateText(ctx, c, provider.TextRequest{Model: "gpt-4o", Prompt: "p", MaxTokens: 10})
			if err != nil || res.FinishReason != want {
				t.Fatalf("%s: got %+v, %v", fixture, res, err)
			}

			if fixture == "chat_refusal.json" && (res.Text != "" || res.Usage.OutputTokens != 6) {
				t.Fatalf("a refusal is an empty text that was still billed, got %+v", res)
			}
		}, providertest.Exchange{Method: "POST", Path: "/v1/chat/completions", AnyBody: true, Response: fixture})
	}
}

func TestFailures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		x          providertest.Exchange
		kind       string
		status     int
		code       string
		retryAfter int
	}{
		{
			"key refused",
			providertest.Exchange{Status: 401, Response: "error_401.json"},
			providers.KindAuth,
			401,
			"invalid_api_key",
			0,
		},
		{"quota", providertest.Exchange{
			Status: 429, Response: "error_429.json", ResponseHeader: map[string]string{"Retry-After": "30"},
		}, providers.KindRateLimited, 429, "insufficient_quota", 30},
		{
			"hostile code",
			providertest.Exchange{Status: 400, Response: "error_400_hostile.json"},
			providers.KindBadRequest,
			400,
			"invalid_request_error",
			0,
		},
		{
			"server error",
			providertest.Exchange{Status: 500, Response: "error_500.json"},
			providers.KindUnavailable,
			500,
			"server_error",
			0,
		},
		{"no body", providertest.Exchange{Status: 503}, providers.KindUnavailable, 503, "", 0},
		{
			"not the documented shape",
			providertest.Exchange{Response: "models.json"},
			providers.KindUnreadable,
			200,
			"",
			0,
		},
		{"no usage", providertest.Exchange{Response: "chat_no_usage.json"}, providers.KindUnreadable, 200, "", 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tc.x.Method, tc.x.Path, tc.x.AnyBody = "POST", "/v1/chat/completions", true

			run(t, func(ctx context.Context, c connections.Credential) {
				_, err := openai.New().
					GenerateText(ctx, c, provider.TextRequest{Model: "gpt-4o", Prompt: "p", MaxTokens: 10})

				pe, ok := providers.AsError(err)
				if !ok || pe.Kind != tc.kind || pe.Status != tc.status || pe.Code != tc.code ||
					pe.RetryAfter != tc.retryAfter {
					t.Fatalf("got %+v (%v)", pe, err)
				}

				// Neither the key nor the words of the provider are in what an error can show.
				for _, banned := range []string{secret, "sk-test", "Ignore", "exceeded", "Incorrect"} {
					if strings.Contains(err.Error(), banned) {
						t.Errorf("the error %q holds %q", err.Error(), banned)
					}
				}
			}, tc.x)
		})
	}
}

func TestAKeyThatTheProviderEchoesNeverReachesAnError(t *testing.T) {
	t.Parallel()

	// error_401.json echoes the key in message, as providers do; only status and a safe code are kept.
	run(t, func(ctx context.Context, c connections.Credential) {
		_, err := openai.New().Models(ctx, c)

		var pe *providers.Error
		if !errors.As(err, &pe) || strings.Contains(pe.Error(), "SECRET") || strings.Contains(pe.Code, "SECRET") {
			t.Fatalf("got %v", err)
		}
	}, providertest.Exchange{Method: "GET", Path: "/v1/models", Status: 401, Response: "error_401.json"})
}

func TestARedirectIsNotFollowed(t *testing.T) {
	t.Parallel()

	rec := run(t, func(ctx context.Context, c connections.Credential) {
		_, err := openai.New().Models(ctx, c)

		if pe, ok := providers.AsError(
			err,
		); !ok || pe.Kind != providers.KindUnreadable ||
			pe.Status != http.StatusFound {
			t.Fatalf("got %v", err)
		}
	}, providertest.Exchange{
		Method: "GET", Path: "/v1/models", Status: http.StatusFound,
		ResponseHeader: map[string]string{"Location": "http://127.0.0.1:1/steal"},
	})

	if len(rec.Requests) != 1 {
		t.Fatalf("one request, the key goes to one host: %d", len(rec.Requests))
	}
}

func TestTheServerGoneIsUnavailable(t *testing.T) {
	t.Parallel()

	rec := providertest.Serve(t, "testdata")
	h := connectionstest.New(t, openai.ID, rec.Endpoint("/v1"), secret)
	rec.Server.Close()

	h.Do(t, func(ctx context.Context, c connections.Credential) {
		_, err := openai.New().Models(ctx, c)

		pe, ok := providers.AsError(err)
		if !ok || pe.Kind != providers.KindUnavailable || strings.Contains(err.Error(), secret) {
			t.Fatalf("got %v", err)
		}
	})
}

func TestFramingIsPositive(t *testing.T) {
	t.Parallel()

	if openai.New().Framing() <= 0 {
		t.Fatal("the estimate must stay an upper bound")
	}
}
