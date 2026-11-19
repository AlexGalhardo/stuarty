package servicetest

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"slices"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/ingest"
	"github.com/AlexGalhardo/stuarty/service"
)

func animatedGIF(t *testing.T, delaysCS []int) []byte {
	t.Helper()

	palette := color.Palette{color.RGBA{255, 0, 0, 255}, color.RGBA{0, 255, 0, 255}, color.RGBA{0, 0, 255, 255}}
	g := &gif.GIF{}

	for i, d := range delaysCS {
		img := image.NewPaletted(image.Rect(0, 0, 16, 12), palette)
		for p := range img.Pix {
			img.Pix[p] = uint8(i % len(palette))
		}

		g.Image, g.Delay = append(g.Image, img), append(g.Delay, d)
	}

	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, g); err != nil {
		t.Fatal(err)
	}

	return buf.Bytes()
}

// RF-028, SDD-011 section 4.7: an animated GIF imported through the service becomes an image asset
// with the delays of its frames, the frames artifact is recorded, and the plan of an item that uses
// it picks the frame by delay (SDD-004 section 3.3).
func TestAnimatedGIFImportKeepsTheDelaysAndThePlanPicksFramesByThem(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{realExport: true, ingest: ingest.Run})

	project, track := m.withTrack()
	m.importOne(project, m.write("a.gif", string(animatedGIF(t, []int{7, 3, 10}))))

	doc, err := timeline.Decode(m.document(project))
	if err != nil {
		t.Fatal(err)
	}

	if len(doc.Assets) != 1 {
		t.Fatalf("assets = %v", doc.Assets)
	}

	var asset timeline.Asset
	for _, a := range doc.Assets {
		asset = a
	}

	want := []int{70, 30, 100}
	if asset.Kind != timeline.AssetImage || !slices.Equal(asset.FrameDelays, want) || asset.FrameCount != 3 {
		t.Fatalf("asset = %+v, want an image with delays %v", asset, want)
	}

	frames, err := m.store.Assets().Artifacts(context.Background(), asset.Hash)
	if err != nil {
		t.Fatal(err)
	}

	if !slices.ContainsFunc(frames, func(a service.ArtifactRow) bool { return a.Kind == "frames" }) {
		t.Errorf("no frames artifact among %d artifacts", len(frames))
	}

	m.mustEdit(ui, project, m.head(project).Revision, op("clip.add", fmt.Sprintf(
		`{"track_id":%q,"asset_id":%q,"start":"0s","duration":"1s"}`, track, asset.ID,
	)))

	w, err := m.svc.PlanWindow(m.as(ui), service.PlanWindowRequest{ProjectID: project, From: 0, Until: 12})
	if err != nil || len(w.Layers) != 1 {
		t.Fatalf("window = %+v, %v", w, err)
	}

	got := w.Layers[0].SourceFrames
	for r := range 12 {
		if len(got) <= r || got[r] != plan.AnimImageSourceFrame(want, timeline.Rational{Num: 30, Den: 1}, r) {
			t.Fatalf("source frames = %v", got)
		}
	}

	if got[0] == got[len(got)-1] {
		t.Errorf("the plan shows one frame throughout: %v", got)
	}
}
