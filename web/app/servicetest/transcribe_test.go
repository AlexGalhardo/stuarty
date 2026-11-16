package servicetest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexGalhardo/vukem-video/engine/transcribe"
	"github.com/AlexGalhardo/vukem-video/service"
)

// stubProvider is a transcription provider that needs no helper. missing makes
// Check report what is not installed; the counters say what was called.
type stubProvider struct {
	missing     *transcribe.MissingError
	checks      atomic.Int64
	transcribes atomic.Int64
}

func (*stubProvider) Name() string  { return "stub" }
func (*stubProvider) Model() string { return "tiny" }

func (p *stubProvider) Check(context.Context) error {
	p.checks.Add(1)

	if p.missing != nil {
		return p.missing
	}

	return nil
}

func (p *stubProvider) Transcribe(
	_ context.Context,
	_ transcribe.Input,
	progress func(done, total time.Duration),
) (transcribe.Raw, error) {
	p.transcribes.Add(1)
	progress(time.Second, 2*time.Second)

	return transcribe.Raw{Language: "en", Words: []transcribe.RawWord{
		{Text: "hello", StartMS: 100, EndMS: 500},
		{Text: "brave", StartMS: 500, EndMS: 900},
		{Text: "world", StartMS: 1000, EndMS: 1500},
	}}, nil
}

func (m *mediaEnv) transcribeArgs(asset string) service.JobStartRequest {
	return service.JobStartRequest{Kind: service.JobTranscribe, Args: []byte(fmt.Sprintf(`{"asset_id":%q}`, asset))}
}

func (m *mediaEnv) startTranscribe(ctx context.Context, project, asset string) (*service.Job, error) {
	req := m.transcribeArgs(asset)
	req.ProjectID = project

	return m.svc.JobStart(ctx, req)
}

// A transcription job reaches succeeded with the stub provider, stores the
// artifact once, and a second start with the same key reuses it (ADR-0021 item
// 4); the words then drive caption.from_transcript.
func TestTranscribeJobSucceedsAndIsReused(t *testing.T) {
	prov := &stubProvider{}
	m := newMediaEnv(t, mediaCfg{transcriber: prov})
	project, clip, captions, hash := m.speechProject()

	asset := ""

	rows, _ := m.store.Assets().ListProjectAssets(context.Background(), project)
	for _, r := range rows {
		if r.AssetHash == hash {
			asset = r.AssetID
		}
	}

	job, err := m.startTranscribe(m.as(ui), project, asset)
	if err != nil || job.State != service.JobSucceeded || job.Result == nil || job.Result.Transcribe == nil ||
		job.Result.Transcribe.Words != 3 || job.Result.Transcribe.Language != "en" {
		t.Fatalf("job = %+v, %v", job, err)
	}

	hex := strings.TrimPrefix(hash, "sha256:")

	files, _ := filepath.Glob(filepath.Join(m.ws, "artifacts", hex, "transcript-1-*.json"))
	if len(files) != 1 {
		t.Fatalf("transcript files = %v", files)
	}

	body, _ := os.ReadFile(files[0])
	if !strings.Contains(string(body), `"untrusted":true`) || !strings.Contains(string(body), `"provider":"stub"`) {
		t.Errorf("artifact = %s", body)
	}

	again, err := m.startTranscribe(m.as(ui), project, asset)
	if err != nil || again.State != service.JobSucceeded || prov.transcribes.Load() != 1 {
		t.Errorf("second start = %+v, %v; the provider ran %d times, want 1", again, err, prov.transcribes.Load())
	}

	// The transcript now feeds an edit.
	m.mustEdit(ui, project, m.head(project).Revision, op(
		"caption.from_transcript", fmt.Sprintf(`{"clip_item_id":%q,"track_id":%q}`, clip, captions),
	))
}

// SDD-007 section 15 check 26 (T-26): with no helper installed the start fails
// at once with the size and the install command, creates no job and makes no
// call that would download anything.
func TestTranscribeWithoutTheHelperIsUnavailable(t *testing.T) {
	prov := &stubProvider{missing: &transcribe.MissingError{
		Items:         []transcribe.Item{{Name: "whisper.cpp", URLHost: "github.com", Bytes: 8_200_000}},
		DownloadBytes: 8_200_000,
	}}
	m := newMediaEnv(t, mediaCfg{transcriber: prov})
	project, _, _, hash := m.speechProject()

	row, _ := m.store.Assets().FindProjectAssetByHash(context.Background(), project, hash)

	jobs0, _ := m.svc.JobList(m.as(ui), service.JobListRequest{ProjectID: project, State: "all"})

	_, err := m.startTranscribe(m.as(ui), project, row.AssetID)

	se := serviceErr(err)
	if se == nil || se.Code != "E_TRANSCRIBE_UNAVAILABLE" ||
		se.Details["install_command"] != "vukem-video tools:install whisper --model tiny" ||
		se.Details["download_bytes"] != int64(8_200_000) ||
		se.Details["tool"] != "whisper" || se.Details["model"] != "tiny" ||
		se.Details["missing"] == nil ||
		se.Hint == "" {
		t.Fatalf("error = %+v", err)
	}

	jobs1, _ := m.svc.JobList(m.as(ui), service.JobListRequest{ProjectID: project, State: "all"})
	if len(jobs1.Jobs) != len(jobs0.Jobs) || prov.transcribes.Load() != 0 {
		t.Error("an unavailable start created a job or ran the provider")
	}

	if entries, _ := os.ReadDir(filepath.Join(m.ws, "tools")); len(entries) != 0 {
		t.Errorf("the start wrote into tools/: %v", entries)
	}
}

