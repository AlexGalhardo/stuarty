package openai_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/provider"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/providers"
	"github.com/AlexGalhardo/stuarty/service/providers/openai"
	"github.com/AlexGalhardo/stuarty/service/providers/providertest"
)

// runPlan is run with the credential of a sign-in: the connection manager says so (SignedIn) and hands the
// adapter the access token as the secret, which here is the one of the test.
func runPlan(t *testing.T, fn func(ctx context.Context, c connections.Credential), xs ...providertest.Exchange) {
	t.Helper()

	run(t, func(ctx context.Context, c connections.Credential) {
		c.SignedIn = true

		fn(ctx, c)
	}, xs...)
}

func TestPlanModelsListTheModelsOfTheAccount(t *testing.T) {
	t.Parallel()

	var got []providers.Model

	runPlan(t, func(ctx context.Context, c connections.Credential) {
		var err error

		got, err = openai.New().Models(ctx, c)
		if err != nil {
			t.Fatal(err)
		}
	}, providertest.Exchange{Method: "GET", Path: "/v1/models", Header: auth, Response: "plan_models.json"})

	// The hidden model is not offered; the registry drops the id that is not a valid one.
	if len(got) != 3 || got[0].ID != "gpt-plan-1" || got[0].Name != "GPT Plan 1" || got[1].Name != "gpt-plan-mini" {
		t.Fatalf("got %+v", got)
	}

	for _, m := range got {
		// A plan has no price per token: the catalogue says zero, so a generation is not refused for a
		// missing price and no ceiling is spent (SDD-016 section 12.2).
		if m.Price == nil || m.Price.Currency != "USD" || m.Price.InputMicrosPerMTok != 0 ||
			m.Price.OutputMicrosPerMTok != 0 {
			t.Errorf("%s: price %+v", m.ID, m.Price)
		}
	}
}

func TestPlanGenerateTextReadsTheStream(t *testing.T) {
	t.Parallel()

	temp := 0.7

	var res providers.TextResult

	runPlan(t, func(ctx context.Context, c connections.Credential) {
		var err error

		res, err = openai.New().GenerateText(ctx, c, provider.TextRequest{
			Model: "gpt-plan-1", System: "You write short captions.", Prompt: "Caption a video about sourdough.",
			MaxTokens: 200, Temperature: &temp,
		})
		if err != nil {
			t.Fatal(err)
		}
	}, providertest.Exchange{
		Method: "POST", Path: "/v1/responses", Header: map[string]string{
			"Authorization": "Bearer " + secret, "Accept": "text/event-stream", "Content-Type": "application/json",
		},
		WantBody: "responses_request.json", Response: "responses_ok.sse",
	})

	if res.Text != "Sourdough, from starter to slice." || res.FinishReason != providers.FinishStop ||
		res.Usage.InputTokens != 24 || res.Usage.OutputTokens != 9 {
		t.Fatalf("got %+v", res)
	}
}

func TestPlanGenerateTextWithNoSystemAndNoTemperature(t *testing.T) {
	t.Parallel()

	runPlan(t, func(ctx context.Context, c connections.Credential) {
		_, err := openai.New().GenerateText(ctx, c, provider.TextRequest{
			Model: "gpt-plan-1", Prompt: "Caption a video about sourdough.", MaxTokens: 1024,
		})
		if err != nil {
			t.Fatal(err)
		}
	}, providertest.Exchange{
		Method: "POST", Path: "/v1/responses", Header: auth, WantBody: "responses_plain_request.json",
		Response: "responses_ok.sse",
	})
}

func TestPlanOutputLimitIsALengthFinishThatWasBilled(t *testing.T) {
	t.Parallel()

	runPlan(t, func(ctx context.Context, c connections.Credential) {
		res, err := openai.New().
			GenerateText(ctx, c, provider.TextRequest{Model: "gpt-plan-1", Prompt: "p", MaxTokens: 200})
		if err != nil || res.FinishReason != providers.FinishLength || res.Text != "Sourdough, from" ||
			res.Usage.OutputTokens != 200 {
			t.Fatalf("got %+v, %v", res, err)
		}
	}, providertest.Exchange{Method: "POST", Path: "/v1/responses", AnyBody: true, Response: "responses_incomplete.sse"})
}

func TestPlanStreamFailures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		x      providertest.Exchange
		kind   string
		status int
		code   string
	}{
		{
			"plan limit reached",
			providertest.Exchange{Response: "responses_usage_limit.sse"},
			providers.KindRateLimited, 200, "subscription_sharing_usage_limit_exceeded",
		},
		{
			"plan unavailable",
			providertest.Exchange{Response: "responses_usage_unavailable.sse"},
			providers.KindUnavailable, 200, "subscription_sharing_usage_unavailable",
		},
		{
			"a code that is text",
			providertest.Exchange{Response: "responses_hostile_code.sse"},
			providers.KindUnavailable,
			200,
			"",
		},
		{
			"ended before the terminal event",
			providertest.Exchange{Response: "responses_truncated.sse"},
			providers.KindUnreadable,
			200,
			"",
		},
		{
			"incomplete for another reason",
			providertest.Exchange{Response: "responses_filtered.sse"},
			providers.KindUnreadable,
			200,
			"",
		},
		{
			"token refused",
			providertest.Exchange{Status: 401, Response: "error_401.json"},
			providers.KindAuth,
			401,
			"invalid_api_key",
		},
		{
			"rate limited",
			providertest.Exchange{
				Status:         429,
				Response:       "error_429.json",
				ResponseHeader: map[string]string{"Retry-After": "7"},
			},
			providers.KindRateLimited,
			429,
			"insufficient_quota",
		},
		{
			"server error",
			providertest.Exchange{Status: 500, Response: "error_500.json"},
			providers.KindUnavailable,
			500,
			"server_error",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tc.x.Method, tc.x.Path, tc.x.AnyBody = "POST", "/v1/responses", true

			runPlan(t, func(ctx context.Context, c connections.Credential) {
				_, err := openai.New().
					GenerateText(ctx, c, provider.TextRequest{Model: "gpt-plan-1", Prompt: "p", MaxTokens: 10})

				pe, ok := providers.AsError(err)
				if !ok || pe.Kind != tc.kind || pe.Status != tc.status || pe.Code != tc.code {
					t.Fatalf("got %+v (%v)", pe, err)
				}

				for _, banned := range []string{secret, "Ignore", "plan limit", "Incorrect", "exceeded your"} {
					if strings.Contains(err.Error(), banned) {
						t.Errorf("the error %q holds %q", err.Error(), banned)
					}
				}
			}, tc.x)
		})
	}
}

func TestPlanRedirectIsNotFollowed(t *testing.T) {
	t.Parallel()

	runPlan(t, func(ctx context.Context, c connections.Credential) {
		_, err := openai.New().
			GenerateText(ctx, c, provider.TextRequest{Model: "gpt-plan-1", Prompt: "p", MaxTokens: 10})

		if pe, ok := providers.AsError(err); !ok || pe.Status != http.StatusFound {
			t.Fatalf("got %v", err)
		}
	}, providertest.Exchange{
		Method: "POST", Path: "/v1/responses", AnyBody: true, Status: http.StatusFound,
		ResponseHeader: map[string]string{"Location": "http://127.0.0.1:1/steal"},
	})
}
