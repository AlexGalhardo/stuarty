package servicetest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/ingest"
	"github.com/AlexGalhardo/stuarty/service"

	"github.com/AlexGalhardo/vukem/queue"
)

const clipBytes = "not really a clip, but long enough for the store"

func (m *mediaEnv) importOne(project, path string) *service.MediaImportResult {
	m.t.Helper()

	res, err := m.svc.MediaImport(m.as(ui), service.MediaImportRequest{
		ProjectID: project, Files: []service.ImportFile{{Path: path}},
	})
	if err != nil {
		m.t.Fatalf("MediaImport: %v", err)
	}

	return res
}

func TestImportRunsTheIngestAndAddsTheAssetToTheDocument(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project := m.newProject("p")
	clip := m.write("clip.mp4", clipBytes)

	res := m.importOne(project, clip)

	if len(res.Assets) != 1 || res.Failed != 0 {
		t.Fatalf("result = %+v", res)
	}

	a := res.Assets[0]
	if a.State != "ready" || a.Kind != "video" || a.Name != "clip.mp4" || a.JobID == "" || a.Deduplicated ||
		a.Probe == nil {
		t.Fatalf("entry = %+v", a)
	}

	if a.Probe.Width != 320 || a.Probe.VideoCodec != "h264" || !a.Probe.HasAudio || a.Probe.Duration == nil ||
		a.Probe.Duration.Frame != 60 {
		t.Errorf("probe = %+v", a.Probe)
	}

	if strings.Contains(res.Text, m.files) {
		t.Errorf("the text names the directory of the path: %s", res.Text)
	}

	// The asset is in the document through an asset.add batch of the importer.
	doc, err := timeline.Decode(m.document(project))
	if err != nil {
		t.Fatal(err)
	}

	got, ok := doc.Assets[a.AssetID]
	if !ok || got.Width != 320 || got.FrameCount != 60 || got.SampleCount != 96000 || !got.HasAudio ||
		got.Kind != timeline.AssetVideo {
		t.Fatalf("asset in the document = %+v (%v)", got, ok)
	}

	if res.Revision != 1 || m.head(project).Revision != 1 {
		t.Errorf("revision = %d, head %d, want 1", res.Revision, m.head(project).Revision)
	}

	rows, _ := m.store.Assets().ListProjectAssets(context.Background(), project)
	if len(rows) != 1 || rows[0].State != service.ProjectAssetActive {
		t.Errorf("project assets = %+v", rows)
	}

	// The job is done and has no result member; the events tell the story.
	job, err := m.svc.JobGet(m.as(ui), service.JobGetRequest{JobID: a.JobID})
	if err != nil || job.State != service.JobSucceeded || job.ProjectID != nil || job.Result != nil {
		t.Fatalf("job = %+v, %v", job, err)
	}

	var states []string
	for _, ev := range m.eventsOf("asset.updated") {
		if ev.AssetID == a.AssetID && ev.ProjectID == project {
			states = append(states, ev.State)
		}
	}

	if len(states) < 3 || states[0] != "pending" || states[1] != "ingesting" || states[len(states)-1] != "ready" {
		t.Errorf("asset.updated states = %v", states)
	}

	if j := m.eventsOf("job.updated"); len(j) < 3 || j[0].State != "queued" || j[len(j)-1].State != "succeeded" ||
		j[0].ProjectID != project {
		t.Errorf("job.updated = %+v", j)
	}
}

