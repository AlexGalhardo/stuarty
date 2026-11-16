package renderhost

import (
	"bytes"
	"context"
	"errors"
	"image"
	"math/rand/v2"
	"strconv"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/plan"
)

// Threat T-36 of SDD-007: the module renders frames of untrusted media from a plan an agent wrote, and runs in the
// owner process. A defect that nevertheless executes must stay in the instance's memory, which is discarded.

// allowedReasons are the statuses of SDD-013 section 4.5; a render that fails any other way is a defect.
var allowedReasons = map[Reason]bool{
	InvalidPlan: true, UnsupportedVersion: true, UnsupportedLayer: true, MissingTexture: true,
	LimitExceeded: true, OutOfMemory: true, InvalidArgument: true,
}

var hostileTokens = []string{
	"null",
	"true",
	"{}",
	"[]",
	`""`,
	"-0",
	"1e308",
	"-1e308",
	"1e-320",
	"1e999",
	"NaN",
	"Infinity",
	"0.1",
	"4096",
	"4097",
	"65",
	"12",
	"-1",
	"9007199254740993",
	`\u0000`,
	`\ud800`,
	`"version"`,
	`"opacity"`,
	`"layers"`,
	`"kind"`,
	`"bitmap_key"`,
	`"a"`,
	`"zz"`,
	",",
	":",
	`{"a":1}`,
	"[1,2,3,4,5,6,7,8,9,10,11,12]",
	`"media"`,
	`"bitmap"`,
	"é",
}

// mutate returns src with a few bytes flipped, cut, inserted or repeated.
func mutate(rng *rand.Rand, src string) []byte {
	b := []byte(src)

	rounds := 1
	if rng.IntN(4) == 0 {
		rounds += 1 + rng.IntN(3)
	}

	for range rounds {
		if len(b) == 0 {
			b = append(b, '{')

			continue
		}

		switch rng.IntN(6) {
		case 0:
			b[rng.IntN(len(b))] = byte(rng.IntN(256))
		case 1:
			b = b[:rng.IntN(len(b))]
		case 2:
			i := rng.IntN(len(b))
			j := min(len(b), i+rng.IntN(40))
			b = append(b[:i], b[j:]...)
		case 3:
			i := rng.IntN(len(b) + 1)
			b = append(b[:i], append([]byte(hostileTokens[rng.IntN(len(hostileTokens))]), b[i:]...)...)
		case 4:
			i := rng.IntN(len(b))
			j := min(len(b), i+rng.IntN(60))
			dup := bytes.Clone(b[i:j])
			at := rng.IntN(len(b) + 1)
			b = append(b[:at], append(dup, b[at:]...)...)
		default:
			b[rng.IntN(len(b))] ^= 1 << rng.IntN(8)
		}
	}

	return b
}

// hostileCorpus is the starting points the mutations grow from: a media layer, a band with matrices and a blur,
// and a bitmap layer.
var hostileCorpus = []string{
	onePixelPlan,
	`{"version":1,"output":{"width":16,"height":12,"scale":0.5,"y0":2,"y1":9},"plan":{"layers":[{"item":"a","kind":"media",` +
		`"color_matrix":[1,0,0,0,0,1,0,0,0,0,1,0],"filter":[0.3,0.6,0.1,0,0.3,0.6,0.1,0,0.3,0.6,0.1,0],"blur":2.5,` +
		`"cx":8,"cy":6,"w":10,"h":10,"rotation":-30,"opacity":1}]}}`,
	`{"version":1,"output":{"width":16,"height":12,"scale":1},"plan":{"background":"#ffffff","layers":[` +
		`{"item":"t","kind":"bitmap","bitmap":{"kind":"text","text":"hi"},"bitmap_key":"k1","cx":8,"cy":6,"w":8,"h":4,"opacity":1},` +
		`{"item":"a","kind":"media","cx":4,"cy":4,"w":8,"h":8,"rotation":90,"opacity":0.75}]}}`,
}

// hostileHost is a host with a few textures of hostile content: not premultiplied, extreme values.
func hostileHost(tb testing.TB, rng *rand.Rand) *Host {
	tb.Helper()

	ctx := context.Background()

	h, err := New(ctx, Config{MaxTextureBytes: 8 << 20, MaxBitmapBytes: 4 << 20})
	if err != nil {
		tb.Fatal(err)
	}

	for _, id := range []string{"a", "t", "k1"} {
		w, ht := 1+rng.IntN(24), 1+rng.IntN(24)
		pix := make([]byte, w*ht*4)

		for i := range pix {
			pix[i] = byte(rng.IntN(256))
		}

		if err := h.PutTexture(ctx, id, w, ht, pix, rng.IntN(4) == 0); err != nil {
			tb.Fatal(err)
		}
	}

	return h
}

// renderHostile runs one hostile input and fails on anything that is not a clean success or a clean refusal.
func renderHostile(tb testing.TB, h *Host, input []byte) {
	tb.Helper()

	ctx := context.Background()
	dst := image.NewRGBA(image.Rect(0, 0, 16, 12))

	err := h.renderText(ctx, input, dst, 0, 12)
	if err == nil {
		return
	}

	var e *Error
	if !errors.As(err, &e) || !allowedReasons[e.Reason] {
		tb.Fatalf("input %q: %v is not a refusal of SDD-013 section 4.5", input, err)
	}

	if h.dead != nil {
		tb.Fatalf("input %q killed the instance: %v", input, h.dead)
	}
}

