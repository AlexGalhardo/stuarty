package renderhost

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tetratelabs/wazero"

	"github.com/AlexGalhardo/stuarty/core/plan"
)

// wasmSection returns the payload of the first section with this id, or nil.
func wasmSection(t *testing.T, mod []byte, id byte) []byte {
	t.Helper()

	if len(mod) < 8 || string(mod[:4]) != "\x00asm" {
		t.Fatal("not a WebAssembly module")
	}

	for i := 8; i < len(mod); {
		sid := mod[i]
		i++

		n, k := uleb(t, mod[i:])
		i += k

		if sid == id {
			return mod[i : i+int(n)]
		}

		i += int(n)
	}

	return nil
}

func uleb(t *testing.T, b []byte) (uint64, int) {
	t.Helper()

	var v uint64

	for i, c := range b {
		v |= uint64(c&0x7f) << (7 * i)
		if c&0x80 == 0 {
			return v, i + 1
		}
	}

	t.Fatal("truncated LEB128")

	return 0, 0
}

func TestSecurity_T36_TamperedModuleIsRefused(t *testing.T) {
	t.Parallel()

	if err := verifyDigest(moduleBytes, moduleDigest); err != nil {
		t.Fatal(err)
	}

	if got := ModuleSHA256(); len(got) != 64 {
		t.Fatalf("digest %q", got)
	}

	tampered := slices.Clone(moduleBytes)
	tampered[len(tampered)/2] ^= 1

	if err := verifyDigest(tampered, moduleDigest); err == nil {
		t.Fatal("a module with one flipped bit passed its digest")
	}

	if err := verifyDigest(moduleBytes, "zz"); err == nil {
		t.Fatal("a malformed digest passed")
	}
}

// SDD-013 section 4.8: the module declares no import, exports its memory and the functions of section 4.2 and
// nothing else, caps its memory and has no start function.
func TestCoreModuleHasNoImports(t *testing.T) {
	t.Parallel()

	if got := wasmSection(t, moduleBytes, 2); got != nil {
		t.Fatalf("the module has an import section of %d bytes", len(got))
	}

	if got := wasmSection(t, moduleBytes, 8); got != nil {
		t.Fatal("the module has a start function")
	}

	exp := wasmSection(t, moduleBytes, 7)
	count, i := uleb(t, exp)

	var names []string

	for range count {
		n, k := uleb(t, exp[i:])
		i += k
		names = append(names, string(exp[i:i+int(n)]))
		i += int(n)
		i++ // the kind
		_, k = uleb(t, exp[i:])
		i += k
	}

	want := append([]string{"memory"}, exports...)
	slices.Sort(names)
	slices.Sort(want)

	if !slices.Equal(names, want) {
		t.Fatalf("exports %v, want %v", names, want)
	}

	mem := wasmSection(t, moduleBytes, 5)
	if mem[1]&1 == 0 {
		t.Fatal("the memory declares no maximum")
	}

	_, k := uleb(t, mem[2:])
	if limit, _ := uleb(t, mem[2+k:]); limit != maxMemoryPages {
		t.Fatalf("memory maximum %d pages, want %d", limit, maxMemoryPages)
	}

	ctx := context.Background()

	_, cm, err := compile(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if len(cm.ImportedFunctions())+len(cm.ImportedMemories()) != 0 {
		t.Fatal("wazero sees an import")
	}

	h, err := New(ctx, Config{})
	if err != nil {
		t.Fatal(err)
	}

	defer h.Close(ctx)

	res, err := h.call(ctx, "vr_api_version")
	if err != nil || res[0] != APIVersion {
		t.Fatalf("vr_api_version = %v, %v", res, err)
	}
}

// A module that asks for the host (WASI or anything named env) is refused by the interface check, and the
// runtime of the host has nothing to satisfy it with.
func TestSecurity_T36_RenderHostIsSandboxed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	// A module whose only import is a function of the given module and name.
	build := func(module, name string) []byte {
		imp := []byte{1, byte(len(module))}
		imp = append(imp, module...)
		imp = append(imp, byte(len(name)))
		imp = append(imp, name...)
		imp = append(imp, 0, 0) // a function of type 0

		var b bytes.Buffer

		b.WriteString("\x00asm\x01\x00\x00\x00")
		b.Write([]byte{1, 4, 1, 0x60, 0, 0})
		b.Write(append([]byte{2, byte(len(imp))}, imp...))

		return b.Bytes()
	}

	for _, imp := range [][2]string{{"wasi_snapshot_preview1", "fd_write"}, {"env", "now"}, {"wasi_snapshot_preview1", "clock_time_get"}} {
		rt := newRuntime(ctx)

		cm, err := rt.CompileModule(ctx, build(imp[0], imp[1]))
		if err != nil {
			t.Fatal(err)
		}

		if err := checkInterface(cm); err == nil || !strings.Contains(err.Error(), "imports") {
			t.Errorf("%v: checkInterface = %v", imp, err)
		}

		if _, err := rt.InstantiateModule(ctx, cm, wazero.NewModuleConfig()); err == nil {
			t.Errorf("%v: the production runtime instantiated a module that imports the host", imp)
		}

		_ = rt.Close(ctx)
	}
}

