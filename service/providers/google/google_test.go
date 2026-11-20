package google_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/provider"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/connections/connectionstest"
	"github.com/AlexGalhardo/stuarty/service/providers"
	"github.com/AlexGalhardo/stuarty/service/providers/google"
	"github.com/AlexGalhardo/stuarty/service/providers/providertest"
)

// The fixtures of testdata are written from the documentation of the Gemini API (SDD-016 section 2.3): no key
// and no call to the real service was available when they were made, and the first real call must confirm them.

const (
	secret  = "AIza-test-SECRET-0123456789"
	taskID  = "v1_task0001"
	maxClip = 1 << 20
)

var (
	key      = map[string]string{"x-goog-api-key": secret}
	clipHead = []byte("\x00\x00\x00\x18ftypisom")
	videoReq = provider.VideoRequest{
		Model: google.Model, Prompt: "A paper boat crossing a puddle at dawn", Seconds: 5, Aspect: "9:16",
	}
)

func run(
	t *testing.T,
	fn func(ctx context.Context, c connections.Credential),
	xs ...providertest.Exchange,
) *providertest.Recorder {
	t.Helper()

	rec := providertest.Serve(t, "testdata", xs...)
	connectionstest.New(t, google.ID, rec.Endpoint("/v1beta"), secret).Do(t, fn)

	return rec
}

func submit(response string, status int) providertest.Exchange {
	return providertest.Exchange{
		Method: "POST", Path: "/v1beta/interactions", Header: key, WantBody: "submit_request.json",
		Status: status, Response: response,
	}
}

func get(response string, status int) providertest.Exchange {
	return providertest.Exchange{
		Method: "GET", Path: "/v1beta/interactions/" + taskID, Header: key, Status: status, Response: response,
	}
}

func kindOf(t *testing.T, err error) string {
	t.Helper()

	pe, ok := providers.AsError(err)
	if !ok {
		t.Fatalf("not a provider error: %v", err)
	}

	return pe.Kind
}

func TestDescriptorAndLimits(t *testing.T) {
	t.Parallel()

	a := google.New()
	d := a.Descriptor()

	if d.ID != "google" || !provider.ValidProviderID(d.ID) || !strings.HasPrefix(d.DefaultEndpoint, "https://") ||
		!slices.Equal(d.Capabilities, []string{provider.CapabilityVideo}) ||
		!slices.Equal(d.CredentialKinds, []string{provider.CredentialAPIKey}) {
		t.Fatalf("%+v", d)
	}

	l := a.VideoLimits(google.Model)
	if l.MinSeconds != 3 || l.MaxSeconds != 15 || !slices.Equal(l.Aspects, []string{"16:9", "9:16"}) || l.NoFirstFrame {
		t.Fatalf("%+v", l)
	}

	if got := a.VideoLimits("another-model"); got.MaxSeconds != 0 || len(got.Aspects) != 0 {
		t.Fatalf("a model the documentation does not name has no documented limit: %+v", got)
	}

	// The adapter is a video provider in the registry's sense.
	var _ providers.VideoGenerator = a
}

func TestModels(t *testing.T) {
	t.Parallel()

	run(t, func(ctx context.Context, c connections.Credential) {
		ms, err := google.New().Models(ctx, c)
		if err != nil {
			t.Fatal(err)
		}

		if len(ms) != 2 || ms[0].ID != google.Model || ms[0].Capability != provider.CapabilityVideo ||
			ms[0].ContextTokens != 131072 || ms[1].ID != "gemini-3.8-flash" || ms[1].Name != "gemini-3.8-flash" ||
			ms[1].Capability != "" {
			t.Fatalf("%+v", ms)
		}
	}, providertest.Exchange{
		Method: "GET", Path: "/v1beta/models?pageSize=1000", Header: key, Response: "models.json",
	})
}

func TestSubmitSendsTheKeyInTheHeaderOnly(t *testing.T) {
	t.Parallel()

	rec := run(t, func(ctx context.Context, c connections.Credential) {
		id, err := google.New().Submit(ctx, c, videoReq, nil)
		if err != nil || id != taskID {
			t.Fatalf("%q %v", id, err)
		}
	}, submit("submit_ok.json", 0))

	if strings.Contains(rec.Last().Path, secret) || strings.Contains(string(rec.Last().Body), secret) {
		t.Fatal("the key must be in the header only")
	}
}

