package ffmpeg

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/AlexGalhardo/stuarty/engine/ffmpeg/ffargs"
)

// Ceilings of SDD-007 9.1 that the probe enforces before any decode (9.2 item 1). A zero
// field takes its default; the variable names are those of SDD-000 section 10.
type Ceilings struct {
	MaxPixels  int64   // STUARTY_MAX_IMPORT_PIXELS
	MaxSeconds float64 // STUARTY_MAX_IMPORT_SECONDS
}

const (
	// MaxSourceEdge is the fixed limit of the longest side of a source.
	MaxSourceEdge = 8192
	// MaxStreams is the stream limit of FF-06.
	MaxStreams = 16

	// DefaultMaxPixels is the default of STUARTY_MAX_IMPORT_PIXELS.
	DefaultMaxPixels int64 = 33_177_600
	// DefaultMaxSeconds is the default of STUARTY_MAX_IMPORT_SECONDS.
	DefaultMaxSeconds float64 = 14400
)

// ProbeStream is the part of an ffprobe stream the checks need. Raw keeps everything for ingest.
type ProbeStream struct {
	Index       int    `json:"index"`
	CodecType   string `json:"codec_type"`
	CodecName   string `json:"codec_name"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	Disposition struct {
		AttachedPic int `json:"attached_pic"`
		Default     int `json:"default"`
	} `json:"disposition"`
}

// ProbeFormat is the container block.
type ProbeFormat struct {
	Name      string  `json:"format_name"`
	NbStreams int     `json:"nb_streams"`
	Duration  string  `json:"duration"`
	Size      string  `json:"size"`
	Seconds   float64 `json:"-"`
}

// ProbeResult is a checked ffprobe answer.
type ProbeResult struct {
	Raw     []byte // the JSON as ffprobe printed it
	Format  ProbeFormat
	Streams []ProbeStream
}

// Probe is the entry of ingest: FF-05 (signature before FFmpeg), the ffprobe run with the
// caps of FF-09, and the checks of FF-06 (format allowlist, stream count, a usable stream,
// the ceilings). path is a file of the store; dir is the run directory of the probe.
func (r *Runner) Probe(ctx context.Context, dir *RunDir, path string, c Ceilings) (*ProbeResult, error) {
	if err := SniffFile(path); err != nil {
		if _, ok := errors.AsType[*Error](err); ok {
			return nil, err
		}
		return nil, &Error{
			Code:    "E_FILE_NOT_FOUND",
			Reason:  "unreadable",
			Message: "The file could not be read.",
			Err:     err,
		}
	}
	argv, err := ffargs.ProbeJSON(r.Policy(dir), path)
	if err != nil {
		return nil, &Error{
			Code:    CodeInternal,
			Reason:  "argv_policy",
			Message: "the ffprobe arguments were refused before any start",
			Err:     err,
		}
	}
	out, res, err := r.RunFFprobe(ctx, dir, argv, ProbeTimeout)
	if err != nil {
		var e *Error
		if errors.As(err, &e) || ctx.Err() != nil || errors.Is(err, ErrDeadline) {
			return nil, err
		}
		line, _, _ := strings.Cut(strings.TrimSpace(res.Stderr), "\n")
		return nil, unsupported(
			"unrecognized_format",
			"The file is not a media file FFmpeg can read.",
			"Copy the original file from the device again. If the recording was cut short, a repair tool that rebuilds the index can recover it.",
			map[string]any{"ffprobe": scrub(line, 200)},
		)
	}
	return CheckProbe(out, c)
}

// CheckProbe applies FF-06 to the JSON of ffprobe -show_format -show_streams.
func CheckProbe(raw []byte, c Ceilings) (*ProbeResult, error) {
	var doc struct {
		Format  ProbeFormat   `json:"format"`
		Streams []ProbeStream `json:"streams"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, unsupported(
			"unrecognized_format",
			"The file is not a media file FFmpeg can read.",
			"",
			map[string]any{"ffprobe": "unreadable probe output"},
		)
	}
	pr := &ProbeResult{Raw: raw, Format: doc.Format, Streams: doc.Streams}
	pr.Format.Seconds, _ = strconv.ParseFloat(doc.Format.Duration, 64)

	if !ffargs.AllowedFormatName(pr.Format.Name) {
		return nil, unsupported("unrecognized_format", "The file is not a media file FFmpeg can read.",
			"Import a video, audio or image file.", map[string]any{"ffprobe": scrub("container "+pr.Format.Name, 100)})
	}
	if len(pr.Streams) == 0 {
		return nil, unsupported(
			"unrecognized_format",
			"The file has no stream FFmpeg can read.",
			"",
			map[string]any{"ffprobe": "no streams"},
		)
	}
	if len(pr.Streams) > MaxStreams || pr.Format.NbStreams > MaxStreams {
		return nil, unsupported(
			"too_many_streams",
			"The file has more streams than the product reads.",
			"Remux the file with only the streams you need.",
			map[string]any{"streams": max(len(pr.Streams), pr.Format.NbStreams)},
		)
	}
	usable := false
	for _, s := range pr.Streams {
		if (s.CodecType == "video" || s.CodecType == "audio") && s.CodecName != "" && s.CodecName != "unknown" {
			usable = true
		}
	}
	if !usable {
		return nil, unsupported("no_usable_stream", "The file has no video or audio stream FFmpeg can use.",
			"Choose a file with video, audio or a picture.", nil)
	}

	maxPixels, maxSeconds := c.MaxPixels, c.MaxSeconds
	if maxPixels <= 0 {
		maxPixels = DefaultMaxPixels
	}
	if maxSeconds <= 0 {
		maxSeconds = DefaultMaxSeconds
	}
	for _, s := range pr.Streams {
		if s.CodecType != "video" || s.Disposition.AttachedPic == 1 {
			continue
		}
		// A picture whose size ffprobe could not read (a PNG header with no data, say) cannot be
		// checked against the ceilings, so it is not trusted.
		if s.Width <= 0 || s.Height <= 0 {
			return nil, unsupported("unreadable_dimensions", "The size of the picture could not be read.",
				"Choose a file that is whole and not corrupt.", nil)
		}
		if edge := max(s.Width, s.Height); edge > MaxSourceEdge {
			return nil, limit(
				"source_edge",
				int64(edge),
				MaxSourceEdge,
				"",
				"The picture is "+strconv.Itoa(
					edge,
				)+" pixels on its longest side; the longest accepted is "+strconv.Itoa(
					MaxSourceEdge,
				)+".",
			)
		}
		if px := int64(s.Width) * int64(s.Height); px > maxPixels {
			return nil, limit(
				"import_pixels",
				px,
				maxPixels,
				"STUARTY_MAX_IMPORT_PIXELS",
				"The picture has "+strconv.FormatInt(
					px,
					10,
				)+" pixels; the limit is "+strconv.FormatInt(
					maxPixels,
					10,
				)+".",
			)
		}
	}
	if pr.Format.Seconds > maxSeconds {
		return nil, limit(
			"import_seconds",
			int64(pr.Format.Seconds),
			int64(maxSeconds),
			"STUARTY_MAX_IMPORT_SECONDS",
			"The media is "+strconv.FormatInt(
				int64(pr.Format.Seconds),
				10,
			)+" seconds long; the longest accepted is "+strconv.FormatInt(
				int64(maxSeconds),
				10,
			)+".",
		)
	}
	return pr, nil
}

// limit is the E_LIMIT_EXCEEDED of SDD-007 9.1: the name, the value seen, the ceiling and,
// where one exists, the variable.
func limit(name string, value, ceiling int64, variable, msg string) *Error {
	d := map[string]any{"limit": name, "value": value, "ceiling": ceiling}
	hint := "Cut it into shorter files or reduce its size first."
	if variable != "" {
		d["variable"] = variable
		hint = "Ask the operator to raise " + variable + ", or cut the file first."
	}
	return &Error{Code: CodeLimitExceeded, Reason: name, Message: msg, Hint: hint, Details: d}
}
