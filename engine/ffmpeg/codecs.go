package ffmpeg

import (
	"context"
	"runtime"
	"slices"
	"strings"
)

// The video codecs an export can write (SDD-004 sections 8.2 and 8.3). They are the names of
// core/export; the engine keeps its own constants so that this package imports no domain package.
const (
	CodecH264 = "h264"
	CodecHEVC = "hevc"
	CodecAV1  = "av1"
)

// HEVCEncoders and AV1Encoders are the allow list of RF-089 and D-067, in the order they are tried:
// the vendor encoders of the three systems, then software encoders with a permissive licence.
// There is no software HEVC encoder here: libx265 is GPL, and the only other one in common builds
// is slower than the H.264 fall back, so HEVC is a hardware codec.
var (
	HEVCEncoders = []string{
		"hevc_videotoolbox", "hevc_nvenc", "hevc_qsv", "hevc_amf", "hevc_mf", "hevc_vaapi",
	}
	AV1Encoders = []string{
		"av1_nvenc", "av1_qsv", "av1_amf", "av1_vaapi", "libsvtav1", "libaom-av1",
	}
)

// RefusedEncoders are the encoders that are never used, even when the user's FFmpeg has them and
// even when one is forced: they are GPL or nonfree (ADR-0011, ADR-0003, NFR-043). libx264 is not
// here because the H.264 detection already allows it for a user's own build with an explicit opt-in.
var RefusedEncoders = []string{"libx265", "libx264rgb", "libxavs2", "libxvid", "libfdk_aac"}

// EncodersFor is the allow list of a codec in the order it is tried; nil for a codec that has none.
func EncodersFor(codec string) []string {
	switch codec {
	case CodecH264:
		return EncoderOrder
	case CodecHEVC:
		return HEVCEncoders
	case CodecAV1:
		return AV1Encoders
	}

	return nil
}

// CodecOf is the codec an allowed encoder writes, or "" for a name that is not on an allow list.
func CodecOf(encoder string) string {
	switch {
	case slices.Contains(EncoderOrder, encoder):
		return CodecH264
	case slices.Contains(HEVCEncoders, encoder):
		return CodecHEVC
	case slices.Contains(AV1Encoders, encoder):
		return CodecAV1
	}

	return ""
}

// IsRefusedEncoder says whether the encoder is one the product never uses.
func IsRefusedEncoder(encoder string) bool { return slices.Contains(RefusedEncoders, encoder) }

// IsHardwareEncoder says whether the encoder is a vendor encoder, whose driver maps very large
// address ranges (SDD-007 section 6.5). The software encoders are libopenh264, libx264, libsvtav1
// and libaom-av1.
func IsHardwareEncoder(encoder string) bool {
	return CodecOf(encoder) != "" && !strings.HasPrefix(encoder, "lib")
}

// ReasonRefusedEncoder is the Reason of a candidate that was never tried because it is a GPL or
// nonfree encoder.
const ReasonRefusedEncoder = "refused_encoder"

// ReasonCodecUnavailable is details.reason of E_NO_ENCODER when the codec an export asks for has no
// encoder that works on this machine.
const ReasonCodecUnavailable = "codec_unavailable"

// platformOKCodec is platformOK for the HEVC and AV1 names.
func platformOKCodec(name string) bool {
	switch name {
	case "hevc_videotoolbox":
		return runtime.GOOS == "darwin"
	case "hevc_mf":
		return runtime.GOOS == "windows"
	case "hevc_vaapi", "av1_vaapi":
		return runtime.GOOS == "linux"
	}

	return true
}

type codecResult struct {
	det *EncoderDetection
	err error
}

// DetectCodec is DetectEncoder for a codec: it runs the test encode of each allowed encoder of the
// codec that the build lists, in order, and caches the answer for the life of the runner. H.264 is
// DetectEncoder itself. When no encoder works the error is E_NO_ENCODER with the reason
// codec_unavailable and the answer of every candidate; the detection is still returned, so a caller
// can report why. A refused encoder (libx265) is never a candidate.
func (r *Runner) DetectCodec(
	ctx context.Context,
	caps *Capabilities,
	codec string,
	opt DetectOptions,
) (*EncoderDetection, error) {
	if codec == "" || codec == CodecH264 {
		return r.DetectEncoder(ctx, caps, opt)
	}

	names := EncodersFor(codec)
	if names == nil {
		return nil, &Error{
			Code: CodeNoEncoder, Reason: ReasonCodecUnavailable,
			Message: "The codec " + scrub(codec, 20) + " is not one the product writes.",
			Hint:    "Use h264, hevc or av1.",
			Details: map[string]any{"codec": scrub(codec, 20), "reason": ReasonCodecUnavailable},
		}
	}

	r.codecMu.Lock()
	defer r.codecMu.Unlock()

	if got, ok := r.codecDet[codec]; ok && !opt.Refresh {
		return got.det, got.err
	}

	opt.Forced = ""

	det, err := r.detectNames(ctx, caps, names, opt, codec)
	if ctx.Err() == nil { // a cancelled detection is not cached
		if r.codecDet == nil {
			r.codecDet = map[string]codecResult{}
		}

		r.codecDet[codec] = codecResult{det, err}
	}

	return det, err
}

// noCodecError is E_NO_ENCODER for a codec that has no working encoder.
func noCodecError(codec string, det *EncoderDetection) *Error {
	details := make([]map[string]any, 0, len(det.Candidates))
	parts := make([]string, 0, len(det.Candidates))

	for _, c := range det.Candidates {
		why := "not in this FFmpeg build"

		switch {
		case c.Status == EncoderFailed:
			first, _, _ := strings.Cut(c.StderrTail, "\n")
			why = "failed: " + scrub(first, 120)
		case c.StderrTail != "":
			why = c.StderrTail
		}

		parts = append(parts, c.Name+" ("+why+")")
		details = append(details, map[string]any{"name": c.Name, "status": string(c.Status), "why": why, "ms": c.MS})
	}

	return &Error{
		Code:   CodeNoEncoder,
		Reason: ReasonCodecUnavailable,
		Message: "No " + strings.ToUpper(codec) + " encoder works on this machine. Tried: " +
			strings.Join(parts, ", ") + ".",
		Hint: "Export with h264, or use an FFmpeg that has a hardware " + strings.ToUpper(codec) +
			" encoder the machine supports (\"stuarty doctor\" lists the encoders and why each one was or was not chosen).",
		Details: map[string]any{"codec": codec, "reason": ReasonCodecUnavailable, "candidates": details},
	}
}
