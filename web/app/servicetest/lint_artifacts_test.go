package servicetest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/engine/ingest"
	"github.com/AlexGalhardo/vukem-video/service"
)

// loudPeaks is a peaks.bin of 2 s at 48 kHz in which every group of samples
// reaches full scale.
func loudPeaks() []byte {
	p := &ingest.Peaks{SampleRate: 48000, SampleCount: 96000, Min: -32767, Max: 32767}

	for i, per := range ingest.PeakLevels {
		n := ingest.PeakCount(p.SampleCount, per)
		p.Levels[i] = make([]ingest.PeakPair, n)

		for j := range p.Levels[i] {
			p.Levels[i][j] = ingest.PeakPair{Min: -32767, Max: 32767}
		}
	}

	return p.Marshal()
}

// SDD-003 section 3.6: the full lint reads the peaks, loudness and transcript
// artifacts the ingest and the transcribe job stored, so L_AUDIO_CLIP,
// L_LOUDNESS and L_CAPTION_DESYNC fire through Service.Lint.
func TestLintFiresFromStoredArtifacts(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project, clip, track2, hash := m.speechProject()
	ctx := context.Background()

	pa, err := m.store.Assets().FindProjectAssetByHash(ctx, project, hash)
	if err != nil {
		t.Fatal(err)
	}

	hex := strings.TrimPrefix(hash, "sha256:")
	if err := os.WriteFile(filepath.Join(m.ws, "artifacts", hex, "peaks-1.bin"), loudPeaks(), 0o600); err != nil {
		t.Fatal(err)
	}

	m.putTranscript(hash)

	// A second source of the same sound plays at once, so the mix peaks above 0 dBFS.
	rev := m.head(project).Revision
	res := m.mustEdit(ui, project, rev,
		op("clip.add", fmt.Sprintf(`{"track_id":%q,"asset_id":%q,"start":"0s","duration":"2s"}`, track2, pa.AssetID)),
		op("track.add", `{"kind":"visual","name":"Captions"}`),
		op("settings.set", `{"loudness_target":-30}`),
	)

	var captions string

	for _, id := range res.Created {
		if strings.HasPrefix(id, "trk_") {
			captions = id
		}
	}

	made := m.mustEdit(ui, project, m.head(project).Revision, op(
		"caption.from_transcript", fmt.Sprintf(`{"clip_item_id":%q,"track_id":%q}`, clip, captions),
	))
	// The first caption moves 10 frames away from where its first word is spoken.
	first := ""

	for _, id := range made.Created {
		if strings.HasPrefix(id, "itm_") {
			first = id
			break
		}
	}

	m.mustEdit(
		ui, project, m.head(project).Revision,
		op("item.move", fmt.Sprintf(`{"item_id":%q,"start":"10f"}`, first)),
	)

	out, lerr := m.svc.Lint(m.as(ui), service.LintRequest{ProjectID: project, Limit: 100})
	if lerr != nil {
		t.Fatal(lerr)
	}

	for _, code := range []string{"L_AUDIO_CLIP", "L_LOUDNESS", "the first word starts at frame 10"} {
		if !strings.Contains(out.Text, code) {
			t.Errorf("the lint lacks %s:\n%s", code, out.Text)
		}
	}
}
