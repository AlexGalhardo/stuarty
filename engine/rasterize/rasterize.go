package rasterize

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"

	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/engine/renderhost"
)

// engine is the one instance of the renderer module that draws bitmaps for the process. Calls are
// serialised: a bitmap is drawn in milliseconds and the callers cache what they draw. An instance that
// trapped is discarded and the next call makes a new one.
var engine struct {
	mu   sync.Mutex
	host *renderhost.Host
}

// maxFontRounds bounds the number of user fonts one request may ask for in turn.
const maxFontRounds = 64

func hostLocked(ctx context.Context) (*renderhost.Host, error) {
	if engine.host != nil {
		return engine.host, nil
	}

	h, err := renderhost.New(ctx, renderhost.Config{Uncancellable: true})
	if err != nil {
		return nil, fmt.Errorf("rasterize: starting the renderer module: %w", err)
	}

	engine.host = h

	return h, nil
}

// run sends a request to the module, giving it the user fonts it asks for, and returns the result.
func run(req []byte, fonts *FontSet, hash string) ([]byte, error) {
	ctx := context.Background()

	engine.mu.Lock()
	defer engine.mu.Unlock()

	if hash != "" && !fonts.wasSupplied(hash) {
		h, err := hostLocked(ctx)
		if err != nil {
			return nil, err
		}

		if err := supplyFont(ctx, h, fonts, hash); err != nil {
			return nil, err
		}
	}

	for range maxFontRounds {
		h, err := hostLocked(ctx)
		if err != nil {
			return nil, err
		}

		out, err := h.RunText(ctx, req)
		if err == nil {
			return out, nil
		}

		var te *renderhost.TextError
		if errors.As(err, &te) && te.Reason == renderhost.FontNeeded {
			if err := supplyFont(ctx, h, fonts, te.Detail); err != nil {
				return nil, err
			}

			continue
		}

		return nil, discardIfDead(err)
	}

	return nil, errors.New("rasterize: a request asked for too many fonts")
}

// discardIfDead drops the instance after a trap, so the next call starts a new one.
func discardIfDead(err error) error {
	var re *renderhost.Error
	if errors.As(err, &re) && re.Reason == renderhost.RenderTrap || errors.Is(err, renderhost.ErrClosed) {
		engine.host = nil
	}

	return err
}

// supplyFont reads the file of a user font from the source and hands it to the module; a file the
// source does not have is the E_FONT_UNKNOWN of its family and weight.
func supplyFont(ctx context.Context, h *renderhost.Host, fonts *FontSet, hash string) error {
	family, weight := "", 0
	if fonts != nil {
		family, weight = fonts.faceOf(hash)
	}

	notStored := &FontError{Code: CodeFontUnknown, Family: family, Weight: weight, Hash: hash}

	if fonts == nil || fonts.src == nil {
		return notStored
	}

	data, err := fonts.src(hash)
	if errors.Is(err, ErrFontNotStored) {
		return notStored
	}

	if err != nil {
		return fmt.Errorf("raster: read font %s: %w", hash, err)
	}

	if sum := sha256.Sum256(data); hex.EncodeToString(sum[:]) != hash {
		return fmt.Errorf("raster: font %s does not match its hash", hash)
	}

	if err := discardIfDead(h.PutFont(ctx, hash, data)); err != nil {
		return err
	}

	fonts.markSupplied(hash)

	return nil
}

// Warm compiles the renderer module and starts the instance that draws bitmaps, so that the first text of the
// process does not wait for it (about a second for the optimizing compiler of wazero). It is safe to call
// from a goroutine at start-up and more than once; a failure is reported by the first Render.
func Warm() {
	engine.mu.Lock()
	defer engine.mu.Unlock()

	_, _ = hostLocked(context.Background())
}

// Render draws the bitmap of the spec. It is deterministic: the same spec gives
// the same bytes on every platform.
func Render(spec plan.BitmapSpec, fonts *FontSet) (Bitmap, error) {
	req, err := request(spec, fonts, modeRender)
	if err != nil {
		return Bitmap{}, err
	}

	out, err := run(req, fonts, fonts.hashFor(spec))
	if err != nil {
		return Bitmap{}, translate(err)
	}

	return decodeBitmap(out)
}

// Measure returns the plan.Measurer that gives the natural size of a bitmap, in
// frame pixels at PixelScale 1: exactly the NaturalW and NaturalH Render
// produces for the same spec, without drawing it (SDD-004 section 4.2). A spec
// the rasterizer cannot draw measures as 0 by 0; Render reports why.
func Measure(fonts *FontSet) plan.Measurer {
	return func(spec plan.BitmapSpec) (w, h float64) {
		req, err := request(spec, fonts, modeMeasure)
		if err != nil {
			return 0, 0
		}

		out, err := run(req, fonts, fonts.hashFor(spec))
		if err != nil {
			return 0, 0
		}

		bm, err := decodeBitmap(out)
		if err != nil {
			return 0, 0
		}

		return bm.NaturalW, bm.NaturalH
	}
}

// ValidateFont checks the bytes of a user font before it is stored or used
// (SDD-007 section 7.7, T-21): size, magic, table directory, then a parse that
// is bounded by construction. It returns *InvalidFontError.
func ValidateFont(data []byte) error {
	if len(data) > MaxFontBytes {
		return &InvalidFontError{
			Reason:  FontTooLarge,
			Message: fmt.Sprintf("%d bytes, the limit is %d", len(data), MaxFontBytes),
		}
	}

	engine.mu.Lock()
	defer engine.mu.Unlock()

	h, err := hostLocked(context.Background())
	if err != nil {
		return err
	}

	return translate(discardIfDead(h.ValidateFont(context.Background(), data)))
}

// translate turns the failure of the module into the errors this package documents.
func translate(err error) error {
	var te *renderhost.TextError
	if !errors.As(err, &te) {
		return err
	}

	switch te.Reason {
	case renderhost.InvalidSpec:
		return fmt.Errorf("%w: %s", ErrInvalidSpec, te.Detail)
	case renderhost.FontUnknown:
		return decodeFontError(te)
	case renderhost.FontInvalid:
		r := reader{b: te.Data}
		reason, message := r.str(), r.str()

		if r.err != nil {
			return fmt.Errorf("rasterize: %w", te)
		}

		return &InvalidFontError{Reason: reason, Message: message}
	default:
		return fmt.Errorf("raster: %s", te.Detail)
	}
}

func decodeFontError(te *renderhost.TextError) error {
	r := reader{b: te.Data}
	fe := &FontError{Code: CodeFontUnknown, Family: r.str(), Weight: r.u32()}

	if r.u8() == 1 {
		fe.Weights = make([]int, 0, 8)

		for range r.u32() {
			fe.Weights = append(fe.Weights, r.u32())
		}
	}

	if r.u8() == 1 {
		fe.Hash = r.str()
	}

	if r.err != nil {
		return fmt.Errorf("rasterize: %w", te)
	}

	return fe
}
