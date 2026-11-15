package otio

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

// ErrInvalid is returned for a file that is not an OTIO timeline this package reads.
var ErrInvalid = errors.New("otio: not a readable OpenTimelineIO timeline")

type entry struct {
	clip     bool
	ref      string
	name     string
	start    int // timeline frame, clips only
	source   int
	duration int
	in, out  int // transition offsets in frames
}

type importTrack struct {
	name    string
	audio   bool
	entries []entry
}

// Import is a parsed OTIO file: what it asks for, not a document. Operations turns
// it into a batch for Apply, so that undo and every rule of the command core hold.
type Import struct {
	Name  string            `json:"name"`
	FPS   timeline.Rational `json:"fps"`   // the frame rate of the file; the caller creates the project with it
	Media []Media           `json:"media"` // every distinct reference, in file order
	Lost  []Lost            `json:"lost"`  // what the batch cannot carry

	tracks []importTrack
}

type reader struct {
	im   *Import
	fps  timeline.Rational
	seen map[string]bool
}

// Parse reads an OTIO file. It never opens a media reference: each target_url is
// only classified (Candidate or Unresolved). Nesting deeper than MaxDepth is an error.
func Parse(data []byte) (*Import, error) {
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalid, err)
	}

	if !schemaIs(root, "Timeline") {
		return nil, fmt.Errorf("%w: the root is not a Timeline", ErrInvalid)
	}

	stack, _ := root["tracks"].(map[string]any)
	if !schemaIs(stack, "Stack") {
		return nil, fmt.Errorf("%w: tracks is not a Stack", ErrInvalid)
	}

	name, _ := root["name"].(string)
	im := &Import{Name: name, Media: []Media{}, Lost: []Lost{}}
	r := &reader{im: im, seen: map[string]bool{}}

	if err := checkDepth(stack, 1); err != nil {
		return nil, err
	}

	r.fps = r.findRate(stack)
	im.FPS = r.fps

	kids, _ := stack["children"].([]any)
	for _, k := range kids {
		t, _ := k.(map[string]any)
		if !schemaIs(t, "Track") {
			r.lose("nested composition or unknown track child")

			continue
		}

		r.track(t)
	}

	return im, nil
}

func schemaIs(m map[string]any, kind string) bool {
	s, _ := m["OTIO_SCHEMA"].(string)

	return strings.HasPrefix(s, kind+".")
}

// checkDepth bounds the nesting of containers.
func checkDepth(m map[string]any, depth int) error {
	if depth > MaxDepth {
		return fmt.Errorf("%w: nesting deeper than %d", ErrInvalid, MaxDepth)
	}

	kids, _ := m["children"].([]any)
	for _, k := range kids {
		if c, ok := k.(map[string]any); ok {
			if err := checkDepth(c, depth+1); err != nil {
				return err
			}
		}
	}

	return nil
}

func (r *reader) lose(feature string) { r.im.Lost = append(r.im.Lost, Lost{Feature: feature}) }

// findRate returns the frame rate of the first time value of the file, 30 fps when
// there is none or it is not a rate the product supports.
func (r *reader) findRate(m map[string]any) timeline.Rational {
	def := timeline.Rational{Num: 30, Den: 1}

	kids, _ := m["children"].([]any)
	for _, k := range kids {
		c, _ := k.(map[string]any)

		if sr, ok := c["source_range"].(map[string]any); ok {
			if st, ok := sr["start_time"].(map[string]any); ok {
				if rate, ok := st["rate"].(float64); ok {
					if f, err := rateToFPS(rate); err == nil {
						return f
					}

					return def
				}
			}
		}

		if len(c) > 0 {
			if f := r.findRate(c); f != def {
				return f
			}
		}
	}

	return def
}

// frames converts an OTIO RationalTime to frames of the project rate.
func (r *reader) frames(v any) (int, bool) {
	m, _ := v.(map[string]any)
	value, ok1 := m["value"].(float64)
	rate, ok2 := m["rate"].(float64)

	if !ok1 || !ok2 || rate <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}

	f := math.Round(value * fpsRate(r.fps) / rate)
	if math.Abs(f) > timeline.MaxFrames {
		return 0, false
	}

	return int(f), true
}

func (r *reader) rangeOf(v any) (start, dur int, ok bool) {
	m, _ := v.(map[string]any)

	s, ok1 := r.frames(m["start_time"])
	d, ok2 := r.frames(m["duration"])

	return s, d, ok1 && ok2
}

func (r *reader) track(t map[string]any) {
	name, _ := t["name"].(string)
	name = clipRunes(name, 60)
	kind, _ := t["kind"].(string)

	tk := importTrack{name: name, audio: kind == kindAud}
	if kind != kindVid && kind != kindAud {
		r.lose("track of kind " + kind)

		return
	}

	cursor := 0
	kids, _ := t["children"].([]any)

	for _, k := range kids {
		c, _ := k.(map[string]any)

		switch {
		case schemaIs(c, "Gap"):
			if _, d, ok := r.rangeOf(c["source_range"]); ok && d > 0 {
				cursor += d
			}
		case schemaIs(c, "Transition"):
			r.transition(&tk, c)
		case schemaIs(c, "Clip"):
			e, ok := r.clip(c, cursor)
			if ok {
				cursor += e.duration
				tk.entries = append(tk.entries, e)
			}
		default:
			r.lose("track child that is not a clip, gap or transition")
		}
	}

	r.im.tracks = append(r.im.tracks, tk)
}

