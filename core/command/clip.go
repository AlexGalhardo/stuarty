package command

import (
	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

func init() {
	Register(Define(Spec{
		Name:    "clip.add",
		Summary: "Places a video or image asset on a visual track.",
		Description: "Adds a clip. The default duration is the rest of the asset from source_in, or 5 seconds for an image. " +
			"The reserved asset ast_placeholder creates a template slot holder and needs an explicit duration.",
		Example: `{"track_id": "trk_f6g7h8j9k0", "asset_id": "ast_3c5v8n2w6d", "start": "0s", "source_in": "2s", "duration": "5s", "id": "itm_m1n2p3q4r5"}`,
		Errors: []string{
			CodeValidation, CodeInvalidTime, CodeNotFound, CodeTypeMismatch, CodeIDExists, CodeLimit,
			CodeOutOfSource, CodeOverlap,
		},
		Scope: ScopeEdit,
		Phase: Prototype,
	}, applyClipAdd))
}

type clipAddArgs struct {
	TrackID  string `json:"track_id"            desc:"A visual track."                                                                           pattern:"^trk_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	AssetID  string `json:"asset_id"            desc:"A video or image asset, or ast_placeholder in a template."                                 pattern:"^ast_([0123456789abcdefghjkmnpqrstvwxyz]{10}|placeholder)$"`
	Start    Time   `json:"start,omitempty"     desc:"Absolute position; omitted, the end of the last item of the track (0s on an empty track)."`
	SourceIn Time   `json:"source_in,omitempty" desc:"Offset into the asset; default 0s; omit for an image."`
	Duration Time   `json:"duration,omitempty"  desc:"A length; default the rest of the asset, or 5s for an image."`
	Fit      string `json:"fit,omitempty"                                                                                                                                                                             enum:"cover,contain,none"`
	ID       string `json:"id,omitempty"                                                                                                         pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
}

func applyClipAdd(c *Context, a *clipAddArgs) error {
	placeholder := a.AssetID == timeline.PlaceholderAsset
	if err := idForm("args.track_id", timeline.PrefixTrack, a.TrackID); err != nil {
		return err
	}
	if !placeholder {
		if err := idForm("args.asset_id", timeline.PrefixAsset, a.AssetID); err != nil {
			return err
		}
	}
	if err := idForm("args.id", timeline.PrefixItem, a.ID); err != nil {
		return err
	}
	start, err := c.startOrTrackEnd(a.TrackID, a.Start)
	if err != nil {
		return err
	}
	if start >= timeline.MaxFrames {
		return Errorf(
			CodeInvalidTime,
			"args.start \"%s\" is at or after the timeline limit of %d frames.",
			a.Start,
			timeline.MaxFrames,
		).
			WithHint("Use a start before \"%df\".", timeline.MaxFrames).
			WithPath("args.start")
	}
	sourceIn := 0
	if a.SourceIn != "" {
		if sourceIn, err = c.length("args.source_in", string(a.SourceIn), true); err != nil {
			return err
		}
	}
	duration := 0
	if a.Duration != "" {
		if duration, err = c.length("args.duration", string(a.Duration), false); err != nil {
			return err
		}
	}
	if placeholder && a.Duration == "" {
		return Errorf(CodeValidation, "args.duration is required with the placeholder asset.").
			WithHint(`Send a duration, for example "5s".`).WithPath("args.duration")
	}

	if _, err := c.track(a.TrackID, "args.track_id"); err != nil {
		return err
	}
	var asset timeline.Asset
	if !placeholder {
		if asset, err = c.asset(a.AssetID, "args.asset_id"); err != nil {
			return err
		}
	}
	if err := c.clipTypes(a, asset, placeholder); err != nil {
		return err
	}
	id, err := c.newID("items", timeline.PrefixItem, a.ID)
	if err != nil {
		return err
	}
	if err := c.limit("items", len(c.Doc.Items), timeline.MaxItems); err != nil {
		return err
	}

	it := timeline.NewItem(id, timeline.ItemClip, a.TrackID, start, duration)
	it.Asset, it.SourceIn = a.AssetID, sourceIn
	fit := a.Fit
	if fit == "" && !placeholder {
		fit = c.overlayImageFit(a.TrackID, &asset)
	}
	if fit != "" {
		it.Fit = fit
	}
	if !placeholder {
		if it.Duration, err = c.clipDuration(&it, &asset, a.Duration == "", start); err != nil {
			return err
		}
	}
	if err := c.checkEnd("args.duration", start, it.Duration); err != nil {
		return err
	}
	if err := c.checkOverlapAppend(id, a.TrackID, start, it.Duration); err != nil {
		return err
	}
	c.Doc.Items[id] = it
	c.SetEffective("id", id)
	c.SetEffectiveTime("start", start, string(a.Start))
	c.SetEffectiveTime("source_in", it.SourceIn, string(a.SourceIn))
	c.SetEffectiveTime("duration", it.Duration, string(a.Duration))
	return nil
}

// overlayImageFit is the default fit of an image added without one (VV-039): an
// image on a track that has a visual track below it is an overlay (a logo, a
// sticker), so it keeps its natural size when it fits the frame and is contained
// when it does not. Any other clip keeps the schema default ("" means cover).
func (c *Context) overlayImageFit(trackID string, asset *timeline.Asset) string {
	if asset.Kind != timeline.AssetImage {
		return ""
	}
	order := c.Doc.Tracks[trackID].Order
	below := false
	for id := range c.Doc.Tracks {
		if t := c.Doc.Tracks[id]; t.Kind == timeline.TrackVisual && t.Order > order {
			below = true
			break
		}
	}
	if !below {
		return ""
	}
	if asset.Width <= c.Doc.Settings.Width && asset.Height <= c.Doc.Settings.Height {
		return "none"
	}
	return "contain"
}

// clipTypes is precondition 3 of clip.add and the image rule of precondition 1.
func (c *Context) clipTypes(a *clipAddArgs, asset timeline.Asset, placeholder bool) *Error {
	if t := c.Doc.Tracks[a.TrackID]; t.Kind != timeline.TrackVisual {
		return Errorf(CodeTypeMismatch, "Track %s is an audio track; clip.add needs a visual track.", a.TrackID).
			WithHint("Use a visual track.").WithPath("args.track_id")
	}
	if placeholder {
		return nil
	}
	if asset.Kind == timeline.AssetAudio {
		return Errorf(CodeTypeMismatch, "Asset %s is an audio asset; clip.add places video or images.", a.AssetID).
			WithHint("Use audio.add on an audio track.").WithPath("args.asset_id")
	}
	if asset.Kind == timeline.AssetLUT {
		return Errorf(CodeTypeMismatch, "Asset %s is a lookup table; clip.add places video or images.", a.AssetID).
			WithHint("Use clip.set_lut to grade a clip with it.").WithPath("args.asset_id")
	}
	if asset.Kind == timeline.AssetImage && a.SourceIn != "" {
		if f, err := c.ParseTime("args.source_in", string(a.SourceIn)); err == nil && f != 0 {
			return Errorf(CodeValidation, "args.source_in must be omitted for image asset %s.", a.AssetID).
				WithHint("Remove source_in; images have no source position.").WithPath("args.source_in")
		}
	}
	return nil
}

// clipDuration returns the duration of a new clip (the default when none was given,
// clamped to the timeline end) after checking its source range (R9).
func (c *Context) clipDuration(it *timeline.Item, asset *timeline.Asset, byDefault bool, start int) (int, *Error) {
	if asset.Kind == timeline.AssetImage {
		if !byDefault {
			return it.Duration, nil
		}
		five, err := c.ParseTime("args.duration", "5s")
		if err != nil {
			return 0, err
		}
		return min(five, timeline.MaxFrames-start), nil
	}
	total, ok := timeline.AssetDuration(*asset, c.Doc.Settings.FPS)
	if !ok {
		return it.Duration, nil
	}
	if it.SourceIn >= total {
		return 0, Errorf(
			CodeOutOfSource,
			"The source_in %d is at or after the end of asset %s, which has %d frames.",
			it.SourceIn,
			it.Asset,
			total,
		).
			WithHint("Use a source_in before \"%df\".", total).
			WithDetail("available", map[string]any{"asset_duration": total, "source_in": it.SourceIn, "max_duration": 0}).
			WithPath("args.source_in")
	}
	if byDefault {
		return min(total-it.SourceIn, timeline.MaxFrames-start), nil
	}
	if err := c.checkSource(it, it.Duration, it.SourceIn); err != nil {
		return 0, err.WithPath("args.duration")
	}
	return it.Duration, nil
}
