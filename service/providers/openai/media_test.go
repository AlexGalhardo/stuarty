package openai_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/provider"
	"github.com/AlexGalhardo/stuarty/internal/outbound"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/connections/connectionstest"
	"github.com/AlexGalhardo/stuarty/service/providers"
	"github.com/AlexGalhardo/stuarty/service/providers/openai"
	"github.com/AlexGalhardo/stuarty/service/providers/providertest"
)

const maxFile = 1 << 20

// pngBytes is the start of a PNG: the signature is all the product reads of it before the ingest probes it.
var pngBytes = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")

var imageReq = provider.ImageRequest{
	Model: "gpt-image-1", Prompt: "A red fox in the snow, soft morning light", Size: "1536x1024", Count: 2,
}

func imageExchange(response string) providertest.Exchange {
	return providertest.Exchange{
		Method:   "POST",
		Path:     "/v1/images/generations",
		Header:   auth,
		WantBody: "images_request.json",
		Response: response,
	}
}

func TestImagesBase64(t *testing.T) {
	t.Parallel()

	run(t, func(ctx context.Context, c connections.Credential) {
		res, err := openai.New().GenerateImages(ctx, c, imageReq, maxFile)
		if err != nil {
			t.Fatal(err)
		}

		if len(res.Files) != 2 || res.Failed != 0 || res.Err != nil {
			t.Fatalf("got %+v", res)
		}

		for _, f := range res.Files {
			if !bytes.HasPrefix(f.Data, []byte("\x89PNG")) {
				t.Fatalf("not decoded: %q", f.Data)
			}
		}
	}, imageExchange("images_b64.json"))
}

func TestImagesByURLAreFetchedFromTheSameServer(t *testing.T) {
	t.Parallel()

	rec := run(
		t,
		func(ctx context.Context, c connections.Credential) {
			res, err := openai.New().GenerateImages(ctx, c, imageReq, maxFile)
			if err != nil {
				t.Fatal(err)
			}

			if len(res.Files) != 2 || !bytes.Equal(res.Files[0].Data, pngBytes) ||
				res.Files[0].RevisedPrompt != "A red fox standing in fresh snow at dawn, soft light" {
				t.Fatalf("got %+v", res)
			}
		},
		imageExchange("images_url.json"),
		providertest.Exchange{
			Method:      "GET",
			Path:        "/files/fox-1.png",
			Absent:      []string{"Authorization"},
			Body:        pngBytes,
			ContentType: "image/png",
		},
		providertest.Exchange{
			Method:      "GET",
			Path:        "/files/fox-2.png",
			Absent:      []string{"Authorization"},
			Body:        pngBytes,
			ContentType: "image/png",
		},
	)

	if len(rec.Requests) != 3 {
		t.Fatalf("requests: %d", len(rec.Requests))
	}
}

func TestAnImageUrlOnAHostTheConnectionDoesNotListIsRefusedAndTheKeyNeverGoesThere(t *testing.T) {
	t.Parallel()

	rec := run(t, func(ctx context.Context, c connections.Credential) {
		req := imageReq
		req.Count = 1

		_, err := openai.New().GenerateImages(ctx, c, req, maxFile)
		if !errors.Is(err, outbound.ErrDestination) {
			t.Fatalf("the gate must refuse the host: %v", err)
		}
	}, providertest.Exchange{
		Method:   "POST",
		Path:     "/v1/images/generations",
		Header:   auth,
		AnyBody:  true,
		Response: "images_foreign_url.json",
	})

	if len(rec.Requests) != 1 {
		t.Fatalf("one request: %d", len(rec.Requests))
	}
}

func TestAPartialBatchKeepsWhatArrived(t *testing.T) {
	t.Parallel()

	run(t, func(ctx context.Context, c connections.Credential) {
		res, err := openai.New().GenerateImages(ctx, c, imageReq, maxFile)
		if err != nil {
			t.Fatal(err)
		}

		pe, ok := providers.AsError(res.Err)
		if len(res.Files) != 1 || res.Failed != 1 || !ok || pe.Kind != providers.KindBadRequest || pe.Status != 404 {
			t.Fatalf("got %+v", res)
		}
	},
		imageExchange("images_url.json"),
		providertest.Exchange{Method: "GET", Path: "/files/fox-1.png", Body: pngBytes},
		providertest.Exchange{Method: "GET", Path: "/files/fox-2.png", Status: 404},
	)
}

