package anthropic_test

import (
	"context"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/provider"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/connections/connectionstest"
	"github.com/AlexGalhardo/stuarty/service/providers"
	"github.com/AlexGalhardo/stuarty/service/providers/anthropic"
	"github.com/AlexGalhardo/stuarty/service/providers/providertest"
)

const secret = "sk-ant-test-SECRET-0123456789"

var auth = map[string]string{"x-api-key": secret, "anthropic-version": "2023-06-01"}

// run starts the fake server with the contract, connects to it and runs fn with the credential.
func run(t *testing.T, fn func(ctx context.Context, c connections.Credential), xs ...providertest.Exchange) {
	t.Helper()

	rec := providertest.Serve(t, "testdata", xs...)
	connectionstest.New(t, anthropic.ID, rec.Endpoint("/v1"), secret).Do(t, fn)
}

func TestDescriptor(t *testing.T) {
	t.Parallel()

	d := anthropic.New().Descriptor()
	if d.ID != "anthropic" || !provider.ValidProviderID(d.ID) || !strings.HasPrefix(d.DefaultEndpoint, "https://") {
		t.Fatalf("%+v", d)
	}

	if len(d.Capabilities) != 1 || d.Capabilities[0] != provider.CapabilityText ||
		len(d.CredentialKinds) != 1 || d.CredentialKinds[0] != provider.CredentialAPIKey {
		t.Fatalf("Anthropic is reached with an API key only (ADR-0036 item 3): %+v", d)
	}
}

func TestModels(t *testing.T) {
	t.Parallel()

	var got []providers.Model

	run(t, func(ctx context.Context, c connections.Credential) {
		var err error

		got, err = anthropic.New().Models(ctx, c)
		if err != nil {
			t.Fatal(err)
		}
	}, providertest.Exchange{Method: "GET", Path: "/v1/models?limit=1000", Header: auth, Response: "models.json"})

	if len(got) != 3 || got[0].ID != "claude-sonnet-x" || got[0].ContextTokens != 200000 ||
		got[0].OutputTokens != 64000 {
		t.Fatalf("the limits the catalogue gives are used: %+v", got)
	}

	if got[1].ContextTokens != 0 || got[0].Price != nil {
		t.Errorf("a model with no limit has none, and the catalogue has no price: %+v", got)
	}
}

func TestGenerateText(t *testing.T) {
	t.Parallel()

	temp := 0.5
	req := provider.TextRequest{
		Model: "claude-sonnet-x", System: "You write short captions.", Prompt: "Caption a video about sourdough.",
		MaxTokens: 300, Temperature: &temp,
	}

	run(t, func(ctx context.Context, c connections.Credential) {
		res, err := anthropic.New().GenerateText(ctx, c, req)
		if err != nil {
			t.Fatal(err)
		}

		// The text blocks are joined; a block of another type is not text.
		if res.Text != "Sourdough, from starter to slice." || res.FinishReason != providers.FinishStop ||
			res.Usage.InputTokens != 21 || res.Usage.OutputTokens != 11 || res.Model != "claude-sonnet-x" {
			t.Fatalf("got %+v", res)
		}
	}, providertest.Exchange{
		Method:   "POST",
		Path:     "/v1/messages",
		Header:   auth,
		WantBody: "messages_request.json",
		Response: "messages_ok.json",
	})
}

func TestGenerateTextWithNoSystemAndNoTemperature(t *testing.T) {
	t.Parallel()

	run(t, func(ctx context.Context, c connections.Credential) {
		_, err := anthropic.New().GenerateText(ctx, c, provider.TextRequest{
			Model: "claude-sonnet-x", Prompt: "Caption a video about sourdough.", MaxTokens: 1024,
		})
		if err != nil {
			t.Fatal(err)
		}
	}, providertest.Exchange{
		Method:   "POST",
		Path:     "/v1/messages",
		Header:   auth,
		WantBody: "messages_request_plain.json",
		Response: "messages_ok.json",
	})
}

func TestGenerateTextMapsTheStopReason(t *testing.T) {
	t.Parallel()

	run(t, func(ctx context.Context, c connections.Credential) {
		res, err := anthropic.New().GenerateText(ctx, c, provider.TextRequest{Model: "m", Prompt: "p", MaxTokens: 300})
		if err != nil || res.FinishReason != providers.FinishLength || res.Usage.OutputTokens != 300 {
			t.Fatalf("got %+v, %v", res, err)
		}
	}, providertest.Exchange{Method: "POST", Path: "/v1/messages", AnyBody: true, Response: "messages_length.json"})
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
			"authentication_error",
			0,
		},
		{"rate limit", providertest.Exchange{
			Status: 429, Response: "error_429.json", ResponseHeader: map[string]string{"Retry-After": "12"},
		}, providers.KindRateLimited, 429, "rate_limit_error", 12},
		{
			"hostile type",
			providertest.Exchange{Status: 400, Response: "error_400_hostile.json"},
			providers.KindBadRequest,
			400,
			"",
			0,
		},
		{
			"overloaded",
			providertest.Exchange{Status: 529, Response: "error_529.json"},
			providers.KindUnavailable,
			529,
			"overloaded_error",
			0,
		},
		{
			"not the documented shape",
			providertest.Exchange{Response: "models.json"},
			providers.KindUnreadable,
			200,
			"",
			0,
		},
		{"no usage", providertest.Exchange{Response: "messages_no_usage.json"}, providers.KindUnreadable, 200, "", 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tc.x.Method, tc.x.Path, tc.x.AnyBody = "POST", "/v1/messages", true

			run(t, func(ctx context.Context, c connections.Credential) {
				_, err := anthropic.New().
					GenerateText(ctx, c, provider.TextRequest{Model: "m", Prompt: "p", MaxTokens: 10})

				pe, ok := providers.AsError(err)
				if !ok || pe.Kind != tc.kind || pe.Status != tc.status || pe.Code != tc.code ||
					pe.RetryAfter != tc.retryAfter {
					t.Fatalf("got %+v (%v)", pe, err)
				}

				for _, banned := range []string{secret, "sk-ant", "Ignore", "per-minute", "invalid x-api-key"} {
					if strings.Contains(err.Error(), banned) {
						t.Errorf("the error %q holds %q", err.Error(), banned)
					}
				}
			}, tc.x)
		})
	}
}

func TestFramingIsPositive(t *testing.T) {
	t.Parallel()

	if anthropic.New().Framing() <= 0 {
		t.Fatal("the estimate must stay an upper bound")
	}
}
