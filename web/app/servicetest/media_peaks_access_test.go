package servicetest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/AlexGalhardo/stuarty/engine/ingest"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/testproject"
)

func userCtx(user string) context.Context {
	return service.WithCaller(
		context.Background(),
		service.Caller{Actor: "user:" + user, Scopes: service.AllScopes, OwnerUserID: user},
	)
}

// longSoundFor imports the same long sound into a new project of the user and returns the reference of its peaks.
func longSoundFor(c context.Context, t *testing.T, m *mediaEnv, name string) service.FileRef {
	t.Helper()

	p, err := testproject.CreateWithTimeline(c, m.svc, name)
	if err != nil {
		t.Fatal(err)
	}

	res, err := m.svc.MediaImport(c, service.MediaImportRequest{
		ProjectID: p.ProjectID, Files: []service.ImportFile{{Path: m.write("podcast.wav", clipBytes)}},
	})
	if err != nil {
		t.Fatal(err)
	}

	return service.FileRef{Kind: "peaks", ProjectID: p.ProjectID, ID: res.Assets[0].AssetID}
}

func longSoundEnv(t *testing.T, cfg mediaCfg) *mediaEnv {
	t.Helper()

	var m *mediaEnv

	cfg.ingest = func(ctx context.Context, in ingest.Input) (*ingest.Result, error) {
		return m.audioWithoutPeaksSized(ctx, in, 1<<30+1)
	}
	m = newMediaEnv(t, cfg)

	return m
}

func hashOf(t *testing.T, m *mediaEnv, ref service.FileRef) string {
	t.Helper()

	pa, err := m.store.Assets().GetProjectAsset(context.Background(), ref.ProjectID, ref.ID)
	if err != nil {
		t.Fatal(err)
	}

	return pa.AssetHash
}

// A read by someone with no access to the asset queues nothing and tells nothing, in network mode.
func TestAReadWithNoAccessNeitherQueuesNorRevealsThePeaksJob(t *testing.T) {
	m := longSoundEnv(t, mediaCfg{})
	alice, bob := userCtx("alice"), userCtx("bob")
	ref := longSoundFor(alice, t, m, "mine")
	hash := hashOf(t, m, ref)

	m.hold.Store(true)

	other, err := testproject.CreateWithTimeline(alice, m.svc, "other")
	if err != nil {
		t.Fatal(err)
	}

	limited := service.WithCaller(context.Background(), service.Caller{
		Actor: "key:k", Scopes: service.AllScopes, OwnerUserID: "alice", ProjectID: other.ProjectID,
	})

	for name, c := range map[string]context.Context{"another user": bob, "a key of another project": limited} {
		_, rerr := m.svc.OpenFile(c, ref)
		if se := serviceError(t, rerr); se.Code == "E_ASSET_NOT_READY" || se.Details["job_id"] != nil {
			t.Errorf("%s: the read answered %+v", name, se)
		}

		if j, jerr := m.store.Jobs().ActiveIngestForHash(context.Background(), hash); jerr == nil {
			t.Fatalf("%s queued job %s", name, j.ID)
		}
	}

	_, rerr := m.svc.OpenFile(alice, ref)
	id := pendingJob(t, rerr)

	if _, gerr := m.svc.JobGet(bob, service.JobGetRequest{JobID: id}); gerr == nil {
		t.Errorf("bob reads the job %s of a file he does not hold", id)
	}

	if _, cerr := m.svc.JobCancel(bob, service.JobCancelRequest{JobID: id}); cerr == nil {
		t.Errorf("bob cancels the job %s of a file he does not hold", id)
	}
}

