package renderhost

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"math"
	"math/rand/v2"
	"strings"
	"testing"
)

// textRequest builds a text request by hand (SDD-013 section 4.2): mode, kind 1, scale, no document
// fonts or the given ones, a plain Inter style of size 40 and the text.
func textRequest(mode byte, family string, docs map[string]string, text string) []byte {
	var b []byte

	u32 := func(v int) { b = binary.LittleEndian.AppendUint32(b, uint32(v)) }
	f64 := func(v float64) { b = binary.LittleEndian.AppendUint64(b, math.Float64bits(v)) }
	str := func(s string) { u32(len(s)); b = append(b, s...) }
	col := func(r, g, bl, a byte) { b = append(b, r, g, bl, a) }

	b = append(b, mode, 1)
	f64(1)
	u32(len(docs))

	for name, hash := range docs {
		str(name)
		u32(1)
		u32(400)
		str(hash)
	}

	str(family)
	u32(400)
	f64(40)
	col(255, 255, 255, 255)
	col(0, 0, 0, 0)
	f64(0)
	col(0, 0, 0, 0)
	f64(0)
	f64(0)
	f64(0)
	b = append(b, 0)
	f64(0)
	f64(0)
	b = append(b, 1)
	f64(1.2)
	f64(800)
	b = append(b, 0, 0)
	col(0, 0, 0, 0)
	b = append(b, 0)
	str(text)

	return b
}

func newTextHost(t *testing.T) *Host {
	t.Helper()

	h, err := New(context.Background(), Config{})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = h.Close(context.Background()) })

	return h
}

func TestTextRendersAndMeasuresTheSameSize(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	h := newTextHost(t)

	drawn, err := h.RunText(ctx, textRequest(0, "Inter", nil, "Wow"))
	if err != nil {
		t.Fatal(err)
	}

	w, ht := int(binary.LittleEndian.Uint32(drawn)), int(binary.LittleEndian.Uint32(drawn[4:]))
	if w == 0 || ht == 0 || len(drawn) != 44+w*ht*4 {
		t.Fatalf("a %dx%d bitmap in %d bytes", w, ht, len(drawn))
	}

	measured, err := h.RunText(ctx, textRequest(1, "Inter", nil, "Wow"))
	if err != nil {
		t.Fatal(err)
	}

	if len(measured) != 44 || !bytes.Equal(measured[8:24], drawn[8:24]) {
		t.Fatalf("measured %d bytes, natural size %x, drawn %x", len(measured), measured[8:24], drawn[8:24])
	}

	again, err := h.RunText(ctx, textRequest(0, "Inter", nil, "Wow"))
	if err != nil || !bytes.Equal(again, drawn) {
		t.Fatalf("a second request drew other bytes (err %v)", err)
	}
}

func TestTextAsksForTheUserFontItNeeds(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	h := newTextHost(t)
	hash := strings.Repeat("ab", 32)
	req := textRequest(0, "Mine", map[string]string{"Mine": hash}, "x")

	var te *TextError
	if _, err := h.RunText(ctx, req); !errors.As(err, &te) || te.Reason != FontNeeded || te.Detail != hash {
		t.Fatalf("error = %v, want font_needed for %s", err, hash)
	}

	if err := h.PutFont(ctx, hash, []byte("not a font")); err != nil {
		t.Fatal(err)
	}

	if _, err := h.RunText(ctx, req); !errors.As(err, &te) || te.Reason != TextFailed {
		t.Fatalf("bytes that do not hash to the digest: %v", err)
	}

	if err := h.PutFont(ctx, strings.Repeat("A", 64), nil); err != nil {
		t.Logf("an upper-case digest is refused: %v", err)
	} else {
		t.Fatal("an upper-case digest was accepted")
	}

	_, err := h.RunText(ctx, textRequest(0, "Nope", nil, "x"))
	if !errors.As(err, &te) || te.Reason != FontUnknown || len(te.Data) == 0 {
		t.Fatalf("an unknown family: %v", err)
	}
}

func TestValidateFontRefusesJunkWithItsReason(t *testing.T) {
	t.Parallel()

	var te *TextError

	err := newTextHost(t).ValidateFont(context.Background(), []byte("this is not a font file at all"))
	if !errors.As(err, &te) || te.Reason != FontInvalid || len(te.Data) == 0 {
		t.Fatalf("error = %v", err)
	}
}

// A request is attacker-shaped input as far as the module is concerned (the plan names the text): the
// module refuses what it cannot read and the instance stays usable.
func TestSecurity_T36_TextRequestsSurviveHostileInput(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	h := newTextHost(t)
	good := textRequest(0, "Inter", nil, "ok")
	rng := rand.New(rand.NewPCG(7, 9))

	for range 300 {
		b := append([]byte(nil), good...)

		switch rng.IntN(3) {
		case 0:
			b = b[:rng.IntN(len(b))]
		case 1:
			b[rng.IntN(len(b))] = byte(rng.IntN(256))
		default:
			b = append(b[:rng.IntN(len(b))], b[rng.IntN(len(b)):]...)
		}

		if _, err := h.RunText(ctx, b); err != nil {
			var re *Error

			var te *TextError

			if h.dead != nil || (!errors.As(err, &re) && !errors.As(err, &te)) {
				t.Fatalf("a hostile request broke the instance: %v", err)
			}
		}
	}

	if _, err := h.RunText(ctx, good); err != nil {
		t.Fatalf("the instance is no longer usable: %v", err)
	}
}
