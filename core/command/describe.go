package command

import (
	"encoding/json"
	"fmt"
	"math/big"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/core/timeval"
)

const (
	maxLine      = 300
	maxValue     = 40
	maxName      = 40
	maxTextStart = 14
)

// frameFields are the top-level fields whose value is a number of frames; an
// updated line writes them as time cells.
var frameFields = map[string]bool{"start": true, "duration": true, "source_in": true, "fade_in": true, "fade_out": true}

// DescribeDiff renders diff as one line per change, in the order given
// (SDD-003 §3.4). doc is the document after the change: it gives the frame rate
// and the track positions. An empty diff is the single line "no changes".
func DescribeDiff(doc *timeline.Document, diff []Change) string {
	if len(diff) == 0 {
		return "no changes"
	}
	d := describer{
		fps:       timeval.FPS{Num: int64(doc.Settings.FPS.Num), Den: int64(doc.Settings.FPS.Den)},
		fpsRat:    doc.Settings.FPS,
		positions: trackPositions(doc),
	}
	if !d.fps.Valid() {
		d.fps = timeval.FPS{Num: 30, Den: 1}
		d.fpsRat = timeline.Rational{Num: 30, Den: 1}
	}
	lines := make([]string, 0, len(diff))
	for _, ch := range diff {
		lines = append(lines, cutLine(d.line(ch)))
	}
	return strings.Join(lines, "\n")
}

type describer struct {
	fps       timeval.FPS
	fpsRat    timeline.Rational
	positions map[string]string
}

// trackPositions maps each track to top, mid or bottom, by its order.
func trackPositions(doc *timeline.Document) map[string]string {
	ids := make([]string, 0, len(doc.Tracks))
	for id := range doc.Tracks {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, func(a, b string) int { return strings.Compare(doc.Tracks[a].Order, doc.Tracks[b].Order) })
	out := map[string]string{}
	for i, id := range ids {
		switch {
		case i == 0:
			out[id] = "top"
		case i == len(ids)-1:
			out[id] = "bottom"
		default:
			out[id] = "mid"
		}
	}
	return out
}

func (d describer) line(ch Change) string {
	o := ch.Object
	switch ch.Kind {
	case KindSettingsUpdated:
		return "~ settings: " + d.fieldChanges(ch.Changes)
	case KindProjectUpdated:
		return "~ project: " + d.fieldChanges(ch.Changes)
	case KindAssetAdded:
		return "+ asset " + ch.ID + " " + d.assetLine(o)
	case KindAssetRemoved:
		return "- asset " + ch.ID + " " + compact(str(o["name"]), maxName)
	case KindTrackAdded:
		pos := d.positions[ch.ID]
		if pos == "" {
			pos = "top"
		}
		return fmt.Sprintf("+ track %s %s %s %s", ch.ID, str(o["kind"]), compact(str(o["name"]), maxName), pos)
	case KindTrackUpdated:
		return "~ track " + ch.ID + ": " + d.fieldChanges(ch.Changes)
	case KindTrackRemoved:
		return "- track " + ch.ID + " " + compact(str(o["name"]), maxName)
	case KindItemAdded:
		return fmt.Sprintf("+ item %s %s on %s %s %s %s", ch.ID, str(o["type"]), str(o["track"]),
			d.cell(o["start"], false), d.cell(o["duration"], true), d.itemDetail(o))
	case KindItemUpdated:
		return "~ item " + ch.ID + ": " + d.fieldChanges(ch.Changes)
	case KindItemRemoved:
		return fmt.Sprintf("- item %s %s was %s %s on %s %s", ch.ID, str(o["type"]),
			d.cell(o["start"], false), d.cell(o["duration"], true), str(o["track"]), d.itemDetail(o))
	case KindFontAdded:
		return "+ font " + compact(ch.ID, maxName)
	case KindFontRemoved:
		return "- font " + compact(ch.ID, maxName)
	}
	return "? " + ch.Kind + " " + ch.ID
}

