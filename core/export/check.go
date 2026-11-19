package export

import (
	"fmt"
	"strconv"
	"strings"
)

// The codes of the findings of Check (SDD-000 section 15).
const (
	CodeDurationTooLong    = "X_DURATION_TOO_LONG"
	CodeDurationTooShort   = "X_DURATION_TOO_SHORT"
	CodeFPSTooHigh         = "X_FPS_TOO_HIGH"
	CodeFPSTooLow          = "X_FPS_TOO_LOW"
	CodeResolutionTooSmall = "X_RESOLUTION_TOO_SMALL"
	CodeResolutionTooLarge = "X_RESOLUTION_TOO_LARGE"
	CodeAspectRatio        = "X_ASPECT_RATIO"
	CodeSizeTooLarge       = "X_SIZE_TOO_LARGE"
)

// SeverityError is the severity of a finding: a project that does not fit is never a warning.
const SeverityError = "error"

// Finding is one thing that does not fit a preset, with the exact limit and what the project has.
type Finding struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Limit    string `json:"limit"`
	Got      string `json:"got"`
}

// CheckName is the name of the verification check that a finding becomes once the file is written
// (SDD-004 section 8.2): X_DURATION_TOO_LONG is preset_duration_too_long.
func (f Finding) CheckName() string {
	return "preset_" + strings.ToLower(strings.TrimPrefix(f.Code, "X_"))
}

// Input is what Check looks at: the picture, the frame rate and the length of the export, and the
// size of the file once there is one.
type Input struct {
	Width, Height  int
	FPSNum, FPSDen int
	DurationMS     int64
	// Bytes is the size of the written file; zero before the encode, when the size is estimated.
	Bytes int64
	// Codec is the codec of the estimate; the empty string is H.264.
	Codec Codec
}

// VideoKbps is the video bit rate in kbit/s the preset asks for an export of w x h at the frame
// rate with the codec: the target of the preset scaled by the pixel count, raised above 30 fps,
// taken as a share of it for HEVC and AV1, never below MinVideoKbps and never above the ceiling of
// the preset.
func (p Preset) VideoKbps(w, h, fpsNum, fpsDen int, c Codec) int {
	t := p.Targets
	if t.Width <= 0 || t.Height <= 0 || w <= 0 || h <= 0 {
		return t.VideoKbps
	}

	kbps := int64(t.VideoKbps) * int64(w) * int64(h) / (int64(t.Width) * int64(t.Height))

	if fpsDen > 0 && fpsNum > 30*fpsDen {
		kbps = kbps * aboveThirtyPermille / 1000
	}

	kbps = int64(c.ScaleKbps(int(kbps)))

	if m := int64(p.Limits.MaxVideoKbps); m > 0 {
		kbps = min(kbps, m)
	}

	return int(kbps)
}

// EstimateBytes is the size an export is expected to have: the video and audio targets times the
// duration. It is an upper bound for a rate-controlled encode, not a promise.
func (p Preset) EstimateBytes(in Input) int64 {
	kbps := int64(p.VideoKbps(in.Width, in.Height, in.FPSNum, in.FPSDen, in.Codec) + p.Targets.AudioKbps)

	return kbps * 1000 / 8 * in.DurationMS / 1000
}

// Check says what of the export does not fit the preset, in a fixed order: duration, frame rate,
// resolution, aspect ratio, size. A limit the preset does not state is not checked. The result is
// empty when everything fits.
func (p Preset) Check(in Input) []Finding {
	var out []Finding

	add := func(code, message, limit, got string) {
		out = append(out, Finding{Code: code, Severity: SeverityError, Message: message, Limit: limit, Got: got})
	}

	l := p.Limits

	if l.MaxDurationMS > 0 && in.DurationMS > l.MaxDurationMS {
		add(CodeDurationTooLong,
			fmt.Sprintf("The video is %s long; %s allows at most %s.",
				seconds(in.DurationMS), p.Label, seconds(l.MaxDurationMS)),
			seconds(l.MaxDurationMS), seconds(in.DurationMS))
	}

	if l.MinDurationMS > 0 && in.DurationMS < l.MinDurationMS {
		add(CodeDurationTooShort,
			fmt.Sprintf("The video is %s long; %s needs at least %s.",
				seconds(in.DurationMS), p.Label, seconds(l.MinDurationMS)),
			seconds(l.MinDurationMS), seconds(in.DurationMS))
	}

	if in.FPSDen > 0 && in.FPSNum > 0 {
		got := fpsText(in.FPSNum, in.FPSDen)

		if l.MaxFPS > 0 && in.FPSNum > l.MaxFPS*in.FPSDen {
			add(CodeFPSTooHigh,
				fmt.Sprintf("The project runs at %s fps; %s allows at most %d.", got, p.Label, l.MaxFPS),
				strconv.Itoa(l.MaxFPS)+" fps", got+" fps")
		}

		if l.MinFPS > 0 && in.FPSNum < l.MinFPS*in.FPSDen {
			add(CodeFPSTooLow,
				fmt.Sprintf("The project runs at %s fps; %s needs at least %d.", got, p.Label, l.MinFPS),
				strconv.Itoa(l.MinFPS)+" fps", got+" fps")
		}
	}

	out = append(out, p.checkResolution(in)...)
	out = append(out, p.checkAspect(in)...)

	size, estimated := in.Bytes, false
	if size <= 0 {
		size, estimated = p.EstimateBytes(in), true
	}

	if l.MaxBytes > 0 && size > l.MaxBytes {
		what := "The file is"
		if estimated {
			what = "The file is estimated at"
		}

		add(CodeSizeTooLarge,
			fmt.Sprintf("%s %s; %s allows at most %s.", what, megabytes(size), p.Label, megabytes(l.MaxBytes)),
			megabytes(l.MaxBytes), megabytes(size))
	}

	return out
}

