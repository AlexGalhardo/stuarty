package export

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"runtime"
	"testing"
)

func pngTestPicture(w, h int, translucent bool) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	s := uint32(7)

	for i := 0; i < len(img.Pix); i += 4 {
		s = s*1664525 + 1013904223
		a := byte(255)

		if translucent {
			a = byte(s >> 24)
			if i%12 == 0 {
				a = 255
			}
		}

		// premultiplied: no channel above the alpha
		img.Pix[i] = byte(uint32(byte(s>>16)) * uint32(a) / 255)
		img.Pix[i+1] = byte(uint32(byte(s>>8)) * uint32(a) / 255)
		img.Pix[i+2] = byte(uint32(byte(s)) * uint32(a) / 255)
		img.Pix[i+3] = a
	}

	return img
}

// TestEncodePNGParallelIsAPNGOfTheSamePixels: what the standard decoder reads back is what the standard encoder
// would have written, for pictures with and without transparency, from one pixel to several segments.
func TestEncodePNGParallelIsAPNGOfTheSamePixels(t *testing.T) {
	for _, tc := range []struct {
		w, h        int
		translucent bool
	}{{1, 1, false}, {1, 1, true}, {7, 5, true}, {33, 127, false}, {40, 128, true}, {40, 129, true}, {64, 300, false}, {200, 513, true}} {
		img := pngTestPicture(tc.w, tc.h, tc.translucent)

		got, err := encodePNGParallel(img)
		if err != nil {
			t.Fatal(err)
		}

		dec, err := png.Decode(bytes.NewReader(got))
		if err != nil {
			t.Fatalf("%dx%d: %v", tc.w, tc.h, err)
		}

		var std bytes.Buffer
		if err := (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&std, img); err != nil {
			t.Fatal(err)
		}

		want, err := png.Decode(&std)
		if err != nil {
			t.Fatal(err)
		}

		for y := range tc.h {
			for x := range tc.w {
				if color.NRGBAModel.Convert(dec.At(x, y)) != color.NRGBAModel.Convert(want.At(x, y)) {
					t.Fatalf("%dx%d translucent %v: pixel %d,%d is %v, want %v",
						tc.w, tc.h, tc.translucent, x, y, dec.At(x, y), want.At(x, y))
				}
			}
		}
	}
}

func TestEncodePNGParallelIsTheSameOnEveryCPUCount(t *testing.T) {
	img := pngTestPicture(90, 700, true)

	many, err := encodePNGParallel(img)
	if err != nil {
		t.Fatal(err)
	}

	prev := runtime.GOMAXPROCS(1)

	defer runtime.GOMAXPROCS(prev)

	one, err := encodePNGParallel(img)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(many, one) {
		t.Fatal("the bytes depend on the number of CPUs")
	}
}

func adlerOf(b []byte) uint32 {
	const base = 65521

	s1, s2 := uint32(1), uint32(0)

	for _, v := range b {
		s1 = (s1 + uint32(v)) % base
		s2 = (s2 + s1) % base
	}

	return s2<<16 | s1
}

func TestAdlerCombine(t *testing.T) {
	a, b := bytes.Repeat([]byte("stuarty"), 5000), bytes.Repeat([]byte{0xff, 0, 17}, 40000)

	got := adlerCombine(adlerOf(a), adlerOf(b), int64(len(b)))
	if want := adlerOf(append(append([]byte(nil), a...), b...)); got != want {
		t.Fatalf("combine %08x, want %08x", got, want)
	}
}
