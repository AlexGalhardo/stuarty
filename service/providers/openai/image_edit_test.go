package openai_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/provider"
	"github.com/AlexGalhardo/stuarty/internal/outbound"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/providers"
	"github.com/AlexGalhardo/stuarty/service/providers/openai"
	"github.com/AlexGalhardo/stuarty/service/providers/providertest"
)

// onePixel is the picture of testdata/images_edit_request.json: the same 70 bytes images_b64.json carries.
const onePixel = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGP4z8DwHwAFAAH/q842iQAAAABJRU5ErkJggg=="

var editReq = provider.EditRequest{
	Model: "gpt-image-1", Operation: provider.OperationRemoveBackground, Size: "1536x1024",
}

func source(t *testing.T) providers.SourceImage {
	t.Helper()

	data, err := base64.StdEncoding.DecodeString(onePixel)
	if err != nil {
		t.Fatal(err)
	}

	return providers.SourceImage{Data: data, Format: "png"}
}

func editExchange(response string) providertest.Exchange {
	return providertest.Exchange{
		Method:   "POST",
		Path:     "/v1/images/edits",
		Header:   auth,
		WantBody: "images_edit_request.json",
		Response: response,
	}
}

func TestRemoveBackgroundSendsThePictureAndReadsTheResult(t *testing.T) {
	t.Parallel()

	run(t, func(ctx context.Context, c connections.Credential) {
		res, err := openai.New().RemoveBackground(ctx, c, editReq, source(t), maxFile)
		if err != nil {
			t.Fatal(err)
		}

		if len(res.Files) != 1 || res.Failed != 0 || !bytes.HasPrefix(res.Files[0].Data, []byte("\x89PNG")) {
			t.Fatalf("got %+v", res)
		}
	}, editExchange("images_edit_b64.json"))
}

func TestRemoveBackgroundWithNoSizeLeavesItToTheProvider(t *testing.T) {
	t.Parallel()

	rec := run(t, func(ctx context.Context, c connections.Credential) {
		req := editReq
		req.Size = ""

		if _, err := openai.New().RemoveBackground(ctx, c, req, source(t), maxFile); err != nil {
			t.Fatal(err)
		}
	}, providertest.Exchange{
		Method: "POST", Path: "/v1/images/edits", Header: auth, AnyBody: true, Response: "images_edit_b64.json",
	})

	if body := string(rec.Last().Body); strings.Contains(body, `"size"`) {
		t.Fatalf("no size was asked for: %s", body)
	}
}

func TestRemoveBackgroundResultByURLIsFetchedThroughTheGate(t *testing.T) {
	t.Parallel()

	run(t, func(ctx context.Context, c connections.Credential) {
		res, err := openai.New().RemoveBackground(ctx, c, editReq, source(t), maxFile)
		if err != nil {
			t.Fatal(err)
		}

		if len(res.Files) != 1 || !bytes.Equal(res.Files[0].Data, pngBytes) {
			t.Fatalf("got %+v", res)
		}
	},
		editExchange("images_edit_url.json"),
		providertest.Exchange{
			Method: "GET", Path: "/files/cutout.png", Absent: []string{"Authorization"}, Body: pngBytes,
			ContentType: "image/png",
		},
	)
}

func TestRemoveBackgroundResultOnAForeignHostIsRefused(t *testing.T) {
	t.Parallel()

	rec := run(t, func(ctx context.Context, c connections.Credential) {
		_, err := openai.New().RemoveBackground(ctx, c, editReq, source(t), maxFile)
		if !errors.Is(err, outbound.ErrDestination) {
			t.Fatalf("the gate must refuse the host: %v", err)
		}
	}, providertest.Exchange{
		Method: "POST", Path: "/v1/images/edits", Header: auth, AnyBody: true, Response: "images_foreign_url.json",
	})

	if len(rec.Requests) != 1 {
		t.Fatalf("one request: %d", len(rec.Requests))
	}
}

func TestRemoveBackgroundResultOverTheLimitIsNotKept(t *testing.T) {
	t.Parallel()

	run(t, func(ctx context.Context, c connections.Credential) {
		_, err := openai.New().RemoveBackground(ctx, c, editReq, source(t), 69)

		pe, ok := providers.AsError(err)
		if !ok || pe.Kind != providers.KindUnreadable {
			t.Fatalf("got %v", err)
		}
	}, editExchange("images_edit_b64.json"))
}

func TestRemoveBackgroundFailures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		x      providertest.Exchange
		kind   string
		status int
	}{
		{
			"a refusal",
			providertest.Exchange{Status: 400, Response: "error_400_policy.json"},
			providers.KindRefused,
			400,
		},
		{
			"a rate limit",
			providertest.Exchange{Status: 429, Response: "error_429.json"},
			providers.KindRateLimited,
			429,
		},
		{"the key refused", providertest.Exchange{Status: 401, Response: "error_401.json"}, providers.KindAuth, 401},
		{"no images", providertest.Exchange{Response: "images_empty.json"}, providers.KindUnreadable, 200},
		{"not base64", providertest.Exchange{Response: "images_bad_base64.json"}, providers.KindUnreadable, 200},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tc.x.Method, tc.x.Path, tc.x.AnyBody = "POST", "/v1/images/edits", true

			run(t, func(ctx context.Context, c connections.Credential) {
				_, err := openai.New().RemoveBackground(ctx, c, editReq, source(t), maxFile)

				pe, ok := providers.AsError(err)
				if !ok || pe.Kind != tc.kind || pe.Status != tc.status {
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

func TestRemoveBackgroundNeedsAPicture(t *testing.T) {
	t.Parallel()

	run(t, func(ctx context.Context, c connections.Credential) {
		_, err := openai.New().RemoveBackground(ctx, c, editReq, providers.SourceImage{}, maxFile)

		pe, ok := providers.AsError(err)
		if !ok || pe.Kind != providers.KindBadRequest {
			t.Fatalf("got %v", err)
		}
	})
}

func TestEditLimits(t *testing.T) {
	t.Parallel()

	a := openai.New()

	l := a.EditLimits("gpt-image-1")
	if len(l.Operations) != 1 || l.Operations[0] != provider.OperationRemoveBackground || len(l.Sizes) != 3 {
		t.Errorf("gpt-image-1: %+v", l)
	}

	if l := a.EditLimits("some-new-model"); len(l.Operations) != 0 || len(l.Sizes) != 0 || l.MaxSourceBytes != 0 {
		t.Errorf("a model the documentation does not name has no limit: %+v", l)
	}

	if f := provider.ValidateEdit(provider.NormalizeEdit(editReq), a.EditLimits(editReq.Model)); f != nil {
		t.Fatalf("the documented limits are the ones core/provider accepts: %v", f)
	}

	if _, ok := any(a).(providers.ImageEditor); !ok {
		t.Fatal("the adapter implements ImageEditor")
	}
}