func TestSubmitWithAFirstFrame(t *testing.T) {
	t.Parallel()

	x := submit("submit_ok.json", 0)
	x.WantBody = "submit_request_frame.json"

	run(t, func(ctx context.Context, c connections.Credential) {
		frame := &providers.VideoFrame{Data: []byte("\x89PNG\r\n\x1a\n"), Format: "png"}

		id, err := google.New().Submit(ctx, c, videoReq, frame)
		if err != nil || id != taskID {
			t.Fatalf("%q %v", id, err)
		}
	}, x)
}

func TestSubmitAnswersThatCarryNoUsableID(t *testing.T) {
	t.Parallel()

	for _, fixture := range []string{"submit_no_id.json", "submit_hostile_id.json"} {
		run(t, func(ctx context.Context, c connections.Credential) {
			id, err := google.New().Submit(ctx, c, videoReq, nil)
			if id != "" || kindOf(t, err) != providers.KindUnreadable {
				t.Fatalf("%s: %q %v", fixture, id, err)
			}
		}, submit(fixture, 0))
	}
}

func TestSubmitFailuresAreMappedByStatus(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		status  int
		fixture string
		kind    string
	}{
		{http.StatusBadRequest, "error_400.json", providers.KindBadRequest},
		{http.StatusUnauthorized, "error_401.json", providers.KindAuth},
		{http.StatusTooManyRequests, "error_429.json", providers.KindRateLimited},
		{http.StatusInternalServerError, "error_500.json", providers.KindUnavailable},
	} {
		run(t, func(ctx context.Context, c connections.Credential) {
			id, err := google.New().Submit(ctx, c, videoReq, nil)
			if id != "" || kindOf(t, err) != tc.kind {
				t.Fatalf("%d: %q %v", tc.status, id, err)
			}

			if strings.Contains(err.Error(), "Invalid value") || strings.Contains(err.Error(), "API key") {
				t.Fatalf("the error forwards the body: %v", err)
			}
		}, submit(tc.fixture, tc.status))
	}
}

func TestPollStates(t *testing.T) {
	t.Parallel()

	// A task that ends with no clip is charged: the documentation of the Omni models does not say it is free.
	for _, tc := range []struct {
		fixture string
		state   string
		kind    string
		code    string
	}{
		{"poll_pending.json", providers.VideoPending, "", ""},
		{"poll_done.json", providers.VideoDone, "", ""},
		{"poll_incomplete.json", providers.VideoDone, "", ""},
		{"poll_failed_safety.json", providers.VideoFailed, providers.KindRefused, ""},
		{"poll_failed_other.json", providers.VideoFailed, providers.KindUnavailable, ""},
		{"poll_cancelled.json", providers.VideoFailed, providers.KindUnavailable, "cancelled"},
	} {
		run(t, func(ctx context.Context, c connections.Credential) {
			st, err := google.New().Poll(ctx, c, taskID)
			if err != nil || st.State != tc.state || st.Charged != (tc.state == providers.VideoFailed) {
				t.Fatalf("%s: %+v %v", tc.fixture, st, err)
			}

			if tc.kind == "" {
				if st.Err != nil {
					t.Fatalf("%s: %v", tc.fixture, st.Err)
				}

				return
			}

			pe, ok := providers.AsError(st.Err)
			if !ok || pe.Kind != tc.kind || pe.Code != tc.code {
				t.Fatalf("%s: %+v", tc.fixture, st.Err)
			}
		}, get(tc.fixture, 0))
	}
}

func TestPollOfAStateThatEndsNothingIsUnreadable(t *testing.T) {
	t.Parallel()

	run(t, func(ctx context.Context, c connections.Credential) {
		_, err := google.New().Poll(ctx, c, taskID)
		if kindOf(t, err) != providers.KindUnreadable {
			t.Fatal(err)
		}
	}, get("poll_unknown_status.json", 0))
}

func TestPollFailuresAreMappedByStatus(t *testing.T) {
	t.Parallel()

	run(t, func(ctx context.Context, c connections.Credential) {
		_, err := google.New().Poll(ctx, c, taskID)
		if kindOf(t, err) != providers.KindRateLimited {
			t.Fatal(err)
		}
	}, get("error_429.json", http.StatusTooManyRequests))
}

