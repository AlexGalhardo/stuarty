package testenv

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/lint"
	"github.com/AlexGalhardo/vukem-video/core/plan"
	"github.com/AlexGalhardo/vukem-video/core/timeline"
	"github.com/AlexGalhardo/vukem-video/engine/ingest"
	"github.com/AlexGalhardo/vukem-video/engine/rasterize"
	"github.com/AlexGalhardo/vukem-video/service"
)

// rasterMeasurer is lint.Measurer as the owner wires it (web/app/owner): the
// box of an item is the layer the plan draws at its first frame, sized by the
// rasterizer's layout, with its rotation applied. The owner's type is not
// exported, so testenv keeps this copy; keep the two in step.
type rasterMeasurer struct{ workspace string }

var _ lint.Measurer = rasterMeasurer{}

func (m rasterMeasurer) Measure(doc *timeline.Document, it *timeline.Item) (lint.Rect, bool) {
	fp := plan.FrameWith(doc, it.Start, plan.Options{Measure: rasterize.Measure(service.FontSetFor(m.workspace, doc))})

	for i := range fp.Layers {
		l := &fp.Layers[i]
		if l.ItemID != it.ID {
			continue
		}

		if l.W <= 0 || l.H <= 0 {
			return lint.Rect{}, false
		}

		rad := l.Rotation * math.Pi / 180
		hw := (math.Abs(l.W*math.Cos(rad)) + math.Abs(l.H*math.Sin(rad))) / 2
		hh := (math.Abs(l.W*math.Sin(rad)) + math.Abs(l.H*math.Cos(rad))) / 2

		return lint.Rect{X0: l.CX - hw, Y0: l.CY - hh, X1: l.CX + hw, Y1: l.CY + hh}, true
	}

	return lint.Rect{}, false
}

// assetHash is the content hash of an asset of a project.
func (e *Env) assetHash(tb testing.TB, project, asset string) string {
	tb.Helper()

	row, err := e.Store.Assets().GetProjectAsset(context.Background(), project, asset)
	if err != nil {
		tb.Fatal(err)
	}

	return row.AssetHash
}

// WriteLoudPeaks replaces the peaks artifact of an asset with 2 s at 48 kHz in
// which every group of samples reaches full scale, so that two copies of the
// sound playing at once clip (L_AUDIO_CLIP).
func (e *Env) WriteLoudPeaks(tb testing.TB, project, asset string) {
	tb.Helper()

	p := &ingest.Peaks{SampleRate: 48000, SampleCount: 96000, Min: -32767, Max: 32767}

	for i, per := range ingest.PeakLevels {
		p.Levels[i] = make([]ingest.PeakPair, ingest.PeakCount(p.SampleCount, per))

		for j := range p.Levels[i] {
			p.Levels[i][j] = ingest.PeakPair{Min: -32767, Max: 32767}
		}
	}

	hex := strings.TrimPrefix(e.assetHash(tb, project, asset), "sha256:")
	if err := os.WriteFile(
		filepath.Join(string(e.Dir), "artifacts", hex, "peaks-1.bin"),
		p.Marshal(),
		0o600,
	); err != nil {
		tb.Fatal(err)
	}
}

// PutTranscript stores a transcript artifact of three words for an asset, as
// the transcribe job does, without the provider.
func (e *Env) PutTranscript(tb testing.TB, project, asset string) {
	tb.Helper()

	hash := e.assetHash(tb, project, asset)
	hex := strings.TrimPrefix(hash, "sha256:")
	rel := "artifacts/" + hex + "/transcript-1-abc123abc123.json"
	body := `{"version":1,"audio_ms":2000,"words":[` +
		`{"text":"hello","start_ms":0,"end_ms":400},{"text":"brave","start_ms":400,"end_ms":900},` +
		`{"text":"world","start_ms":1000,"end_ms":1500}],` +
		`"segments":[{"start_ms":0,"end_ms":900,"first_word":0,"last_word":1},` +
		`{"start_ms":1000,"end_ms":1500,"first_word":2,"last_word":2}]}`

	if err := os.WriteFile(filepath.Join(string(e.Dir), filepath.FromSlash(rel)), []byte(body), 0o600); err != nil {
		tb.Fatal(err)
	}

	if err := e.Store.Assets().PutArtifact(context.Background(), service.ArtifactRow{
		AssetHash: hash, Kind: "transcript", Profile: 1, Variant: "abc123abc123", Ext: "json", RelPath: rel,
		SizeBytes: int64(len(body)), CreatedMs: 1,
	}); err != nil {
		tb.Fatal(err)
	}
}
