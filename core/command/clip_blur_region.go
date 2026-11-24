package command

import (
	"cmp"
	"math"
	"slices"
	"strconv"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// clip.blur_region (SDD-002 C.90): a blurred rectangle of a clip for a time range, built from the blur and the
// mask the plan and the export already draw (SDD-004 sections 3.6 and 3.9). It adds a copy of the clip on a track
// above it; the source is not changed.

const (
	// BlurTrackName is the name of the visual track that clip.blur_region makes (SDD-000 section 17).
	BlurTrackName = "Blur"
	// BlurItemName is the name of the clip item that clip.blur_region adds.
	BlurItemName = "Blur region"

	minBlurShare     = 0.015 // of the shorter side of the project: the least radius, below which text can be read
	defaultBlurShare = 0.04  // of the shorter side of the project: the default radius
	maxBlurRadius    = 100   // the limit of the blur field (SDD-001 section 3.4)
)

func init() {
	Register(Define(Spec{
		Name:    "clip.blur_region",
		Summary: "Blurs a rectangle of a clip for a time range.",
		Description: "Hides part of the picture (an e-mail, a key, an address): adds a copy of the clip, limited to " +
			"start and duration, on a Blur track above the clip, with a blur of radius and a rectangular mask of the " +
			"region. x, y, width and height are fractions of the picture. The radius has a minimum, 1.5% of the " +
			"shorter side of the project, and a default of 4%, so that the text cannot be read. A region that touches " +
			"an edge of the picture is blurred as much as one in the middle. A feather is soft on both sides of " +
			"the border of the region, so within half of it of an edge of the picture a little of the sharp " +
			"picture shows and effective.near_edge is true: use feather 0 there. " +
			"The clip is not changed; edit or delete the item named Blur region to change it.",
		Example: `{"item_id": "itm_m1n2p3q4r5", "x": 0.1, "y": 0.62, "width": 0.8, "height": 0.08, "start": "1s", "duration": "2s"}`,
		Errors: []string{
			CodeValidation, CodeInvalidTime, CodeNotFound, CodeTypeMismatch, CodeInvalidState, CodeIDExists,
			CodeLimit, CodeOverlap,
		},
		Scope: ScopeEdit,
	}, applyClipBlurRegion))
}

type clipBlurRegionArgs struct {
	ItemID     string   `json:"item_id"                desc:"A clip."                                                                                   pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	X          float64  `json:"x"                      desc:"Left edge of the region, a fraction of the picture."                                                                                              min:"0"     max:"1"`
	Y          float64  `json:"y"                      desc:"Top edge of the region, a fraction of the picture."                                                                                               min:"0"     max:"1"`
	Width      float64  `json:"width"                  desc:"Width of the region, a fraction of the picture."                                                                                                  min:"0.001" max:"1"`
	Height     float64  `json:"height"                 desc:"Height of the region, a fraction of the picture."                                                                                                 min:"0.001" max:"1"`
	Start      Time     `json:"start,omitempty"        desc:"Absolute start of the blur, inside the clip. Default the start of the clip."`
	Duration   Time     `json:"duration,omitempty"     desc:"A length. Default up to the end of the clip."`
	Radius     *float64 `json:"radius,omitempty"       desc:"Blur radius in pixels at project resolution, 1.5% of the shorter side to 100. Default 4%."                                                                    max:"100"`
	Feather    *int     `json:"feather,omitempty"      desc:"Soft edge of the region in pixels, 0 to 200. Default 0."                                                                                          min:"0"     max:"200"`
	TrackID    string   `json:"track_id,omitempty"     desc:"A visual track above the clip. Default a Blur track above it, made if there is none."      pattern:"^trk_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	NewID      string   `json:"new_id,omitempty"       desc:"The id of the blur item."                                                                  pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	NewTrackID string   `json:"new_track_id,omitempty" desc:"The id of the track when one is made."                                                     pattern:"^trk_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
}

// BlurRadii are the least and the default blur radius for a project (SDD-002 C.90).
func BlurRadii(s timeline.Settings) (least, def float64) {
	side := float64(min(s.Width, s.Height))
	return min(maxBlurRadius, math.Ceil(minBlurShare*side)), min(maxBlurRadius, math.Ceil(defaultBlurShare*side))
}

func applyClipBlurRegion(c *Context, a *clipBlurRegionArgs) error {
	for _, f := range []struct{ path, prefix, id string }{
		{"args.item_id", timeline.PrefixItem, a.ItemID},
		{"args.track_id", timeline.PrefixTrack, a.TrackID},
		{"args.new_id", timeline.PrefixItem, a.NewID},
		{"args.new_track_id", timeline.PrefixTrack, a.NewTrackID},
	} {
		if err := idForm(f.path, f.prefix, f.id); err != nil {
			return err
		}
	}
	if err := checkBlurRegion(a); err != nil {
		return err
	}
	src, err := c.item(a.ItemID)
	if err != nil {
		return err
	}
	if src.Type != timeline.ItemClip {
		return Errorf(CodeTypeMismatch, "Item %s is a %s item; only a clip can be blurred.", a.ItemID, src.Type).
			WithHint("Use a clip.").WithPath("args.item_id")
	}
	if _, reads := c.sourceAsset(&src); reads {
		if err := rejectCurve(&src, "a blurred part"); err != nil {
			return err
		}
	}

	least, def := BlurRadii(c.Doc.Settings)
	radius := def
	if a.Radius != nil {
		radius = round3(*a.Radius)
		if radius < least {
			return invalidArg(
				"args.radius",
				"args.radius %s is below the least blur, %s, that keeps text unreadable at %d pixels.",
				trimFloat(radius),
				trimFloat(least),
				min(c.Doc.Settings.Width, c.Doc.Settings.Height),
			).
				WithHint("Send a radius from %s to %d, or leave it out.", trimFloat(least), maxBlurRadius)
		}
	}

	start, end, err := c.blurRange(&src, a)
	if err != nil {
		return err
	}
	duration := end - start

	trackID, created, err := c.blurTrack(&src, a, start, duration)
	if err != nil {
		return err
	}
	id, err := c.newID("items", timeline.PrefixItem, a.NewID)
	if err != nil {
		return err
	}
	if err := c.limit("items", len(c.Doc.Items), timeline.MaxItems); err != nil {
		return err
	}

	piece, err := c.blurPiece(&src, start, end)
	if err != nil {
		return err
	}
	mask := &timeline.Mask{
		Shape: "rect", X: round3(a.X), Y: round3(a.Y), Width: round3(a.Width), Height: round3(a.Height),
	}
	if a.Feather != nil {
		mask.Feather = *a.Feather
	}
	makeBlurItem(&piece, id, trackID, radius, mask)
	if !created {
		if err := c.checkOverlap(id, trackID, start, duration); err != nil {
			return err
		}
	}

	if created {
		order, perr := c.placeOrder("", "", src.Track, "")
		if perr != nil {
			return perr
		}
		c.Doc.Tracks[trackID] = timeline.Track{
			ID: trackID, Kind: timeline.TrackVisual, Name: BlurTrackName, Order: order,
		}
		c.SetEffective("new_track_id", trackID)
	}
	c.Doc.Items[id] = piece

	w, h := float64(c.Doc.Settings.Width)*src.Scale, float64(c.Doc.Settings.Height)*src.Scale
	// The blur repeats the edge pixels of the picture, so an edge is as unreadable as the middle (VV-392); only
	// a feather is soft there, centred on the border of the region, half of it outside the picture.
	reach := float64(mask.Feather) / 2
	near := reach > 0 && (mask.X*w < reach || mask.Y*h < reach ||
		(1-mask.X-mask.Width)*w < reach || (1-mask.Y-mask.Height)*h < reach)

	c.SetEffective("new_id", id)
	c.SetEffective("track_id", trackID)
	c.SetEffectiveTime("start", start, cmp.Or(string(a.Start), strconv.Itoa(start)+"f"))
	c.SetEffectiveTime("duration", duration, cmp.Or(string(a.Duration), strconv.Itoa(duration)+"f"))
	c.SetEffective("radius", radius)
	c.SetEffective("min_radius", least)
	c.SetEffective("mask", map[string]any{"x": mask.X, "y": mask.Y, "width": mask.Width, "height": mask.Height})
	c.SetEffective("near_edge", near)
	return nil
}

// checkBlurRegion holds the region inside the picture (C.90 preconditions).
func checkBlurRegion(a *clipBlurRegionArgs) *Error {
	const eps = 1e-9
	for _, f := range []struct {
		path string
		v    float64
	}{{"args.x", a.X}, {"args.y", a.Y}} {
		if f.v < 0 || f.v >= 1 {
			return invalidArg(f.path, "%s %s is outside the picture.", f.path, trimFloat(f.v)).
				WithHint("Send a fraction from 0 up to under 1.")
		}
	}
	for _, f := range []struct {
		path        string
		size, start float64
	}{{"args.width", a.Width, a.X}, {"args.height", a.Height, a.Y}} {
		if f.size < 0.001 || f.start+f.size > 1+eps {
			return invalidArg(f.path, "%s %s with the corner at %s goes outside the picture.",
				f.path, trimFloat(f.size), trimFloat(f.start)).
				WithHint("Send a size from 0.001 up to %s.", trimFloat(round3(1-f.start)))
		}
	}
	return nil
}

// blurRange is the range [start, end) of the blur inside the clip.
func (c *Context) blurRange(src *timeline.Item, a *clipBlurRegionArgs) (start, end int, err *Error) {
	start = src.Start
	if a.Start != "" {
		if start, err = c.ParseTime("args.start", string(a.Start)); err != nil {
			return 0, 0, err
		}
	}
	if start < src.Start || start >= itemEnd(src) {
		return 0, 0, Errorf(CodeInvalidTime, "args.start %q is outside clip %s (frames %d-%d).",
			string(a.Start), src.ID, src.Start, itemEnd(src)-1).
			WithHint("Use a start from \"%df\" to \"%df\".", src.Start, itemEnd(src)-1).WithPath("args.start")
	}
	end = itemEnd(src)
	if a.Duration != "" {
		n, derr := c.length("args.duration", string(a.Duration), false)
		if derr != nil {
			return 0, 0, derr
		}
		end = start + n
	}
	if end > itemEnd(src) {
		return 0, 0, Errorf(
			CodeInvalidTime,
			"args.duration %q ends at frame %d, after clip %s, which ends at frame %d.",
			string(a.Duration),
			end,
			src.ID,
			itemEnd(src),
		).
			WithHint("Use a duration of at most \"%df\".", itemEnd(src)-start).
			WithPath("args.duration")
	}
	return start, end, nil
}

// blurTrack is the track of the blur and whether it has to be made (C.90, Track).
func (c *Context) blurTrack(
	src *timeline.Item,
	a *clipBlurRegionArgs,
	start, duration int,
) (id string, created bool, err *Error) {
	order := c.tracksByOrder("")
	at := slices.Index(order, src.Track)
	if a.TrackID != "" {
		t, terr := c.track(a.TrackID, "args.track_id")
		if terr != nil {
			return "", false, terr
		}
		if t.Kind != timeline.TrackVisual {
			return "", false, Errorf(CodeTypeMismatch, "Track %s is an audio track and cannot hold a blur.", a.TrackID).
				WithHint("Use a visual track above the clip.").WithPath("args.track_id")
		}
		if slices.Index(order, a.TrackID) >= at {
			return "", false, invalidArg(
				"args.track_id",
				"Track %s is not above track %s, so it would not draw over the clip.",
				a.TrackID,
				src.Track,
			).
				WithHint("Use a visual track above the track of the clip, or leave track_id out.")
		}
		return a.TrackID, false, nil
	}
	for i := at - 1; i >= 0; i-- {
		t := c.Doc.Tracks[order[i]]
		if t.Kind == timeline.TrackVisual && t.Name == BlurTrackName &&
			len(c.conflicts(t.ID, start, duration, "")) == 0 {
			return t.ID, false, nil
		}
	}
	id, err = c.newID("tracks", timeline.PrefixTrack, a.NewTrackID)
	if err != nil {
		return "", false, err
	}
	if lerr := c.limit("tracks", len(c.Doc.Tracks), timeline.MaxTracks); lerr != nil {
		return "", false, lerr
	}
	return id, true, nil
}

// blurPiece is the part [start, end) of the clip, cut as item.split cuts it, so that speed, freeze and reverse
// read the same source frames.
func (c *Context) blurPiece(src *timeline.Item, start, end int) (timeline.Item, *Error) {
	piece := timeline.CloneItem(*src)
	const scratch = "itm_zzzzzzzzzz"
	if start > piece.Start {
		_, right, err := c.splitParts(&piece, start, scratch)
		if err != nil {
			return piece, err
		}
		piece = right
	}
	if end < itemEnd(&piece) {
		left, _, err := c.splitParts(&piece, end, scratch)
		if err != nil {
			return piece, err
		}
		piece = left
	}
	return piece, nil
}

// makeBlurItem turns the piece of the clip into the blur item: what the picture needs is kept, and what would
// repeat or fight the source (sound, links, transitions, effects, its own mask and blur) is dropped.
func makeBlurItem(it *timeline.Item, id, track string, radius float64, mask *timeline.Mask) {
	it.ID, it.Track, it.Name = id, track, BlurItemName
	it.Slot, it.Link, it.Markers = nil, "", nil
	it.TransitionIn, it.TransitionOut, it.FadeIn, it.FadeOut = nil, nil, 0, 0
	it.AudioMuted, it.Denoise, it.Voice, it.Processors = true, 0, "", nil
	it.Effects, it.Shadow, it.Outline, it.Blend = nil, nil, nil, ""
	it.Blur, it.Mask = radius, mask
	for prop := range it.Keyframes {
		if prop == "volume" || len(prop) > 5 && prop[:5] == "mask_" {
			delete(it.Keyframes, prop)
		}
	}
	if len(it.Keyframes) == 0 {
		it.Keyframes = nil
	}
}
