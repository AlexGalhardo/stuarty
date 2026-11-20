package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func pngOf2(t *testing.T, img image.Image) []byte {
	t.Helper()

	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}

	return b.Bytes()
}

func TestImageEdit_HasTransparency(t *testing.T) {
	t.Parallel()

	opaque := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for i := 3; i < len(opaque.Pix); i += 4 {
		opaque.Pix[i] = 0xFF
	}

	oneHole := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	copy(oneHole.Pix, opaque.Pix)
	oneHole.Pix[len(oneHole.Pix)-1] = 0xFE

	halfAlpha := image.NewRGBA(image.Rect(0, 0, 2, 2))
	halfAlpha.SetRGBA(1, 1, color.RGBA{R: 10, G: 10, B: 10, A: 100})

	paletted := image.NewPaletted(image.Rect(0, 0, 2, 2), color.Palette{
		color.NRGBA{R: 1, G: 2, B: 3, A: 255}, color.NRGBA{A: 0},
	})
	paletted.SetColorIndex(1, 1, 1)

	gray := image.NewGray(image.Rect(0, 0, 3, 3))

	cases := []struct {
		name string
		data []byte
		want bool
	}{
		{"an opaque picture", pngOf2(t, opaque), false},
		{"one pixel that is not opaque", pngOf2(t, oneHole), true},
		{"a picture of the other layout", pngOf2(t, halfAlpha), true},
		{"a palette with a transparent colour", pngOf2(t, paletted), true},
		{"a picture with no alpha channel", pngOf2(t, gray), false},
		{"bytes that are not a picture", []byte("not a picture"), false},
		{"the start of a picture", pngOf2(t, opaque)[:30], false},
		{"nothing", nil, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			if got := hasTransparency(c.data); got != c.want {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
	}
}

// A picture that declares more pixels than the largest canvas is refused before it is decoded.
func TestImageEdit_HasTransparencyRefusesAPictureThatIsTooLargeBeforeDecodingIt(t *testing.T) {
	t.Parallel()

	// A PNG header that declares 100,000 by 100,000 pixels and nothing else: decoding it would need 40 GB.
	head := pngOf2(t, image.NewNRGBA(image.Rect(0, 0, 2, 2)))
	big := bytes.Clone(head)
	// IHDR width and height are at bytes 16 to 23 of a PNG.
	copy(big[16:24], []byte{0x00, 0x01, 0x86, 0xA0, 0x00, 0x01, 0x86, 0xA0})

	if hasTransparency(big) {
		t.Fatal("a picture over the pixel limit was looked into")
	}
}

func TestImageEdit_OperationsAreWellFormed(t *testing.T) {
	t.Parallel()

	got := operation("layer.fit", map[string]any{"layer_id": "lyr_aaaaaaaaaa", "mode": "cover"})

	var args map[string]string
	if err := json.Unmarshal(got.Args, &args); err != nil || got.Op != "layer.fit" || args["mode"] != "cover" {
		t.Fatalf("got %+v (%v)", got, err)
	}
}

func TestImageEdit_ArgumentsAreStrictAndAKeyLimitedToAProjectIsRefusedBeforeAnythingIsRead(t *testing.T) {
	t.Parallel()

	r := newGenerateRig(t)
	ctx := WithCaller(context.Background(), Caller{Actor: "local:x", Scopes: AllScopes})
	start := func(raw string) error {
		_, err := r.s.prepareImageEdit(ctx, JobStartRequest{Kind: JobImageEdit, Args: json.RawMessage(raw)})

		return err
	}

	for name, raw := range map[string]string{
		"empty":          ``,
		"not an object":  `[1]`,
		"unknown member": `{"connection_id":"c","image_id":"img_0000000001","operation":"generate","model":"m","count":3}`,
		"wrong type":     `{"connection_id":"c","image_id":3}`,
	} {
		var e *Error
		if err := start(raw); !errors.As(err, &e) || e.Code != "E_VALIDATION" || e.Path != "args" {
			t.Errorf("%s: got %v", name, err)
		}
	}

	var e *Error
	if err := start(
		`{"connection_id":"c","image_id":"nope","operation":"generate","model":"m"}`,
	); !errors.As(
		err,
		&e,
	) ||
		e.Path != "args.image_id" {
		t.Errorf("a bad image id: %v", err)
	}

	limited := WithCaller(context.Background(), Caller{Actor: "key:k", Scopes: AllScopes, ProjectID: "prj_0000000001"})
	_, err := r.s.prepareImageEdit(limited, JobStartRequest{
		Kind: JobImageEdit,
		Args: json.RawMessage(`{"connection_id":"c","image_id":"img_0000000001","operation":"generate","model":"m"}`),
	})

	if !errors.As(err, &e) || e.Code != CodeForbidden {
		t.Errorf("a key limited to one project: %v", err)
	}

	if n := r.maker.calls.Load(); n != 0 {
		t.Fatalf("Prepare made %d calls to the provider", n)
	}
}