func TestImportingTheSameBytesTwice(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	p1, p2 := m.newProject("one"), m.newProject("two")
	clip := m.write("clip.mp4", clipBytes)
	copyOf := m.write("copy.mp4", clipBytes)

	first := m.importOne(p1, clip).Assets[0]
	again := m.importOne(p1, copyOf).Assets[0]

	if again.AssetID != first.AssetID || !again.Deduplicated || again.JobID != "" || again.State != "ready" {
		t.Errorf("same project: first %+v, again %+v", first, again)
	}

	other := m.importOne(p2, clip).Assets[0]
	if other.AssetID == first.AssetID || other.State != "ready" || other.JobID != "" || !other.Deduplicated {
		t.Errorf("other project: %+v (first %+v)", other, first)
	}

	if m.ingests.Load() != 1 {
		t.Errorf("ingest ran %d times for one file", m.ingests.Load())
	}

	// The second project got the asset in its document without a job.
	doc, _ := timeline.Decode(m.document(p2))
	if _, ok := doc.Assets[other.AssetID]; !ok {
		t.Error("the asset is not in the document of the second project")
	}
}

func TestOneIngestServesEveryProjectThatWaits(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{noQueue: true})
	p1, p2 := m.newProject("one"), m.newProject("two")
	clip := m.write("clip.mp4", clipBytes)

	a1 := m.importOne(p1, clip).Assets[0]
	a2 := m.importOne(p2, clip).Assets[0]

	if a1.JobID == "" || a1.JobID != a2.JobID || a1.AssetID == a2.AssetID || a1.State != "pending" {
		t.Fatalf("a1 = %+v, a2 = %+v: one job, two identifiers", a1, a2)
	}

	if err := m.runner.RunJob(context.Background(), a1.JobID); err != nil {
		t.Fatal(err)
	}

	for p, a := range map[string]service.ImportedAsset{p1: a1, p2: a2} {
		doc, _ := timeline.Decode(m.document(p))
		if _, ok := doc.Assets[a.AssetID]; !ok {
			t.Errorf("project %s lacks its asset", p)
		}
	}

	// job.updated of the ingest reached the stream of both projects.
	seen := map[string]bool{}
	for _, ev := range m.eventsOf("job.updated") {
		if ev.JobID == a1.JobID {
			seen[ev.ProjectID] = true
		}
	}

	if !seen[p1] || !seen[p2] {
		t.Errorf("job.updated streams = %v", seen)
	}
}

func TestMediaListShowsEveryAssetIncludingPendingAndFailed(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{noQueue: true})
	project := m.newProject("p")
	m.importOne(project, m.write("a.mp4", clipBytes))

	res, err := m.svc.MediaList(m.as(ui), service.MediaListRequest{ProjectID: project})
	if err != nil || len(res.Assets) != 1 {
		t.Fatalf("list = %+v, %v", res, err)
	}

	if e := res.Assets[0]; e.State != "pending" || e.Name != "a.mp4" || e.Trashed {
		t.Errorf("entry = %+v", e)
	}

	if !strings.Contains(res.Text, "pending") || !strings.Contains(res.Text, `"a.mp4"`) {
		t.Errorf("text = %s", res.Text)
	}

	insp, err := m.svc.MediaInspect(
		m.as(ui),
		service.MediaInspectRequest{ProjectID: project, AssetID: res.Assets[0].AssetID},
	)
	if err != nil || insp.State != "pending" || insp.Probe != nil {
		t.Errorf("an asset that is not ready answers with its state: %+v, %v", insp, err)
	}
}

