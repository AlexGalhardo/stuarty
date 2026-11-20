package provider

import (
	"bytes"
	"math"
	"regexp"
	"slices"
	"strings"
)

// Limits of SDD-016 section 5.4 (epic 11).
const (
	MaxVideoPrompt      = 4_000
	MinVideoSeconds     = 1
	MaxVideoSeconds     = 60
	DefaultVideoSeconds = 5
	DefaultVideoAspect  = "16:9"
	MaxTaskID           = 512
	maxFrameAssetID     = 64
)

// UnitSecond is the unit of the price of a video: the cost of one second of it (SDD-016 section 4.5).
const UnitSecond = "second"

// VideoAspects are the aspect ratios a video may be asked in.
var VideoAspects = []string{"16:9", "9:16", "1:1", "4:3", "3:4"}

var (
	frameAssetRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,63}$`)
	taskIDRe     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/=@+-]{0,511}$`)
)

// ValidTaskID reports whether s can be the task id a provider gave for a generation: letters, digits and
// . _ : / = @ + -, the first a letter or a digit, at most 512 characters. An id that does not match is not
// stored, so what is kept in a job and sent back to the provider carries no text.
func ValidTaskID(s string) bool { return taskIDRe.MatchString(s) }

// VideoRequest is one request for a clip (SDD-016 section 5.4).
type VideoRequest struct {
	Model   string `json:"model"`
	Prompt  string `json:"prompt"`
	Seconds int    `json:"seconds,omitempty"`
	Aspect  string `json:"aspect,omitempty"`
	// FirstFrame is the id of an image asset of the project, the first frame of the clip.
	FirstFrame string `json:"first_frame,omitempty"`
}

// VideoLimits are what a provider documents for one model; a zero value is "not documented".
type VideoLimits struct {
	MinSeconds int
	MaxSeconds int
	Aspects    []string
	// NoFirstFrame is set when the model documents that it takes no image.
	NoFirstFrame bool
}

// NormalizeVideo trims the model, the aspect and the first frame and applies the defaults of length and aspect.
func NormalizeVideo(r VideoRequest) VideoRequest {
	r.Model = strings.TrimSpace(r.Model)
	r.Aspect = strings.TrimSpace(r.Aspect)
	r.FirstFrame = strings.TrimSpace(r.FirstFrame)

	if r.Seconds == 0 {
		r.Seconds = DefaultVideoSeconds
	}

	if r.Aspect == "" {
		r.Aspect = DefaultVideoAspect
	}

	return r
}

// ValidateVideo checks a request, in stored form (NormalizeVideo), against SDD-016 section 5.4 and the limits
// the adapter documents for the model.
func ValidateVideo(r VideoRequest, l VideoLimits) *FieldError {
	if !ValidModel(r.Model) {
		return bad("model", "must be 1 to %d characters of letters, digits and . _ : / @ + -", MaxModelID)
	}

	if f := checkText("prompt", r.Prompt, 1, MaxVideoPrompt); f != nil {
		return f
	}

	lo, hi := MinVideoSeconds, MaxVideoSeconds
	if l.MinSeconds > 0 {
		lo = max(lo, l.MinSeconds)
	}

	if l.MaxSeconds > 0 {
		hi = min(hi, l.MaxSeconds)
	}

	if r.Seconds < lo || r.Seconds > hi {
		return bad("seconds", "must be %d to %d", lo, hi)
	}

	if !slices.Contains(VideoAspects, r.Aspect) {
		return bad("aspect", "must be one of %s", strings.Join(VideoAspects, ", "))
	}

	if len(l.Aspects) > 0 && !slices.Contains(l.Aspects, r.Aspect) {
		return bad("aspect", "the model makes only %s", strings.Join(l.Aspects, ", "))
	}

	if r.FirstFrame != "" {
		if !frameAssetRe.MatchString(r.FirstFrame) {
			return bad(
				"first_frame", "must be the id of an image asset of the project (1 to %d characters)", maxFrameAssetID,
			)
		}

		if l.NoFirstFrame {
			return bad("first_frame", "the model takes no image as the first frame")
		}
	}

	return nil
}

// VideoCost is the cost of a number of seconds of video at a price per second; it saturates at the largest int64.
func VideoCost(seconds int, p Price) Cost {
	if seconds <= 0 || p.UnitMicros <= 0 {
		return Cost{Currency: p.Currency}
	}

	if p.UnitMicros > math.MaxInt64/int64(seconds) {
		return Cost{Micros: math.MaxInt64, Currency: p.Currency}
	}

	return Cost{Micros: int64(seconds) * p.UnitMicros, Currency: p.Currency}
}

// EstimateVideo is VideoCost of the seconds the request asks for.
func EstimateVideo(r VideoRequest, p Price) Cost { return VideoCost(r.Seconds, p) }

// CheckVideo returns the container of a file that has the signature of an MP4, a QuickTime movie or a WebM
// file, and false for anything else (SDD-016 section 5.4).
func CheckVideo(b []byte) (string, bool) {
	switch {
	case len(b) >= 12 && bytes.Equal(b[4:8], []byte("ftyp")):
		if bytes.Equal(b[8:12], []byte("qt  ")) {
			return "mov", true
		}

		return "mp4", true
	case bytes.HasPrefix(b, []byte{0x1A, 0x45, 0xDF, 0xA3}):
		return "webm", true
	}

	return "", false
}
