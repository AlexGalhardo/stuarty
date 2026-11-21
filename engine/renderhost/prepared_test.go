package renderhost

import (
	"bytes"
	"context"
	"image"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/plan"
)

// A layer prepared on one instance and drawn on others is the layer drawn whole (SDD-013 section 4.2.2,
// epic 55): every case of the golden frames of the image operations, with every layer prepared on one
// instance, uploaded to fresh instances that never held the pictures, and drawn whole and in six bands, gives
// the golden frame byte for byte.
func TestPreparedLayersEqualTheGoldenFramesOnOtherInstances(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", "renderer", "testdata", "image-ops", "*.json"))
	if err != nil || len(files) < 20 {
		t.Fatalf("the cases are missing: %d files, %v", len(files), err)
	}

	slices.Sort(files)

	ctx := context.Background()

	maker, err := New(ctx, Config{Uncancellable: true})
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = maker.Close(ctx) }()

	prepared := 0

	for _, file := range files {
		name := strings.TrimSuffix(filepath.Base(file), ".json")

		t.Run(name, func(t *testing.T) {
			c, _ := readOpsCase(t, file)
			blocks := map[string]Prepared{}

			for i := range c.Plan.Layers {
				l := c.Plan.Layers[i]
				pix := opsPattern(t, strings.SplitN(l.ItemID, "#", 2)[0])

				if err := maker.PutTexture(ctx, l.ItemID, opsSide, opsSide, pix, false); err != nil {
					t.Fatal(err)
				}

				l.Clip, l.Opacity = nil, 1

				p, err := maker.Prepare(ctx, RenderInput{
					Output: Output{Width: c.Output.Width, Height: c.Output.Height, Scale: c.Output.Scale},
					Plan:   plan.FramePlan{Width: c.Plan.Width, Height: c.Plan.Height, Layers: []plan.Layer{l}},
				})
				if err != nil {
					t.Fatal(err)
				}

				if !p.Empty() {
					blocks[l.ItemID] = p
					prepared++
				}
			}

			in := RenderInput{
				Output: Output{Width: c.Output.Width, Height: c.Output.Height, Scale: c.Output.Scale},
				Plan:   c.Plan,
			}
			golden, err := os.ReadFile(strings.TrimSuffix(file, ".json") + ".rgba")
			if err != nil {
				t.Fatal(err)
			}

			banded := image.NewRGBA(image.Rect(0, 0, in.Output.Width, in.Output.Height))
			compact := image.NewRGBA(banded.Rect) // the same bands, each drawn into a view of its own rows
			rows := in.Output.Height / 6

			for b := range 6 {
				inst, err := New(ctx, Config{Uncancellable: true})
				if err != nil {
					t.Fatal(err)
				}

				for id, p := range blocks {
					if err := inst.PutPrepared(ctx, id, p); err != nil {
						t.Fatal(err)
					}
				}

				bin := in
				bin.Output.Y0, bin.Output.Y1 = b*rows, (b+1)*rows

				if err := inst.Render(ctx, bin, banded); err != nil {
					t.Fatal(err)
				}

				view := compact.SubImage(image.Rect(0, b*rows, in.Output.Width, (b+1)*rows)).(*image.RGBA)
				if err := inst.Render(ctx, bin, view); err != nil {
					t.Fatal(err)
				}

				_ = inst.Close(ctx)
			}

			if !bytes.Equal(banded.Pix, golden) {
				t.Fatalf("the prepared bands differ from the golden frame in %d bytes", differing(banded.Pix, golden))
			}

			if !bytes.Equal(compact.Pix[:6*rows*in.Output.Width*4], golden[:6*rows*in.Output.Width*4]) {
				t.Fatalf("the bands drawn into views of their rows differ from the golden frame in %d bytes",
					differing(compact.Pix[:6*rows*in.Output.Width*4], golden[:6*rows*in.Output.Width*4]))
			}
		})
	}

	if prepared < 40 {
		t.Fatalf("only %d layers were prepared", prepared)
	}
}

func TestPrepareRefusesWhatIsNotOneLayer(t *testing.T) {
	ctx := context.Background()

	h, err := New(ctx, Config{Uncancellable: true})
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = h.Close(ctx) }()

	in := RenderInput{Output: Output{Width: 4, Height: 4, Scale: 1}, Plan: plan.FramePlan{Width: 4, Height: 4}}
	if _, err := h.Prepare(ctx, in); err == nil {
		t.Fatal("a prepare of no layer was accepted")
	}

	if err := h.PutPrepared(ctx, "a", Prepared{}); err == nil {
		t.Fatal("an empty block was accepted")
	}
}
