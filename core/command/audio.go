package command

import (
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func init() {
	Register(Define(Spec{
		Name:    "audio.add",
		Summary: "Places an audio asset, or the audio of a video asset, on an audio track.",
		Description: "Adds an audio item. The default duration is the rest of the asset from source_in. The reserved " +
			"asset ast_placeholder creates a template slot holder and needs an explicit duration.",
		Example: `{"track_id": "trk_m1n2p3q4r5", "asset_id": "ast_4h6j8k2m3n", "start": "1s", "id": "itm_s6t7v8w9x0"}`,
		Errors: []string{
			CodeValidation, CodeInvalidTime, CodeNotFound, CodeTypeMismatch, CodeIDExists, CodeLimit,
			CodeOutOfSource, CodeOverlap,
		},
		Scope: ScopeEdit,
	}, applyAudioAdd))
}

type audioAddArgs struct {
	TrackID  string `json:"track_id"            desc:"An audio track."                                                                           pattern:"^trk_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	AssetID  string `json:"asset_id"            desc:"An audio asset, a video asset with audio, or ast_placeholder in a template."               pattern:"^ast_([0123456789abcdefghjkmnpqrstvwxyz]{10}|placeholder)$"`
	Start    Time   `json:"start,omitempty"     desc:"Absolute position; omitted, the end of the last item of the track (0s on an empty track)."`
	SourceIn Time   `json:"source_in,omitempty" desc:"Offset into the asset; default 0s."`
	Duration Time   `json:"duration,omitempty"  desc:"A length; default the rest of the asset."`
	ID       string `json:"id,omitempty"                                                                                                         pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
}

func applyAudioAdd(c *Context, a *audioAddArgs) error {
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
	track, err := c.track(a.TrackID, "args.track_id")
	if err != nil {
		return err
	}
	var asset timeline.Asset
	if !placeholder {
		if asset, err = c.asset(a.AssetID, "args.asset_id"); err != nil {
			return err
		}
	}
	if track.Kind != timeline.TrackAudio {
		return Errorf(CodeTypeMismatch, "Track %s is a visual track; audio.add needs an audio track.", a.TrackID).
			WithHint("Use an audio track.").WithPath("args.track_id")
	}
	if !placeholder {
		if err := audioAssetKind(a.AssetID, &asset); err != nil {
			return err
		}
	}
	id, err := c.newID("items", timeline.PrefixItem, a.ID)
	if err != nil {
		return err
	}
	if err := c.limit("items", len(c.Doc.Items), timeline.MaxItems); err != nil {
		return err
	}
	it := timeline.NewItem(id, timeline.ItemAudio, a.TrackID, start, duration)
	it.Asset, it.SourceIn = a.AssetID, sourceIn
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

// audioAssetKind is the E_TYPE_MISMATCH of an asset that has no audio.
func audioAssetKind(id string, asset *timeline.Asset) *Error {
	switch {
	case asset.Kind == timeline.AssetImage:
		return Errorf(CodeTypeMismatch, "Asset %s is an image; audio.add needs audio.", id).
			WithHint("Use clip.add on a visual track to show it.").WithPath("args.asset_id")
	case asset.Kind == timeline.AssetLUT:
		return Errorf(CodeTypeMismatch, "Asset %s is a lookup table; audio.add needs audio.", id).
			WithHint("Use clip.set_lut to grade a clip with it.").WithPath("args.asset_id")
	case asset.Kind == timeline.AssetVideo && !asset.HasAudio:
		return Errorf(CodeTypeMismatch, "Asset %s is a video without audio; audio.add needs audio.", id).
			WithHint("Use clip.add on a visual track to show it.").WithPath("args.asset_id")
	}
	return nil
}