func (r *reader) transition(tk *importTrack, c map[string]any) {
	typ, _ := c["transition_type"].(string)
	in, ok1 := r.frames(c["in_offset"])
	out, ok2 := r.frames(c["out_offset"])

	if typ != dissolve || !ok1 || !ok2 {
		r.lose("transition " + typ)

		return
	}

	tk.entries = append(tk.entries, entry{in: in, out: out})
}

func (r *reader) clip(c map[string]any, cursor int) (entry, bool) {
	name, _ := c["name"].(string)
	name = clipRunes(name, 120)

	start, dur, ok := r.rangeOf(c["source_range"])
	if !ok || dur < 1 || start < 0 {
		r.lose("clip " + name + " without a usable source_range")

		return entry{}, false
	}

	if ms, _ := c["markers"].([]any); len(ms) > 0 {
		r.lose("markers of clip " + name)
	}

	if es, _ := c["effects"].([]any); len(es) > 0 {
		r.lose("effects of clip " + name)
	}

	ref := ""
	if mr, ok := c["media_reference"].(map[string]any); ok && schemaIs(mr, "ExternalReference") {
		ref, _ = mr["target_url"].(string)
	}

	if !r.seen[ref] {
		r.seen[ref] = true
		m := classify(ref)

		if ref == "" && c["media_reference"] != nil {
			m.Reason = "media reference without a target_url"
		}

		r.im.Media = append(r.im.Media, m)
	}

	return entry{clip: true, ref: ref, name: name, start: cursor, source: start, duration: dur}, true
}

// Operations returns the batch that builds what the file describes, bottom track
// first. assets maps a Media.Ref to the id of an asset the caller has imported
// from that candidate; a clip whose reference is not in assets is left out and
// reported. newID supplies the identifiers (SDD-001 §2 item 6). The batch applies
// to a project that has the file's FPS and those assets.
func (im *Import) Operations(assets map[string]string, newID func(prefix string) string) ([]command.Operation, []Lost) {
	b := &batch{assets: assets, newID: newID}

	for i := range im.tracks {
		b.track(&im.tracks[i])
	}

	return append(b.ops, b.pending...), b.lost
}

// placed is a clip the batch has added.
type placed struct {
	id     string
	visual bool
}

type batch struct {
	assets  map[string]string
	newID   func(prefix string) string
	ops     []command.Operation
	pending []command.Operation // transitions, after every clip exists
	lost    []Lost
}

func (b *batch) push(op string, args any) {
	b.ops = append(b.ops, command.Operation{Op: op, Args: mustJSON(args)})
}

func (b *batch) track(tk *importTrack) {
	trackID := b.newID(timeline.PrefixTrack)
	kind := "visual"

	if tk.audio {
		kind = "audio"
	}

	b.push("track.add", map[string]any{"kind": kind, "name": tk.name, "id": trackID, "position": "top"})

	ids := make([]*placed, len(tk.entries))

	for i := range tk.entries {
		if e := &tk.entries[i]; e.clip {
			ids[i] = b.clip(trackID, tk.audio, e)
		}
	}

	for i, e := range tk.entries {
		if e.clip {
			continue
		}

		b.transition(ids, i+1, "in", e.out)
		b.transition(ids, i-1, "out", e.in)
	}
}

// clip adds one clip; it returns nil when the file's reference has no asset.
func (b *batch) clip(trackID string, audio bool, e *entry) *placed {
	asset, ok := b.assets[e.ref]
	if !ok {
		b.lost = append(b.lost, Lost{Name: e.name, Feature: "clip without an imported asset for " + e.ref})

		return nil
	}

	id := b.newID(timeline.PrefixItem)
	op := "clip.add"

	if audio {
		op = "audio.add"
	}

	b.push(op, map[string]any{
		"track_id": trackID, "asset_id": asset, "id": id,
		"start": frameTime(e.start), "source_in": frameTime(e.source), "duration": frameTime(e.duration),
	})

	if e.name != "" {
		b.push("item.rename", map[string]any{"item_id": id, "name": e.name})
	}

	return &placed{id: id, visual: !audio}
}

// transition sets a fade on the clip at idx, when there is one.
func (b *batch) transition(ids []*placed, idx int, edge string, dur int) {
	if dur < 1 || idx < 0 || idx >= len(ids) || ids[idx] == nil {
		return
	}

	if !ids[idx].visual {
		b.lost = append(b.lost, Lost{Feature: "transition on an audio clip"})

		return
	}

	b.pending = append(b.pending, command.Operation{Op: "item.set_transition", Args: mustJSON(map[string]any{
		"item_id": ids[idx].id, "edge": edge, "kind": "fade", "duration": frameTime(dur),
	})})
}

func frameTime(n int) string { return fmt.Sprintf("%df", n) }

func mustJSON(v any) json.RawMessage {
	raw, err := json.Marshal(v)
	if err != nil {
		panic("otio: " + err.Error())
	}

	return raw
}

// clipRunes cuts s to at most n characters, the length limits of the names in
// SDD-001 (track 60, item 120).
func clipRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}

	return s
}
