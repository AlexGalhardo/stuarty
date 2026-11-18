package export

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

// renders numbers the work folders of the tests that compose frames.
var renders atomic.Int32

var referenceScenes = []string{"clip-caption", "two-clips-cut", "contain-text"}

// sceneFrames is 30 frames spread over the scene (SDD-008 section 4).
func sceneFrames(total int) []int {
	const n = 30
	out := make([]int, 0, n)
	for i := range n {
		f := i * (total - 1) / (n - 1)
		if len(out) == 0 || out[len(out)-1] != f {
			out = append(out, f)
		}
	}
	return out
}

type goldenFrame struct {
	Frame  int    `json:"frame"`
	SHA256 string `json:"sha256"`
}

// renderFrames composes the frames of a scene in order and returns the pixels of each.
func renderFrames(t *testing.T, name string, bands int, frames []int) [][]byte {
	t.Helper()
	return renderDoc(t, loadScene(t, name), name, bands, frames)
}

// renderDoc is renderFrames for a document that is already loaded.
func renderDoc(t *testing.T, doc *timeline.Document, name string, bands int, frames []int) [][]byte {
	t.Helper()
	job := newJob(t, doc, "golden-"+name+"-"+strconv.Itoa(int(renders.Add(1))), "unused.mp4")
	s := testState(t, job, job.JobID)
	rd := s.newRenderer(bands)
	defer rd.close()

	dst := image.NewRGBA(image.Rect(0, 0, doc.Settings.Width, doc.Settings.Height))
	var out [][]byte
	for _, f := range frames {
		if err := rd.render(dst, f); err != nil {
			t.Fatalf("%s frame %d: %v", name, f, err)
		}
		out = append(out, bytes.Clone(dst.Pix))
	}
	return out
}

// SDD-004 section 11, NFR-010: the composed frames of the reference scenes are the same bytes on every
// run and every operating system. Regenerate with go test ./engine/export -run Golden -update; every
// regeneration is a reviewed change.
func TestReferenceScenesEqualTheirGoldenFrames(t *testing.T) {
	for _, name := range referenceScenes {
		t.Run(name, func(t *testing.T) {
			doc := loadScene(t, name)
			frames := sceneFrames(endFrame(doc))
			pix := renderFrames(t, name, 0, frames)

			var got []goldenFrame
			for i, f := range frames {
				sum := sha256.Sum256(pix[i])
				got = append(got, goldenFrame{f, hex.EncodeToString(sum[:])})
			}
			path := filepath.Join("testdata", "golden", name+".json")
			if *update {
				b, err := json.MarshalIndent(got, "", "  ")
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, append(b, '\n'), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var want []goldenFrame
			if err := json.Unmarshal(raw, &want); err != nil {
				t.Fatal(err)
			}
			if len(want) != len(got) {
				t.Fatalf("%d golden frames, %d composed", len(want), len(got))
			}
			for i := range got {
				if got[i] != want[i] {
					t.Errorf("frame %d: %s, golden %s", got[i].Frame, got[i].SHA256, want[i].SHA256)
					saveMismatch(t, name, got[i].Frame, doc.Settings.Width, doc.Settings.Height, pix[i])
				}
			}
		})
	}
}

// SDD-004 section 12 rule 3: splitting the work in bands never changes a pixel.
func TestCompositorBandsEqual(t *testing.T) {
	for _, name := range referenceScenes {
		t.Run(name, func(t *testing.T) {
			doc := loadScene(t, name)
			frames := sceneFrames(endFrame(doc))
			one := renderFrames(t, name, 1, frames)
			eight := renderFrames(t, name, 8, frames)
			for i, f := range frames {
				if !bytes.Equal(one[i], eight[i]) {
					t.Fatalf("frame %d differs between 1 band and 8 bands", f)
				}
			}
		})
	}
}

// Two runs in one process give the same frames, and a frame asked out of order (the decoder restarts)
// gives the same pixels as in order.
func TestCompositionIsRepeatableAndOrderIndependent(t *testing.T) {
	name := "two-clips-cut"
	doc := loadScene(t, name)
	frames := sceneFrames(endFrame(doc))
	a := renderFrames(t, name, 0, frames)
	b := renderFrames(t, name, 0, frames)
	for i := range a {
		if !bytes.Equal(a[i], b[i]) {
			t.Fatalf("frame %d differs between two runs", frames[i])
		}
	}

	job := newJob(t, doc, "golden-order", "unused.mp4")
	s := testState(t, job, job.JobID)
	rd := s.newRenderer(0)
	defer rd.close()
	dst := image.NewRGBA(image.Rect(0, 0, doc.Settings.Width, doc.Settings.Height))
	for _, i := range []int{len(frames) - 1, 3, 3, 20, 0, 10} {
		if err := rd.render(dst, frames[i]); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(dst.Pix, a[i]) {
			t.Fatalf("frame %d out of order differs from in order", frames[i])
		}
	}
}