func TestTranscribeRefusals(t *testing.T) {
	prov := &stubProvider{}
	m := newMediaEnv(t, mediaCfg{transcriber: prov, limits: service.Limits{MaxTranscribeSeconds: 1}})
	project, _, _, hash := m.speechProject()
	row, _ := m.store.Assets().FindProjectAssetByHash(context.Background(), project, hash)

	// A 2 s asset over a 1 s ceiling.
	_, err := m.startTranscribe(m.as(ui), project, row.AssetID)

	se := serviceErr(err)
	if se == nil || se.Code != "E_LIMIT_EXCEEDED" || se.Details["limit"] != "transcribe_seconds" || se.Hint == "" {
		t.Errorf("over the ceiling = %v", err)
	}

	for name, req := range map[string]service.JobStartRequest{
		"no asset":      {ProjectID: project, Kind: service.JobTranscribe, Args: []byte(`{}`)},
		"unknown field": {ProjectID: project, Kind: service.JobTranscribe, Args: []byte(`{"asset_id":"x","nope":1}`)},
		"bad language":  {ProjectID: project, Kind: service.JobTranscribe, Args: []byte(`{"asset_id":"x","language":"!!"}`)},
		"unknown asset": {ProjectID: project, Kind: service.JobTranscribe, Args: []byte(`{"asset_id":"ast_0000000001"}`)},
	} {
		if _, err := m.svc.JobStart(m.as(ui), req); code(err) == "" {
			t.Errorf("%s: accepted", name)
		}
	}

	if prov.transcribes.Load() != 0 {
		t.Error("a refused start ran the provider")
	}
}

func TestTranscribeNeedsTheMediaScopeAndOnlyThat(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{transcriber: &stubProvider{}})
	project, _, _, hash := m.speechProject()
	row, _ := m.store.Assets().FindProjectAssetByHash(context.Background(), project, hash)

	mediaOnly := service.WithCaller(context.Background(), service.Caller{
		Actor: "key:media", Scopes: []string{service.ScopeRead, service.ScopeMedia},
	})
	if _, err := m.startTranscribe(mediaOnly, project, row.AssetID); err != nil {
		t.Errorf("a media key: %v", err)
	}

	renderOnly := service.WithCaller(context.Background(), service.Caller{
		Actor: "key:render", Scopes: []string{service.ScopeRead, service.ScopeRender},
	})
	if _, err := m.startTranscribe(renderOnly, project, row.AssetID); code(err) != service.CodeForbidden {
		t.Errorf("a render-only key = %v, want E_FORBIDDEN", err)
	}
}

func TestTranscribeConcurrencyIsBounded(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{transcriber: &stubProvider{}})
	project, _, _, hash := m.speechProject()
	row, _ := m.store.Assets().FindProjectAssetByHash(context.Background(), project, hash)

	// The queue drops what it is given: the first job stays queued.
	m.hold.Store(true)

	first, err := m.startTranscribe(m.as(ui), project, row.AssetID)
	if err != nil || first.State != service.JobQueued {
		t.Fatalf("first = %+v, %v", first, err)
	}

	if _, err := m.startTranscribe(m.as(ui), project, row.AssetID); code(err) != service.CodeQueueFull {
		t.Errorf("second concurrent transcription = %v, want E_QUEUE_FULL", err)
	}
}

// One actor cannot hold the transcription slots: with room for three, a key
// that already has one queued gets E_QUEUE_FULL for a second, and another key
// still gets its own.
func TestTranscribeLimitPerActor(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{
		transcriber: &stubProvider{}, limits: service.Limits{MaxTranscribes: 3, MaxTranscribesPerActor: 1},
	})
	project, _, _, hash := m.speechProject()
	row, _ := m.store.Assets().FindProjectAssetByHash(context.Background(), project, hash)

	keyA := service.WithCaller(context.Background(), service.Caller{Actor: "key:a", Scopes: service.AllScopes})
	keyB := service.WithCaller(context.Background(), service.Caller{Actor: "key:b", Scopes: service.AllScopes})

	m.hold.Store(true)

	if _, err := m.startTranscribe(keyA, project, row.AssetID); err != nil {
		t.Fatal(err)
	}

	if _, err := m.startTranscribe(keyA, project, row.AssetID); code(err) != service.CodeQueueFull {
		t.Errorf("a second transcription of the same key = %v, want E_QUEUE_FULL", err)
	}

	if _, err := m.startTranscribe(keyB, project, row.AssetID); err != nil {
		t.Errorf("another key: %v", err)
	}
}