// The file is shared by content: a user cannot stop the job another user is waiting on, as with an import.
func TestAUserCannotCancelThePeaksJobOfAFileAnotherHolds(t *testing.T) {
	m := longSoundEnv(t, mediaCfg{})
	alice, bob := userCtx("alice"), userCtx("bob")
	mine, theirs := longSoundFor(alice, t, m, "mine"), longSoundFor(bob, t, m, "theirs")

	m.hold.Store(true)

	_, err := m.svc.OpenFile(alice, mine)
	id := pendingJob(t, err)

	got, cerr := m.svc.JobCancel(bob, service.JobCancelRequest{JobID: id})
	if cerr != nil {
		t.Fatal(cerr)
	}

	if got.State == service.JobCancelled {
		t.Fatalf("bob cancelled the job alice waits on: %+v", got)
	}

	for name, r := range map[string]struct {
		c   context.Context
		ref service.FileRef
	}{"alice": {alice, mine}, "bob": {bob, theirs}} {
		_, rerr := m.svc.OpenFile(r.c, r.ref)
		if again := pendingJob(t, rerr); again != id {
			t.Errorf("%s follows job %s, want %s", name, again, id)
		}
	}

	if pa, perr := m.store.Assets().GetProjectAsset(context.Background(), theirs.ProjectID, theirs.ID); perr != nil ||
		pa.State != service.ProjectAssetActive {
		t.Errorf("the attempt touched the asset of bob: %+v, %v", pa, perr)
	}
}

// A cancel is the choice of the one who made it: a later user of the same file still gets its waveform.
func TestACancelDoesNotTakeTheWaveformFromALaterUser(t *testing.T) {
	m := longSoundEnv(t, mediaCfg{})
	alice, bob := userCtx("alice"), userCtx("bob")
	mine := longSoundFor(alice, t, m, "mine")

	m.hold.Store(true)

	_, err := m.svc.OpenFile(alice, mine)
	id := pendingJob(t, err)

	if got, cerr := m.svc.JobCancel(alice, service.JobCancelRequest{JobID: id}); cerr != nil ||
		got.State != service.JobCancelled {
		t.Fatalf("JobCancel = %+v, %v", got, cerr)
	}

	_, err = m.svc.OpenFile(alice, mine)
	if se := serviceError(t, err); se.Code != "E_LIMIT_EXCEEDED" {
		t.Fatalf("alice after her cancel = %+v", se)
	}

	m.hold.Store(false)
	theirs := longSoundFor(bob, t, m, "theirs")
	m.hold.Store(true)

	_, err = m.svc.OpenFile(bob, theirs)
	if next := pendingJob(t, err); next == id {
		t.Errorf("bob follows the cancelled job %s", id)
	}
}

