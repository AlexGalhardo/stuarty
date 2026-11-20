package textview

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/core/timeval"
)

// ProjectOptions select what Project renders.
type ProjectOptions struct {
	Full     bool   // the full view of SDD-003 section 3.3
	Revision int64  // the revision the document is at
	After    string // key of the last row of the previous page; empty for the first page
	Page     int    // 1 for the first page
	Limit    int    // items per page; 0 means all, at most 500
}

// MaxItemLimit is the largest page of items (SDD-003 section 3.10).
const MaxItemLimit = 500

func fpsOf(doc *timeline.Document) timeval.FPS {
	return timeval.FPS{Num: int64(doc.Settings.FPS.Num), Den: int64(doc.Settings.FPS.Den)}
}

// EndFrame is the end of the last item, 0 for an empty project.
func EndFrame(doc *timeline.Document) int64 {
	var end int64

	for id := range doc.Items {
		it := doc.Items[id]
		end = max(end, int64(it.Start+it.Duration))
	}

	return end
}

// row is one line group of the view: a track line or an item with its
// continuation lines. key orders rows and is the cursor.
type row struct {
	key   string
	track string
	item  string // empty for a track row
}

// rowKey orders by track order, then the track line before its items, then
// start, then id.
func rowKey(order string, start int, id string, isItem bool) string {
	if !isItem {
		return order + "|0|0000000000|"
	}

	return fmt.Sprintf("%s|1|%010d|%s", order, start, id)
}

func trackRows(doc *timeline.Document) (tracks []string, byTrack map[string][]string) {
	tracks = slices.SortedFunc(maps.Keys(doc.Tracks), func(a, b string) int {
		return strings.Compare(doc.Tracks[a].Order+a, doc.Tracks[b].Order+b)
	})
	byTrack = map[string][]string{}

	for id := range doc.Items {
		it := doc.Items[id]
		byTrack[it.Track] = append(byTrack[it.Track], id)
	}

	for _, ids := range byTrack {
		slices.SortFunc(ids, func(a, b string) int {
			ia, ib := doc.Items[a], doc.Items[b]
			if ia.Start != ib.Start {
				return ia.Start - ib.Start
			}

			return strings.Compare(a, b)
		})
	}

	return tracks, byTrack
}

// Project renders the concise or the full timeline view (SDD-003 sections 3.2
// and 3.3). It stops before the item that would pass the character cap, and
// returns the key of the last row printed when more follow; the caller turns it
// into the cursor and appends the next_cursor line.
func Project(doc *timeline.Document, o ProjectOptions) (text, next string) {
	fps := fpsOf(doc)
	capChars := CapConcise

	if o.Full {
		capChars = CapFull
	}

	capChars -= cursorReserve

	tracks, byTrack := trackRows(doc)

	var rows []row

	for _, tid := range tracks {
		order := doc.Tracks[tid].Order
		rows = append(rows, row{key: rowKey(order, 0, "", false), track: tid})

		for _, iid := range byTrack[tid] {
			rows = append(rows, row{key: rowKey(order, doc.Items[iid].Start, iid, true), track: tid, item: iid})
		}
	}

	var out strings.Builder

	emit := func(s string) { out.WriteString(s); out.WriteByte('\n') }

	head(doc, o, fps, emit)

	if len(doc.Tracks) == 0 {
		emit("no tracks yet: add one with edit op track.add")
		return strings.TrimRight(out.String(), "\n"), ""
	}

	if o.Page <= 1 {
		emit("Tracks run top to bottom: the first track is drawn over the others.")
	}

	start := 0
	for start < len(rows) && o.After != "" && rows[start].key <= o.After {
		start++
	}

	limit := o.Limit
	if limit <= 0 || limit > MaxItemLimit {
		limit = MaxItemLimit
	}

	used := runeLen(out.String())
	items := 0
	lastKey := ""
	trackShown := ""

	for i := start; i < len(rows); i++ {
		r := rows[i]
		lines := rowLines(doc, fps, r, o.Full, byTrack, trackShown != r.track)

		cost := 0
		for _, l := range lines {
			cost += runeLen(l) + 1
		}

		if r.item != "" && items > 0 && (used+cost > capChars || items >= limit) {
			return strings.TrimRight(out.String(), "\n"), lastKey
		}

		for _, l := range lines {
			emit(l)
		}

		used += cost
		lastKey = r.key
		trackShown = r.track

		if r.item != "" {
			items++
		}
	}

	if o.Page <= 1 {
		for _, l := range assetLines(doc, fps, o.Full) {
			emit(l)
		}
	}

	return strings.TrimRight(out.String(), "\n"), ""
}