func TestAFileLargerThanTheLimitIsNotKept(t *testing.T) {
	t.Parallel()

	run(t, func(ctx context.Context, c connections.Credential) {
		_, err := openai.New().GenerateImages(ctx, c, imageReq, int64(len(pngBytes)-1))

		pe, ok := providers.AsError(err)
		if !ok || pe.Kind != providers.KindUnreadable {
			t.Fatalf("got %v", err)
		}
	},
		imageExchange("images_url.json"),
		providertest.Exchange{Method: "GET", Path: "/files/fox-1.png", Body: pngBytes},
		providertest.Exchange{Method: "GET", Path: "/files/fox-2.png", Body: pngBytes},
	)
}

func TestImageFailures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		x      providertest.Exchange
		kind   string
		status int
		code   string
	}{
		{
			"a refused prompt",
			providertest.Exchange{Status: 400, Response: "error_400_policy.json"},
			providers.KindRefused,
			400,
			"content_policy_violation",
		},
		{
			"a rate limit",
			providertest.Exchange{Status: 429, Response: "error_429.json"},
			providers.KindRateLimited,
			429,
			"insufficient_quota",
		},
		{
			"the key refused",
			providertest.Exchange{Status: 401, Response: "error_401.json"},
			providers.KindAuth,
			401,
			"invalid_api_key",
		},
		{"no images", providertest.Exchange{Response: "images_empty.json"}, providers.KindUnreadable, 200, ""},
		{"not base64", providertest.Exchange{Response: "images_bad_base64.json"}, providers.KindUnreadable, 200, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tc.x.Method, tc.x.Path, tc.x.AnyBody = "POST", "/v1/images/generations", true

			run(t, func(ctx context.Context, c connections.Credential) {
				_, err := openai.New().GenerateImages(ctx, c, imageReq, maxFile)

				pe, ok := providers.AsError(err)
				if !ok || pe.Kind != tc.kind || pe.Status != tc.status || pe.Code != tc.code {
					t.Fatalf("got %+v (%v)", pe, err)
				}

				for _, banned := range []string{secret, "safety system", "exceeded"} {
					if strings.Contains(err.Error(), banned) {
						t.Errorf("the error %q holds %q", err.Error(), banned)
					}
				}
			}, tc.x)
		})
	}
}

func TestImageLimits(t *testing.T) {
	t.Parallel()

	a := openai.New()

	if l := a.ImageLimits("dall-e-3"); l.MaxCount != 1 || len(l.Sizes) != 3 {
		t.Errorf("dall-e-3: %+v", l)
	}

	if l := a.ImageLimits("gpt-image-1"); l.MaxCount != 10 || len(l.Sizes) != 3 {
		t.Errorf("gpt-image-1: %+v", l)
	}

	if l := a.ImageLimits("some-new-model"); l.MaxCount != 0 || len(l.Sizes) != 0 {
		t.Errorf("a model the documentation does not name has no limit: %+v", l)
	}

	// The documented limits are the ones core/provider accepts.
	req := provider.ImageRequest{Model: "dall-e-3", Prompt: "p", Size: "1792x1024", Count: 1}
	if f := provider.ValidateImage(req, a.ImageLimits("dall-e-3")); f != nil {
		t.Fatal(f)
	}

	req.Count = 2
	if f := provider.ValidateImage(req, a.ImageLimits("dall-e-3")); f == nil || f.Path != "count" {
		t.Fatalf("dall-e-3 makes one image at a time: %v", f)
	}
}

var speechReq = provider.VoiceRequest{Model: "tts-1", Text: "Welcome to the channel.", Voice: "alloy", Format: "mp3"}

func TestSpeech(t *testing.T) {
	t.Parallel()

	audio := []byte("ID3\x04\x00\x00\x00\x00\x00\x00 fake mp3 frames")

	rec := run(t, func(ctx context.Context, c connections.Credential) {
		res, err := openai.New().GenerateVoice(ctx, c, speechReq, maxFile)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(res.File.Data, audio) {
			t.Fatalf("got %q", res.File.Data)
		}
	}, providertest.Exchange{
		Method: "POST", Path: "/v1/audio/speech", Header: auth, WantBody: "speech_request.json", Body: audio,
		ContentType: "audio/mpeg",
	})

	if got := rec.Last().Header.Get("Accept"); got != "*/*" {
		t.Errorf("accept %q", got)
	}
}