func TestMediaInspectSections(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project := m.newProject("p")
	a := m.importOne(project, m.write("a.mp4", clipBytes)).Assets[0]

	res, err := m.svc.MediaInspect(m.as(ui), service.MediaInspectRequest{
		ProjectID: project, AssetID: a.AssetID, Sections: []string{"probe", "loudness", "silences", "transcript"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if res.Strip == nil || res.Strip.Count != 2 || res.Loudness == nil || *res.Loudness.IntegratedLUFS != -17.2 {
		t.Errorf("result = %+v", res)
	}

	// 24000..48000 samples at 48 kHz is 0.5 s to 1 s: 15 to 30 whole frames at 30 fps.
	if len(res.Silences) != 1 || res.Silences[0].Start.Frame != 15 || res.Silences[0].End.Frame != 30 {
		t.Errorf("silences = %+v", res.Silences)
	}

	for _, want := range []string{"probe: mov,mp4 | 320x240 | 30/1fps", "loudness: integrated -17.2 LUFS", "silences (min 400ms", "transcript: unavailable (E_TRANSCRIBE_UNAVAILABLE)"} {
		if !strings.Contains(res.Text, want) {
			t.Errorf("text lacks %q:\n%s", want, res.Text)
		}
	}

	// VV-097: the beat grid is visible, with the tempo and the first beats (0, 0.5, 1 and 1.5 s).
	beats, err := m.svc.MediaInspect(m.as(ui), service.MediaInspectRequest{
		ProjectID: project, AssetID: a.AssetID, Sections: []string{"beats"},
	})
	if err != nil || beats.Beats == nil || beats.Beats.TempoBPM != 120 || beats.Beats.Count != 4 ||
		!slices.Equal(beats.Beats.FirstMS, []int64{0, 500, 1000, 1500}) ||
		!strings.Contains(beats.Text, "beats: 120.0 BPM") {
		t.Errorf("beats = %+v, %v %s", beats.Beats, err, beats.Text)
	}

	if _, err := m.svc.MediaInspect(m.as(ui), service.MediaInspectRequest{
		ProjectID: project, AssetID: a.AssetID, Sections: []string{"colour"},
	}); code(err) != command.CodeValidation {
		t.Errorf("unknown section: %v", err)
	}

	if _, err := m.svc.MediaInspect(m.as(ui), service.MediaInspectRequest{
		ProjectID: project, AssetID: "ast_0000009999",
	}); code(err) != command.CodeNotFound {
		t.Errorf("unknown asset: %v", err)
	}
}

func TestOpenFileResolvesTheArtifactsOfAReadyAsset(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project := m.newProject("p")
	a := m.importOne(project, m.write("a.mp4", clipBytes)).Assets[0]

	cases := map[string]struct {
		ref   service.FileRef
		ctype string
	}{
		"proxy":     {service.FileRef{Kind: "proxy"}, "video/mp4"},
		"thumbnail": {service.FileRef{Kind: "thumbnail", Variant: "320"}, "image/jpeg"},
		"strip":     {service.FileRef{Kind: "strip"}, "image/jpeg"},
		"peaks":     {service.FileRef{Kind: "peaks"}, "application/octet-stream"},
		"chunks":    {service.FileRef{Kind: "chunks"}, "application/json"},
	}

	for name, c := range cases {
		c.ref.ProjectID, c.ref.ID = project, a.AssetID

		f, err := m.svc.OpenFile(m.as(ui), c.ref)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}

		if f.ContentType != c.ctype || !strings.HasPrefix(f.Path, "artifacts/") || f.Size == 0 || f.ETag == "" {
			t.Errorf("%s: %+v", name, f)
		}

		if _, err := os.Stat(filepath.Join(m.ws, filepath.FromSlash(f.Path))); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}

	if _, err := m.svc.OpenFile(m.as(ui), service.FileRef{
		Kind: "thumbnail", Variant: "999", ProjectID: project, ID: a.AssetID,
	}); code(err) != command.CodeValidation {
		t.Errorf("a width that is not offered: %v", err)
	}
}

func TestImportRefusalsAreEntriesOfTheCall(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project := m.newProject("p")
	good := m.write("good.mp4", clipBytes)
	empty := m.write("empty.mp4", "")

	// One file that is refused is the error of the call.
	_, err := m.svc.MediaImport(m.as(ui), service.MediaImportRequest{
		ProjectID: project, Files: []service.ImportFile{{Path: "http://169.254.169.254/x.mp4"}},
	})
	if code(err) != "E_PATH_NOT_ALLOWED" {
		t.Errorf("a URL: %v", err)
	}

	// Several files: the call succeeds, the refusals are entries, in order.
	res, err := m.svc.MediaImport(m.as(ui), service.MediaImportRequest{
		ProjectID: project, Files: []service.ImportFile{
			{Path: filepath.Join(m.files, "missing.mp4")}, {Path: good}, {Path: empty}, {Path: "relative.mp4"},
		},
	})
	if err != nil || res.Failed != 3 || len(res.Assets) != 4 {
		t.Fatalf("result = %+v, %v", res, err)
	}

	want := []string{"E_FILE_NOT_FOUND", "", "E_UNSUPPORTED_MEDIA", "E_PATH_NOT_ALLOWED"}
	for i, a := range res.Assets {
		got := ""
		if a.Error != nil {
			got = a.Error.Code
		}

		if got != want[i] {
			t.Errorf("entry %d: %q, want %q (%+v)", i, got, want[i], a)
		}
	}

	if res.Imported != 1 || res.Imported+res.Failed != len(res.Assets) {
		t.Errorf("imported = %d, failed = %d", res.Imported, res.Failed)
	}

	if res.Assets[1].State != "ready" || !strings.Contains(res.Text, "3 of 4 files failed, 1 imported") {
		t.Errorf("good entry %+v\n%s", res.Assets[1], res.Text)
	}

	for _, a := range res.Assets {
		if strings.Contains(a.Name, string(filepath.Separator)) || a.Name == "" {
			t.Errorf("name %q holds a directory or is empty", a.Name)
		}
	}
}

func TestImportLimits(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{limits: service.Limits{MaxImportBytes: 20}})
	project := m.newProject("p")

	_, err := m.svc.MediaImport(m.as(ui), service.MediaImportRequest{
		ProjectID: project, Files: []service.ImportFile{{Path: m.write("big.mp4", clipBytes)}},
	})

	se := serviceError(t, err)
	if se.Code != "E_LIMIT_EXCEEDED" || se.Details["limit"] != "import_bytes" || se.Details["ceiling"] != int64(20) ||
		se.Details["variable"] != "STUARTY_MAX_IMPORT_BYTES" {
		t.Errorf("error = %+v", se)
	}

	if entries, _ := os.ReadDir(filepath.Join(m.ws, "store")); len(entries) != 0 {
		t.Errorf("a refused file left something in the store: %v", entries)
	}
}

func TestImportScopesAndProjectState(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project := m.newProject("p")
	clip := m.write("a.mp4", clipBytes)
	req := service.MediaImportRequest{ProjectID: project, Files: []service.ImportFile{{Path: clip}}}

	readOnly := service.WithCaller(
		context.Background(),
		service.Caller{Actor: "key:k", Scopes: []string{service.ScopeRead}},
	)
	if _, err := m.svc.MediaImport(readOnly, req); code(err) != service.CodeForbidden {
		t.Errorf("a caller without the media scope: %v", err)
	}

	if _, err := m.svc.ProjectTrash(m.as(ui), service.ProjectTrashRequest{ProjectID: project}); err != nil {
		t.Fatal(err)
	}

	if _, err := m.svc.MediaImport(m.as(ui), req); code(err) != command.CodeInvalidState {
		t.Errorf("a trashed project: %v", err)
	}

	if _, err := m.svc.MediaImport(
		m.as(ui),
		service.MediaImportRequest{ProjectID: project},
	); code(
		err,
	) != command.CodeValidation {
		t.Errorf("no files: %v", err)
	}
}

func TestDryRunCopiesNothing(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project := m.newProject("p")

	res, err := m.svc.MediaImport(m.as(ui), service.MediaImportRequest{
		ProjectID: project, DryRun: true, Files: []service.ImportFile{{Path: m.write("a.mp4", clipBytes)}},
	})
	if err != nil || res.Assets[0].AssetID != "" {
		t.Fatalf("dry run = %+v, %v", res, err)
	}

	if entries, _ := os.ReadDir(filepath.Join(m.ws, "store")); len(entries) != 0 {
		t.Errorf("the store holds %v", entries)
	}

	if rows, _ := m.store.Assets().ListProjectAssets(context.Background(), project); len(rows) != 0 {
		t.Errorf("rows = %+v", rows)
	}
}

// A text file renamed .mp4 ends with the asset failed and E_UNSUPPORTED_MEDIA,
// and the event the UI listens to carries the hint (P1-SRV-004 assert 3).
func TestImportingATextFileRenamedMp4(t *testing.T) {
	// The real ingest: the file is refused by its first bytes, before any process starts.
	m := newMediaEnv(t, mediaCfg{realIngest: true})

	project := m.newProject("p")
	text := m.write("notes.mp4", "this is a text file that somebody renamed to .mp4 and nothing else")

	res := m.importOne(project, text)
	a := res.Assets[0]

	if a.State != "failed" || a.Error == nil || a.Error.Code != "E_UNSUPPORTED_MEDIA" {
		t.Fatalf("entry = %+v", a)
	}

	// VV-193: the bytes are now in the store, but a file that did not enter is not "deduplicated" the
	// second time it is refused.
	if again := m.importOne(m.newProject("q"), text).Assets[0]; again.Deduplicated || again.Error == nil {
		t.Errorf("second refusal = %+v", again)
	}

	// VV-004: a failed ingest counts, so the CLI exits 1.
	if res.Failed != 1 {
		t.Errorf("failed = %d, want 1", res.Failed)
	}

	list, err := m.svc.MediaList(m.as(ui), service.MediaListRequest{ProjectID: project})
	if err != nil || list.Assets[0].State != "failed" || list.Assets[0].Error == nil ||
		list.Assets[0].Error.Code != "E_UNSUPPORTED_MEDIA" || list.Assets[0].Error.Hint == "" {
		t.Fatalf("list = %+v, %v", list, err)
	}

	var failed *service.Event

	for _, ev := range m.eventsOf("asset.updated") {
		if ev.State == "failed" {
			failed = &ev
		}
	}

	if failed == nil || failed.Error == nil || failed.Error.Code != "E_UNSUPPORTED_MEDIA" || failed.Error.Hint == "" {
		t.Errorf("the failed event = %+v", failed)
	}

	job, _ := m.svc.JobGet(m.as(ui), service.JobGetRequest{JobID: a.JobID})
	if job.State != service.JobFailed || job.Error == nil || job.Error.Code != "E_UNSUPPORTED_MEDIA" {
		t.Errorf("job = %+v", job)
	}

	// Nothing reached the document, and a retry runs a new job over the stored original.
	if doc, _ := timeline.Decode(m.document(project)); len(doc.Assets) != 0 {
		t.Errorf("assets in the document: %v", doc.Assets)
	}

	again := m.importOne(project, text).Assets[0]
	if again.JobID == "" || again.JobID == a.JobID || !again.Deduplicated || again.AssetID != a.AssetID {
		t.Errorf("retry = %+v (first %+v)", again, a)
	}
}

func TestEveryQueuedJobIsRunExactlyOnceUnderConcurrentDeliveries(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{noQueue: true})
	project := m.newProject("p")
	a := m.importOne(project, m.write("a.mp4", clipBytes)).Assets[0]

	var wg sync.WaitGroup

	for range 20 {
		wg.Go(func() { _ = m.runner.RunJob(context.Background(), a.JobID) })
	}

	wg.Wait()

	if m.ingests.Load() != 1 {
		t.Errorf("the ingest ran %d times for 20 deliveries", m.ingests.Load())
	}

	// A redelivery for a finished job does nothing.
	before := m.ingests.Load()
	if err := m.runner.RunJob(context.Background(), a.JobID); err != nil || m.ingests.Load() != before {
		t.Errorf("a finished job ran again: %v", err)
	}

	if err := m.runner.RunJob(context.Background(), "job_0000009999"); err != nil {
		t.Errorf("a message for a job that does not exist: %v", err)
	}
}