func head(doc *timeline.Document, o ProjectOptions, fps timeval.FPS, emit func(string)) {
	emit(Note)

	if o.Page > 1 {
		emit(fmt.Sprintf("project %s rev %d (page %d)", doc.ID, o.Revision, o.Page))
		return
	}

	emit(fmt.Sprintf("project %s %s rev %d", doc.ID, Quote(doc.Name, 60), o.Revision))
	emit(fmt.Sprintf(
		"%dx%d %sfps duration=%s tracks=%d items=%d assets=%d",
		doc.Settings.Width,
		doc.Settings.Height,
		fps,
		Cell(fps, EndFrame(doc)),
		len(doc.Tracks),
		len(doc.Items),
		len(doc.Assets),
	))

	if o.Full {
		emit(
			fmt.Sprintf(
				"settings background=%s loudness_target=%s",
				doc.Settings.Background,
				num(doc.Settings.LoudnessTarget),
			),
		)

		if len(doc.Markers) > 0 {
			emit("markers " + markerList(fps, doc.Markers))
		}

		if len(doc.Fonts) > 0 {
			families := slices.Sorted(maps.Keys(doc.Fonts))
			quoted := make([]string, len(families))

			for i, f := range families {
				quoted[i] = Quote(f, 40)
			}

			emit("fonts " + strings.Join(quoted, " "))
		}
	}
}

// rowLines renders a track row, or an item row with the track line before it
// when the page resumes in that track.
func rowLines(
	doc *timeline.Document,
	fps timeval.FPS,
	r row,
	full bool,
	byTrack map[string][]string,
	trackLine bool,
) []string {
	var lines []string

	if r.item == "" || trackLine {
		lines = append(lines, trackLineOf(doc, r.track, len(byTrack[r.track])))
	}

	if r.item == "" {
		return lines
	}

	it := doc.Items[r.item]
	lines = append(lines, itemLine(fps, &it))

	if full {
		lines = append(lines, continuation(fps, &it)...)
	}

	return lines
}

func trackLineOf(doc *timeline.Document, id string, items int) string {
	t := doc.Tracks[id]
	order := slices.SortedFunc(maps.Keys(doc.Tracks), func(a, b string) int {
		return strings.Compare(doc.Tracks[a].Order+a, doc.Tracks[b].Order+b)
	})

	pos := "mid"

	switch {
	case order[0] == id:
		pos = "top"
	case order[len(order)-1] == id:
		pos = "bottom"
	}

	s := fmt.Sprintf("%s %s %s %s items=%d", id, pos, t.Kind, Quote(t.Name, 40), items)
	if t.Hidden {
		s += " hidden"
	}

	if t.Muted {
		s += " muted"
	}

	return s
}

func num(f float64) string {
	b := marshalOrNull(f)
	return string(b)
}

func itemLine(fps timeval.FPS, it *timeline.Item) string {
	s := fmt.Sprintf("%s %s %s %s", it.ID, it.Type, Cell(fps, int64(it.Start)), Dur(fps, int64(it.Duration)))

	if d := itemDetail(it); d != "" {
		s += " " + d
	}

	if f := itemFlags(it); len(f) > 0 {
		s += " [" + strings.Join(f, " ") + "]"
	}

	return s
}

func itemDetail(it *timeline.Item) string {
	switch it.Type {
	case timeline.ItemClip, timeline.ItemAudio:
		d := it.Asset
		if len(it.SpeedCurve) > 0 {
			speeds := make([]string, len(it.SpeedCurve))
			for i, p := range it.SpeedCurve {
				speeds[i] = num(float64(p.Speed.Num) / float64(p.Speed.Den))
			}

			d += " curve:" + strings.Join(speeds, ">")
		} else if it.Speed.Num != it.Speed.Den && it.Speed.Den > 0 {
			d += " x" + num(float64(it.Speed.Num)/float64(it.Speed.Den))
		}

		return d
	case timeline.ItemText:
		return Quote(it.Text, 10)
	case timeline.ItemCaption:
		return fmt.Sprintf("%dw", len(it.Words))
	case timeline.ItemShape:
		return fmt.Sprintf("%s %sx%s", it.Shape, num(it.Width), num(it.Height))
	case timeline.ItemComposition:
		return it.Timeline
	}

	return ""
}

