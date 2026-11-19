package export

import (
	"context"
	"errors"

	exportcore "github.com/AlexGalhardo/stuarty/core/export"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
)

// CodecPlan is the codec an export writes, and what the plan has to say about it (SDD-004 section 8.3).
type CodecPlan struct {
	// Codec is what is written.
	Codec exportcore.Codec
	// Wanted is the codec that the preset preferred when it could not be had, empty otherwise.
	Wanted exportcore.Codec
	// Notes tell a person why the codec is not the preferred one.
	Notes []string
}

// PlanCodec decides the codec of an export before any work: the codec the caller named, which must
// be offered (else E_NO_ENCODER with the reason codec_unavailable, never a silent change), or the
// first codec of the preset's list that the machine offers, or H.264 with a note. A codec is offered
// when an allowed encoder passes its test encode (ffmpeg.Runner.DetectCodec). runner is the one that
// runs the detection; the same answers are cached for the export that follows.
func PlanCodec(
	ctx context.Context,
	runner *ffmpeg.Runner,
	caps *ffmpeg.Capabilities,
	opt ffmpeg.DetectOptions,
	asked exportcore.Codec,
	preset *exportcore.Preset,
) (CodecPlan, error) {
	offered := func(c exportcore.Codec) bool {
		_, err := runner.DetectCodec(ctx, caps, string(c), opt)

		return err == nil
	}

	switch {
	case asked != "" && asked != exportcore.CodecH264:
		if _, err := runner.DetectCodec(ctx, caps, string(asked), opt); err != nil {
			return CodecPlan{}, err
		}

		return CodecPlan{Codec: asked}, nil
	case asked == "" && preset != nil:
		p := exportcore.PlanCodec(preset.Codecs, func(c exportcore.Codec) bool {
			return ctx.Err() == nil && offered(c)
		})
		if err := ctx.Err(); err != nil {
			return CodecPlan{}, err
		}

		plan := CodecPlan{Codec: p.Codec, Wanted: p.Wanted}
		if p.Wanted != "" {
			plan.Notes = []string{fallbackNote(preset, p)}
		}

		return plan, nil
	}

	return CodecPlan{Codec: exportcore.CodecH264}, nil
}

func fallbackNote(p *exportcore.Preset, plan exportcore.Plan) string {
	names := map[exportcore.Codec]string{
		exportcore.CodecHEVC: "HEVC (H.265)", exportcore.CodecAV1: "AV1", exportcore.CodecH264: "H.264",
	}

	return names[plan.Wanted] + " is not available on this machine, so the video for " + p.Label + " was exported as " +
		names[plan.Codec] + "."
}

// errCodecWithFormat is the refusal of a codec or a preset for a format that has no video codec to choose.
var errCodecWithFormat = errors.New("a codec or a preset is for mp4")