const onePixelPlan = `{"version":1,"output":{"width":4,"height":4,"scale":1},"plan":{"background":"#000000","layers":[` +
	`{"item":"a","kind":"media","cx":2,"cy":2,"w":2,"h":2,"opacity":1}]}}`

func newHost(t *testing.T, cfg Config) *Host {
	t.Helper()

	h, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = h.Close(context.Background()) })

	return h
}

func wantReason(t *testing.T, err error, reason Reason) *Error {
	t.Helper()

	var e *Error
	if !errors.As(err, &e) || e.Reason != reason {
		t.Fatalf("error %v, want reason %s", err, reason)
	}

	return e
}

func TestHostRendersAStoredTexture(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	h := newHost(t, Config{})

	if err := h.PutTexture(ctx, "a", 2, 2, bytes.Repeat([]byte{255}, 16), true); err != nil {
		t.Fatal(err)
	}

	in := RenderInput{
		Output: Output{Width: 4, Height: 4, Scale: 1},
		Plan: plan.FramePlan{
			Background: "#102030",
			Layers:     []plan.Layer{{ItemID: "a", Kind: plan.Media, CX: 2, CY: 2, W: 2, H: 2, Opacity: 1}},
		},
	}
	dst := image.NewRGBA(image.Rect(0, 0, 4, 4))

	if err := h.Render(ctx, in, dst); err != nil {
		t.Fatal(err)
	}

	if got := dst.Pix[(1*4+1)*4 : (1*4+1)*4+4]; !bytes.Equal(got, []byte{255, 255, 255, 255}) {
		t.Errorf("inside = %v", got)
	}

	if got := dst.Pix[:4]; !bytes.Equal(got, []byte{0x10, 0x20, 0x30, 255}) {
		t.Errorf("background = %v", got)
	}

	// A band writes only its rows.
	dst = image.NewRGBA(image.Rect(0, 0, 4, 4))
	in.Output.Y0, in.Output.Y1 = 2, 3

	if err := h.Render(ctx, in, dst); err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(dst.Pix[:2*16], make([]byte, 2*16)) || !bytes.Equal(dst.Pix[3*16:], make([]byte, 16)) {
		t.Error("a band wrote outside its rows")
	}

	if dst.Pix[2*16+4+3] != 255 {
		t.Error("a band did not write its row")
	}

	// A texture is released and then missing again.
	if err := h.Release(ctx, "a"); err != nil {
		t.Fatal(err)
	}

	e := wantReason(t, h.Render(ctx, in, dst), MissingTexture)
	if e.Detail != "a" {
		t.Errorf("detail %q names neither the item", e.Detail)
	}
}

func TestRenderInputEncodesThePlan(t *testing.T) {
	t.Parallel()

	spec := &plan.BitmapSpec{Kind: "text", Text: "hello", ActiveWord: -1, PixelScale: 1}
	fp := plan.FramePlan{
		Frame: 3, Width: 8, Height: 6, Background: "#ffffff",
		Layers: []plan.Layer{
			{
				ItemID:   "a",
				Kind:     plan.Media,
				Crop:     plan.Rect{W: 2, H: 2},
				Filter:   &[12]float64{1},
				Blur:     2.5,
				CX:       1,
				CY:       2,
				W:        3,
				H:        4,
				Rotation: 15,
				Opacity:  0.5,
			},
			{ItemID: "t", Kind: plan.Bitmap, Bitmap: spec, CX: 4, CY: 5, W: 6, H: 7, Opacity: 1},
		},
	}

	raw, err := json.Marshal(RenderInput{Output: Output{Width: 8, Height: 6, Scale: 0.5, Y0: 1, Y1: 4}, Plan: fp})
	if err != nil {
		t.Fatal(err)
	}

	var got, want map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}

	planJSON, err := json.Marshal(fp)
	if err != nil {
		t.Fatal(err)
	}

	if err := json.Unmarshal(planJSON, &want); err != nil {
		t.Fatal(err)
	}

	// The plan is the plan of core/plan, plus bitmap_key on the bitmap layer.
	want["layers"].([]any)[1].(map[string]any)["bitmap_key"] = spec.Key()

	if !jsonEqual(t, got["plan"], want) {
		t.Fatalf("plan %s\nwant %v", raw, want)
	}

	if got["version"] != float64(1) ||
		!jsonEqual(t, got["output"], map[string]any{"width": 8.0, "height": 6.0, "scale": 0.5, "y0": 1.0, "y1": 4.0}) {
		t.Fatalf("envelope %s", raw)
	}

	raw, err = json.Marshal(RenderInput{Output: Output{Width: 8, Height: 6, Scale: 1}, Plan: fp})
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(string(raw), `"y0"`) {
		t.Error("a whole frame names rows")
	}
}

