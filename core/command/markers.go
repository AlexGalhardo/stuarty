package command

import (
	"cmp"
	"fmt"
	"math"
	"slices"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func init() {
	Register(Define(Spec{
		Name:    "audio.mark_beats",
		Summary: "Writes markers on a music item at the beats of its audio.",
		Description: "Places a marker at each beat of the visible part of the item, or at every nth beat, so that " +
			"cuts and transitions can be aligned to the music. Beats are frames of the asset and come from the beat " +
			"grid of the asset, supplied by the service in resolved; the command never starts a job. Markers have " +
			"no effect on the picture or the sound. When the analysis finds no steady pulse, give bpm (and offset, the " +
			"time of the first beat on the asset's own time axis, default 0) and the beats are those of that tempo; " +
			"the grid of the asset is then not used, for example {\"item_id\": \"itm_s6t7v8w9x0\", \"bpm\": 120, \"offset\": \"0.25s\"}.",
		Example: `{"item_id": "itm_s6t7v8w9x0", "every": 2, "label": "beat"}`,
		Errors: []string{
			CodeValidation, CodeNotFound, CodeTypeMismatch, CodeInvalidState, CodeLimit,
		},
		Scope: ScopeEdit,
	}, applyMarkBeats))
}

// maxBeats bounds the beat grid a caller may supply: one beat per frame of the
// longest timeline.
const maxBeats = timeline.MaxFrames

// ResolvedBeats is the analysis result the service puts in `resolved` (A.9).
type ResolvedBeats struct {
	Beats []int `json:"beats" desc:"Strictly increasing beat positions in asset frames, from 0."`
}

type markBeatsArgs struct {
	ItemID   string         `json:"item_id"            desc:"An audio item of an audio asset, or of a video asset with audio."                                                         pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Every    int            `json:"every,omitempty"    desc:"Mark every nth beat of the visible part, from the first; default 1."                                                                                                             min:"1"  max:"64"`
	Label    string         `json:"label,omitempty"    desc:"Label of every marker, up to 60 characters; default none."                                                                                                                                max:"60"`
	BPM      *float64       `json:"bpm,omitempty"      desc:"Manual tempo in beats per minute, 20 to 400, for music whose pulse the analysis cannot find; replaces the detected grid."                                                        min:"20" max:"400"`
	Offset   Time           `json:"offset,omitempty"   desc:"With bpm: the time of the first beat on the asset's time axis, from its start; default \"0s\"."`
	Replace  *bool          `json:"replace,omitempty"  desc:"true (default) replaces the item's markers; false keeps them and adds the beats."`
	Resolved *ResolvedBeats `json:"resolved,omitempty" desc:"Filled by the service from the beat grid of the asset; never sent by a caller."                                                                                                                     internal:"true"`
}

func checkBeats(r *ResolvedBeats) *Error {
	if r == nil {
		return Errorf(CodeValidation, "The beat grid of the asset is not known.").
			WithHint("Wait until the asset is analysed (media_inspect shows its state), then retry.").
			WithPath("args.item_id")
	}
	if len(r.Beats) > maxBeats {
		return Errorf(CodeValidation, "args.resolved.beats has %d beats; the limit is %d.", len(r.Beats), maxBeats).
			WithHint("Send the beats of one asset as a sorted list.").WithPath("args.resolved.beats")
	}
	prev := -1
	for i, b := range r.Beats {
		if b < 0 || b <= prev {
			path := fmt.Sprintf("args.resolved.beats[%d]", i)
			return Errorf(CodeValidation, "%s is %d, not a frame after %d.", path, b, prev).
				WithHint("Send beats as strictly increasing frames from 0.").WithPath(path)
		}
		prev = b
	}
	return nil
}

// audioSource checks that an item is an audio item whose asset has audio, the
// kind of item that carries music or speech for the analysis commands.
func (c *Context) audioSource(it *timeline.Item, what string) *Error {
	if it.Type != timeline.ItemAudio {
		return Errorf(CodeTypeMismatch, "Item %s is %s item, not an audio item; %s", it.ID, article(it.Type), what).
			WithHint("Choose an audio item.").WithPath("args.item_id")
	}
	if asset, ok := c.Doc.Assets[it.Asset]; !ok || !assetHasAudio(asset) {
		return Errorf(CodeTypeMismatch, "Item %s has no asset with audio; %s", it.ID, what).
			WithHint("Choose an audio item placed from an asset with audio.").WithPath("args.item_id")
	}
	return nil
}

// stillImageNoAudio refuses an audio setting on a clip of an image: an image has no sound.
func (c *Context) stillImageNoAudio(it *timeline.Item, what string) *Error {
	if it.Type != timeline.ItemClip {
		return nil
	}
	if asset, ok := c.Doc.Assets[it.Asset]; ok && asset.Kind == timeline.AssetImage {
		return Errorf(CodeTypeMismatch, "Item %s shows an image, which %s.", it.ID, what).
			WithHint("Audio settings apply to clips of a video with audio, and to audio items.").
			WithPath("args.item_id")
	}
	return nil
}

func assetHasAudio(a timeline.Asset) bool {
	return a.Kind == timeline.AssetAudio || (a.Kind == timeline.AssetVideo && a.HasAudio)
}

// beatGrid returns the beats, in asset frames, that the command marks: the manual grid of
// bpm and offset when given (VV-153), otherwise the grid the service resolved.
func (c *Context) beatGrid(a *markBeatsArgs, it *timeline.Item) ([]int, *Error) {
	if a.BPM == nil {
		if a.Offset != "" {
			return nil, Errorf(CodeValidation, "args.offset needs args.bpm.").
				WithHint("Send bpm with offset, or omit offset.").WithPath("args.offset")
		}
		if err := checkBeats(a.Resolved); err != nil {
			return nil, err
		}
		return a.Resolved.Beats, nil
	}
	bpm := *a.BPM
	if !(bpm >= 20 && bpm <= 400) {
		return nil, Errorf(CodeValidation, "args.bpm is %v, not between 20 and 400.", bpm).
			WithHint("Send the tempo in beats per minute, like 120.").WithPath("args.bpm")
	}
	offset, err := c.length("args.offset", cmp.Or(string(a.Offset), "0f"), true)
	if err != nil {
		return nil, err
	}
	fps := c.FPS()
	srcEnd := it.SourceIn + ceilDiv(it.Duration*it.Speed.Num, it.Speed.Den)
	// A beat is at offset + k*60/bpm seconds; the tempo is taken to the milli-bpm so the
	// rounding is integer arithmetic.
	milli := int64(math.Round(bpm * 1000))
	var beats []int
	for k := int64(0); len(beats) < maxBeats; k++ {
		num := k * 60000 * fps.Num
		den := fps.Den * milli
		f := offset + int((2*num+den)/(2*den))
		if f >= srcEnd {
			break
		}
		if len(beats) == 0 || f > beats[len(beats)-1] {
			beats = append(beats, f)
		}
	}
	return beats, nil
}

func applyMarkBeats(c *Context, a *markBeatsArgs) error {
	if err := idForm("args.item_id", timeline.PrefixItem, a.ItemID); err != nil {
		return err
	}
	it, err := c.item(a.ItemID)
	if err != nil {
		return err
	}
	if err := c.audioSource(&it, "beats are marked on music."); err != nil {
		return err
	}
	if err := rejectRemap(&it, "beat markers"); err != nil {
		return err
	}
	grid, err := c.beatGrid(a, &it)
	if err != nil {
		return err
	}
	if len(grid) == 0 {
		return Errorf(CodeInvalidState, "The asset of item %s has no beats: its beat grid is empty.", a.ItemID).
			WithHint("Beats exist only in music with a steady rhythm; a sound effect or speech has none. " +
				"Choose another item, or place cuts by time.").
			WithPath("args.item_id")
	}
	every := max(a.Every, 1)
	sp := it.Speed

	var frames []int
	for _, b := range grid {
		if b < it.SourceIn {
			continue
		}
		f := ceilDiv((b-it.SourceIn)*sp.Den, sp.Num)
		if f >= it.Duration {
			break
		}
		if len(frames) == 0 || f > frames[len(frames)-1] {
			frames = append(frames, f)
		}
	}
	if len(frames) == 0 {
		return Errorf(CodeInvalidState, "The beat grid has no beat inside the visible part of item %s.", a.ItemID).
			WithHint("Check that the item holds music, or widen it with item.trim.")
	}

	marks := make([]timeline.Marker, 0, len(frames)/every+1)
	for i := 0; i < len(frames); i += every {
		marks = append(marks, timeline.Marker{Frame: frames[i], Label: a.Label})
	}
	added := len(marks)
	if a.Replace != nil && !*a.Replace {
		marks = mergeMarkers(it.Markers, marks)
	}
	if len(marks) > timeline.MaxMarkers {
		return Errorf(
			CodeLimit,
			"Item %s would have %d markers; the limit is %d.",
			a.ItemID,
			len(marks),
			timeline.MaxMarkers,
		).
			WithHint("Use a larger every, or trim the item first.")
	}
	it.Markers = marks
	c.Doc.Items[a.ItemID] = it
	c.SetEffective("beats", added)
	c.SetEffective("markers", len(marks))
	return nil
}

// mergeMarkers returns the markers of both lists in frame order; at a frame that
// both have, the existing marker stays.
func mergeMarkers(existing, added []timeline.Marker) []timeline.Marker {
	out := slices.Clone(existing)
	for _, m := range added {
		if !slices.ContainsFunc(existing, func(e timeline.Marker) bool { return e.Frame == m.Frame }) {
			out = append(out, m)
		}
	}
	slices.SortFunc(out, func(x, y timeline.Marker) int { return x.Frame - y.Frame })
	return out
}

// Marker rules M1 to M4 of SDD-002 (Part B, markers): they follow the item when
// it is trimmed, split, sped up or retimed, as keyframes do.

// mShift is M1: the item loses its first d frames (d < 0: it gains -d). Markers
// that fall before the new start or past the new end are dropped.
func mShift(it *timeline.Item, d int) {
	if len(it.Markers) == 0 {
		return
	}
	out := make([]timeline.Marker, 0, len(it.Markers))
	for _, m := range it.Markers {
		if m.Frame -= d; m.Frame >= 0 && m.Frame < it.Duration {
			out = append(out, m)
		}
	}
	it.Markers = nilIfEmpty(out)
}

// mCut is M2: the item is cut to l frames at the back.
func mCut(it *timeline.Item, l int) {
	if len(it.Markers) == 0 {
		return
	}
	out := make([]timeline.Marker, 0, len(it.Markers))
	for _, m := range it.Markers {
		if m.Frame < l {
			out = append(out, m)
		}
	}
	it.Markers = nilIfEmpty(out)
}

// mSplit is M3: markers before tc stay on the left part, the others go to the
// right part, counted from its start.
func mSplit(left, right *timeline.Item, tc int) {
	var l, r []timeline.Marker
	for _, m := range left.Markers {
		if m.Frame < tc {
			l = append(l, m)
			continue
		}
		m.Frame -= tc
		r = append(r, m)
	}
	left.Markers, right.Markers = nilIfEmpty(l), nilIfEmpty(r)
}

// mRescale is M4: marker frames are scaled by of and rounded; a marker past the
// end of the item is dropped, and of two on the same frame the first stays.
func mRescale(it *timeline.Item, of func(int) int) {
	if len(it.Markers) == 0 {
		return
	}
	out := make([]timeline.Marker, 0, len(it.Markers))
	for _, m := range it.Markers {
		m.Frame = of(m.Frame)
		if m.Frame >= it.Duration || (len(out) > 0 && m.Frame <= out[len(out)-1].Frame) {
			continue
		}
		out = append(out, m)
	}
	it.Markers = nilIfEmpty(out)
}

func nilIfEmpty(m []timeline.Marker) []timeline.Marker {
	if len(m) == 0 {
		return nil
	}
	return m
}

// rippleMarkers makes the markers of the timeline follow a ripple all that closes the frames
// [from, to): a marker inside is removed and one at or after to moves earlier by to - from (VV-201).
// It records how many markers were removed and moved in effective.
func (c *Context) rippleMarkers(from, to int) {
	if len(c.Doc.Markers) == 0 || to <= from {
		return
	}
	var kept []timeline.Marker
	removed, moved := 0, 0
	for _, m := range c.Doc.Markers {
		switch {
		case m.Frame < from:
			kept = append(kept, m)
		case m.Frame < to:
			removed++
		default:
			m.Frame -= to - from
			moved++
			kept = append(kept, m)
		}
	}
	if removed == 0 && moved == 0 {
		return
	}
	c.Doc.Markers = nilIfEmpty(kept)
	c.SetEffective("markers_removed", removed)
	c.SetEffective("markers_moved", moved)
}
