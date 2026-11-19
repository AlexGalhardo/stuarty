package export

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"testing"

	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
	"github.com/AlexGalhardo/stuarty/engine/renderhost"
)

func TestParsePixelRenderer(t *testing.T) {
	t.Parallel()

	for in, want := range map[string]PixelRenderer{"": PixelRendererGo, "go": PixelRendererGo, "rust": PixelRendererRust, "gpu": PixelRendererGPU} {
		got, err := ParsePixelRenderer(in)
		if err != nil || got != want {
			t.Errorf("ParsePixelRenderer(%q) = %q, %v, want %q", in, got, err, want)
		}
	}

	for _, in := range []string{"Rust", "wasm", "go ", "GPU"} {
		if _, err := ParsePixelRenderer(in); err == nil {
			t.Errorf("ParsePixelRenderer(%q) was accepted", in)
		}
	}
}

// SDD-013 section 7.5, TestExportMapsRendererErrors: each renderer failure becomes the code and details of
// section 7.3, and a cancel stays a cancel.
func TestExportMapsRendererErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		err        error
		code       string
		details    map[string]any
		isCanceled bool
	}{
		{
			err:     &renderhost.Error{Reason: renderhost.LimitExceeded, Detail: "render_texture_bytes"},
			code:    ffmpeg.CodeLimitExceeded,
			details: map[string]any{"limit": "render_texture_bytes"},
		},
		{
			err:     &renderhost.Error{Reason: renderhost.MissingTexture, Detail: "itm_1"},
			code:    CodeExportFailed,
			details: map[string]any{"reason": ReasonRender, "renderer_reason": "missing_texture", "item": "itm_1"},
		},
		{
			err:     &renderhost.Error{Reason: renderhost.InvalidPlan, Detail: "plan: unknown field x"},
			code:    CodeExportFailed,
			details: map[string]any{"reason": ReasonRender, "renderer_reason": "invalid_plan"},
		},
		{
			err:     &renderhost.Error{Reason: renderhost.RenderTrap, Detail: "vr_render: unreachable"},
			code:    CodeExportFailed,
			details: map[string]any{"reason": ReasonRender, "renderer_reason": "render_trap"},
		},
		{
			err:     errors.Join(nil, &renderhost.Error{Reason: renderhost.OutOfMemory}),
			code:    CodeExportFailed,
			details: map[string]any{"reason": ReasonRender, "renderer_reason": "out_of_memory"},
		},
		{err: errors.New("something else"), code: CodeExportFailed, details: map[string]any{"reason": ReasonRender}},
		{err: context.Canceled, isCanceled: true},
		{err: errors.Join(context.Canceled, context.Canceled), isCanceled: true},
	}

	for _, c := range cases {
		got := rendererError(c.err)
		if c.isCanceled {
			if !errors.Is(got, context.Canceled) {
				t.Errorf("a cancel became %v", got)
			}

			continue
		}

		fe, ok := errors.AsType[*ffmpeg.Error](got)
		if !ok {
			t.Errorf("%v became %T", c.err, got)

			continue
		}

		if fe.Code != c.code {
			t.Errorf("%v: code %s, want %s", c.err, fe.Code, c.code)
		}

		for k, v := range c.details {
			if fe.Details[k] != v {
				t.Errorf("%v: details[%s] = %v, want %v", c.err, k, fe.Details[k], v)
			}
		}

		if fe.Message == "" || fe.Hint == "" {
			t.Errorf("%v: a message and a hint are required", c.err)
		}
	}

	if rendererError(nil) != nil {
		t.Error("nil became an error")
	}
}

// The export with the Rust renderer writes the same file as the export with the Go compositor: the frames
// are the same bytes (TestRustCPUPathEqualsGoCompositor) and the encoder is deterministic for the same
// frames, so the two files must be equal.
func TestExportWithTheRustRenderer(t *testing.T) {
	setup(t)

	name := "clip-caption"
	doc := loadScene(t, name)
	sums := map[PixelRenderer][32]byte{}

	for _, kind := range []PixelRenderer{PixelRendererGo, PixelRendererRust} {
		job := newJob(t, doc, "export-"+string(kind)+"-"+name, string(kind)+"-"+name+".mp4")
		job.PixelRenderer = kind

		res, err := Run(context.Background(), job)
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}

		raw, err := os.ReadFile(res.Path)
		if err != nil {
			t.Fatal(err)
		}

		sums[kind] = sha256.Sum256(raw)
		t.Logf("%s: %d frames, %d bytes", kind, res.Frames, res.Size)
	}

	if sums[PixelRendererGo] != sums[PixelRendererRust] {
		t.Error("the file exported with the Rust renderer differs from the one exported with the Go compositor")
	}
}
