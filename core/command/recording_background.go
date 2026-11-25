package command

import (
	"math"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// BackgroundTrackName is the visual track at the bottom of the stack that holds the background of a screen clip
// (SDD-000 section 19).
const (
	BackgroundTrackName = "Background"
	backgroundItemName  = "Background "
	defaultBackground   = "#0F172A"
)

func init() {
	Register(Define(Spec{
		Name:    "recording.set_background",
		Summary: "Sets a colour or image behind a screen clip.",
		Description: "Puts a colour or an image asset behind a screen clip, on a visual track named Background at the " +
			"bottom of the stack, for the time the clip plays, and gives the clip a drop shadow (shadow, default true; " +
			"false removes it). kind none removes the background and the shadow. Each run replaces the items of the " +
			"clip's background. Use recording.apply_layout first: its padding leaves the room the background shows in. " +
			"There is no gradient and no rounded corner: make a gradient as an image and use kind image.",
		Example: `{"item_id": "itm_m1n2p3q4r5", "kind": "color", "color": "#1E293B"}`,
		Errors:  []string{CodeValidation, CodeNotFound, CodeTypeMismatch, CodeLimit},
		Scope:   ScopeEdit,
	}, applySetBackground))
}

type setBackgroundArgs struct {
	ItemID  string `json:"item_id"            desc:"The screen clip."                                              pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Kind    string `json:"kind"               desc:"color, image or none (removes the background and the shadow)."                                                        enum:"color,image,none"`
	Color   string `json:"color,omitempty"    desc:"For color: a colour. Default #0F172A."`
	AssetID string `json:"asset_id,omitempty" desc:"For image: an image asset."                                    pattern:"^ast_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Shadow  *bool  `json:"shadow,omitempty"   desc:"Draw a drop shadow under the clip. Default true."`
}

func (a *setBackgroundArgs) check() *Error {
	if err := idForm("args.asset_id", timeline.PrefixAsset, a.AssetID); err != nil {
		return err
	}

	switch a.Kind {
	case "color":
		if a.AssetID != "" {
			return invalidArg("args.asset_id", "args.asset_id applies to kind image, not color.").
				WithHint("Remove asset_id, or use kind image.")
		}
	case "image":
		if a.AssetID == "" {
			return invalidArg("args.asset_id", "args.asset_id: kind image needs an image asset.").
				WithHint("Send asset_id, or use kind color.")
		}

		if a.Color != "" {
			return invalidArg("args.color", "args.color applies to kind color, not image.").
				WithHint("Remove color, or use kind color.")
		}
	case "none":
		if a.Color != "" || a.AssetID != "" || a.Shadow != nil {
			return invalidArg("args.kind", "kind none takes no color, asset_id or shadow.").
				WithHint("Remove them, or choose kind color or image.")
		}
	default:
		return invalidArg("args.kind", "args.kind %q is not a kind of background.", a.Kind).
			WithHint("Use color, image or none.")
	}

	return nil
}

// backgroundTrack is the Background track the new item goes on: an unlocked visual track of that name free for the
// range, or a new one at the bottom of the stack.
func (c *Context) backgroundTrack(start, length int) (string, *Error) {
	for _, id := range c.tracksByOrder("") {
		tr := c.Doc.Tracks[id]
		if tr.Kind == timeline.TrackVisual && !tr.Locked && tr.Name == BackgroundTrackName &&
			len(c.conflicts(id, start, length, "")) == 0 {
			return id, nil
		}
	}

	id, err := c.newID("tracks", timeline.PrefixTrack, "")
	if err != nil {
		return "", err
	}

	if lerr := c.limit("tracks", len(c.Doc.Tracks), timeline.MaxTracks); lerr != nil {
		return "", lerr
	}

	ord, oerr := c.placeOrder("", "bottom", "", "")
	if oerr != nil {
		return "", oerr
	}

	c.Doc.Tracks[id] = timeline.Track{ID: id, Kind: timeline.TrackVisual, Name: BackgroundTrackName, Order: ord}

	return id, nil
}

// clipShadow is the drop shadow of a framed screen: black, a little below the clip, sized to the frame.
func clipShadow(s *timeline.Settings) *timeline.Shadow {
	return &timeline.Shadow{
		Color:   "#000000",
		Opacity: 0.5,
		DY:      round3(0.012 * float64(s.Height)),
		Blur:    round3(0.03 * math.Min(float64(s.Width), float64(s.Height))),
	}
}

func applySetBackground(c *Context, a *setBackgroundArgs) error {
	if err := a.check(); err != nil {
		return err
	}

	clip, err := c.clipItem(a.ItemID, "a background")
	if err != nil {
		return err
	}

	var asset timeline.Asset

	if a.Kind == "image" {
		got, ok := c.Doc.Assets[a.AssetID]
		if !ok {
			return Errorf(CodeNotFound, "Asset %s does not exist.", a.AssetID).
				WithHint("Use an asset of the project: see the project's assets.").WithPath("args.asset_id")
		}

		if got.Kind != timeline.AssetImage {
			return Errorf(
				CodeTypeMismatch,
				"Asset %s is a %s asset; a background image needs an image.",
				a.AssetID,
				got.Kind,
			).
				WithHint("Use an asset of kind image.").
				WithPath("args.asset_id")
		}

		asset = got
	}

	colour, cerr := c.optionalColour("args.color", a.Color)
	if cerr != nil {
		return cerr
	}

	freed := c.removeNamed(backgroundItemName + a.ItemID)

	if a.Kind == "none" || (a.Shadow != nil && !*a.Shadow) {
		clip.Shadow = nil
	} else {
		clip.Shadow = clipShadow(&c.Doc.Settings)
	}

	c.Doc.Items[clip.ID] = clip
	c.SetEffective("kind", a.Kind)

	if a.Kind == "none" {
		c.dropEmptyBackgroundTracks(freed)
		c.SetEffective("items", 0)

		return nil
	}

	if lerr := c.limit("items", len(c.Doc.Items), timeline.MaxItems); lerr != nil {
		return lerr
	}

	trackID, terr := c.backgroundTrack(clip.Start, clip.Duration)
	if terr != nil {
		return terr
	}

	id, ierr := c.newID("items", timeline.PrefixItem, "")
	if ierr != nil {
		return ierr
	}

	bg := c.backgroundItem(id, trackID, &clip, &asset, a.AssetID, colour)
	bg.Name = backgroundItemName + a.ItemID
	c.Doc.Items[id] = bg
	c.SetEffective("items", 1)
	c.SetEffective("track_id", trackID)

	return nil
}

// backgroundItem is the one item of a background: a full-frame rectangle of a colour, or a clip of an image that
// covers the frame.
func (c *Context) backgroundItem(
	id, trackID string,
	over *timeline.Item,
	asset *timeline.Asset,
	assetID string,
	colour timeline.Color,
) timeline.Item {
	if assetID != "" {
		it := timeline.NewItem(id, timeline.ItemClip, trackID, over.Start, over.Duration)
		it.Asset, it.Fit = asset.ID, "cover"

		return it
	}

	if colour == "" {
		colour = defaultBackground
	}

	s := &c.Doc.Settings
	it := timeline.NewItem(id, timeline.ItemShape, trackID, over.Start, over.Duration)
	it.Shape, it.Width, it.Height, it.Fill = "rect", float64(s.Width), float64(s.Height), colour

	return it
}

// removeNamed deletes the items of the given name, whatever their type, and returns the tracks they were on.
func (c *Context) removeNamed(name string) []string {
	var tracks []string

	for id := range c.Doc.Items {
		if it := c.Doc.Items[id]; it.Name == name {
			tracks = append(tracks, it.Track)
			delete(c.Doc.Items, id)
		}
	}

	return tracks
}

// dropEmptyBackgroundTracks removes the Background tracks in the list that no item uses any more, so that taking a
// background away leaves no empty track behind.
func (c *Context) dropEmptyBackgroundTracks(ids []string) {
	for _, id := range ids {
		if tr, ok := c.Doc.Tracks[id]; ok && tr.Name == BackgroundTrackName && !tr.Locked &&
			len(c.trackItems(id)) == 0 {
			delete(c.Doc.Tracks, id)
		}
	}
}