// A waiting import returns only after the revision that adds the asset to the
// document exists: an agent that edits with the returned revision must not get
// a stale base (the ready state is written before the asset.add commits).
func TestWaitingImportReturnsTheRevisionThatAddsTheAsset(t *testing.T) {
	// The event of the ready state is published before the asset.add commits;
	// holding it open the window that a fast machine leaves too narrow to hit.
	m := newMediaEnv(t, mediaCfg{durable: true, onEvent: func(ev service.Event) {
		if ev.Type == "asset.updated" && ev.State == "ready" {
			time.Sleep(150 * time.Millisecond)
		}
	}})
	project := m.newProject("p")

	ctx, stop := context.WithCancel(context.Background())
	worked := make(chan error, 1)

	go func() {
		worked <- m.queue.Work(ctx, queue.WorkOptions{
			Queues: []string{"ingest", "export", "default"}, Concurrency: 2, Sleep: time.Millisecond,
		})
	}()

	defer func() {
		stop()
		<-worked
	}()

	for i := range 3 {
		clip := m.write("clip"+strconv.Itoa(i)+".mp4", clipBytes+strconv.Itoa(i))

		res, err := m.svc.MediaImport(m.as(ui), service.MediaImportRequest{
			ProjectID: project, Files: []service.ImportFile{{Path: clip}}, WaitMS: 10000,
		})
		if err != nil {
			t.Fatal(err)
		}

		if res.Assets[0].State != "ready" || res.Revision != int64(i+1) {
			t.Fatalf(
				"import %d: state %s, revision %d, want ready at revision %d",
				i,
				res.Assets[0].State,
				res.Revision,
				i+1,
			)
		}
	}
}

