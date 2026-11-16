package service

import (
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

// VV-050: an analysis of an asset with no audio is a type mismatch that says what the
// asset is, never "wait until the asset is ready".
func TestNoSoundRefusesAssetsWithoutAudio(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		asset timeline.Asset
		want  string
	}{
		{"image", timeline.Asset{ID: "ast_img", Kind: timeline.AssetImage}, "an image"},
		{"silent video", timeline.Asset{ID: "ast_vid", Kind: timeline.AssetVideo}, "a video with no audio track"},
	}

	for _, c := range cases {
		e := noSound(2, "itm_x", c.asset, "beat grid")
		if e == nil || e.Code != command.CodeTypeMismatch || !strings.Contains(e.Message, c.want) ||
			strings.Contains(e.Hint, "wait") || e.OpIndex == nil || *e.OpIndex != 2 {
			t.Errorf("%s: %+v", c.name, e)
		}
	}

	for _, a := range []timeline.Asset{
		{ID: "ast_vid", Kind: timeline.AssetVideo, HasAudio: true},
		{ID: "ast_aud", Kind: timeline.AssetAudio},
	} {
		if e := noSound(0, "itm_x", a, "beat grid"); e != nil {
			t.Errorf("%s has audio but was refused: %+v", a.ID, e)
		}
	}
}