// SDD-013 section 7.5: 10,000 random and mutated render inputs, on textures of hostile content, never trap,
// never return a status outside section 4.5 and leave the instance's memory far below its limit.
func TestSecurity_T36_RenderHostSurvivesHostileInput(t *testing.T) {
	rng := rand.New(rand.NewPCG(36, 2026))
	h := hostileHost(t, rng)

	defer h.Close(context.Background())

	// A number that is not JSON is a refusal, not a pixel.
	renderHostile(t, h, []byte(strings.Replace(onePixelPlan, `"cx":2`, `"cx":NaN`, 1)))

	var refused, drawn int

	for range 10000 {
		input := mutate(rng, hostileCorpus[rng.IntN(len(hostileCorpus))])

		dst := image.NewRGBA(image.Rect(0, 0, 16, 12))
		if err := h.renderText(context.Background(), input, dst, 0, 12); err == nil {
			drawn++
		} else {
			refused++
		}

		renderHostile(t, h, input)
	}

	if drawn == 0 || refused == 0 {
		t.Fatalf("the mutations reached only one path: %d drawn, %d refused", drawn, refused)
	}

	if pages := h.mem.Size() / 65536; pages > 1024 {
		t.Errorf("the instance grew to %d pages (%d MiB) on 16 x 12 frames", pages, pages/16)
	}
}

// The same inputs as a Go fuzz target: go test -fuzz FuzzRenderInput ./engine/renderhost.
func FuzzRenderInput(f *testing.F) {
	for _, s := range hostileCorpus {
		f.Add([]byte(s))
	}

	rng := rand.New(rand.NewPCG(1, 2))
	h := hostileHost(f, rng)

	f.Cleanup(func() { _ = h.Close(context.Background()) })

	f.Fuzz(func(t *testing.T, input []byte) {
		renderHostile(t, h, input)
	})
}

// The budgets of vr_init bound what an agent's project can make the module hold: a put that would pass one
// fails with limit_exceeded and changes nothing, and the memory of the instance stays close to the budget.
func TestSecurity_T36_MemoryIsBoundedByTheBudgets(t *testing.T) {
	ctx := context.Background()

	const budget = 1 << 20

	h, err := New(ctx, Config{MaxTextureBytes: budget, MaxBitmapBytes: budget / 4})
	if err != nil {
		t.Fatal(err)
	}

	defer h.Close(ctx)

	pix := make([]byte, 128*128*4)

	var held, refused int

	for i := range 100 {
		err := h.PutTexture(ctx, "t"+strconv.Itoa(i), 128, 128, pix, true)
		if err == nil {
			held++

			continue
		}

		if e := wantReason(t, err, LimitExceeded); e.Detail != "render_texture_bytes" {
			t.Fatalf("detail %q", e.Detail)
		}

		refused++
	}

	if held != budget/len(pix) || refused != 100-held {
		t.Fatalf("%d held and %d refused with a budget of %d bytes", held, refused, budget)
	}

	if size := h.mem.Size(); size > budget+16<<20 {
		t.Errorf("the instance holds %d bytes for a budget of %d", size, budget)
	}

	// A texture of the largest side is refused before it is copied into the module.
	wantReason(t, h.PutTexture(ctx, "huge", 9000, 9000, nil, true), LimitExceeded)

	// Replacing a texture does not count it twice.
	if err := h.PutTexture(ctx, "t0", 128, 128, pix, true); err != nil {
		t.Fatalf("replacing a held texture: %v", err)
	}
}

// What the budgets do not count is the scratch of one layer: the blur of the largest texture at the largest frame
// is more than the module's 1 GiB. That must end as an error of the call, never as a crash of the process, and a
// new instance must work afterwards. (The product refuses sources above 8192 on a side earlier: SDD-007 9.1.)
func TestSecurity_T36_MemoryExhaustionIsContained(t *testing.T) {
	if testing.Short() {
		// skip-env: short-mode
		t.Skip("allocates up to 1 GiB")
	}

	ctx := context.Background()

	h, err := New(ctx, Config{})
	if err != nil {
		t.Fatal(err)
	}

	defer h.Close(ctx)

	big := make([]byte, 8192*8192*4)
	if err := h.PutTexture(ctx, "big", 8192, 8192, big, true); err != nil {
		t.Fatal(err)
	}

	in := RenderInput{
		Output: Output{Width: 4096, Height: 4096, Scale: 1},
		Plan: plan.FramePlan{Layers: []plan.Layer{
			{ItemID: "big", Kind: plan.Media, CX: 2048, CY: 2048, W: 8192, H: 8192, Blur: 50, Opacity: 1},
		}},
	}

	err = h.Render(ctx, in, image.NewRGBA(image.Rect(0, 0, 4096, 4096)))
	t.Logf("the render of the largest blur ended with: %v", err)

	if err != nil {
		var e *Error
		if !errors.As(err, &e) || (e.Reason != OutOfMemory && e.Reason != RenderTrap) {
			t.Fatalf("exhausting the memory returned %v", err)
		}

		if _, err := h.call(ctx, "vr_api_version"); err == nil {
			t.Error("an instance that trapped kept answering")
		}
	}

	h2, err := New(ctx, Config{})
	if err != nil {
		t.Fatalf("a new instance after the failure: %v", err)
	}

	defer h2.Close(ctx)

	if err := h2.PutTexture(ctx, "a", 2, 2, bytes.Repeat([]byte{255}, 16), true); err != nil {
		t.Fatal(err)
	}

	if err := h2.renderText(ctx, []byte(onePixelPlan), image.NewRGBA(image.Rect(0, 0, 4, 4)), 0, 4); err != nil {
		t.Fatal(err)
	}
}