// The limit error says what to do, and the file and the ceiling never read the same.
func TestImportLimitMessageSeparatesTheFileFromTheCeiling(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{limits: service.Limits{MaxImportBytes: 1<<20 + 1}})
	project := m.newProject("p")

	_, err := m.svc.MediaImport(m.as(ui), service.MediaImportRequest{
		ProjectID: project, Files: []service.ImportFile{{Path: m.write("big.mp4", strings.Repeat("x", 1<<20+3))}},
	})

	e := serviceErr(err)
	if e == nil || e.Code != "E_LIMIT_EXCEEDED" || e.Hint == "" || !strings.Contains(e.Message, "1048579 bytes") ||
		!strings.Contains(e.Message, "1048577 bytes") {
		t.Errorf("error = %+v", e)
	}
}

// readyOnRead makes a shared ingest end between the moment an import links its
// asset and the moment it reads it back: the hash is ready, the job that adds
// the asset to each document has not finished.
type readyOnRead struct {
	service.Store
	armed *atomic.Bool
}

func (s readyOnRead) Assets() service.AssetRepo {
	return readyOnReadAssets{AssetRepo: s.Store.Assets(), store: s.Store, armed: s.armed}
}

type readyOnReadAssets struct {
	service.AssetRepo
	store service.Store
	armed *atomic.Bool
}