func TestFetchDecodesTheClip(t *testing.T) {
	t.Parallel()

	// An incomplete interaction is a completed one with incomplete results: its clip, when there is one, is taken.
	for _, fixture := range []string{"poll_done.json", "poll_incomplete.json"} {
		run(t, func(ctx context.Context, c connections.Credential) {
			f, err := google.New().Fetch(ctx, c, taskID, maxClip)
			if err != nil || !bytes.HasPrefix(f.Data, clipHead) || !isVideo(f.Data) {
				t.Fatalf("%s: %q %v", fixture, f.Data, err)
			}
		}, get(fixture, 0))
	}
}

func TestFetchOfAClipThatIsNotThere(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct{ fixture, kind string }{
		{"fetch_no_clip.json", providers.KindRefused},
		{"fetch_uri_only.json", providers.KindRefused},
		{"fetch_bad_base64.json", providers.KindUnreadable},
		{"poll_pending.json", providers.KindUnreadable},
	} {
		run(t, func(ctx context.Context, c connections.Credential) {
			f, err := google.New().Fetch(ctx, c, taskID, maxClip)
			if len(f.Data) != 0 || kindOf(t, err) != tc.kind {
				t.Fatalf("%s: %v", tc.fixture, err)
			}
		}, get(tc.fixture, 0))
	}
}

func TestFetchHoldsTheClipToTheLimit(t *testing.T) {
	t.Parallel()

	run(t, func(ctx context.Context, c connections.Credential) {
		_, err := google.New().Fetch(ctx, c, taskID, 8)
		if kindOf(t, err) != providers.KindUnreadable {
			t.Fatal(err)
		}
	}, get("poll_done.json", 0))
}

func TestCancelNeverConfirmsThatNothingIsCharged(t *testing.T) {
	t.Parallel()

	run(t, func(ctx context.Context, c connections.Credential) {
		confirmed, err := google.New().Cancel(ctx, c, taskID)
		if err != nil || confirmed {
			t.Fatalf("%v %v", confirmed, err)
		}
	}, providertest.Exchange{
		Method: "POST", Path: "/v1beta/interactions/" + taskID + "/cancel", Header: key, Response: "cancel_ok.json",
	})
}

func TestCancelFailureIsReported(t *testing.T) {
	t.Parallel()

	run(t, func(ctx context.Context, c connections.Credential) {
		confirmed, err := google.New().Cancel(ctx, c, taskID)
		if confirmed || kindOf(t, err) != providers.KindAuth {
			t.Fatalf("%v %v", confirmed, err)
		}
	}, providertest.Exchange{
		Method: "POST", Path: "/v1beta/interactions/" + taskID + "/cancel", Header: key,
		Status: http.StatusUnauthorized, Response: "error_401.json",
	})
}

func TestATaskIDThatIsNotValidMakesNoRequest(t *testing.T) {
	t.Parallel()

	// The recorder has no exchange: any request fails the test.
	run(t, func(ctx context.Context, c connections.Credential) {
		a := google.New()

		for _, id := range []string{"", "../models", "a b", "x?key=1"} {
			if _, err := a.Poll(ctx, c, id); kindOf(t, err) != providers.KindBadRequest {
				t.Fatalf("poll %q: %v", id, err)
			}

			if _, err := a.Fetch(ctx, c, id, maxClip); kindOf(t, err) != providers.KindBadRequest {
				t.Fatalf("fetch %q: %v", id, err)
			}

			if _, err := a.Cancel(ctx, c, id); kindOf(t, err) != providers.KindBadRequest {
				t.Fatalf("cancel %q: %v", id, err)
			}
		}
	})
}

func TestACancelledContextEndsTheCall(t *testing.T) {
	t.Parallel()

	// A context that is already cancelled ends the call before any socket, and the error is the context, not a provider kind.
	run(t, func(ctx context.Context, c connections.Credential) {
		ctx, cancel := context.WithCancel(ctx)
		cancel()

		if _, err := google.New().Poll(ctx, c, taskID); !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	})
}

func isVideo(b []byte) bool {
	_, ok := provider.CheckVideo(b)

	return ok
}
