package servicetest

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/core/timeline"
	"github.com/AlexGalhardo/vukem-video/service"
)

// speechProject is a project with a 2 s imported clip on a visual track and a
// second visual track for captions.
func (m *mediaEnv) speechProject() (project, clip, captionTrack, hash string) {
	m.t.Helper()

	project, track := m.withTrack()
	asset := m.importOne(project, m.write("speech.mp4", clipBytes)).Assets[0].AssetID

	row, err := m.store.Assets().GetProjectAsset(context.Background(), project, asset)
	if err != nil {
		m.t.Fatal(err)
	}

	added := m.mustEdit(ui, project, m.head(project).Revision, op(
		"clip.add", fmt.Sprintf(`{"track_id":%q,"asset_id":%q,"start":"0s","duration":"2s"}`, track, asset),
	), op("track.add", `{"kind":"visual","name":"C"}`))

	for _, id := range added.Created {
		switch {
		case strings.HasPrefix(id, "itm_") && clip == "":
			clip = id
		case strings.HasPrefix(id, "trk_"):
			captionTrack = id
		}
	}

	return project, clip, captionTrack, row.AssetHash
}

// putTranscript stores a transcript artifact for the asset, as the transcribe
// job will (SDD-011 section 10.5).
func (m *mediaEnv) putTranscript(hash string) {
	m.t.Helper()

	hex := strings.TrimPrefix(hash, "sha256:")
	rel := "artifacts/" + hex + "/transcript-1-abc123abc123.json"
	body := `{"version":1,"audio_ms":2000,"words":[` +
		`{"text":"hello","start_ms":0,"end_ms":400},{"text":"brave","start_ms":400,"end_ms":900},` +
		`{"text":"world","start_ms":1000,"end_ms":1500}]}`

	if err := os.WriteFile(filepath.Join(m.ws, filepath.FromSlash(rel)), []byte(body), 0o600); err != nil {
		m.t.Fatal(err)
	}

	if err := m.store.Assets().PutArtifact(context.Background(), service.ArtifactRow{
		AssetHash: hash, Kind: "transcript", Profile: 1, Variant: "abc123abc123", Ext: "json", RelPath: rel,
		SizeBytes: int64(len(body)), CreatedMs: 1,
	}); err != nil {
		m.t.Fatal(err)
	}
}

func storedOps(t *testing.T, m *mediaEnv, project string, revision int64) []command.Operation {
	t.Helper()

	row, err := m.store.Operations().Get(context.Background(), project, revision)
	if err != nil {
		t.Fatal(err)
	}

	var ops []command.Operation
	if err := json.Unmarshal(row.Ops, &ops); err != nil {
		t.Fatal(err)
	}

	return ops
}

// RF-016: the silences come from the artifact of the asset, the batch is one
// undo step, and the stored operation carries what it was resolved with.
func TestResolveSilencesFromTheArtifact(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project, clip, _, _ := m.speechProject()
	rev := m.head(project).Revision

	res := m.mustEdit(ui, project, rev, op(
		"clip.remove_silences", fmt.Sprintf(`{"item_id":%q,"min_silence":"0.1s","padding":"0s"}`, clip),
	))

	doc, _ := timeline.Decode(m.document(project))

	clips := 0

	for _, it := range doc.Items {
		if it.Type == timeline.ItemClip {
			clips++
		}
	}

	if clips != 2 || res.Revision != rev+1 {
		t.Errorf("clips = %d, revision %d; one clip becomes two in one revision", clips, res.Revision)
	}

	stored := storedOps(t, m, project, rev+1)
	if len(stored) != 1 ||
		!strings.Contains(string(stored[0].Args), `"resolved":{"silences":[{"end":30,"start":15}]}`) {
		t.Errorf("stored operation = %s", stored[0].Args)
	}

	if _, err := m.undo(ui, project); err != nil {
		t.Fatal(err)
	}

	after, _ := timeline.Decode(m.document(project))

	clips = 0

	for _, it := range after.Items {
		if it.Type == timeline.ItemClip {
			clips++
		}
	}

	if clips != 1 {
		t.Errorf("one undo left %d clips, want 1", clips)
	}
}

// SDD-007 section 15 check 26, last clause: without a transcript the edit
// names the job to run and creates none.
func TestResolveWithoutATranscriptNamesTheJob(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project, clip, captions, _ := m.speechProject()
	jobsBefore, _ := m.svc.JobList(m.as(ui), service.JobListRequest{ProjectID: project, State: "all"})

	_, err := m.edit(ui, project, m.head(project).Revision, op(
		"caption.from_transcript", fmt.Sprintf(`{"clip_item_id":%q,"track_id":%q}`, clip, captions),
	))

	se := serviceErr(err)
	if se == nil || se.Code != command.CodeInvalidState || !strings.Contains(se.Hint, "job_start") ||
		!strings.Contains(se.Hint, "transcribe") || se.OpIndex == nil || *se.OpIndex != 0 {
		t.Fatalf("error = %+v", err)
	}

	jobsAfter, _ := m.svc.JobList(m.as(ui), service.JobListRequest{ProjectID: project, State: "all"})
	if len(jobsAfter.Jobs) != len(jobsBefore.Jobs) {
		t.Error("an edit created a job")
	}
}

// With a transcript the captions come from its words, and replaying the log
// from revision 0 gives the same document bytes (TestReplayWithResolved).
func TestReplayWithResolved(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project, clip, captions, hash := m.speechProject()
	m.putTranscript(hash)

	rev := m.head(project).Revision
	m.mustEdit(ui, project, rev, op(
		"caption.from_transcript", fmt.Sprintf(`{"clip_item_id":%q,"track_id":%q}`, clip, captions),
	))

	last := m.head(project).Revision
	if !strings.Contains(string(storedOps(t, m, project, last)[0].Args), `"resolved":{"words":[`) {
		t.Fatalf("the stored operation lacks resolved.words: %s", storedOps(t, m, project, last)[0].Args)
	}

	// Replay: the empty document of ProjectCreate, then every stored batch.
	doc := &timeline.Document{
		SchemaVersion: timeline.CurrentSchemaVersion, ID: project, Name: "p", Settings: timeline.DefaultSettings(),
		Assets: map[string]timeline.Asset{}, Tracks: map[string]timeline.Track{}, Items: map[string]timeline.Item{},
	}

	reg := command.Default()

	for r := int64(1); r <= last; r++ {
		ops := storedOps(t, m, project, r)

		out, err := reg.ApplyBatch(doc, ops, command.Env{NewIDs: func(string) string { return "" }})
		if err != nil {
			t.Fatalf("replay of r%d: %v", r, err)
		}

		doc = out.Doc
	}

	if got, want := string(timeline.Canonical(doc)), string(m.document(project)); got != want {
		t.Errorf("replay differs:\n got  %s\n want %s", got, want)
	}
}

func TestResolveRefusesASilenceEditWhenTheAssetHasNoAnalysis(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project, clip, _, hash := m.speechProject()

	hex := strings.TrimPrefix(hash, "sha256:")
	if err := os.Remove(filepath.Join(m.ws, "artifacts", hex, "silence-1.json")); err != nil {
		t.Fatal(err)
	}

	_, err := m.edit(ui, project, m.head(project).Revision, op(
		"clip.remove_silences", fmt.Sprintf(`{"item_id":%q}`, clip),
	))
	if se := serviceErr(err); se == nil || se.Code != command.CodeInvalidState || se.Hint == "" {
		t.Errorf("without the silence artifact = %v", err)
	}
}