func (a readyOnReadAssets) Get(ctx context.Context, hash string) (service.AssetRow, error) {
	if a.armed.CompareAndSwap(true, false) {
		err := a.store.Write(ctx, func(ctx context.Context, tx service.Tx) error {
			return tx.Assets().SetIngest(ctx, hash, service.IngestReady, 1, nil, 1)
		})
		if err != nil {
			return service.AssetRow{}, err
		}
	}

	return a.AssetRepo.Get(ctx, hash)
}

// While another project's ingest of the same file is finishing, an import
// must not say "ready" for an asset that is not yet in this project's
// document: the next clip.add would answer E_NOT_FOUND.
func TestImportDoesNotSayReadyBeforeTheAssetIsInTheDocument(t *testing.T) {
	var armed atomic.Bool

	m := newMediaEnv(t, mediaCfg{noQueue: true, wrapStore: func(s service.Store) service.Store {
		return readyOnRead{Store: s, armed: &armed}
	}})
	p1, p2 := m.newProject("one"), m.newProject("two")
	clip := m.write("clip.mp4", clipBytes)

	first := m.importOne(p1, clip).Assets[0]

	armed.Store(true)

	a := m.importOne(p2, clip).Assets[0]
	if a.State == "ready" || a.JobID != first.JobID || a.Progress >= 1 {
		t.Fatalf("import = %+v: it must still wait for the job %s", a, first.JobID)
	}

	if err := m.runner.RunJob(context.Background(), first.JobID); err != nil {
		t.Fatal(err)
	}

	doc, _ := timeline.Decode(m.document(p2))
	if _, ok := doc.Assets[a.AssetID]; !ok {
		t.Error("the asset never reached the document of the second project")
	}
}

