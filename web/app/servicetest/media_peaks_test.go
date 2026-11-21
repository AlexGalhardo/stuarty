package servicetest

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"
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

// A sound whose normalized WAV is over the ceiling of a request is refused with a limit error that is not
// retryable and does not send the client to a job that does not exist (SDD-011 section 6.5, SDD-007 section 9).
func TestPeaksOfALongSoundWithNoneAreRefusedAsALimit(t *testing.T) {
	var m *mediaEnv

	m = newMediaEnv(t, mediaCfg{ingest: func(ctx context.Context, in ingest.Input) (*ingest.Result, error) {
		return m.audioWithoutPeaksSized(ctx, in, 1<<30+1)
	}})
	project := m.newProject("p")
	a := m.importOne(project, m.write("podcast.wav", clipBytes)).Assets[0]

	_, err := m.svc.OpenFile(m.as(ui), service.FileRef{Kind: "peaks", ProjectID: project, ID: a.AssetID})

	se := serviceError(t, err)
	if se.Code != "E_LIMIT_EXCEEDED" || se.Retryable || se.Details["limit"] != "peaks_bytes" {
		t.Fatalf("OpenFile of the peaks of a long sound = %+v; want a non-retryable peaks_bytes limit", se)
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
