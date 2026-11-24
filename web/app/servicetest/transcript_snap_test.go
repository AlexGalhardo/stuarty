package servicetest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
)

// putSnapTalk stores a transcript whose first word ends 3 frames before the silence of the test asset (frames 15 to
// 30 at 30 fps, SDD-011 9.1) and whose next word starts after it.
func (m *mediaEnv) putSnapTalk(hash string) {
	m.t.Helper()

	hex := strings.TrimPrefix(hash, "sha256:")
	rel := "artifacts/" + hex + "/transcript-1-def456def456.json"
	body := `{"version":1,"language":"en","audio_ms":2000,"words":[` +
		`{"text":"so","start_ms":0,"end_ms":400},{"text":"start","start_ms":700,"end_ms":1000},` +
		`{"text":"now","start_ms":1100,"end_ms":1500}],` +
		`"segments":[{"start_ms":0,"end_ms":1500,"first_word":0,"last_word":2}]}`

	if err := os.WriteFile(filepath.Join(m.ws, filepath.FromSlash(rel)), []byte(body), 0o600); err != nil {
		m.t.Fatal(err)
	}

	if err := m.store.Assets().PutArtifact(context.Background(), service.ArtifactRow{
		AssetHash: hash, Kind: "transcript", Profile: 1, Variant: "def456def456", Ext: "json", RelPath: rel,
		SizeBytes: int64(len(body)), CreatedMs: 1,
	}); err != nil {
		m.t.Fatal(err)
	}
}

func rangeOf(t *testing.T, res *service.EditResult) (start, end int) {
	t.Helper()

	rng, ok := res.Effective[0].Effective["range"].(map[string]any)
	if !ok {
		t.Fatalf("effective = %v", res.Effective[0].Effective)
	}

	return rng["start"].(int), rng["end"].(int)
}

// Epic 89: the service reads the silence artifact of the asset into resolved, the cut snaps to it, and the
// description of the result says where the edges went.
func TestTranscriptDeleteSnapsToTheSilencesOfTheAsset(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project, clip, _, hash := m.speechProject()
	asset := m.assetOf(project, clip)
	m.putSnapTalk(hash)

	res := m.mustEdit(ui, project, m.head(project).Revision,
		op("transcript.delete_range", fmt.Sprintf(`{"asset_id":%q,"from_word":0,"to_word":0}`, asset)))

	if start, end := rangeOf(t, res); start != 0 || end != 15 {
		t.Errorf("the cut is [%d,%d), want [0,15): the end edge moved 3 frames to the silence", start, end)
	}

	if !strings.Contains(res.Description, "end edge moved 3 frames into a silence") {
		t.Errorf("description = %q", res.Description)
	}

	stored := storedOps(t, m, project, res.Revision)
	if len(stored) != 1 || !strings.Contains(string(stored[0].Args), `"silences":[{"end":30,"start":15}]`) ||
		!strings.Contains(string(stored[0].Args), `"silence_analysis":"read"`) {
		t.Errorf("the stored operation = %v, want the resolved silences", stored)
	}
}

// Acceptance criterion 4 of epic 89: an asset analysed before the silence artifact existed still cuts, at the word
// times, and the result says that no silence analysis was available.
func TestTranscriptDeleteWithoutSilenceAnalysisCutsAtTheWordTimes(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project, clip, _, hash := m.speechProject()
	asset := m.assetOf(project, clip)
	m.putSnapTalk(hash)

	arts, err := m.store.Assets().Artifacts(context.Background(), hash)
	if err != nil {
		t.Fatal(err)
	}

	removed := 0

	for _, a := range arts {
		if a.Kind == "silence" {
			if err := os.Remove(filepath.Join(m.ws, filepath.FromSlash(a.RelPath))); err != nil {
				t.Fatal(err)
			}

			removed++
		}
	}

	if removed == 0 {
		t.Fatal("the test asset has no silence artifact to remove")
	}

	res := m.mustEdit(ui, project, m.head(project).Revision,
		op("transcript.delete_range", fmt.Sprintf(`{"asset_id":%q,"from_word":0,"to_word":0}`, asset)))

	if start, end := rangeOf(t, res); start != 0 || end != 12 {
		t.Errorf("the cut is [%d,%d), want the word cut [0,12)", start, end)
	}

	if !strings.Contains(res.Description, "no silence analysis was available") {
		t.Errorf("description = %q", res.Description)
	}

	if snap, _ := res.Effective[0].Effective["snap"].(map[string]any); snap["analysis"] != "missing" {
		t.Errorf("effective.snap = %v", snap)
	}
}

// With snap "0s" the cut is the word cut even when silences are known.
func TestTranscriptDeleteSnapOffCutsAtTheWordTimes(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project, clip, _, hash := m.speechProject()
	asset := m.assetOf(project, clip)
	m.putSnapTalk(hash)

	res := m.mustEdit(ui, project, m.head(project).Revision,
		op("transcript.delete_range", fmt.Sprintf(`{"asset_id":%q,"from_word":0,"to_word":0,"snap":"0s"}`, asset)))

	if start, end := rangeOf(t, res); start != 0 || end != 12 {
		t.Errorf("the cut is [%d,%d), want [0,12)", start, end)
	}

	if strings.Contains(res.Description, "Cut edges") {
		t.Errorf("description = %q, want no snap line", res.Description)
	}
}
