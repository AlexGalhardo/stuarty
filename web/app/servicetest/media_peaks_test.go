package servicetest

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/engine/ingest"
	"github.com/AlexGalhardo/stuarty/service"
)

// pcmWAV is a stereo 48 kHz 16-bit WAV of the given sample frames (a ramp), with the 44-byte header.
func pcmWAV(frames int) []byte {
	data := make([]byte, frames*4)
	for i := range frames {
		v := int16(i%2000 - 1000)
		binary.LittleEndian.PutUint16(data[i*4:], uint16(v))
		binary.LittleEndian.PutUint16(data[i*4+2:], uint16(v))
	}

	h := make([]byte, 44)
	copy(h[0:], "RIFF")
	binary.LittleEndian.PutUint32(h[4:], uint32(36+len(data)))
	copy(h[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(h[16:], 16)
	binary.LittleEndian.PutUint16(h[20:], 1)
	binary.LittleEndian.PutUint16(h[22:], 2)
	binary.LittleEndian.PutUint32(h[24:], 48000)
	binary.LittleEndian.PutUint32(h[28:], 48000*4)
	binary.LittleEndian.PutUint16(h[32:], 4)
	binary.LittleEndian.PutUint16(h[34:], 16)
	copy(h[36:], "data")
	binary.LittleEndian.PutUint32(h[40:], uint32(len(data)))

	return slices.Concat(h, data)
}

// audioWithoutPeaks is an ingest of a sound that stores its normalized WAV and no peaks file, the state of a
// file ingested before peaks existed.
func (m *mediaEnv) audioWithoutPeaks(ctx context.Context, in ingest.Input) (*ingest.Result, error) {
	return m.audioWithoutPeaksSized(ctx, in, 0)
}

// audioWithoutPeaksSized is audioWithoutPeaks whose normalized WAV is registered with the given size (0 for its
// real one), the record of a long file without writing one.
func (m *mediaEnv) audioWithoutPeaksSized(_ context.Context, in ingest.Input, size int64) (*ingest.Result, error) {
	res, err := fakeResult(m.ws, in)
	if err != nil {
		return nil, err
	}

	res.Kind = ingest.KindAudio
	res.Summary.Kind = ingest.KindAudio
	res.Summary.Video = nil

	body := pcmWAV(96000)
	name := ingest.ArtifactName("normalized", ingest.ProfileNormalized, "", "wav")

	if err := os.WriteFile(filepath.Join(m.ws, "artifacts", in.Hash, name), body, 0o600); err != nil {
		return nil, err
	}

	sum := sha256.Sum256(body)
	kept := res.Artifacts[:0]

	for _, a := range res.Artifacts {
		switch a.Kind {
		case "peaks", "thumbnails", "chunks":
			continue
		case "normalized":
			a.Ext, a.RelPath, a.Size = "wav", "artifacts/"+in.Hash+"/"+name, int64(len(body))
			a.SHA256 = hex.EncodeToString(sum[:])

			if size > 0 {
				a.Size = size
			}
		}

		kept = append(kept, a)
	}

	res.Artifacts = kept

	return res, nil
}

func TestPeaksOfASoundWithNoneAreComputedByTheServerOnRequest(t *testing.T) {
	var m *mediaEnv

	m = newMediaEnv(t, mediaCfg{ingest: func(ctx context.Context, in ingest.Input) (*ingest.Result, error) {
		return m.audioWithoutPeaks(ctx, in)
	}})
	project := m.newProject("p")
	a := m.importOne(project, m.write("sting.wav", clipBytes)).Assets[0]

	ref := service.FileRef{Kind: "peaks", ProjectID: project, ID: a.AssetID}

	f, err := m.svc.OpenFile(m.as(ui), ref)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(m.ws, filepath.FromSlash(f.Path)))
	if err != nil {
		t.Fatal(err)
	}

	pk, err := ingest.ParsePeaks(raw)
	if err != nil || pk.SampleCount != 96000 {
		t.Fatalf("the served peaks: %v, %+v", err, pk)
	}

	// The second request finds the artifact the first one registered.
	again, err := m.svc.OpenFile(m.as(ui), ref)
	if err != nil || again.Path != f.Path || again.Size != f.Size {
		t.Errorf("second request = %+v, %v", again, err)
	}

	missing := service.FileRef{Kind: "peaks", ProjectID: project, ID: "ast_0000009999"}
	if _, err := m.svc.OpenFile(m.as(ui), missing); code(err) != command.CodeNotFound {
		t.Errorf("unknown asset: %v", err)
	}
}

// longSound imports a sound whose normalized WAV is registered over the ceiling of a request (the record of a long
// file without writing one), then holds the queue so that the job of its peaks stays queued until a test runs it.
func longSound(t *testing.T, cfg mediaCfg) (*mediaEnv, string, service.FileRef, string) {
	t.Helper()

	var m *mediaEnv

	cfg.ingest = func(ctx context.Context, in ingest.Input) (*ingest.Result, error) {
		return m.audioWithoutPeaksSized(ctx, in, 1<<30+1)
	}
	m = newMediaEnv(t, cfg)
	project := m.newProject("p")
	a := m.importOne(project, m.write("podcast.wav", clipBytes)).Assets[0]

	pa, err := m.store.Assets().GetProjectAsset(context.Background(), project, a.AssetID)
	if err != nil {
		t.Fatal(err)
	}

	m.hold.Store(true)

	return m, project, service.FileRef{Kind: "peaks", ProjectID: project, ID: a.AssetID}, pa.AssetHash
}

// pendingJob is the job id that a read of the peaks of a long sound points at.
func pendingJob(t *testing.T, err error) string {
	t.Helper()

	se := serviceError(t, err)
	if se.Code != "E_ASSET_NOT_READY" || !se.Retryable || se.Details["reason"] != "peaks_pending" {
		t.Fatalf("OpenFile of the peaks of a long sound = %+v; want a retryable peaks_pending", se)
	}

	id, _ := se.Details["job_id"].(string)
	if id == "" {
		t.Fatalf("no job_id in %+v", se.Details)
	}

	return id
}

func assertReady(t *testing.T, m *mediaEnv, hash string) {
	t.Helper()

	ar, err := m.store.Assets().Get(context.Background(), hash)
	if err != nil || ar.IngestState != service.IngestReady {
		t.Fatalf("the asset must stay ready: %+v, %v", ar, err)
	}
}

// The peaks of a long sound are the work of a profile_upgrade ingest job (SDD-011 section 6.3.1): the read queues
// it once, answers a retryable pending error, and is served the file when the job has run. The asset stays ready.
func TestPeaksOfALongSoundAreComputedByAProfileUpgradeJob(t *testing.T) {
	m, _, ref, hash := longSound(t, mediaCfg{})

	_, err := m.svc.OpenFile(m.as(ui), ref)
	id := pendingJob(t, err)

	_, err = m.svc.OpenFile(m.as(ui), ref)
	if again := pendingJob(t, err); again != id {
		t.Errorf("a second read queued job %s, want the same job %s", again, id)
	}

	row, err := m.store.Jobs().Get(context.Background(), id)
	if err != nil || row.Kind != service.JobIngest || row.State != service.JobQueued ||
		!strings.Contains(string(row.Input), "profile_upgrade") || !strings.Contains(string(row.Input), "peaks") {
		t.Fatalf("the job row = %+v, %v", row, err)
	}

	assertReady(t, m, hash)

	if err := m.runner.RunJob(context.Background(), id); err != nil {
		t.Fatal(err)
	}

	if j := m.job(id); j.State != service.JobSucceeded {
		t.Fatalf("the job = %+v", j)
	}

	assertReady(t, m, hash)

	f, err := m.svc.OpenFile(m.as(ui), ref)
	if err != nil {
		t.Fatalf("OpenFile after the job: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(m.ws, filepath.FromSlash(f.Path)))
	if err != nil {
		t.Fatal(err)
	}

	if pk, perr := ingest.ParsePeaks(raw); perr != nil || pk.SampleCount != 96000 {
		t.Fatalf("the peaks the job wrote: %v, %+v", perr, pk)
	}
}

// A cancelled job is not queued again by the next read (a page that polls would undo the cancel): the read answers
// the limit error, and the asset stays ready.
func TestPeaksJobOfALongSoundCanBeCancelled(t *testing.T) {
	m, _, ref, hash := longSound(t, mediaCfg{})

	_, err := m.svc.OpenFile(m.as(ui), ref)
	id := pendingJob(t, err)

	got, cerr := m.svc.JobCancel(m.as(ui), service.JobCancelRequest{JobID: id})
	if cerr != nil || got.State != service.JobCancelled {
		t.Fatalf("JobCancel = %+v, %v", got, cerr)
	}

	assertReady(t, m, hash)

	_, err = m.svc.OpenFile(m.as(ui), ref)

	se := serviceError(t, err)
	if se.Code != "E_LIMIT_EXCEEDED" || se.Retryable || se.Details["limit"] != "peaks_bytes" {
		t.Fatalf("a read after the cancel = %+v; want a non-retryable peaks_bytes limit", se)
	}
}

// The job counts in the ingest limits: with no room in the queue the read answers E_QUEUE_FULL and queues nothing.
func TestPeaksJobOfALongSoundRespectsTheIngestQueueLimit(t *testing.T) {
	m, project, ref, _ := longSound(t, mediaCfg{limits: service.Limits{MaxIngests: 1, MaxQueuedIngests: 1}})

	// The queue holds two ingest jobs (one running slot and one waiting): fill it with imports no worker takes.
	for _, name := range []string{"a.wav", "b.wav"} {
		m.importOne(project, m.write(name, clipBytes+name))
	}

	_, err := m.svc.OpenFile(m.as(ui), ref)
	if se := serviceError(t, err); se.Code != "E_QUEUE_FULL" {
		t.Fatalf("a read with a full ingest queue = %+v; want E_QUEUE_FULL", se)
	}
}

// A job a shutdown left running is requeued by the next start and finishes: the peaks appear after a restart.
func TestPeaksJobOfALongSoundResumesAfterARestart(t *testing.T) {
	m, _, ref, _ := longSound(t, mediaCfg{})

	_, err := m.svc.OpenFile(m.as(ui), ref)
	id := pendingJob(t, err)

	// The process died with the job claimed.
	if ok, cerr := m.store.Jobs().Claim(context.Background(), id, 1); cerr != nil || !ok {
		t.Fatalf("claim: %v %v", ok, cerr)
	}

	m.hold.Store(false)

	if err := m.runner.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}

	if j := m.job(id); j.State != service.JobSucceeded {
		t.Fatalf("after the restart: %+v", j)
	}

	if _, err := m.svc.OpenFile(m.as(ui), ref); err != nil {
		t.Fatalf("OpenFile after the restart: %v", err)
	}
}

// peaksRace counts the peaks artifacts the service registers and, once armed, holds the first reads of the
// artifacts of a file until every caller has made one, so the callers all start from a file with no peaks.
type peaksRace struct {
	puts    atomic.Int64
	armed   atomic.Bool
	reads   atomic.Int64
	callers int64
	all     chan struct{}
}

type peaksCountStore struct {
	service.Store
	race *peaksRace
}

func (s peaksCountStore) Assets() service.AssetRepo {
	return peaksCountRepo{AssetRepo: s.Store.Assets(), race: s.race}
}

type peaksCountRepo struct {
	service.AssetRepo
	race *peaksRace
}

func (r peaksCountRepo) PutArtifact(ctx context.Context, a service.ArtifactRow) error {
	if a.Kind == "peaks" {
		r.race.puts.Add(1)
	}

	return r.AssetRepo.PutArtifact(ctx, a)
}

func (r peaksCountRepo) Artifacts(ctx context.Context, hash string) ([]service.ArtifactRow, error) {
	if r.race.armed.Load() {
		if n := r.race.reads.Add(1); n <= r.race.callers {
			if n == r.race.callers {
				close(r.race.all)
			}

			select {
			case <-r.race.all:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}

	return r.AssetRepo.Artifacts(ctx, hash)
}

// A page of sounds asks for every waveform at once: the peaks of a file are computed once, and the requests
// that waited are served the file the first one wrote (SDD-011 section 6.5).
func TestPeaksOnRequestAreComputedOnceForConcurrentRequests(t *testing.T) {
	const callers = 8

	var m *mediaEnv

	race := &peaksRace{callers: callers, all: make(chan struct{})}
	m = newMediaEnv(t, mediaCfg{
		ingest: func(ctx context.Context, in ingest.Input) (*ingest.Result, error) {
			return m.audioWithoutPeaks(ctx, in)
		},
		wrapStore: func(s service.Store) service.Store { return peaksCountStore{Store: s, race: race} },
	})
	project := m.newProject("p")
	a := m.importOne(project, m.write("sting.wav", clipBytes)).Assets[0]
	ref := service.FileRef{Kind: "peaks", ProjectID: project, ID: a.AssetID}

	race.armed.Store(true)

	var (
		wg   sync.WaitGroup
		errs = make([]error, callers)
	)

	for i := range callers {
		wg.Go(func() {
			_, errs[i] = m.svc.OpenFile(m.as(ui), ref)
		})
	}

	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("caller %d: %v", i, err)
		}
	}

	if n := race.puts.Load(); n != 1 {
		t.Errorf("the peaks were registered %d times for %d concurrent requests, want once", n, callers)
	}
}
