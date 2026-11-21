package servicetest

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"
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
func (m *mediaEnv) audioWithoutPeaks(_ context.Context, in ingest.Input) (*ingest.Result, error) {
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
