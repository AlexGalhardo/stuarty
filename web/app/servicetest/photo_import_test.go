package servicetest

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/AlexGalhardo/stuarty/engine/ingest"
)

func photoPixels() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 640, 360))
	for y := range 360 {
		for x := range 640 {
			img.Set(x, y, color.RGBA{R: uint8(x / 3), G: uint8(y / 2), B: uint8((x + y) / 5), A: 255})
		}
	}

	return img
}

// VV-405: a still PNG and a still JPEG imported through the service, with the real ingest, are ready.
func TestStillPhotoImportBecomesReady(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{realExport: true, ingest: ingest.Run})
	project, _ := m.withTrack()

	var p, j bytes.Buffer
	if err := png.Encode(&p, photoPixels()); err != nil {
		t.Fatal(err)
	}

	if err := jpeg.Encode(&j, photoPixels(), nil); err != nil {
		t.Fatal(err)
	}

	for name, data := range map[string]string{"photo.png": p.String(), "photo.jpg": j.String()} {
		res := m.importOne(project, m.write(name, data))
		if res.Failed != 0 || len(res.Assets) != 1 || res.Assets[0].State != "ready" || res.Assets[0].Kind != "image" {
			t.Errorf("%s: result = %+v", name, res)
		}
	}
}