// The ingest job asks for the speech map and the beat grid, and the artifacts table accepts both
// kinds (SDD-005 section 5.2, RF-043, RF-044).
func TestIngestJobStoresTheSoundMaps(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project := m.newProject("p")
	a := m.importOne(project, m.write("clip.mp4", clipBytes)).Assets[0]

	doc, err := timeline.Decode(m.document(project))
	if err != nil {
		t.Fatal(err)
	}

	arts, err := m.store.Assets().Artifacts(context.Background(), doc.Assets[a.AssetID].Hash)
	if err != nil {
		t.Fatal(err)
	}

	found := map[string]bool{}
	for _, r := range arts {
		found[r.Kind] = true
	}

	if !found["speech"] || !found["beats"] {
		t.Errorf("artifact kinds = %v, want speech and beats", found)
	}
}

// An edit that names an asset whose ingest has not finished is E_ASSET_NOT_READY, not
// E_NOT_FOUND (VV-022).
func TestEditWithAnAssetStillIngestingIsNotReady(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{noQueue: true})
	project := m.newProject("p")
	asset := m.importOne(project, m.write("clip.mp4", clipBytes)).Assets[0]

	_, err := m.svc.Edit(m.as(ui), service.EditRequest{
		ProjectID: project,
		Operations: []command.Operation{
			op("track.add", `{"kind":"visual","id":"trk_f6g7h8j9k0"}`),
			op("clip.add", fmt.Sprintf(`{"track_id":"trk_f6g7h8j9k0","asset_id":%q,"start":"0s"}`, asset.AssetID)),
		},
	})

	var se *service.Error
	if !errors.As(err, &se) || se.Code != "E_ASSET_NOT_READY" || !se.IsRetryable() {
		t.Fatalf("error = %v", err)
	}

	// VV-204: the error names the call that follows the asset and says how far the ingest is.
	if !strings.Contains(se.Hint, "media_inspect") || !strings.Contains(se.Hint, asset.AssetID) {
		t.Errorf("hint = %q, want the media_inspect call for %s", se.Hint, asset.AssetID)
	}

	if !strings.Contains(se.Message, "% done") {
		t.Errorf("message = %q, want the progress", se.Message)
	}

	progress, isNumber := se.Details["progress"].(float64)
	if se.Details["asset_id"] != asset.AssetID || se.Details["state"] != "pending" || !isNumber ||
		progress < 0 || progress > 1 {
		t.Errorf("details = %v, want asset_id, state pending and progress from 0 to 1", se.Details)
	}
}

