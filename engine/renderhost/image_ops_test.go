package renderhost

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/plan"
)

// The golden frames of the image operations of SDD-013 section 3.11 (epic 29). The cases and the frames the
// Rust CPU path draws are in renderer/testdata/image-ops (renderer/stuarty-renderer/tests/image_ops.rs writes
// them); the embedded module, run under wazero, must draw the same bytes.

const opsSide = 32

// opsPattern is the texture a layer of a case draws, by the part of its item id before a '#': the same four
// functions as the Rust test.
func opsPattern(t *testing.T, name string) []byte {
	t.Helper()

	pix := make([]byte, 0, opsSide*opsSide*4)

	for y := range opsSide {
		for x := range opsSide {
			var px [4]int

			switch name {
			case "grad":
				px = [4]int{x * 8, y * 8, (x + y) * 4, 255}
			case "bars":
				px = [4]int{40, 40, 40, 255}
				if x%8 < 4 {
					px = [4]int{230, 230, 230, 255}
				}
			case "ramp":
				v := (x*8 + y) & 255
				px = [4]int{v, v, v, 255}
			case "disc":
				d2 := (x-16)*(x-16) + (y-16)*(y-16)

				a := 0

				switch {
				case d2 <= 100:
					a = 255
				case d2 <= 144:
					a = 128
				}

				px = [4]int{(200*a + 127) / 255, (60*a + 127) / 255, (30*a + 127) / 255, a}
			default:
				t.Fatalf("no pattern %q", name)
			}

			for _, v := range px {
				pix = append(pix, byte(v))
			}
		}
	}

	return pix
}

type opsCase struct {
	Version int `json:"version"`
	Output  struct {
		Width  int     `json:"width"`
		Height int     `json:"height"`
		Scale  float64 `json:"scale"`
	} `json:"output"`
	Plan plan.FramePlan `json:"plan"`
}

func readOpsCase(t *testing.T, path string) (opsCase, []byte) {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var c opsCase

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()

	if err := dec.Decode(&c); err != nil {
		t.Fatalf("%s: %v", path, err)
	}

	return c, raw
}

func TestImageOpsEqualTheirGoldenFrames(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", "renderer", "testdata", "image-ops", "*.json"))
	if err != nil || len(files) < 20 {
		t.Fatalf("the cases are missing: %d files, %v", len(files), err)
	}

	slices.Sort(files)

	ctx := context.Background()

	h, err := New(ctx, Config{})
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = h.Close(ctx) }()

	for _, file := range files {
		name := strings.TrimSuffix(filepath.Base(file), ".json")

		t.Run(name, func(t *testing.T) {
			c, raw := readOpsCase(t, file)

			// The Go plan types carry every member of the case: what they encode is what the file says.
			var want, got map[string]any

			if err := json.Unmarshal(raw, &want); err != nil {
				t.Fatal(err)
			}

			enc, err := json.Marshal(c.Plan)
			if err != nil {
				t.Fatal(err)
			}

			if err := json.Unmarshal(enc, &got); err != nil {
				t.Fatal(err)
			}

			if !reflect.DeepEqual(got, want["plan"]) {
				t.Fatalf("the plan types lose something of the case:\n%s\n%s", enc, raw)
			}

			for i := range c.Plan.Layers {
				l := &c.Plan.Layers[i]
				pix := opsPattern(t, strings.SplitN(l.ItemID, "#", 2)[0])

				opaque := true

				for p := 3; p < len(pix); p += 4 {
					if pix[p] != 255 {
						opaque = false
					}
				}

				if err := h.PutTexture(ctx, l.ItemID, opsSide, opsSide, pix, opaque); err != nil {
					t.Fatal(err)
				}
			}

			in := RenderInput{
				Output: Output{Width: c.Output.Width, Height: c.Output.Height, Scale: c.Output.Scale},
				Plan:   c.Plan,
			}

			whole := image.NewRGBA(image.Rect(0, 0, in.Output.Width, in.Output.Height))
			if err := h.Render(ctx, in, whole); err != nil {
				t.Fatal(err)
			}

			golden, err := os.ReadFile(strings.TrimSuffix(file, ".json") + ".rgba")
			if err != nil {
				t.Fatal(err)
			}

			if !bytes.Equal(whole.Pix, golden) {
				t.Fatalf("%d bytes differ from the golden frame", differing(whole.Pix, golden))
			}

			// Six instances, one per band of rows, draw the same frame (SDD-013 section 4.6).
			banded := image.NewRGBA(whole.Rect)
			rows := in.Output.Height / 6

			for b := range 6 {
				band, err := New(ctx, Config{})
				if err != nil {
					t.Fatal(err)
				}

				for i := range c.Plan.Layers {
					l := &c.Plan.Layers[i]
					pix := opsPattern(t, strings.SplitN(l.ItemID, "#", 2)[0])

					if err := band.PutTexture(ctx, l.ItemID, opsSide, opsSide, pix, false); err != nil {
						t.Fatal(err)
					}
				}

				bin := in
				bin.Output.Y0, bin.Output.Y1 = b*rows, (b+1)*rows

				if err := band.Render(ctx, bin, banded); err != nil {
					t.Fatal(err)
				}

				_ = band.Close(ctx)
			}

			if !bytes.Equal(banded.Pix, golden) {
				t.Fatalf("the bands differ from the golden frame in %d bytes", differing(banded.Pix, golden))
			}
		})
	}
}

func differing(a, b []byte) int {
	n := 0

	for i := range min(len(a), len(b)) {
		if a[i] != b[i] {
			n++
		}
	}

	return n + max(len(a), len(b)) - min(len(a), len(b))
}
