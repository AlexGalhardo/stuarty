package mediagen

import (
	"context"
	"fmt"
	"strconv"
)

// AlphaInput is one of the inputs with alpha that the ingest accepts (SDD-011 section 4.3.6).
type AlphaInput string

// The accepted inputs with alpha.
const (
	AlphaProRes AlphaInput = "prores" // ProRes 4444 in MOV
	AlphaVP9    AlphaInput = "vp9"    // VP9 with an alpha plane in WebM
	AlphaQtrle  AlphaInput = "qtrle"  // QuickTime Animation in MOV
	AlphaPNG    AlphaInput = "png"    // a still PNG
	AlphaAPNG   AlphaInput = "apng"   // an animated PNG
	AlphaWebP   AlphaInput = "webp"   // an animated WebP
)

// AlphaInputs lists them in the order of the table of SDD-011 section 4.3.6.
func AlphaInputs() []AlphaInput {
	return []AlphaInput{AlphaProRes, AlphaVP9, AlphaQtrle, AlphaPNG, AlphaAPNG, AlphaWebP}
}

// Ext is the file extension the input is written with.
func (a AlphaInput) Ext() string {
	switch a {
	case AlphaProRes, AlphaQtrle:
		return "mov"
	case AlphaVP9:
		return "webm"
	case AlphaWebP:
		return "webp"
	case AlphaPNG, AlphaAPNG:
		return "png"
	}

	return "png"
}

// Video reports whether the input is a video (the others are images).
func (a AlphaInput) Video() bool { return a == AlphaProRes || a == AlphaVP9 || a == AlphaQtrle }

// edgeAlpha is the alpha of the soft edge fixtures: 0 left of a quarter of the width, rising linearly to 255 at the
// half, opaque to the right. The picture is red everywhere, so a half transparent pixel is (255, 0, 0, ~128) when
// the alpha is read as straight, and (128, 0, 0, ~128) when a decoder premultiplies it.
const edgeAlpha = "clip((X-W/4)*255/(W/4),0,255)"

// AlphaEdge is a clip or an image of the given input with a red picture and the soft alpha edge of edgeAlpha. A
// video lasts secs seconds at fps; an animated image has frames frames; a still has one.
func (g Gen) AlphaEdge(
	ctx context.Context, in AlphaInput, name string, w, h, fps int, secs float64, frames int,
) (string, error) {
	return g.alphaFixture(ctx, in, name, w, h, fps, secs, frames, edgeAlpha)
}

// AlphaOpaque is AlphaEdge with an alpha plane that is 255 everywhere: a file that declares alpha and has none.
func (g Gen) AlphaOpaque(
	ctx context.Context, in AlphaInput, name string, w, h, fps int, secs float64, frames int,
) (string, error) {
	return g.alphaFixture(ctx, in, name, w, h, fps, secs, frames, "255")
}

// AlphaLater is AlphaEdge with a first frame that is opaque everywhere and a transparent left half on every later
// frame: an animation that a sample of its first picture alone would call opaque.
func (g Gen) AlphaLater(
	ctx context.Context, in AlphaInput, name string, w, h, fps int, secs float64, frames int,
) (string, error) {
	return g.alphaFixture(ctx, in, name, w, h, fps, secs, frames, "if(eq(N,0),255,if(lt(X,W/2),0,255))")
}

func (g Gen) alphaFixture(
	ctx context.Context, in AlphaInput, name string, w, h, fps int, secs float64, frames int, alpha string,
) (string, error) {
	rate := fps
	if !in.Video() {
		rate = 15
	}

	args := []string{"-f", "lavfi", "-i", fmt.Sprintf("color=c=red:s=%dx%d:r=%d:d=%g", w, h, rate, secs)}

	yuv := func(pix string) string {
		return fmt.Sprintf("format=yuva444p,geq=lum='lum(X,Y)':cb='cb(X,Y)':cr='cr(X,Y)':a='%s',format=%s", alpha, pix)
	}
	rgb := func(pix string) string {
		return fmt.Sprintf("format=rgba,geq=r='r(X,Y)':g='g(X,Y)':b='b(X,Y)':a='%s',format=%s", alpha, pix)
	}

	switch in {
	case AlphaProRes:
		args = append(args, "-vf", yuv("yuva444p10le"),
			"-c:v", "prores_ks", "-profile:v", "4444", "-pix_fmt", "yuva444p10le", "-f", "mov")
	case AlphaVP9:
		args = append(
			args,
			"-vf",
			fmt.Sprintf("format=yuva420p,geq=lum='lum(X,Y)':cb='cb(X,Y)':cr='cr(X,Y)':a='%s'", alpha),
			"-c:v",
			"libvpx-vp9",
			"-pix_fmt",
			"yuva420p",
			"-auto-alt-ref",
			"0",
			"-crf",
			"20",
			"-b:v",
			"0",
			"-f",
			"webm",
		)
	case AlphaQtrle:
		args = append(args, "-vf", rgb("argb"), "-c:v", "qtrle", "-pix_fmt", "argb", "-f", "mov")
	case AlphaPNG:
		args = append(args, "-vf", rgb("rgba"), "-frames:v", "1", "-c:v", "png", "-pix_fmt", "rgba", "-f", "image2")
	case AlphaAPNG:
		args = append(args, "-vf", rgb("rgba"), "-frames:v", strconv.Itoa(frames), "-plays", "0", "-f", "apng")
	case AlphaWebP:
		args = append(args, "-vf", rgb("rgba"), "-frames:v", strconv.Itoa(frames),
			"-c:v", "libwebp_anim", "-loop", "0", "-lossless", "1", "-f", "webp")
	default:
		return "", fmt.Errorf("mediagen: %q is not an input with alpha", in)
	}

	return g.ff(ctx, name, args...)
}
