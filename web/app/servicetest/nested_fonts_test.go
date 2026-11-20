package servicetest

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	enginestore "github.com/AlexGalhardo/stuarty/engine/store"
	"github.com/AlexGalhardo/stuarty/service"
)

// TestNestedTextKeepsItsUserFont: a timeline whose text uses a user font of its own draws that text with the same
// font where another timeline holds it as a composition, though the other timeline does not name the font: the
// bitmap of the parent is the bitmap of the nested timeline.
func TestNestedTextKeepsItsUserFont(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{noQueue: true, realExport: true})

	anton, err := enginestore.NewFonts(m.ws).Put(bundledFontFile(t, "Anton-Regular.ttf"))
	if err != nil {
		t.Fatal(err)
	}

	intro := m.brandProject(anton)

	added, err := m.svc.ProjectItemAdd(m.as(ui), service.ProjectItemAddRequest{
		ProjectID: intro, Kind: "timeline", Title: "Video",
	})
	if err != nil {
		t.Fatal(err)
	}

	video := added.Item.TimelineID
	track := m.editIn(t, video, op("track.add", `{"kind":"visual"}`)).Created[0]
	m.editIn(t, video, op("clip.add_composition",
		fmt.Sprintf(`{"track_id":%q,"timeline_id":%q,"start":"0s"}`, track, intro)))

	keyIntro, pngIntro := m.timelineBitmap(intro)
	keyVideo, pngVideo := m.timelineBitmap(video)

	if keyVideo != keyIntro || !bytes.Equal(pngVideo, pngIntro) {
		t.Errorf("the text of the composition is drawn with another font: key %s, want %s", keyVideo, keyIntro)
	}
}

// timelineBitmap is the key and the picture of the one bitmap of the first second of a timeline.
func (m *mediaEnv) timelineBitmap(tl string) (key string, png []byte) {
	m.t.Helper()

	ctx := m.as(ui)

	w, err := m.svc.PlanWindow(ctx, service.PlanWindowRequest{ProjectID: tl, TimelineID: tl, From: 0, Until: 30})
	if err != nil {
		m.t.Fatal(err)
	}

	if len(w.Layers) != 1 || w.Layers[0].Bitmaps == nil {
		m.t.Fatalf("window of %s = %+v", tl, w)
	}

	key = w.Layers[0].Bitmaps.Keys[0]

	f, err := m.svc.OpenFile(ctx, service.FileRef{Kind: "bitmap", ProjectID: tl, ID: key})
	if err != nil {
		m.t.Fatal(err)
	}

	png, err = os.ReadFile(filepath.Join(m.ws, filepath.FromSlash(f.Path)))
	if err != nil {
		m.t.Fatal(err)
	}

	return key, png
}
