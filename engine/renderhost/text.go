package renderhost

import (
	"context"
	"errors"
)

// TextError is a failure of a text request that carries the structured detail the module encoded:
// for font_unknown the family, weight, weights and digest; for font_invalid the reason and message.
type TextError struct {
	Reason Reason
	Detail string
	Data   []byte
}

func (e *TextError) Error() string {
	if e.Detail == "" {
		return "renderhost: " + string(e.Reason)
	}

	return "renderhost: " + string(e.Reason) + ": " + e.Detail
}

// maxRequestBytes bounds a text request: a caption of thousands of words is far below it.
const maxRequestBytes = 16 << 20

// maxFontBytes is the largest user font the module accepts (SDD-007 section 7.7).
const maxFontBytes = 16 << 20

// PutFont stores the bytes of a user font under its lower-case hex SHA-256 for the requests that name
// it; empty data records that the file is not stored. The caller has checked that the bytes hash to the
// digest.
func (h *Host) PutFont(ctx context.Context, hash string, data []byte) error {
	if len(hash) != 64 || len(data) > maxFontBytes {
		return &Error{InvalidArgument, "PutFont"}
	}

	id, err := h.put(ctx, []byte(hash))
	if err != nil {
		return err
	}

	px, err := h.put(ctx, data)
	if err != nil {
		h.release(ctx, id)

		return err
	}

	defer h.release(ctx, id, px)

	return h.status(ctx, "vr_text_font_put", uint64(id.ptr), uint64(id.n), uint64(px.ptr), uint64(px.n))
}

// RunText draws or measures a shape, a text or a caption (the request format is in SDD-013 section 4.2)
// and returns the encoded result. A failure that has a structured detail is a *TextError.
func (h *Host) RunText(ctx context.Context, request []byte) ([]byte, error) {
	if len(request) > maxRequestBytes {
		return nil, &Error{LimitExceeded, "request_bytes"}
	}

	in, err := h.put(ctx, request)
	if err != nil {
		return nil, err
	}

	defer h.release(ctx, in)

	if err := h.textStatus(ctx, "vr_text_run", uint64(in.ptr), uint64(in.n)); err != nil {
		return nil, err
	}

	return h.takeResult(ctx)
}

// ValidateFont checks a user font file; a refusal is a *TextError of reason font_invalid.
func (h *Host) ValidateFont(ctx context.Context, data []byte) error {
	if len(data) > maxFontBytes {
		return &Error{LimitExceeded, "font_bytes"}
	}

	in, err := h.put(ctx, data)
	if err != nil {
		return err
	}

	defer h.release(ctx, in)

	err = h.textStatus(ctx, "vr_text_validate_font", uint64(in.ptr), uint64(in.n))
	h.clearResult(ctx)

	return err
}

// textStatus is status, with the structured detail of the failure attached.
func (h *Host) textStatus(ctx context.Context, name string, args ...uint64) error {
	err := h.status(ctx, name, args...)

	var e *Error
	if !errors.As(err, &e) {
		return err
	}

	switch e.Reason {
	case FontUnknown, FontInvalid, FontNeeded, InvalidSpec, TextFailed:
		data, rerr := h.takeResult(ctx)
		if rerr != nil {
			return rerr
		}

		return &TextError{Reason: e.Reason, Detail: e.Detail, Data: data}
	default:
		return err
	}
}

// takeResult copies the result of the last text call out of the module and releases it there.
func (h *Host) takeResult(ctx context.Context) ([]byte, error) {
	ptr, err := h.call(ctx, "vr_text_result_ptr")
	if err != nil {
		return nil, err
	}

	n, err := h.call(ctx, "vr_text_result_len")
	if err != nil {
		return nil, err
	}

	var out []byte

	if n[0] > 0 {
		b, ok := h.mem.Read(uint32(ptr[0]), uint32(n[0])) //nolint:gosec // G115: offsets and lengths of a 32-bit memory
		if !ok {
			return nil, &Error{OutOfMemory, "reading the text result"}
		}

		out = make([]byte, len(b))
		copy(out, b)
	}

	h.clearResult(ctx)

	return out, nil
}

func (h *Host) clearResult(ctx context.Context) {
	_, _ = h.call(ctx, "vr_text_result_clear")
}
