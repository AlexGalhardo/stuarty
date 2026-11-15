package servicetest

import (
	"fmt"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

// musicOnClipAsset adds an audio track and an audio item of the clip's asset, so the same
// analysed asset is both the speech and the music.
func (m *mediaEnv) musicOnClipAsset(project, clip string) (music string) {
	m.t.Helper()

	doc, _ := timeline.Decode(m.document(project))
	asset := doc.Items[clip].Asset

	res := m.mustEdit(ui, project, m.head(project).Revision, op("track.add", `{"kind":"audio","name":"M"}`))

	var track string

	for _, id := range res.Created {
		if strings.HasPrefix(id, "trk_") {
			track = id
		}
	}

	res = m.mustEdit(ui, project, m.head(project).Revision, op(
		"audio.add", fmt.Sprintf(`{"track_id":%q,"asset_id":%q,"start":"0s","duration":"2s"}`, track, asset),
	))

	for _, id := range res.Created {
		if strings.HasPrefix(id, "itm_") {
			music = id
		}
	}

	return music
}

// SDD-002 C.51: the speech comes from the speech artifact of the asset of each speech item, in
// asset frames, and the stored operation carries it.
func TestResolveSpeechFromTheArtifact(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{beta: true})
	project, clip, _, _ := m.speechProject()
	music := m.musicOnClipAsset(project, clip)

	m.mustEdit(ui, project, m.head(project).Revision, op(
		"audio.duck", fmt.Sprintf(`{"item_id":%q,"speech_item_ids":[%q]}`, music, clip),
	))

	args := string(storedOps(t, m, project, m.head(project).Revision)[0].Args)
	want := fmt.Sprintf(
		`"resolved":{"speech":[{"item_id":%q,"ranges":[{"end":15,"start":0},{"end":60,"start":30}]}]}`,
		clip,
	)

	if !strings.Contains(args, want) {
		t.Errorf("stored operation = %s, want it to hold %s", args, want)
	}
}

// SDD-002 C.52: the beats of the grid become increasing asset frames.
func TestResolveBeatsFromTheArtifact(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{beta: true})
	project, clip, _, _ := m.speechProject()
	music := m.musicOnClipAsset(project, clip)

	m.mustEdit(ui, project, m.head(project).Revision, op("audio.mark_beats", fmt.Sprintf(`{"item_id":%q}`, music)))

	args := string(storedOps(t, m, project, m.head(project).Revision)[0].Args)
	if !strings.Contains(args, `"resolved":{"beats":[0,15,30,45]}`) {
		t.Errorf("stored operation = %s", args)
	}
}
