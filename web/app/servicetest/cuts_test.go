package servicetest

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/facetrack"
	"github.com/AlexGalhardo/stuarty/service"
)

// putLongTalk stores a transcript of sentences of 8 words, 3 s long, 4 s apart, for a minute.
func (m *mediaEnv) putLongTalk(hash string) {
	m.t.Helper()

	var words []string

	for k := range 15 {
		for w := range 8 {
			text := "word"
			if w == 7 {
				text = "end."
			}

			start := k*4000 + w*375
			words = append(words, fmt.Sprintf(`{"text":%q,"start_ms":%d,"end_ms":%d}`, text, start, start+350))
		}
	}

	hex := strings.TrimPrefix(hash, "sha256:")
	rel := "artifacts/" + hex + "/transcript-1-def456def456.json"
	body := `{"version":1,"language":"en","audio_ms":60000,"words":[` + strings.Join(words, ",") + `],` +
		`"segments":[{"start_ms":0,"end_ms":59000,"first_word":0,"last_word":119}]}`

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

// putFaces stores a face track of a minute at 1 sample per second, with no face between absentFrom and absentTo.
func (m *mediaEnv) putFaces(hash string, absentFrom, absentTo int) {
	m.t.Helper()

	tr := facetrack.Track{
		Version: facetrack.Version, Model: "yunet-2023mar", Provider: "cpu",
		Source: facetrack.Size{Width: 320, Height: 240}, FPS: 1, EndMS: 60_000,
	}

	for s := range 60 {
		sample := facetrack.Sample{T: s * 1000}
		if s < absentFrom || s >= absentTo {
			sample.Faces = []facetrack.Face{{X: 4000, Y: 2000, W: 1500, H: 3000, Score: 900}}
		}

		tr.Samples = append(tr.Samples, sample)
	}

	body, err := json.Marshal(tr)
	if err != nil {
		m.t.Fatal(err)
	}

	hex := strings.TrimPrefix(hash, "sha256:")
	rel := "artifacts/" + hex + "/facetrack-1-manual.json"

	if err := os.WriteFile(filepath.Join(m.ws, filepath.FromSlash(rel)), body, 0o600); err != nil {
		m.t.Fatal(err)
	}

	if err := m.store.Assets().PutArtifact(context.Background(), service.ArtifactRow{
		AssetHash: hash, Kind: "facetrack", Profile: 1, Variant: "manual", Ext: "json", RelPath: rel,
		SizeBytes: int64(len(body)), CreatedMs: 1,
	}); err != nil {
		m.t.Fatal(err)
	}
}

// Acceptance criterion 5 of epic 70: a proposal changes nothing until a person accepts it.
func TestCutsProposeNeedsTheTranscriptAndTheFaceTrackAndChangesNothing(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project, clip, _, hash := m.speechProject()
	asset := m.assetOf(project, clip)
	req := service.CutsProposeRequest{ProjectID: project, AssetID: asset, Count: 3, MinMS: 8_000, MaxMS: 12_000}

	if _, err := m.svc.CutsPropose(
		m.as(ui),
		req,
	); code(err) != "E_INVALID_STATE" ||
		!strings.Contains(err.Error(), "transcript") {
		t.Fatalf("no transcript: %v", err)
	}

	m.putLongTalk(hash)

	if _, err := m.svc.CutsPropose(
		m.as(ui),
		req,
	); code(err) != "E_INVALID_STATE" ||
		!strings.Contains(err.Error(), "face") {
		t.Fatalf("no face track: %v", err)
	}

	m.putFaces(hash, 20, 30)

	before := m.head(project)
	docBefore := string(m.document(project))

	res, err := m.svc.CutsPropose(m.as(ui), req)
	if err != nil {
		t.Fatal(err)
	}

	if len(res.Proposals) == 0 || len(res.Proposals) > 3 {
		t.Fatalf("%d proposals", len(res.Proposals))
	}

	for _, p := range res.Proposals {
		if p.EndMS-p.StartMS < 8_000 || p.EndMS-p.StartMS > 12_000 || p.FaceShare < 700 || len(p.Batch) != 4 ||
			p.Batch[3].Op != "clip.face_frame" || p.Title == "" {
			t.Errorf("proposal = %+v", p)
		}

		if p.StartMS >= 20_000 && p.EndMS <= 30_000 {
			t.Errorf("a proposal lies where nobody is on screen: %+v", p)
		}
	}

	if !strings.Contains(res.Text, "nothing was changed") || res.UntrustedNotice == "" {
		t.Errorf("text = %q", res.Text)
	}

	if after := m.head(project); after.Revision != before.Revision || string(m.document(project)) != docBefore {
		t.Errorf("the revision went from %d to %d, or the document changed", before.Revision, after.Revision)
	}

	// Asking again gives the same proposals: the answer depends on the two artifacts only.
	again, err := m.svc.CutsPropose(m.as(ui), req)
	if err != nil || len(again.Proposals) != len(res.Proposals) ||
		again.Proposals[0].StartMS != res.Proposals[0].StartMS {
		t.Errorf("second answer = %+v, %v", again, err)
	}
}

func TestCutsProposeNeedsTheEditScopeAndValidOptions(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project, clip, _, hash := m.speechProject()
	m.putLongTalk(hash)
	m.putFaces(hash, 0, 0)

	asset := m.assetOf(project, clip)
	read := service.Caller{Actor: "key:r", Scopes: []string{service.ScopeRead}}

	if _, err := m.svc.CutsPropose(service.WithCaller(context.Background(), read), service.CutsProposeRequest{
		ProjectID: project, AssetID: asset,
	}); code(err) != service.CodeForbidden {
		t.Errorf("a read key = %v, want E_FORBIDDEN", err)
	}

	for name, r := range map[string]service.CutsProposeRequest{
		"count":    {ProjectID: project, AssetID: asset, Count: 99},
		"min>max":  {ProjectID: project, AssetID: asset, MinMS: 9_000, MaxMS: 5_000},
		"no asset": {ProjectID: project, AssetID: "ast_zzzzzzzzzz"},
	} {
		if _, err := m.svc.CutsPropose(m.as(ui), r); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