func jsonEqual(t *testing.T, a, b any) bool {
	t.Helper()

	x, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}

	y, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}

	return bytes.Equal(x, y)
}

// SDD-013 section 7.5: each reason with a status comes back from an input that causes it.
func TestRenderReasons(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	h := newHost(t, Config{MaxTextureBytes: 1 << 12})
	white := bytes.Repeat([]byte{255}, 16)

	if err := h.PutTexture(ctx, "a", 2, 2, white, true); err != nil {
		t.Fatal(err)
	}

	dst := image.NewRGBA(image.Rect(0, 0, 4, 4))
	render := func(text string) error { return h.renderText(ctx, []byte(text), dst, 0, 4) }

	wantReason(t, render(strings.Replace(onePixelPlan, `"opacity":1`, `"opacity":1,"blurry":1`, 1)), InvalidPlan)
	wantReason(t, render(strings.Replace(onePixelPlan, `"version":1`, `"version":2`, 1)), UnsupportedVersion)
	wantReason(t, render(strings.Replace(onePixelPlan, `"media"`, `"video"`, 1)), UnsupportedLayer)
	wantReason(t, render(strings.Replace(onePixelPlan, `"item":"a"`, `"item":"zz"`, 1)), MissingTexture)
	wantReason(t, render(`{"version":1,"output":{"width":4,"height":4,"scale":1},"plan":{"layers":[`+
		strings.TrimSuffix(strings.Repeat(`{"item":"a","kind":"media","opacity":1},`, 65), ",")+`]}}`), LimitExceeded)
	wantReason(t, render(strings.Replace(onePixelPlan, `"width":4`, `"width":4097`, 1)), LimitExceeded)
	wantReason(t, render(onePixelPlan+strings.Repeat(" ", 5<<20)), LimitExceeded)
	wantReason(t, render(strings.Replace(onePixelPlan, `"scale":1`, `"scale":1,"y0":3,"y1":3`, 1)), InvalidArgument)
	wantReason(t, h.PutTexture(ctx, "b", 8193, 1, make([]byte, 8193*4), true), LimitExceeded)
	wantReason(t, h.PutTexture(ctx, "b", 2, 2, white[:15], true), InvalidArgument)
	wantReason(t, h.PutTexture(ctx, strings.Repeat("x", 65), 1, 1, white[:4], true), InvalidArgument)
	wantReason(t, h.PutTexture(ctx, "big", 40, 40, make([]byte, 6400), true), LimitExceeded)

	// An out_len one byte short.
	in, err := h.put(ctx, []byte(onePixelPlan))
	if err != nil {
		t.Fatal(err)
	}

	out, err := h.alloc(ctx, 64)
	if err != nil {
		t.Fatal(err)
	}

	err = h.status(ctx, "vr_render", uint64(in.ptr), uint64(in.n), uint64(out), 63)
	wantReason(t, err, InvalidArgument)

	// The instance is still usable after every refusal.
	if err := render(onePixelPlan); err != nil {
		t.Fatal(err)
	}
}

// SDD-013 section 4.8: cancelling the context of a long render returns context.Canceled within a second and
// leaves no goroutine behind.
func TestRenderHostCancels(t *testing.T) {
	ctx := context.Background()
	h := newHost(t, Config{})

	if err := h.PutTexture(
		ctx,
		"big",
		1024,
		1024,
		bytes.Repeat([]byte{200, 100, 50, 255}, 1024*1024),
		true,
	); err != nil {
		t.Fatal(err)
	}

	before := runtime.NumGoroutine()
	in := RenderInput{
		Output: Output{Width: 4096, Height: 4096, Scale: 1},
		Plan: plan.FramePlan{Layers: []plan.Layer{
			{
				ItemID:   "big",
				Kind:     plan.Media,
				CX:       2048,
				CY:       2048,
				W:        4000,
				H:        4000,
				Rotation: 17,
				Blur:     60,
				Opacity:  0.9,
			},
		}},
	}
	dst := image.NewRGBA(image.Rect(0, 0, 4096, 4096))

	cctx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)

	go func() { done <- h.Render(cctx, in, dst) }()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Render = %v, want context.Canceled (the frame may have finished before the cancel)", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Render did not stop within a second of the cancel")
	}

	if err := h.Render(ctx, in, dst); err == nil {
		t.Error("a cancelled instance kept rendering")
	}

	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	if n := runtime.NumGoroutine(); n > before {
		t.Errorf("%d goroutines after the cancel, %d before", n, before)
	}
}