func cutLine(s string) string {
	if utf8.RuneCountInString(s) <= maxLine {
		return s
	}
	return string([]rune(s)[:maxLine-1]) + "…"
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func frames(v any) (int64, bool) {
	n, ok := v.(json.Number)
	if !ok {
		return 0, false
	}
	i, err := strconv.ParseInt(string(n), 10, 64)
	return i, err == nil
}

// cell writes a number of frames as <seconds>s/<frames>f; a length gets a leading +
// when plus is true.
func (d describer) cell(v any, plus bool) string {
	f, ok := frames(v)
	if !ok {
		return "none"
	}
	c := d.fps.FormatSeconds(f) + "/" + strconv.FormatInt(f, 10) + "f"
	if plus {
		return "+" + c
	}
	return c
}

// fieldChanges renders "<field> <before> -> <after>; ..." with the fields sorted.
func (d describer) fieldChanges(changes map[string]Pair) string {
	keys := make([]string, 0, len(changes))
	for k := range changes {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		p := changes[k]
		parts = append(parts, fmt.Sprintf("%s %s -> %s", k, d.value(k, p[0]), d.value(k, p[1])))
	}
	return strings.Join(parts, "; ")
}

func (d describer) value(field string, v any) string {
	var s string
	switch x := v.(type) {
	case nil:
		return "none"
	case string:
		return compact(x, maxValue)
	case bool:
		return strconv.FormatBool(x)
	case json.Number:
		if _, whole := frames(x); frameFields[field] && whole {
			s = d.cell(x, false)
		} else {
			s = string(x)
		}
	case []any:
		if field == "markers" {
			return markersValue(x)
		}
		b, err := json.Marshal(x)
		if err != nil {
			return "none"
		}
		s = string(b)
	default:
		b, err := json.Marshal(x)
		if err != nil {
			return "none"
		}
		s = string(b)
	}
	if utf8.RuneCountInString(s) > maxValue {
		s = string([]rune(s)[:maxValue-1]) + "…"
	}
	return s
}

// markersValue is a marker list as its count and the first frames, so a long list is
// not cut in the middle of an object (VV-014); the whole list is in project_get with detail full.
func markersValue(list []any) string {
	const shown = 6
	frames := make([]string, 0, shown)
	for _, m := range list[:min(len(list), shown)] {
		obj, ok := m.(map[string]any)
		if !ok {
			return fmt.Sprintf("%d markers", len(list))
		}
		frames = append(frames, numStr(obj["frame"])+"f")
	}
	s := fmt.Sprintf("%d markers at %s", len(list), strings.Join(frames, " "))
	if len(list) > shown {
		s += " …"
	}
	return s
}

// compact is the compact form of an untrusted string (SDD-003 §3.1): quoted, with
// control and bidirectional characters removed, cut at budget characters with an
// ellipsis, and \, ", < and & escaped.
func compact(s string, budget int) string {
	var clean []rune
	for _, r := range s {
		if unicode.IsControl(r) || isBidi(r) {
			continue
		}
		clean = append(clean, r)
	}
	cut := false
	if len(clean) > budget {
		clean, cut = clean[:budget], true
	}
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range clean {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '<':
			b.WriteString("&lt;")
		case '&':
			b.WriteString("&amp;")
		default:
			b.WriteRune(r)
		}
	}
	if cut {
		b.WriteString("…")
	}
	b.WriteByte('"')
	return b.String()
}

func isBidi(r rune) bool {
	return r >= 0x202A && r <= 0x202E || r >= 0x2066 && r <= 0x2069 || r == 0x200E || r == 0x200F || r == 0x061C
}

func (d describer) assetLine(o map[string]any) string {
	kind := str(o["kind"])
	name := compact(str(o["name"]), maxName)
	size := numStr(o["width"]) + "x" + numStr(o["height"])
	switch kind {
	case "video":
		line := fmt.Sprintf("video %s %s %s", name, d.assetDuration(o), size)
		if b, _ := o["has_audio"].(bool); b {
			line += " audio"
		}
		return line
	case "audio":
		return fmt.Sprintf("audio %s %s", name, d.assetDuration(o))
	default:
		return fmt.Sprintf("%s %s %s", kind, name, size)
	}
}

func numStr(v any) string {
	if n, ok := v.(json.Number); ok {
		return string(n)
	}
	return "0"
}

func intOf(v any) int64 {
	i, _ := frames(v)
	return i
}

// assetDuration is the asset length in project frames (SDD-001 §3.2) as a time cell
// without the plus.
func (d describer) assetDuration(o map[string]any) string {
	fps := d.fpsRat
	var n, den *big.Int
	if str(o["kind"]) == "video" {
		fr := objectOf(o["frame_rate"])
		if intOf(fr["num"]) < 1 || intOf(fr["den"]) < 1 {
			return "none"
		}
		n = new(big.Int).Mul(big.NewInt(intOf(o["frame_count"])), big.NewInt(int64(fps.Num)))
		n.Mul(n, big.NewInt(intOf(fr["den"])))
		den = new(big.Int).Mul(big.NewInt(int64(fps.Den)), big.NewInt(intOf(fr["num"])))
	} else {
		n = new(big.Int).Mul(big.NewInt(intOf(o["sample_count"])), big.NewInt(int64(fps.Num)))
		den = new(big.Int).Mul(big.NewInt(48000), big.NewInt(int64(fps.Den)))
	}
	f := new(big.Int).Div(n, den)
	return d.cell(json.Number(f.String()), false)
}

// itemDetail is the concise detail of SDD-003 §3.2 plus what a person needs to check
// the change (SDD-003 §3.4).
func (d describer) itemDetail(o map[string]any) string {
	switch str(o["type"]) {
	case "clip", "audio":
		parts := []string{str(o["asset"])}
		if src := intOf(o["source_in"]); src != 0 {
			parts = append(parts, "srcin="+d.cell(o["source_in"], false))
		}
		if sp := objectOf(o["speed"]); sp != nil {
			n, den := intOf(sp["num"]), intOf(sp["den"])
			if den > 0 && n != den {
				x := strconv.FormatFloat(float64(n)/float64(den), 'f', 3, 64)
				parts = append(parts, "x"+strings.TrimRight(strings.TrimRight(x, "0"), "."))
			}
		}
		if fit := str(o["fit"]); fit != "" && fit != "cover" {
			parts = append(parts, "fit="+fit)
		}
		return strings.Join(parts, " ")
	case "text":
		return firstChars(str(o["text"]))
	case "caption":
		words, _ := o["words"].([]any)
		texts := make([]string, 0, len(words))
		for _, w := range words {
			texts = append(texts, str(objectOf(w)["text"]))
		}
		return fmt.Sprintf("%dw %s", len(words), firstChars(strings.Join(texts, " ")))
	case "shape":
		return fmt.Sprintf("%s %sx%s", str(o["shape"]), numStr(o["width"]), numStr(o["height"]))
	}
	return ""
}

func firstChars(s string) string {
	return compact(s, maxTextStart)
}