// A source with alpha has a WebM proxy and PNG posters and sheet (VV-013): they are served with their
// own content type, and the asset has no chunk index.
func TestOpenFileServesTheAlphaProxyAndPNGThumbnails(t *testing.T) {
	var ws string

	alphaIngest := func(_ context.Context, in ingest.Input) (*ingest.Result, error) {
		res, err := fakeResult(ws, in)
		if err != nil {
			return nil, err
		}

		kept := res.Artifacts[:0]
		for _, a := range res.Artifacts {
			if a.Kind == "chunks" {
				continue
			}

			if a.Kind == "proxy" || a.Kind == "thumbnails" {
				ext := map[string]string{"proxy": "webm", "thumbnails": "png"}[a.Kind]
				old := filepath.Join(ws, filepath.FromSlash(a.RelPath))
				a.RelPath = strings.TrimSuffix(a.RelPath, "."+a.Ext) + "." + ext
				a.Ext = ext

				if err := os.Rename(old, filepath.Join(ws, filepath.FromSlash(a.RelPath))); err != nil {
					return nil, err
				}
			}

			kept = append(kept, a)
		}
		res.Artifacts = kept
		res.Facts.HasAlpha = true

		return res, nil
	}

	m := newMediaEnv(t, mediaCfg{ingest: alphaIngest})
	ws = m.ws
	project := m.newProject("p")
	a := m.importOne(project, m.write("a.mp4", clipBytes)).Assets[0]

	for name, c := range map[string]struct {
		ref   service.FileRef
		ctype string
	}{
		"proxy":     {service.FileRef{Kind: "proxy"}, "video/webm"},
		"thumbnail": {service.FileRef{Kind: "thumbnail", Variant: "320"}, "image/png"},
		"strip":     {service.FileRef{Kind: "strip"}, "image/png"},
	} {
		c.ref.ProjectID, c.ref.ID = project, a.AssetID

		f, err := m.svc.OpenFile(m.as(ui), c.ref)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}

		if f.ContentType != c.ctype || f.Size == 0 {
			t.Errorf("%s: %+v", name, f)
		}
	}

	if _, err := m.svc.OpenFile(
		m.as(ui),
		service.FileRef{Kind: "chunks", ProjectID: project, ID: a.AssetID},
	); code(
		err,
	) != command.CodeNotFound {
		t.Errorf("an alpha asset has no chunk index: %v", err)
	}
}

// VV-105: one bad asset id fails the call before any file is imported.
func TestMediaImportChecksEveryAssetIDBeforeImporting(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project := m.newProject("p")

	_, err := m.svc.MediaImport(m.as(ui), service.MediaImportRequest{
		ProjectID: project,
		Files: []service.ImportFile{
			{Path: m.write("a.mp4", clipBytes)},
			{Path: m.write("b.mp4", clipBytes), AssetID: "ast_mus0000001"},
		},
	})
	if code(err) != command.CodeValidation {
		t.Fatalf("a bad asset id = %v, want E_VALIDATION", err)
	}

	list, lerr := m.svc.MediaList(m.as(ui), service.MediaListRequest{ProjectID: project})
	if lerr != nil || len(list.Assets) != 0 {
		t.Errorf("the other file was imported: %+v, %v", list, lerr)
	}
}

// VV-117: an import answers the content hash, and the library lists the files by hash with an optional kind.
func TestAssetListShowsTheFilesByHash(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project := m.newProject("p")
	a := m.importOne(project, m.write("a.mp4", clipBytes)).Assets[0]

	if !strings.HasPrefix(a.Hash, "sha256:") || len(a.Hash) != len("sha256:")+64 {
		t.Fatalf("the import answered no content hash: %+v", a)
	}

	all, err := m.svc.AssetList(m.as(ui), service.AssetListRequest{})
	if err != nil || len(all.Assets) != 1 || all.Assets[0].Hash != a.Hash || all.Assets[0].Projects != 1 {
		t.Fatalf("library = %+v, %v", all, err)
	}

	images, err := m.svc.AssetList(m.as(ui), service.AssetListRequest{Kind: "image"})
	if err != nil || len(images.Assets) != 0 {
		t.Errorf("images = %+v, %v", images, err)
	}

	if _, err := m.svc.AssetList(m.as(ui), service.AssetListRequest{Kind: "gif"}); code(err) != command.CodeValidation {
		t.Errorf("an unknown kind = %v", err)
	}
}