// itemFlags are in the order of SDD-003 section 3.2.
func itemFlags(it *timeline.Item) []string {
	var f []string

	if it.TransitionIn != nil {
		f = append(f, "tin:"+it.TransitionIn.Kind)
	}

	if it.TransitionOut != nil {
		f = append(f, "tout:"+it.TransitionOut.Kind)
	}

	if n := len(it.Keyframes); n > 0 {
		f = append(f, fmt.Sprintf("kf:%d", n))
	}

	if n := len(it.Markers); n > 0 {
		f = append(f, fmt.Sprintf("mk:%d", n))
	}

	if it.Volume != 1 && (it.Type == timeline.ItemClip || it.Type == timeline.ItemAudio) {
		f = append(f, "vol:"+num(it.Volume))
	}

	if it.AudioMuted {
		f = append(f, "muted")
	}

	if it.Denoise != 0 && (it.Type == timeline.ItemClip || it.Type == timeline.ItemAudio) {
		f = append(f, "nr:"+num(it.Denoise))
	}

	if it.Voice != "" && (it.Type == timeline.ItemClip || it.Type == timeline.ItemAudio) {
		f = append(f, "voice:"+it.Voice)
	}

	if it.Reverse {
		f = append(f, "rev")
	}

	if it.Type == timeline.ItemClip {
		f = append(f, colourFlags(it)...)
	}

	if it.Link != "" {
		f = append(f, "linked")
	}

	if n := len(it.Effects); n > 0 {
		f = append(f, fmt.Sprintf("fx:%d", n))
	}

	if it.Disabled {
		f = append(f, "disabled")
	}

	if it.Slot != nil {
		f = append(f, "slot:"+it.Slot.Name)

		if it.Slot.Required && !it.Slot.Filled {
			f = append(f, "unfilled")
		}
	}

	return f
}

// colourFlags lists the colour work of a clip, so a reader of the summary sees a graded clip (VV-150):
// grade:<adjustments that are not neutral>, hsl:<bands>, wheels, curves and lut:<asset>[@intensity].
func colourFlags(it *timeline.Item) []string {
	var f []string

	var grade []string

	add := func(name string, v, neutral float64) {
		if v != neutral {
			grade = append(grade, name+num(v))
		}
	}

	add("bri", it.Brightness, 0)
	add("con", it.Contrast, 1)
	add("sat", it.Saturation, 1)
	add("temp", it.Temperature, 0)
	add("tint", it.Tint, 0)
	add("hue", it.Hue, 0)

	if len(grade) > 0 {
		f = append(f, "grade:"+strings.Join(grade, ","))
	}

	if n := len(it.Hsl); n > 0 {
		f = append(f, fmt.Sprintf("hsl:%d", n))
	}

	if it.Wheels != nil {
		f = append(f, "wheels")
	}

	if it.Curves != nil {
		f = append(f, "curves")
	}

	if it.Lut != nil {
		l := "lut:" + it.Lut.Asset
		if it.Lut.Intensity != 1 {
			l += "@" + num(it.Lut.Intensity)
		}

		f = append(f, l)
	}

	return f
}

// assetDuration is the duration of an asset in project frames (SDD-001 section 3.2).
func assetDuration(doc *timeline.Document, a *timeline.Asset) int64 {
	fps := doc.Settings.FPS

	switch a.Kind {
	case timeline.AssetVideo:
		if a.FrameRate.Num == 0 || fps.Den == 0 {
			return 0
		}

		return int64(a.FrameCount) * int64(fps.Num) * int64(a.FrameRate.Den) / (int64(fps.Den) * int64(a.FrameRate.Num))
	case timeline.AssetAudio:
		return int64(a.SampleCount) * int64(fps.Num) / (48000 * int64(fps.Den))
	default:
		return 0
	}
}

func assetLines(doc *timeline.Document, fps timeval.FPS, full bool) []string {
	if len(doc.Assets) == 0 {
		return nil
	}

	lines := []string{"assets"}

	for _, id := range slices.Sorted(maps.Keys(doc.Assets)) {
		a := doc.Assets[id]
		s := id

		if full {
			s += fmt.Sprintf(" %s %s", a.Kind, Quote(a.Name, 30))
		} else {
			s += " " + string(a.Kind)
		}

		switch a.Kind {
		case timeline.AssetVideo:
			s += fmt.Sprintf(" %s %dx%d", Cell(fps, assetDuration(doc, &a)), a.Width, a.Height)
			if a.HasAudio {
				s += " audio"
			}
		case timeline.AssetAudio:
			s += " " + Cell(fps, assetDuration(doc, &a))
		case timeline.AssetImage:
			s += fmt.Sprintf(" %dx%d", a.Width, a.Height)
		case timeline.AssetLUT:
			// A lookup table has no picture facts.
		}

		lines = append(lines, s)
	}

	return lines
}

const wrapAt = 160

// continuation lists the fields of an item that are not in its line and not at
// their default, as key=value (SDD-003 section 3.3): end first, a caption's
// words next, then the rest sorted by key; wrapped at 160 characters.
func continuation(fps timeval.FPS, it *timeline.Item) []string {
	end := "end=" + Cell(fps, int64(it.Start+it.Duration))
	parts := []string{end}

	if it.Type == timeline.ItemCaption {
		words := make([]string, len(it.Words))
		for i, w := range it.Words {
			words[i] = fmt.Sprintf("%d+%d %s", w.Start, w.Duration, Quote(w.Text, 20))
		}

		parts = append(parts, "words=["+strings.Join(words, ", ")+"]")
	}

	fields := extraFields(fps, it)
	for _, k := range slices.Sorted(maps.Keys(fields)) {
		parts = append(parts, k+"="+fields[k])
	}

	return wrap(parts)
}