// SDD-003 section 3.8: the inspection says "none yet" only when a provider can
// run; without one it says unavailable and how to install it.
func TestInspectTranscriptSectionFollowsTheProvider(t *testing.T) {
	missing := &transcribe.MissingError{
		Items:         []transcribe.Item{{Name: "whisper.cpp", URLHost: "github.com", Bytes: 8_200_000}},
		DownloadBytes: 8_200_000,
	}

	for name, tc := range map[string]struct {
		prov *stubProvider
		want string
	}{
		"ready":   {&stubProvider{}, "transcript: none yet (start a transcribe job for this asset)"},
		"missing": {&stubProvider{missing: missing}, "transcript: unavailable (E_TRANSCRIBE_UNAVAILABLE): Run \"vukem-video tools:install whisper --model tiny\""},
	} {
		t.Run(name, func(t *testing.T) {
			m := newMediaEnv(t, mediaCfg{transcriber: tc.prov})
			project, _, _, hash := m.speechProject()
			row, _ := m.store.Assets().FindProjectAssetByHash(context.Background(), project, hash)

			res, err := m.svc.MediaInspect(m.as(ui), service.MediaInspectRequest{
				ProjectID: project, AssetID: row.AssetID, Sections: []string{"transcript"},
			})
			if err != nil || !strings.Contains(res.Text, tc.want) {
				t.Errorf("text = %q, %v; want it to contain %q", res.Text, err, tc.want)
			}
		})
	}
}

// pickerProvider is a stub that can run the models it lists as installed.
type pickerProvider struct {
	stubProvider
	model     string
	installed []string
	root      *pickerProvider
	used      atomic.Value
}

func (p *pickerProvider) Model() string { return p.model }

func (p *pickerProvider) InstalledModels() []string { return p.installed }

func (p *pickerProvider) WithModel(model string) transcribe.Provider {
	return &pickerProvider{model: model, installed: p.installed, root: p}
}

func (p *pickerProvider) Transcribe(
	ctx context.Context, in transcribe.Input, progress func(done, total time.Duration),
) (transcribe.Raw, error) {
	if p.root != nil {
		p.root.used.Store(p.model)
	}

	return p.stubProvider.Transcribe(ctx, in, progress)
}

func TestTranscribeModelIsCheckedAgainstTheInstalledOnes(t *testing.T) {
	prov := &pickerProvider{model: "small", installed: []string{"base"}}
	m := newMediaEnv(t, mediaCfg{transcriber: prov})
	project, _, _, hash := m.speechProject()
	row, _ := m.store.Assets().FindProjectAssetByHash(context.Background(), project, hash)

	for name, model := range map[string]string{
		"known but not installed": "tiny",
		"unknown name":            "huge",
		"a path":                  "../models/ggml-base.bin",
	} {
		req := service.JobStartRequest{
			ProjectID: project, Kind: service.JobTranscribe,
			Args: []byte(fmt.Sprintf(`{"asset_id":%q,"model":%q}`, row.AssetID, model)),
		}

		_, err := m.svc.JobStart(m.as(ui), req)

		se := serviceErr(err)
		if se == nil || se.Code != "E_VALIDATION" || se.Path != "args.model" || !strings.Contains(se.Hint, "base") {
			t.Errorf("%s: error %v, want E_VALIDATION on args.model listing the installed model", name, err)
		}
	}

	job, err := m.svc.JobStart(m.as(ui), service.JobStartRequest{
		ProjectID: project, Kind: service.JobTranscribe, WaitMS: 20000,
		Args: []byte(fmt.Sprintf(`{"asset_id":%q,"model":"base"}`, row.AssetID)),
	})
	if err != nil || job.State != "succeeded" {
		t.Fatalf("installed model: job %+v, error %v", job, err)
	}

	if got := prov.used.Load(); got != "base" {
		t.Errorf("the job ran on model %v, want base", got)
	}
}

func TestTranscribeModelNeedsAProviderThatTakesOne(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{transcriber: &stubProvider{}})
	project, _, _, hash := m.speechProject()
	row, _ := m.store.Assets().FindProjectAssetByHash(context.Background(), project, hash)

	_, err := m.svc.JobStart(m.as(ui), service.JobStartRequest{
		ProjectID: project, Kind: service.JobTranscribe,
		Args: []byte(fmt.Sprintf(`{"asset_id":%q,"model":"base"}`, row.AssetID)),
	})

	if se := serviceErr(err); se == nil || se.Code != "E_VALIDATION" || se.Path != "args.model" {
		t.Errorf("error %v, want E_VALIDATION on args.model", err)
	}
}
