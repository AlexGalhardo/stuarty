package export

import (
	"image"
	"math/rand/v2"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/plan"
	"github.com/AlexGalhardo/vukem-video/engine/compositor"
)

const benchFPS = 30.0

func benchImage(w, h int) compositor.Image {
	rng := rand.New(rand.NewPCG(1, 2))
	pix := make([]byte, w*h*4)

	for i := 0; i < len(pix); i += 4 {
		pix[i], pix[i+1], pix[i+2], pix[i+3] = uint8(rng.UintN(256)), uint8(rng.UintN(256)), uint8(rng.UintN(256)), 255
	}

	return compositor.Image{Pix: pix, W: w, H: h, Opaque: true}
}

// benchCompose composes a full 1080x1920 base layer and a second one that mod changes every frame,
// and reports the speed in times real time at 30 fps (SDD-004 section 12).
func benchCompose(b *testing.B, mod func(frame int, l *plan.Layer)) {
	b.Helper()

	imgs := map[string]compositor.Image{"base": benchImage(1080, 1920), "top": benchImage(1080, 1920)}
	c := &compositor.Compositor{
		Media: func(l *plan.Layer) (compositor.Image, error) { return imgs[l.ItemID], nil },
	}
	dst := image.NewRGBA(image.Rect(0, 0, 1080, 1920))
	layer := func(id string) plan.Layer {
		return plan.Layer{ItemID: id, Kind: plan.Media, CX: 540, CY: 960, W: 1080, H: 1920, Opacity: 1}
	}

	b.SetBytes(1080 * 1920 * 4)
	b.ResetTimer()

	for i := range b.N {
		top := layer("top")
		mod(i, &top)

		fp := plan.FramePlan{Background: "#000000", Layers: []plan.Layer{layer("base"), top}}
		if err := c.Compose(dst, fp, 1); err != nil {
			b.Fatal(err)
		}
	}

	b.ReportMetric(float64(b.N)/benchFPS/b.Elapsed().Seconds(), "x_realtime")
}

// BenchmarkComposeRotated: a second 1080x1920 layer scaled and rotated on every frame, which only the
// fast sampler keeps near real time.
func BenchmarkComposeRotated(b *testing.B) {
	benchCompose(b, func(frame int, l *plan.Layer) {
		l.Rotation = 2 + float64(frame%7)
		l.W, l.H = 1000+float64(frame%5)*10, 1800+float64(frame%5)*10
	})
}

// BenchmarkComposeColorMatrix: a colour matrix on every pixel of the second layer.
func BenchmarkComposeColorMatrix(b *testing.B) {
	m := [12]float64{1.1, 0, 0, 0.01, 0, 1.0, 0, 0, 0, 0, 0.9, -0.01}

	benchCompose(b, func(_ int, l *plan.Layer) { l.ColorMatrix = &m })
}

// BenchmarkComposeBlur: the second 1080x1920 layer blurred with a standard deviation of 12 pixels
// (SDD-004 section 12: at least 1.0 times real time).
func BenchmarkComposeBlur(b *testing.B) {
	benchCompose(b, func(_ int, l *plan.Layer) { l.Blur = 12 })
}

// BenchmarkComposeFilter: a filter matrix on every pixel of the second layer.
func BenchmarkComposeFilter(b *testing.B) {
	m := plan.FilterMatrix("sepia", 1)

	benchCompose(b, func(_ int, l *plan.Layer) { l.Filter = m })
}
