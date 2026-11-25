package command

import (
	"math"
	"slices"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// The cursor and the click highlight of a screen take are shape items on tracks of their own (SDD-025 section
// 17.3, names in SDD-000 section 18). The commands find what they made before by these names.
const (
	// CursorTrackName is the visual track of the cursor items.
	CursorTrackName = "Cursor"
	// HighlightTrackName is the visual track of the click highlight items.
	HighlightTrackName = "Click highlight"
	cursorItemName     = "Cursor "
	clickItemName      = "Click "
)

// srcMilli maps a position of the captured source (a w by h grid) to the grid of the asset, in milli-pixels from
// the top left corner.
func (t *takeClip) srcMilli(xm, ym int64, w, h int) (int64, int64) {
	return xm * int64(t.asset.Width) / int64(w), ym * int64(t.asset.Height) / int64(h)
}

// transformAt is the transform the clip has at a frame of the item: the keyframes, or the static value. The scale is
// in thousandths and the centre in milli-pixels of the frame.
func (t *takeClip) transformAt(c *Context, frame int) (scale, cx, cy int64) {
	s := &c.Doc.Settings
	cxs, cys := float64(s.Width)/2, float64(s.Height)/2

	if t.it.X != nil {
		cxs = *t.it.X
	}

	if t.it.Y != nil {
		cys = *t.it.Y
	}

	milli := func(v float64) int64 { return int64(math.Round(v * 1000)) }

	return milli(timeline.ValueAt(t.it.Keyframes["scale"], frame, t.it.Scale)),
		milli(timeline.ValueAt(t.it.Keyframes["x"], frame, cxs)),
		milli(timeline.ValueAt(t.it.Keyframes["y"], frame, cys))
}

// removeOverlays deletes the shape items whose name matches.
func (c *Context) removeOverlays(match func(name string) bool) {
	for id := range c.Doc.Items {
		if it := c.Doc.Items[id]; it.Type == timeline.ItemShape && match(it.Name) {
			delete(c.Doc.Items, id)
		}
	}
}

// overlayName reports whether name is the name an overlay of the clip carries: the base and the id, and for the
// highlights a number after them.
func overlayName(name, base, itemID string, numbered bool) bool {
	rest, ok := strings.CutPrefix(name, base+itemID)
	if !ok {
		return false
	}

	if !numbered {
		return rest == ""
	}

	n, found := strings.CutPrefix(rest, " ")

	return found && n != "" && strings.Trim(n, "0123456789") == ""
}

// overlayTrack is the track an overlay of the clip goes on: the nearest unlocked visual track of that name above
// the track of the clip that is free for the range, or a new one directly above the clip (above the highlight
// track for the cursor, so the cursor is drawn over the highlights). created says the track is new.
func (c *Context) overlayTrack(t *takeClip, name string, start, length int) (id string, err *Error) {
	order := c.tracksByOrder("")
	at := slices.Index(order, t.it.Track)

	for i := at - 1; i >= 0; i-- {
		tr := c.Doc.Tracks[order[i]]
		if tr.Kind == timeline.TrackVisual && !tr.Locked && tr.Name == name &&
			len(c.conflicts(tr.ID, start, length, "")) == 0 {
			return tr.ID, nil
		}
	}

	above := t.it.Track

	if name == CursorTrackName {
		for i := at - 1; i >= 0; i-- {
			if tr := c.Doc.Tracks[order[i]]; tr.Kind == timeline.TrackVisual && tr.Name == HighlightTrackName {
				above = tr.ID

				break
			}
		}
	}

	id, err = c.newID("tracks", timeline.PrefixTrack, "")
	if err != nil {
		return "", err
	}

	if lerr := c.limit("tracks", len(c.Doc.Tracks), timeline.MaxTracks); lerr != nil {
		return "", lerr
	}

	ord, oerr := c.placeOrder("", "", above, "")
	if oerr != nil {
		return "", oerr
	}

	c.Doc.Tracks[id] = timeline.Track{ID: id, Kind: timeline.TrackVisual, Name: name, Order: ord}

	return id, nil
}

// newShape adds a shape item of the overlay to the document, placed at the first value of its keyframes.
func (c *Context) newShape(trackID, name, shape string, start, duration int, size float64) (timeline.Item, *Error) {
	id, err := c.newID("items", timeline.PrefixItem, "")
	if err != nil {
		return timeline.Item{}, err
	}

	it := timeline.NewItem(id, timeline.ItemShape, trackID, start, duration)
	it.Name, it.Shape, it.Width, it.Height = name, shape, size, size

	return it, nil
}
