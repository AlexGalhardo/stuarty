package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/core/timeval"
)

// decodeRow reads the stored document of a project row.
func decodeRow(row *ProjectRow) (*timeline.Document, error) {
	doc, err := timeline.Decode(row.Document)
	if err != nil {
		if ve, ok := errors.AsType[*timeline.ViolationsError](err); ok {
			for _, v := range ve.Violations {
				if v.Code == timeline.VSchemaVersion {
					return nil, newError(command.CodeValidation, "%s", v.Message).
						withHint("update stuarty, then open the project again").
						withDetail("code", timeline.VSchemaVersion)
				}
			}
		}

		// The store holds what Canonical wrote; a document that no longer
		// decodes is a bug or a damaged file, and says nothing useful to a caller.
		return nil, internalError()
	}

	return doc, nil
}

// maxViolationPath is the longest path of a violation an error repeats: a plain key of 64 characters with the members
// and the indexes around it.
const maxViolationPath = 128

var (
	// violationSegment is one member of a generated path: a name, and at most one index as its suffix.
	violationSegment = regexp.MustCompile(`^[A-Za-z0-9_-]+(\[\d{1,6}\])?$`)
	// plainDocKey is a key of a document map that a path may hold. It has no dot and no bracket, so it cannot pose as
	// more members or as an index of the path.
	plainDocKey = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
)

// violationLine is a violation of doc as an error may repeat it: its code and its path. A bundle or a template file
// chooses the keys of its maps and a violation repeats them, so the message is left out, and the path is written only
// when every key of the document is plain and the path has the shape the validator generates (SDD-003 section 1.2
// rule 9). A path alone cannot tell a key "a[0]" from the index 0 of a member "a", which is why the keys are read.
func violationLine(doc *timeline.Document, v timeline.Violation) string {
	if docKeysArePlain(doc) && isViolationPath(v.Path) {
		return v.Code + " at " + v.Path
	}

	return v.Code + " at ?"
}

func isViolationPath(p string) bool {
	if p == "" || len(p) > maxViolationPath {
		return false
	}

	for segment := range strings.SplitSeq(p, ".") {
		if !violationSegment.MatchString(segment) {
			return false
		}
	}

	return true
}

// docKeysArePlain checks every map of the document whose keys the validator writes into a path.
func docKeysArePlain(doc *timeline.Document) bool {
	if !plainKeys(doc.Items) || !plainKeys(doc.Tracks) || !plainKeys(doc.Assets) || !plainKeys(doc.Fonts) {
		return false
	}

	if doc.Template != nil && !plainKeys(doc.Template.Colors) {
		return false
	}

	for id := range doc.Items {
		if !plainKeys(doc.Items[id].Keyframes) {
			return false
		}
	}

	return true
}

func plainKeys[V any](m map[string]V) bool {
	for k := range m {
		if !plainDocKey.MatchString(k) {
			return false
		}
	}

	return true
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// endFrame is the end of the last item, 0 when the project is empty.
func endFrame(doc *timeline.Document) int64 {
	var end int64

	for id := range doc.Items {
		it := doc.Items[id]
		end = max(end, int64(it.Start+it.Duration))
	}

	return end
}

// summarize computes the denormalized columns from the document (SDD-005
// section 5.2): plain traversal, so listing never parses a document.
func summarize(doc *timeline.Document) DocSummary {
	return DocSummary{
		Width: doc.Settings.Width, Height: doc.Settings.Height,
		FPSNum: doc.Settings.FPS.Num, FPSDen: doc.Settings.FPS.Den,
		DurationFrames: endFrame(doc),
		Tracks:         len(doc.Tracks), Items: len(doc.Items), Assets: len(doc.Assets),
	}
}

func fpsString(num, den int) string {
	if num == 0 || den == 0 {
		return "" // a project with no timeline has no frame rate
	}

	if den == 1 {
		return strconv.Itoa(num)
	}

	return fmt.Sprintf("%d/%d", num, den)
}

func timeValue(fps timeline.Rational, frame int64) TimeValue {
	if fps.Num == 0 || fps.Den == 0 {
		return TimeValue{Frame: frame}
	}

	f := timeval.FPS{Num: int64(fps.Num), Den: int64(fps.Den)}
	return TimeValue{Frame: frame, Seconds: round3(f.Seconds(frame))}
}

func round3(x float64) float64 { return math.Round(x*1000) / 1000 }

func msTime(ms int64) time.Time { return time.UnixMilli(ms).UTC() }

// canonicalJSON re-encodes JSON with sorted keys and no whitespace; numbers
// keep their text.
func canonicalJSON(raw []byte) ([]byte, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return []byte("null"), nil
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()

	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}

	return json.Marshal(v) // map keys are sorted by encoding/json
}

// requestHash is the sha256 of the canonical JSON of the operations as
// received: before identifiers are filled in and before analysis is added
// (SDD-005 section 10).
func requestHash(ops []command.Operation) (string, error) {
	canon := make([]json.RawMessage, 0, len(ops))

	for _, op := range ops {
		args, err := canonicalJSON(op.Args)
		if err != nil {
			return "", err
		}

		entry, err := json.Marshal(struct {
			Args json.RawMessage `json:"args"`
			Op   any             `json:"op"`
		}{
			Args: json.RawMessage(args),
			Op:   op.Op,
		})
		if err != nil {
			return "", err
		}

		canon = append(canon, json.RawMessage(entry))
	}

	b, err := json.Marshal(canon)
	if err != nil {
		return "", err
	}

	return sha256Hex(b), nil
}

// idempotencyKeyOK reports a key of 8 to 64 characters from [A-Za-z0-9._~-].
func idempotencyKeyOK(k string) bool {
	if len(k) < 8 || len(k) > 64 {
		return false
	}

	for i := range len(k) {
		c := k[i]
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && !slices.Contains([]byte("._~-"), c) {
			return false
		}
	}

	return true
}

// stateString is the API form of a project state.
func stateString(s ProjectState) string {
	if s == "" {
		return string(ProjectActive)
	}

	return string(s)
}

// productionSummary is the item of the list of projects: the project, with the numbers of its first timeline.
func productionSummary(h *ProductionHead) ProjectSummary {
	t := &h.Timeline
	fps := timeline.Rational{Num: t.Summary.FPSNum, Den: t.Summary.FPSDen}

	return ProjectSummary{
		ID: h.ID, ProjectID: h.ID, Name: h.Name, Revision: t.Revision, State: stateString(h.State),
		Width: t.Summary.Width, Height: t.Summary.Height, FPS: fpsString(fps.Num, fps.Den),
		Duration: timeValue(fps, t.Summary.DurationFrames),
		Tracks:   t.Summary.Tracks, Items: t.Summary.Items, Assets: t.Summary.Assets,
		TimelineID: t.ID, Timelines: h.Timelines, UpdatedAt: msTime(h.UpdatedMs),
		Complete: contextOf(h).Complete,
	}
}

// marshalOrNull encodes a value of the document model, which cannot fail to
// encode; the fallback exists so that no error is ignored.
func marshalOrNull(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("null")
	}

	return b
}