func (p Preset) checkResolution(in Input) []Finding {
	if in.Width <= 0 || in.Height <= 0 {
		return nil
	}

	var out []Finding

	l := p.Limits
	got := fmt.Sprintf("%dx%d", in.Width, in.Height)

	if (l.MinWidth > 0 && in.Width < l.MinWidth) || (l.MinHeight > 0 && in.Height < l.MinHeight) {
		limit := fmt.Sprintf("%dx%d", l.MinWidth, l.MinHeight)
		out = append(out, Finding{
			Code: CodeResolutionTooSmall, Severity: SeverityError, Limit: limit, Got: got,
			Message: fmt.Sprintf("The project is %s pixels; %s needs at least %s.", got, p.Label, limit),
		})
	}

	if (l.MaxWidth > 0 && in.Width > l.MaxWidth) || (l.MaxHeight > 0 && in.Height > l.MaxHeight) {
		limit := fmt.Sprintf("%d pixels wide and %d high", l.MaxWidth, l.MaxHeight)

		switch {
		case l.MaxHeight == 0:
			limit = fmt.Sprintf("%d pixels wide", l.MaxWidth)
		case l.MaxWidth == 0:
			limit = fmt.Sprintf("%d pixels high", l.MaxHeight)
		}

		out = append(out, Finding{
			Code: CodeResolutionTooLarge, Severity: SeverityError, Limit: limit, Got: got,
			Message: fmt.Sprintf("The project is %s pixels; %s allows at most %s.", got, p.Label, limit),
		})
	}

	return out
}

func (p Preset) checkAspect(in Input) []Finding {
	if in.Width <= 0 || in.Height <= 0 {
		return nil
	}

	l := p.Limits
	w, h := int64(in.Width), int64(in.Height)
	tol := int64(l.AspectTolerancePermille)
	low, high := l.AspectMin, l.AspectMax

	under := low.Num > 0 && low.Den > 0 && w*int64(low.Den)*1000 < h*int64(low.Num)*(1000-tol)
	over := high.Num > 0 && high.Den > 0 && w*int64(high.Den)*1000 > h*int64(high.Num)*(1000+tol)

	if !under && !over {
		return nil
	}

	limit := aspectRange(low, high)
	got := fmt.Sprintf("%d:%d", in.Width, in.Height)

	return []Finding{{
		Code: CodeAspectRatio, Severity: SeverityError, Limit: limit, Got: got,
		Message: fmt.Sprintf("The picture is %s (%s); %s takes %s.", got, ratioText(w, h), p.Label, limit),
	}}
}

func aspectRange(low, high Ratio) string {
	switch {
	case low == high && low.Num > 0:
		return fmt.Sprintf("%d:%d", low.Num, low.Den)
	case low.Num > 0 && high.Num > 0:
		return fmt.Sprintf("%d:%d to %d:%d", low.Num, low.Den, high.Num, high.Den)
	case high.Num > 0:
		return fmt.Sprintf("at most %d:%d (width over height)", high.Num, high.Den)
	}

	return fmt.Sprintf("at least %d:%d (width over height)", low.Num, low.Den)
}

func ratioText(w, h int64) string { return strconv.FormatFloat(float64(w)/float64(h), 'f', 3, 64) }

// seconds writes a duration in milliseconds as "12.5 s", with no trailing zeros.
func seconds(ms int64) string {
	s := strconv.FormatFloat(float64(ms)/1000, 'f', 3, 64)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")

	return s + " s"
}

func fpsText(num, den int) string {
	if den == 1 {
		return strconv.Itoa(num)
	}

	return strconv.FormatFloat(float64(num)/float64(den), 'f', 3, 64)
}

// megabytes writes a size in bytes as megabytes of a million bytes, or as gigabytes from 1000 MB.
func megabytes(b int64) string {
	if b >= 1_000_000_000 {
		return strconv.FormatFloat(float64(b)/1e9, 'f', 2, 64) + " GB"
	}

	return strconv.FormatFloat(float64(b)/1e6, 'f', 1, 64) + " MB"
}

// Plan is the codec an export will write.
type Plan struct {
	// Codec is what is written.
	Codec Codec
	// Wanted is the codec of the preference list that came first when it is not the one written;
	// empty when the preference was met.
	Wanted Codec
}

// PlanCodec picks the first codec of the list that offered says the machine can encode. When none
// can, the export falls back to H.264 and Wanted names what was wanted first (SDD-004 section 8.3).
func PlanCodec(preferred []Codec, offered func(Codec) bool) Plan {
	for i, c := range preferred {
		if c == CodecH264 || offered(c) {
			plan := Plan{Codec: c}
			if i > 0 {
				plan.Wanted = preferred[0]
			}

			return plan
		}
	}

	plan := Plan{Codec: CodecH264}
	if len(preferred) > 0 {
		plan.Wanted = preferred[0]
	}

	return plan
}
