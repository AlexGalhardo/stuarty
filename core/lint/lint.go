package lint

import (
	"cmp"
	"slices"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// Severities of a finding. An error blocks an export (E_LINT_BLOCKING).
const (
	SeverityError   = "error"
	SeverityWarning = "warning"
)

// Rule codes of SDD-003 §3.6 implemented here.
const (
	CodeEmptyTimeline = "L_EMPTY_TIMELINE"
	CodeGap           = "L_GAP"
	CodeBlackFrames   = "L_BLACK_FRAMES"
	CodeMediaMissing  = "L_MEDIA_MISSING"
	CodeMediaUnready  = "L_MEDIA_UNREADY"
	CodeSourceRange   = "L_SOURCE_RANGE"
	CodeOddDimensions = "L_ODD_DIMENSIONS"
	CodeSilentExport  = "L_SILENT_EXPORT"
	CodeSliver        = "L_SLIVER"
	CodeSlotUnfilled  = "L_SLOT_UNFILLED"
	CodeSafeArea      = "L_SAFE_AREA"

	CodeOccluded     = "L_OCCLUDED"
	CodeAudioOverlap = "L_AUDIO_OVERLAP"

	CodeAudioLimiterNotLast        = "L_AUDIO_LIMITER_NOT_LAST"
	CodeAudioNoiseReductionRepeats = "L_AUDIO_NOISE_REDUCTION_REPEATED"

	CodeTransitionEmpty = "L_TRANSITION_EMPTY"
	CodeDurationLimit   = "L_DURATION_LIMIT"
	CodeOffFrame        = "L_OFF_FRAME"
	CodeCaptionDesync   = "L_CAPTION_DESYNC"
	CodeAudioClip       = "L_AUDIO_CLIP"
	CodeLoudness        = "L_LOUDNESS"
	CodeAlphaMissing    = "L_ALPHA_MISSING"
)

// Finding is one result of a rule (SDD-003 §3.6).
type Finding struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`        // "error" | "warning"
	Item     string `json:"item,omitempty"`  // itm_ id, or trk_/ast_ id when the finding is about those
	Range    *Range `json:"range,omitempty"` // frames; the text adds seconds
	Message  string `json:"message"`
	Hint     string `json:"hint"`
}

// Range is a frame range, the end exclusive.
type Range struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
}

// Blocking is the set of rules whose findings refuse an export, stated once
// in SDD-003 §3.6.
func Blocking(code string) bool {
	switch code {
	case CodeEmptyTimeline, CodeMediaMissing, CodeMediaUnready, CodeSourceRange, CodeOddDimensions, CodeSlotUnfilled:
		return true
	}
	return false
}

// Mode picks the rules to run.
type Mode int

const (
	// Fast runs the rules marked doc or item: pure functions of the document
	// and the Env, run after every edit (SDD-003 §3.6).
	Fast Mode = iota
	// Full also runs the rules that need what only the store knows
	// (L_SOURCE_RANGE): a lint call and the export gate.
	Full
)

// Env is what the document alone cannot tell. The zero value runs every rule
// that needs nothing else; a rule whose input is nil is skipped, never guessed.
type Env struct {
	// Unready reports that the normalized media of an asset is not available
	// (being rebuilt after a cache clean or a profile change): L_MEDIA_UNREADY.
	Unready func(assetID string) bool
	// SafeArea is plan.SafeArea of SDD-004 §4.1, the only place the margins
	// exist, as x, y, width and height in project pixels. With TextSize it
	// enables L_SAFE_AREA.
	SafeArea func(width, height int) (x, y, w, h int)
	// TextSize is the natural size of a text or caption item as the rasterizer
	// lays it out (its widest layout, before scale and rotation), in project
	// pixels. ok is false when the item cannot be measured.
	TextSize func(doc *timeline.Document, item *timeline.Item) (w, h float64, ok bool)
	// Measurer gives the box of an item with its rotation applied; it enables
	// L_OFF_FRAME and replaces TextSize for L_SAFE_AREA (layout.go).
	Measurer Measurer
	// Artifacts gives the rules that read peaks, loudness and transcripts what ingest
	// stored (artifacts.go); without it those rules are silent.
	Artifacts ArtifactReader
	// MediaFrames is the frame count of the normalized media actually in the
	// store, on the asset's own frame rate (Full only): L_SOURCE_RANGE.
	MediaFrames func(assetID string) (frames int, ok bool)
	// AlphaOpaque reports that an asset which declares an alpha channel has only opaque pictures in what the
	// ingest sampled (Full only): L_ALPHA_MISSING. known is false when the fact is not stored.
	AlphaOpaque func(assetID string) (opaque, known bool)
}

// Lint runs the rules over doc and returns the findings in report order:
// errors first, then by code, range start and item id.
func Lint(doc *timeline.Document, env Env, mode Mode) []Finding {
	if dm, ok := env.Measurer.(DocumentMeasurer); ok {
		env.Measurer = dm.ForDocument(doc)
	}

	c := &check{doc: doc, env: env}

	c.emptyTimeline()
	c.dimensions()
	c.media()
	c.slots()
	c.layers()
	c.silent()
	c.slivers()
	c.safeArea()
	c.offFrame()
	c.occluded()
	c.audioOverlap()
	c.processorChains()
	c.transitionEmpty()
	c.durationLimit()

	c.captionDesync(mode == Full)

	if mode == Full {
		c.sourceRange()
		c.audioClip()
		c.loudness()
		c.alphaMissing()
	}

	Sort(c.out)

	return c.out
}

// Sort orders findings: errors first, then code, range start, item id. The
// order is total, so a report never changes between two runs.
func Sort(fs []Finding) {
	slices.SortStableFunc(fs, func(a, b Finding) int {
		return cmp.Or(
			cmp.Compare(severityRank(a.Severity), severityRank(b.Severity)),
			cmp.Compare(a.Code, b.Code),
			cmp.Compare(rangeStart(a), rangeStart(b)),
			cmp.Compare(a.Item, b.Item),
			cmp.Compare(rangeEnd(a), rangeEnd(b)),
		)
	})
}

func severityRank(s string) int {
	if s == SeverityError {
		return 0
	}
	return 1
}

func rangeStart(f Finding) int64 {
	if f.Range == nil {
		return -1
	}
	return f.Range.Start
}

func rangeEnd(f Finding) int64 {
	if f.Range == nil {
		return -1
	}
	return f.Range.End
}

// Totals counts the errors and the warnings.
func Totals(fs []Finding) (errors, warnings int) {
	for i := range fs {
		if fs[i].Severity == SeverityError {
			errors++
		} else {
			warnings++
		}
	}
	return errors, warnings
}

type key struct {
	code, item string
	start      int64
}

func keyOf(f *Finding) key { return key{f.Code, f.Item, rangeStart(*f)} }

// New returns the findings present in after whose key (code, item, range
// start) is absent from before: what a batch introduced (RF-107, SDD-003
// §2.7 rule 2). Pass the findings of the fast rules of both documents.
func New(before, after []Finding) []Finding {
	seen := make(map[key]bool, len(before))
	for i := range before {
		seen[keyOf(&before[i])] = true
	}

	var out []Finding

	for i := range after {
		if !seen[keyOf(&after[i])] {
			out = append(out, after[i])
		}
	}

	Sort(out)

	return out
}
