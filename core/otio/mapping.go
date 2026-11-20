package otio

import (
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// The OTIO schema names this package writes and reads.
const (
	schemaTimeline   = "Timeline.1"
	schemaStack      = "Stack.1"
	schemaTrack      = "Track.1"
	schemaClip       = "Clip.2"
	schemaGap        = "Gap.1"
	schemaTransition = "Transition.1"
	schemaMarker     = "Marker.2"
	schemaExternal   = "ExternalReference.1"
	schemaTimeRange  = "TimeRange.1"
	schemaRational   = "RationalTime.1"

	// metaNamespace is the key of the metadata Vukem writes on tracks and clips (SDD-001 section 9).
	metaNamespace = "vukem"

	dissolve = "SMPTE_Dissolve"
	kindVid  = "Video"
	kindAud  = "Audio"
)

// MaxDepth bounds the nesting of stacks and tracks on import.
const MaxDepth = 64

// Lost is a feature of the document that has no OTIO equivalent (export) or that
// the batch cannot carry (import). It is reported, never dropped silently.
type Lost struct {
	ID      string `json:"id,omitempty"` // an item or track id; empty on import
	Name    string `json:"name,omitempty"`
	Feature string `json:"feature"`
}

// Media statuses of an imported reference.
const (
	Candidate  = "candidate"
	Unresolved = "unresolved"
)

// Media is one media reference of an imported file. A Candidate is a local path the
// caller must still pass through the path rules of MediaImport (SDD-007 §7.2);
// nothing here opens it. An Unresolved reference is never to be opened.
type Media struct {
	Ref    string `json:"ref"`    // the target_url as written
	Status string `json:"status"` // Candidate or Unresolved
	Reason string `json:"reason,omitempty"`
}

var (
	schemePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.\-]+:`)
	drivePattern  = regexp.MustCompile(`^[A-Za-z]:([\\/]|$)`)
)

// classify decides whether a target_url may be offered as a local path. URLs, any
// scheme, UNC and device paths and control characters are refused (T-10, T-11).
func classify(ref string) Media {
	m := Media{Ref: ref, Status: Unresolved}

	switch {
	case ref == "":
		m.Reason = "empty reference"
	case strings.ContainsFunc(ref, func(r rune) bool { return r < 0x20 || r == 0x7f }):
		m.Reason = "control character in the path"
	case strings.HasPrefix(ref, `\\`) || strings.HasPrefix(ref, "//") || strings.HasPrefix(ref, `\??\`):
		m.Reason = "UNC or device path"
	case schemePattern.MatchString(ref) && !drivePattern.MatchString(ref):
		m.Reason = "URL or scheme path"
	default:
		m.Status = Candidate
	}

	return m
}

// fpsRate is the frame rate of a project as a float.
func fpsRate(f timeline.Rational) float64 { return float64(f.Num) / float64(f.Den) }

// rateToFPS recovers a project frame rate from an OTIO rate: an integer, or n/1001.
func rateToFPS(rate float64) (timeline.Rational, error) {
	if math.IsNaN(rate) || math.IsInf(rate, 0) || rate < 1 || rate > 120 {
		return timeline.Rational{}, fmt.Errorf("unsupported frame rate %v", rate)
	}

	if r := math.Round(rate); math.Abs(rate-r) < 1e-6 {
		return timeline.Rational{Num: int(r), Den: 1}, nil
	}

	if n := math.Round(rate * 1001); math.Abs(rate*1001-n) < 1e-3 {
		return timeline.Rational{Num: int(n), Den: 1001}, nil
	}

	return timeline.Rational{}, fmt.Errorf("unsupported frame rate %v", rate)
}