// A job that failed is a fact about the file: no user queues it again in this process, and the asset stays ready.
func TestAFailedPeaksJobIsNotQueuedAgain(t *testing.T) {
	m, _, ref, hash := longSound(t, mediaCfg{})

	name := ingest.ArtifactName("normalized", ingest.ProfileNormalized, "", "wav")
	dir := filepath.Join(m.ws, "artifacts", hash[len("sha256:"):], name)

	if err := os.WriteFile(dir, []byte("not a wav"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := m.svc.OpenFile(m.as(ui), ref)
	id := pendingJob(t, err)

	if err := m.runner.RunJob(context.Background(), id); err != nil {
		t.Fatal(err)
	}

	if j := m.job(id); j.State != service.JobFailed {
		t.Fatalf("the job = %+v", j)
	}

	assertReady(t, m, hash)

	_, err = m.svc.OpenFile(m.as(ui), ref)
	if se := serviceError(t, err); se.Code != "E_LIMIT_EXCEEDED" {
		t.Fatalf("a read after the failure = %+v", se)
	}
}

// A header that declares no audio ends the job as a failure, with no empty waveform registered.
func TestAPeaksJobOfAWAVWithNoSamplesFails(t *testing.T) {
	m, _, ref, hash := longSound(t, mediaCfg{})

	name := ingest.ArtifactName("normalized", ingest.ProfileNormalized, "", "wav")
	path := filepath.Join(m.ws, "artifacts", hash[len("sha256:"):], name)

	if err := os.WriteFile(path, pcmWAV(0), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := m.svc.OpenFile(m.as(ui), ref)
	id := pendingJob(t, err)

	if err := m.runner.RunJob(context.Background(), id); err != nil {
		t.Fatal(err)
	}

	if j := m.job(id); j.State != service.JobFailed {
		t.Fatalf("the job = %+v", j)
	}

	arts, err := m.store.Assets().Artifacts(context.Background(), hash)
	if err != nil {
		t.Fatal(err)
	}

	for _, a := range arts {
		if a.Kind == "peaks" {
			t.Errorf("an empty waveform was registered: %+v", a)
		}
	}
}

// A page of long sounds cannot take the ingest queue from the imports: one peaks job is active at a time, the
// other reads answer pending with no job, and each gets its turn when it asks again.
func TestALibraryOfLongSoundsCannotFillTheIngestQueue(t *testing.T) {
	m := longSoundEnv(t, mediaCfg{limits: service.Limits{MaxIngests: 1, MaxQueuedIngests: 2}})
	project := m.newProject("p")

	var refs []service.FileRef

	for i := range 4 {
		name := fmt.Sprintf("long-%d.wav", i)
		a := m.importOne(project, m.write(name, clipBytes+name)).Assets[0]
		refs = append(refs, service.FileRef{Kind: "peaks", ProjectID: project, ID: a.AssetID})
	}

	m.hold.Store(true)

	_, err := m.svc.OpenFile(m.as(ui), refs[0])
	first := pendingJob(t, err)

	for _, r := range refs[1:] {
		_, rerr := m.svc.OpenFile(m.as(ui), r)

		se := serviceError(t, rerr)
		if se.Code != "E_ASSET_NOT_READY" || !se.Retryable || se.Details["reason"] != "peaks_pending" {
			t.Fatalf("a read while another sound is computed = %+v", se)
		}

		if id, _ := se.Details["job_id"].(string); id != "" {
			t.Fatalf("a second peaks job %s was queued", id)
		}
	}

	// The queue still has room for an import.
	if res := m.importOne(project, m.write("new.wav", clipBytes+"new")); len(res.Assets) != 1 {
		t.Fatalf("an import with sounds waiting = %+v", res)
	}

	if err := m.runner.RunJob(context.Background(), first); err != nil {
		t.Fatal(err)
	}

	_, err = m.svc.OpenFile(m.as(ui), refs[1])
	if next := pendingJob(t, err); next == first {
		t.Errorf("the next sound follows the finished job %s", first)
	}
}

type writeCountStore struct {
	service.Store
	writes *atomic.Int64
}

func (s writeCountStore) Write(ctx context.Context, fn func(context.Context, service.Tx) error) error {
	s.writes.Add(1)

	return s.Store.Write(ctx, fn)
}

// A page polls every few seconds: a read that finds the job running takes no write transaction.
func TestPollingThePeaksOfALongSoundWritesNothing(t *testing.T) {
	var writes atomic.Int64

	m, _, ref, _ := longSound(t, mediaCfg{wrapStore: func(s service.Store) service.Store {
		return writeCountStore{Store: s, writes: &writes}
	}})

	_, err := m.svc.OpenFile(m.as(ui), ref)
	id := pendingJob(t, err)

	before := writes.Load()

	for range 5 {
		_, rerr := m.svc.OpenFile(m.as(ui), ref)
		if again := pendingJob(t, rerr); again != id {
			t.Fatalf("job %s, want %s", again, id)
		}
	}

	if n := writes.Load() - before; n != 0 {
		t.Errorf("five polls took %d write transactions", n)
	}
}

// A peaks job adds no asset to a project, so a reference whose add was refused is not shown as still ingesting
// behind it, and the import does not wait for it.
func TestAPeaksJobIsNotTakenForTheImportOfAPendingReference(t *testing.T) {
	m, project, ref, _ := longSound(t, mediaCfg{})

	_, err := m.svc.OpenFile(m.as(ui), ref)
	id := pendingJob(t, err)

	if err := m.store.Assets().SetProjectAssetState(
		context.Background(), project, ref.ID, service.ProjectAssetPending, "user:ui", 1, nil,
	); err != nil {
		t.Fatal(err)
	}

	res, err := m.svc.MediaImport(m.as(ui), service.MediaImportRequest{
		ProjectID: project, Files: []service.ImportFile{{Path: m.write("podcast.wav", clipBytes)}},
	})
	if err != nil {
		t.Fatal(err)
	}

	if a := res.Assets[0]; a.State != string(service.IngestReady) || a.JobID == id {
		t.Errorf("the reference = state %q, job %q; want ready, not the peaks job %s", a.State, a.JobID, id)
	}
}
