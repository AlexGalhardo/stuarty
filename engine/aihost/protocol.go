package aihost

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/facetrack"
)

// Protocol is the version of the helper protocol this host speaks (SDD-024 section 5).
const Protocol = 1

// Limits of what the host accepts from the helper.
const (
	// MaxLine is the longest line of the helper's output.
	MaxLine = 64 << 10
	// MaxFaces is the most faces the host keeps per frame.
	MaxFaces = facetrack.MaxFacesPerShot
)

// Codes the helper may name in an error line (SDD-024 section 5). Anything else is reported as "protocol".
var helperCodes = map[string]bool{
	"usage": true, "frame_size": true, "runtime_load": true, "model_load": true, "model": true, "session": true,
	"provider_unavailable": true, "inference": true, "short_frame": true, "io": true,
}

// Error is a failure of the helper or of its protocol. Reason is the helper's code, "protocol" for a line
// the host refuses, or "timeout"; Message is printable text that carries no path.
type Error struct {
	Reason  string
	Message string
}

func (e *Error) Error() string { return "aihost: " + e.Reason + ": " + e.Message }

func protocolError(format string, a ...any) *Error {
	return &Error{Reason: "protocol", Message: fmt.Sprintf(format, a...)}
}

// line is every field a line of the helper may carry.
type line struct {
	Type     string            `json:"type"`
	Protocol int               `json:"protocol"`
	Provider string            `json:"provider"`
	Width    int               `json:"width"`
	Height   int               `json:"height"`
	Index    *int              `json:"index"`
	Faces    []json.RawMessage `json:"faces"`
	Frames   *int              `json:"frames"`
	InferMS  int64             `json:"infer_ms"`
	Code     string            `json:"code"`
	Message  string            `json:"message"`
}

type rawFace struct {
	X     json.Number `json:"x"`
	Y     json.Number `json:"y"`
	W     json.Number `json:"w"`
	H     json.Number `json:"h"`
	Score json.Number `json:"score"`
}

// pixelFace is a face in tenths of a pixel of the padded analysis frame, with its score in thousandths.
type pixelFace struct {
	X, Y, W, H int // tenths of a pixel
	Score      int // thousandths
}

// fixed reads a decimal number as an integer count of 10^-digits with no floating point: with one digit
// "79.4" is 794, "3" is 30 and "-1.0" is -10. The next digit rounds half up; an exponent, a sign other
// than a leading minus or text is refused.
func fixed(n json.Number, digits int) (int, error) {
	s := strings.TrimSpace(n.String())
	if s == "" || len(s) > 14 {
		return 0, errors.New("not a number")
	}

	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")

	whole, frac, _ := strings.Cut(s, ".")
	if whole == "" || len(whole) > 9 {
		return 0, errors.New("not a decimal number")
	}

	for _, c := range whole + frac {
		if c < '0' || c > '9' {
			return 0, errors.New("not a decimal number")
		}
	}

	v := 0

	for _, c := range whole {
		v = v*10 + int(c-'0')
	}

	for i := range digits {
		d := 0
		if i < len(frac) {
			d = int(frac[i] - '0')
		}

		v = v*10 + d
	}

	if len(frac) > digits && frac[digits] >= '5' {
		v++
	}

	if neg {
		v = -v
	}

	return v, nil
}

// tenths is a number of tenths of a pixel; score is a score from 0 to 1 in thousandths.
func tenths(n json.Number) (int, error) { return fixed(n, 1) }

func score(n json.Number) (int, error) {
	v, err := fixed(n, 3)
	if err != nil || v < 0 || v > 1000 {
		return 0, errors.New("a score is between 0 and 1")
	}

	return v, nil
}

// parseLine decodes one line. The line is plain JSON of a known shape; the faces are validated here.
func parseLine(b []byte) (line, []pixelFace, error) {
	var l line

	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()

	if err := dec.Decode(&l); err != nil {
		return line{}, nil, protocolError("a line of the helper is not valid JSON")
	}

	if l.Type != "frame" {
		return l, nil, nil
	}

	if l.Index == nil || *l.Index < 0 {
		return line{}, nil, protocolError("a frame line has no index")
	}

	if len(l.Faces) > MaxFaces {
		return line{}, nil, protocolError("a frame has %d faces, over %d", len(l.Faces), MaxFaces)
	}

	faces := make([]pixelFace, 0, len(l.Faces))

	for _, raw := range l.Faces {
		var rf rawFace

		d := json.NewDecoder(bytes.NewReader(raw))
		d.UseNumber()

		if err := d.Decode(&rf); err != nil {
			return line{}, nil, protocolError("a face is not valid")
		}

		var pf pixelFace

		var err error

		if pf.X, err = tenths(rf.X); err == nil {
			if pf.Y, err = tenths(rf.Y); err == nil {
				if pf.W, err = tenths(rf.W); err == nil {
					if pf.H, err = tenths(rf.H); err == nil {
						pf.Score, err = score(rf.Score)
					}
				}
			}
		}

		if err != nil || pf.W <= 0 || pf.H <= 0 {
			return line{}, nil, protocolError("a face has numbers the host refuses")
		}

		faces = append(faces, pf)
	}

	return l, faces, nil
}