// markerList is every marker frame, relative to the item start: an agent cuts on the beat
// from this list, so none is dropped.
func markerList(fps timeval.FPS, markers []timeline.Marker) string {
	marks := make([]string, len(markers))
	for i, m := range markers {
		marks[i] = Cell(fps, int64(m.Frame))
		if m.Label != "" {
			marks[i] += " " + Quote(m.Label, 60)
		}
	}

	return strings.Join(marks, ", ")
}

// addJSON sets f[key] to the JSON of v when present is true.
func addJSON(f map[string]string, key string, present bool, v any) {
	if present {
		f[key] = string(marshalOrNull(v))
	}
}

func setIf(f map[string]string, k string, v, def float64) {
	if v != def {
		f[k] = num(v)
	}
}

// visualFields adds the fields of a visual item that are not the defaults.
func visualFields(f map[string]string, it *timeline.Item) {
	setIf(f, "scale", it.Scale, 1)
	setIf(f, "rotation", it.Rotation, 0)
	setIf(f, "opacity", it.Opacity, 1)

	if it.Blend != "" {
		f["blend"] = it.Blend
	}

	if it.FlipX {
		f["flip_x"] = "true"
	}

	if it.FlipY {
		f["flip_y"] = "true"
	}

	addJSON(f, "mask", it.Mask != nil, it.Mask)

	if it.X != nil {
		f["x"] = num(*it.X)
	}

	if it.Y != nil {
		f["y"] = num(*it.Y)
	}
}

func extraFields(fps timeval.FPS, it *timeline.Item) map[string]string {
	f := map[string]string{}
	frames := func(v int) string { return Cell(fps, int64(v)) }
	jsonOf := func(v any) string { return string(marshalOrNull(v)) }

	if it.Name != "" {
		f["name"] = Quote(it.Name, 80)
	}

	if it.SourceIn != 0 && it.Asset != "" {
		f["source_in"] = frames(it.SourceIn)
	}

	if it.Type == timeline.ItemClip && it.Fit != "cover" {
		f["fit"] = it.Fit
	}

	if it.Type != timeline.ItemAudio {
		visualFields(f, it)
	}

	if it.Type == timeline.ItemClip {
		setIf(f, "brightness", it.Brightness, 0)
		setIf(f, "contrast", it.Contrast, 1)
		setIf(f, "saturation", it.Saturation, 1)
		setIf(f, "temperature", it.Temperature, 0)
		setIf(f, "tint", it.Tint, 0)
		setIf(f, "hue", it.Hue, 0)

		addJSON(f, "crop", it.Crop != nil, it.Crop)
		addJSON(f, "chroma_key", it.ChromaKey != nil, it.ChromaKey)
	}

	if it.FadeIn != 0 {
		f["fade_in"] = frames(it.FadeIn)
	}

	if it.FadeOut != 0 {
		f["fade_out"] = frames(it.FadeOut)
	}

	if it.TransitionIn != nil {
		f["transition_in"] = fmt.Sprintf("%s %s", it.TransitionIn.Kind, frames(it.TransitionIn.Duration))
	}

	if it.TransitionOut != nil {
		f["transition_out"] = fmt.Sprintf("%s %s", it.TransitionOut.Kind, frames(it.TransitionOut.Duration))
	}

	if len(it.Keyframes) > 0 {
		f["keyframes"] = jsonOf(it.Keyframes)
	}

	if len(it.Markers) > 0 {
		f["markers"] = markerList(fps, it.Markers)
	}

	if it.Type == timeline.ItemText && it.Text != "" {
		f["text"] = Quote(it.Text, 80)
	}

	if it.Type == timeline.ItemCaption && it.LinkedItem != "" {
		f["linked"] = it.LinkedItem
	}

	if it.Style != nil {
		f["style"] = jsonOf(it.Style)
	}

	if it.Type == timeline.ItemShape {
		f["fill"] = string(it.Fill)

		if it.Radius != 0 {
			f["radius"] = num(it.Radius)
		}
	}

	return f
}

// wrap joins parts with spaces on continuation lines of at most wrapAt characters.
func wrap(parts []string) []string {
	var lines []string

	cur := " "

	for _, p := range parts {
		if len(cur) > 1 && runeLen(cur)+1+runeLen(p) > wrapAt {
			lines = append(lines, cur)
			cur = " "
		}

		cur += " " + p
	}

	return append(lines, cur)
}
