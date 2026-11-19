package command

import (
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// Golden cases of clip.add with an animated image and a sticker (SDD-002 C.5,
// SDD-001 section 3.2): the command is unchanged, the image just has frame delays.

const (
	animAsset  = "ast_2d4f6g8h9j"
	upperTrack = "trk_p2q3r4s5t6"
)

func init() {
	anim := func() *B {
		return base().with(func(d *timeline.Document) {
			d.Assets[animAsset] = timeline.Asset{
				ID: animAsset, Hash: "sha256:" + strings.Repeat("c3", 32), Kind: timeline.AssetImage,
				Name: "party.gif", Width: 256, Height: 256, FrameCount: 3, FrameDelays: []int{67, 33, 100},
			}
		})
	}
	withUpper := func() *B {
		return anim().with(func(d *timeline.Document) {
			d.Tracks[upperTrack] = timeline.Track{
				ID:    upperTrack,
				Kind:  timeline.TrackVisual,
				Name:  "Stickers",
				Order: "a05",
			}
		}).clip(itemA, vidTr, 0, 150, 0)
	}
	ca := func(name, args string, b func() *B) { gc("clip.add", name, b, op("clip.add", args)) }
	args := func(track, extra string) string {
		return `{"track_id": "` + track + `", "asset_id": "` + animAsset + `", "start": "1s", "id": "` + itemB + `"` + extra + `}`
	}

	ca("ok_animated_image_default_5s_loops", args(vidTr, ``), anim)
	ca("ok_animated_image_explicit_duration", args(vidTr, `, "duration": "2s"`), anim)
	ca("ok_sticker_on_an_upper_track", args(upperTrack, `, "duration": "3s"`), withUpper)
	bigLogo := func() *B {
		return withUpper().with(func(d *timeline.Document) {
			a := d.Assets[imgAsset]
			a.Width, a.Height = 4000, 3000
			d.Assets[imgAsset] = a
		})
	}
	logoArgs := func(extra string) string {
		return `{"track_id": "` + upperTrack + `", "asset_id": "` + imgAsset + `", "start": "1s", "id": "` + itemB + `"` + extra + `}`
	}
	ca("ok_image_on_an_upper_track_keeps_natural_size", logoArgs(``), withUpper)
	ca("ok_image_on_an_upper_track_larger_than_frame_is_contained", logoArgs(``), bigLogo)
	ca("ok_image_on_an_upper_track_explicit_fit", logoArgs(`, "fit": "cover"`), withUpper)
	ca("err_validation_source_in_on_animated_image", args(vidTr, `, "source_in": "1s"`), anim)
}
