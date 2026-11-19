package export

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"slices"

	coreimage "github.com/AlexGalhardo/stuarty/core/image"
	"github.com/AlexGalhardo/stuarty/engine/compositor"
)

// The formats of an image export (SDD-021 section 6).
const (
	ImagePNG  = "png"
	ImageJPEG = "jpeg"
	ImageWebP = "webp"
)

// ImageFormats lists them.
var ImageFormats = []string{ImagePNG, ImageJPEG, ImageWebP}

// ReasonWebPUnavailable is details.reason of an export in WebP when no encoder is wired.
const ReasonWebPUnavailable = "webp_unavailable"

// DefaultJPEGQuality is the quality of a JPEG when none is asked.
const DefaultJPEGQuality = 90

// WebPEncoder encodes a picture as WebP. It is a seam: the encoder is FFmpeg with libwebp, started through the
// argument grammar of engine/ffmpeg, which has no builder for it yet (see the question of epic 27); until one
// is given the WebP export answers E_EXPORT_FAILED with reason webp_unavailable.
type WebPEncoder interface {
	EncodeWebP(ctx context.Context, png []byte, quality int) ([]byte, error)
}

// ImageEncodeOptions say how a picture becomes a file.
type ImageEncodeOptions struct {
	Format string
	// Quality is 1 to 100 for JPEG (default 90) and for lossy WebP; 0 is the default (WebP: lossless).
	Quality int
	// Width and Height resample the picture to another size; 0 keeps it. The resampling is the integer
	// sampler of the compositor, so it is the same on every machine.
	Width, Height int
	WebP          WebPEncoder
}

// EncodeImage encodes a picture in the format of the options, after resampling it when a size is asked.
func EncodeImage(ctx context.Context, img *image.RGBA, o ImageEncodeOptions) ([]byte, error) {
	if !slices.Contains(ImageFormats, o.Format) {
		return nil, exportErr(
			ReasonRender,
			"The image format is not png, jpeg or webp.",
			fmt.Errorf("format %q", o.Format),
			"",
		)
	}

	if o.Quality < 0 || o.Quality > 100 {
		return nil, exportErr(ReasonRender, "The quality must be 1 to 100.", fmt.Errorf("quality %d", o.Quality), "")
	}

	if err := checkImageSize(img, o.Width, o.Height); err != nil {
		return nil, err
	}

	img, err := resample(img, o.Width, o.Height)
	if err != nil {
		return nil, err
	}

	switch o.Format {
	case ImageJPEG:
		return encodeJPEG(img, o.Quality)
	case ImageWebP:
		if o.WebP == nil {
			e := exportErr(
				ReasonWebPUnavailable,
				"WebP export is not available on this machine.",
				errors.New("no webp encoder"),
				"",
			)
			e.Hint = "Export PNG or JPEG."

			return nil, e
		}

		raw, err := encodePNG(img)
		if err != nil {
			return nil, err
		}

		return o.WebP.EncodeWebP(ctx, raw, o.Quality)
	}

	return encodePNG(img)
}

// checkImageSize refuses a picture with no pixels and a size that is negative or past the limits of SDD-021
// section 6, before anything is allocated for it.
func checkImageSize(img *image.RGBA, w, h int) error {
	if img == nil || img.Bounds().Dx() < 1 || img.Bounds().Dy() < 1 {
		return exportErr(ReasonRender, "There is no picture to export.", errors.New("empty picture"), "")
	}

	if w < 0 || h < 0 || w > coreimage.MaxExportSide || h > coreimage.MaxExportSide {
		return exportErr(ReasonBadFormat, "The size of the export is outside 1 to 8192 pixels on each side.",
			fmt.Errorf("size %dx%d", w, h), "")
	}

	b := img.Bounds()
	tw, th := w, h

	if tw == 0 && th != 0 {
		tw = max(round(float64(b.Dx())*float64(th)/float64(b.Dy())), 1)
	}

	if th == 0 && tw != 0 {
		th = max(round(float64(b.Dy())*float64(tw)/float64(b.Dx())), 1)
	}

	if tw > coreimage.MaxExportSide || th > coreimage.MaxExportSide || tw*th > coreimage.MaxExportPixels {
		return exportErr(ReasonBadFormat, "The export would have more pixels than the limit of 33177600.",
			fmt.Errorf("size %dx%d", tw, th), "")
	}

	return nil
}

func resample(img *image.RGBA, w, h int) (*image.RGBA, error) {
	b := img.Bounds()
	if (w == 0 && h == 0) || (w == b.Dx() && h == b.Dy()) {
		return img, nil
	}

	if w == 0 {
		w = max(round(float64(b.Dx())*float64(h)/float64(b.Dy())), 1)
	}

	if h == 0 {
		h = max(round(float64(b.Dy())*float64(w)/float64(b.Dx())), 1)
	}

	out := compositor.Resize(compositor.Image{Pix: img.Pix, W: b.Dx(), H: b.Dy()}, w, h)
	if out.W != w || out.H != h || len(out.Pix) < w*h*4 {
		return nil, exportErr(
			ReasonRender,
			"The picture could not be resized.",
			errors.New("resize returned no pixels"),
			"",
		)
	}

	return &image.RGBA{Pix: out.Pix, Stride: w * 4, Rect: image.Rect(0, 0, w, h)}, nil
}

func encodePNG(img *image.RGBA) ([]byte, error) {
	var buf bytes.Buffer

	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, img); err != nil {
		return nil, exportErr(ReasonRender, "The image could not be encoded.", err, "")
	}

	return buf.Bytes(), nil
}

// encodeJPEG flattens the picture on white, since a JPEG has no alpha, and encodes it.
func encodeJPEG(img *image.RGBA, quality int) ([]byte, error) {
	if quality == 0 {
		quality = DefaultJPEGQuality
	}

	b := img.Bounds()
	flat := image.NewRGBA(b)

	for i := 0; i+3 < len(img.Pix); i += 4 {
		a := int(img.Pix[i+3])
		// the picture is premultiplied: over white, each channel gains what the alpha leaves uncovered
		flat.Pix[i] = over(img.Pix[i], a)
		flat.Pix[i+1] = over(img.Pix[i+1], a)
		flat.Pix[i+2] = over(img.Pix[i+2], a)
		flat.Pix[i+3] = 255
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, flat, &jpeg.Options{Quality: quality}); err != nil {
		return nil, exportErr(ReasonRender, "The image could not be encoded.", err, "")
	}

	return buf.Bytes(), nil
}

// over is a premultiplied channel over white: it gains what the alpha leaves uncovered.
func over(c byte, a int) byte { return byte(min(int(c)+255-a, 255) & 0xff) }