func TestSpeechFailures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		x    providertest.Exchange
		kind string
	}{
		{
			"a refused text",
			providertest.Exchange{Status: 400, Response: "error_400_policy.json"},
			providers.KindRefused,
		},
		{"a rate limit", providertest.Exchange{Status: 429, Response: "error_429.json"}, providers.KindRateLimited},
		{"server error", providertest.Exchange{Status: 500, Response: "error_500.json"}, providers.KindUnavailable},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tc.x.Method, tc.x.Path, tc.x.AnyBody = "POST", "/v1/audio/speech", true

			run(t, func(ctx context.Context, c connections.Credential) {
				_, err := openai.New().GenerateVoice(ctx, c, speechReq, maxFile)

				if pe, ok := providers.AsError(err); !ok || pe.Kind != tc.kind {
					t.Fatalf("got %v", err)
				}
			}, tc.x)
		})
	}
}

func TestSpeechLargerThanTheLimitIsUnreadable(t *testing.T) {
	t.Parallel()

	run(t, func(ctx context.Context, c connections.Credential) {
		_, err := openai.New().GenerateVoice(ctx, c, speechReq, 8)

		if pe, ok := providers.AsError(err); !ok || pe.Kind != providers.KindUnreadable {
			t.Fatalf("got %v", err)
		}
	}, providertest.Exchange{Method: "POST", Path: "/v1/audio/speech", AnyBody: true, Body: bytes.Repeat([]byte("x"), 100)})
}

func TestVoicesAreTheDocumentedOnesAndNeedNoRequest(t *testing.T) {
	t.Parallel()

	// No exchange: a request would fail the contract.
	run(t, func(ctx context.Context, c connections.Credential) {
		got, err := openai.New().Voices(ctx, c)
		if err != nil || len(got) < 10 {
			t.Fatalf("got %v, %v", got, err)
		}

		for _, v := range got {
			if v.Source != providers.SourceDocumented || !provider.ValidModel(v.ID) || v.Name == "" {
				t.Fatalf("%+v", v)
			}
		}
	})
}

func TestVoiceLimitsAreTheOnesCoreAccepts(t *testing.T) {
	t.Parallel()

	l := openai.New().VoiceLimits("tts-1")
	if l.MaxChars != 4096 || len(l.Formats) != len(provider.VoiceFormats) {
		t.Fatalf("%+v", l)
	}

	long := speechReq
	long.Text = strings.Repeat("a", 4097)

	if f := provider.ValidateVoice(long, l); f == nil || f.Path != "text" {
		t.Fatalf("the endpoint takes 4096 characters: %v", f)
	}
}

func TestTheCapabilityHintOfTheCatalogue(t *testing.T) {
	t.Parallel()

	rec := providertest.Serve(t, "testdata",
		providertest.Exchange{Method: "GET", Path: "/v1/models", Response: "models_media.json"})
	h := connectionstest.New(t, openai.ID, rec.Endpoint("/v1"), secret)

	h.Do(t, func(ctx context.Context, c connections.Credential) {
		got, err := openai.New().Models(ctx, c)
		if err != nil {
			t.Fatal(err)
		}

		want := map[string]string{
			"gpt-4o": "text", "o3-mini": "text", "dall-e-3": "image", "gpt-image-1": "image", "tts-1": "voice",
			"gpt-4o-mini-tts": "voice", "text-embedding-3-small": "", "whisper-1": "",
		}

		for _, m := range got {
			if w, ok := want[m.ID]; !ok || m.Capability != w {
				t.Errorf("%s: capability %q, want %q", m.ID, m.Capability, w)
			}

			delete(want, m.ID)
		}

		if len(want) != 0 {
			t.Errorf("missing %v", want)
		}
	})
}

// The client of a connection gives up after its timeout; that is a failure of its own, not an unreadable answer.
func TestARequestThatTimesOutIsATimeout(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))

	t.Cleanup(func() {
		close(release)
		srv.Close()
	})

	h := connectionstest.New(t, openai.ID, srv.URL+"/v1", secret)

	err := h.Manager.WithTimeout(context.Background(), connectionstest.ID, 100*time.Millisecond,
		func(c connections.Credential) error {
			_, gerr := openai.New().GenerateImages(context.Background(), c, imageReq, maxFile)

			if pe, ok := providers.AsError(gerr); !ok || pe.Kind != providers.KindTimeout {
				t.Errorf("got %v", gerr)
			}

			_, verr := openai.New().GenerateVoice(context.Background(), c, speechReq, maxFile)
			if pe, ok := providers.AsError(verr); !ok || pe.Kind != providers.KindTimeout {
				t.Errorf("speech: got %v", verr)
			}

			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
}
