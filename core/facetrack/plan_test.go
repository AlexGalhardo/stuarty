package facetrack

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden files")

const fixtureFPS = 10

// movingSpeaker is the track of the drawn fixture of aihelper/testdata/make-face-fixture.sh in a 1280x720
// source: a face of 190 x 240 pixels crossing the frame in 10 s at 10 fps, with a small vertical sway made
// of a triangle wave so the input is the same integers on every system.
func movingSpeaker() *Track {
	t := &Track{
		Version: Version, Model: "yunet-2023mar", Provider: "cpu", Source: Size{Width: 1280, Height: 720},
		FPS: fixtureFPS, StartMS: 0, EndMS: 10_000,
	}

	for i := range 100 {
		ms := i * 1000 / fixtureFPS
		cx := 1875 + 625*ms/1000
		phase := ms % 3000
		sway := min(phase, 3000-phase) - 750 // -750 to 750 in 1/10000 of the height, triangle of period 3 s
		cy := 4167 + sway/2

		t.Samples = append(
			t.Samples,
			Sample{T: ms, Faces: []Face{{X: cx - 742, Y: cy - 1667, W: 1484, H: 3333, Score: 880}}},
		)
	}

	return t
}

func vertical(t *Track) (vw, vh int) { return Visible(t.Source, 1080, 1920) }

func TestVisibleIsTheShareACoverFrameShows(t *testing.T) {
	vw, vh := Visible(Size{Width: 1280, Height: 720}, 1080, 1920)
	if vw != 3164 || vh != Unit {
		t.Errorf("16:9 into 9:16 shows %d x %d, want 3164 x 10000", vw, vh)
	}

	vw, vh = Visible(Size{Width: 1080, Height: 1920}, 1080, 1920)
	if vw != Unit || vh != Unit {
		t.Errorf("the same aspect shows %d x %d, want the whole frame", vw, vh)
	}

	vw, vh = Visible(Size{Width: 1920, Height: 1080}, 1080, 1080)
	if vw != 5625 || vh != Unit {
		t.Errorf("16:9 into 1:1 shows %d x %d", vw, vh)
	}
}

// Acceptance criterion 3 of epic 70: the face stays inside the frame in at least 95% of frames.
func TestTheFaceStaysInTheVerticalFrame(t *testing.T) {
	tr := movingSpeaker()
	vw, vh := vertical(tr)
	kfs := Plan(tr, DefaultOptions(), vw, vh, 0, 10_000)

	inside := 0

	for _, s := range tr.Samples {
		cx, cy := At(kfs, int64(s.T))
		w := WindowAt(vw, vh, cx, cy)
		f := s.Faces[0]

		if f.X >= w.Left && f.X+f.W <= Unit-w.Right {
			inside++
		}
	}

	if inside < 95 {
		t.Errorf("the face was inside the window in %d of 100 frames, want at least 95", inside)
	}
}

// Acceptance criterion 4: the same track and options give the same keyframes.
func TestPlanIsDeterministic(t *testing.T) {
	tr := movingSpeaker()
	vw, vh := vertical(tr)

	a := Plan(tr, DefaultOptions(), vw, vh, 0, 10_000)
	b := Plan(tr, DefaultOptions(), vw, vh, 0, 10_000)

	ja, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}

	jb, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(ja, jb) || !reflect.DeepEqual(a, b) {
		t.Error("two runs gave different keyframes")
	}
}

func TestGoldenKeyframes(t *testing.T) {
	tr := movingSpeaker()
	vw, vh := vertical(tr)
	got, err := json.MarshalIndent(Plan(tr, DefaultOptions(), vw, vh, 0, 10_000), "", " ")
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join("testdata", "moving-speaker.golden.json")
	got = append(got, '\n')

	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n")), got) {
		t.Errorf("the keyframes differ from %s; run the test with -update to see the change in a diff", path)
	}
}

func still(n int) *Track {
	t := &Track{Version: Version, Source: Size{Width: 1280, Height: 720}, FPS: 5, EndMS: n * 200}
	for i := range n {
		t.Samples = append(t.Samples, Sample{T: i * 200, Faces: nil})
	}

	return t
}

func face(cx, w, h int) Face { return Face{X: cx - w/2, Y: 2000, W: w, H: h, Score: 900} }

func TestTheDeadZoneKeepsTheFrameStill(t *testing.T) {
	tr := still(30)
	vw, vh := vertical(tr)

	// A speaker who nods: the centre moves by 100 units, far inside the dead zone of 8% of the window.
	for i := range tr.Samples {
		tr.Samples[i].Faces = []Face{face(5000+(i%2)*100, 1400, 3000)}
	}

	for _, k := range Plan(tr, DefaultOptions(), vw, vh, 0, 5800) {
		if k.X != 5050 && k.X != 5000 {
			t.Fatalf("the frame moved to %d inside the dead zone", k.X)
		}
	}

	kfs := Plan(tr, DefaultOptions(), vw, vh, 0, 5800)
	if first, last := kfs[0], kfs[len(kfs)-1]; first.X != last.X {
		t.Errorf("the centre went from %d to %d", first.X, last.X)
	}
}

func TestThePanSpeedIsLimited(t *testing.T) {
	tr := still(20)
	vw, vh := vertical(tr)

	// The speaker jumps from the left to the right at the tenth sample.
	for i := range tr.Samples {
		cx := 2000
		if i >= 10 {
			cx = 8000
		}

		tr.Samples[i].Faces = []Face{face(cx, 1400, 3000)}
	}

	o := DefaultOptions()
	kfs := Plan(tr, o, vw, vh, 0, 3800)

	for i := 1; i < len(kfs); i++ {
		dt := int(kfs[i].T - kfs[i-1].T)
		if moved := abs(kfs[i].X - kfs[i-1].X); moved > o.MaxPan*dt/1000 {
			t.Fatalf("keyframe %d moved %d in %d ms, over %d per second", i, moved, dt, o.MaxPan)
		}
	}

	if kfs[len(kfs)-1].X <= kfs[9].X {
		t.Error("the frame did not follow the speaker")
	}
}

func TestTheLastPositionIsHeldThenTheFrameEasesToTheCentre(t *testing.T) {
	tr := still(40)
	vw, vh := vertical(tr)

	for i := range tr.Samples {
		tr.Samples[i].Faces = nil
		if i < 10 {
			tr.Samples[i].Faces = []Face{face(2500, 1400, 3000)}
		}
	}

	kfs := Plan(tr, DefaultOptions(), vw, vh, 0, 7800)
	atHold, _ := At(kfs, 1800+1400)
	atEnd, _ := At(kfs, 7800)

	if atHold != kfs[0].X {
		t.Errorf("the frame left %d during the hold, at %d", kfs[0].X, atHold)
	}

	if centre := Unit / 2; abs(atEnd-centre) >= abs(kfs[0].X-centre) {
		t.Errorf("the frame did not ease towards the centre: %d", atEnd)
	}
}

func TestAStrongerFaceTakesOverOnlyAfterThreeSamples(t *testing.T) {
	left, right := face(2500, 1200, 2600), face(7500, 1800, 3800) // right is 2.2 times the area

	tr := still(12)
	vw, vh := vertical(tr)

	for i := range tr.Samples {
		tr.Samples[i].Faces = []Face{left, right}
		if i == 0 {
			tr.Samples[i].Faces = []Face{left}
		}
	}

	o := DefaultOptions().WithDefaults()
	o.SmoothingMS = 50

	kfs := Plan(tr, o, vw, vh, 0, 2200)

	if x, _ := At(kfs, 400); x > 3000 {
		t.Errorf("the frame jumped to the other face by sample 2: %d", x)
	}

	if x, _ := At(kfs, 2200); x < 6000 {
		t.Errorf("the frame never went to the stronger face: %d", x)
	}

	// A face only slightly bigger never takes over.
	right = face(7500, 1300, 2800)

	for i := range tr.Samples[1:] {
		tr.Samples[i+1].Faces = []Face{left, right}
	}

	if x, _ := At(Plan(tr, o, vw, vh, 0, 2200), 2200); x > 3000 {
		t.Errorf("a face of 1.2 times the area took over: %d", x)
	}
}

func TestWindowsAreAlwaysInsideTheSource(t *testing.T) {
	for _, src := range []Size{{1280, 720}, {1080, 1920}, {3000, 3000}} {
		for _, fr := range [][2]int{{1080, 1920}, {1920, 1080}, {1080, 1080}, {1080, 1350}} {
			vw, vh := Visible(src, fr[0], fr[1])

			for cx := -2000; cx <= 12000; cx += 700 {
				for cy := -2000; cy <= 12000; cy += 900 {
					w := WindowAt(vw, vh, cx, cy)
					if w.Left < 0 || w.Right < 0 || w.Top < 0 || w.Bottom < 0 ||
						w.Left+w.Right+vw != Unit || w.Top+w.Bottom+vh != Unit {
						t.Fatalf("window %+v of %dx%d at (%d,%d) is not inside the frame", w, vw, vh, cx, cy)
					}
				}
			}
		}
	}
}

func TestNoFacesGiveAStillCentre(t *testing.T) {
	tr := still(10)
	vw, vh := vertical(tr)

	kfs := Plan(tr, DefaultOptions(), vw, vh, 0, 1800)
	for _, k := range kfs {
		if k.X != Unit/2 {
			t.Errorf("keyframe %+v is not centred", k)
		}
	}

	if kfs := Plan(&Track{Version: Version}, DefaultOptions(), vw, vh, 0, 1000); len(kfs) != 2 {
		t.Errorf("an empty track gave %d keyframes, want 2", len(kfs))
	}
}

func TestOptions(t *testing.T) {
	if err := (Options{}).WithDefaults().Validate(); err != nil {
		t.Fatal(err)
	}

	for name, o := range map[string]Options{
		"smoothing": {SmoothingMS: 9000, DeadZone: 1, MaxPan: 1000, HoldMS: 1, KeyframeMS: 100},
		"dead zone": {SmoothingMS: 1, DeadZone: -1, MaxPan: 1000, HoldMS: 1, KeyframeMS: 100},
		"max pan":   {SmoothingMS: 1, DeadZone: 1, MaxPan: 5, HoldMS: 1, KeyframeMS: 100},
		"keyframe":  {SmoothingMS: 1, DeadZone: 1, MaxPan: 1000, HoldMS: 1, KeyframeMS: 10},
	} {
		if o.Validate() == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestTrackValidation(t *testing.T) {
	if err := movingSpeaker().Validate(); err != nil {
		t.Fatal(err)
	}

	for name, mut := range map[string]func(*Track){
		"version":  func(t *Track) { t.Version = 2 },
		"size":     func(t *Track) { t.Source.Width = 0 },
		"fps":      func(t *Track) { t.FPS = 0 },
		"range":    func(t *Track) { t.EndMS = -1 },
		"order":    func(t *Track) { t.Samples[3].T = t.Samples[2].T },
		"outside":  func(t *Track) { t.Samples[0].Faces[0].X = Unit },
		"score":    func(t *Track) { t.Samples[0].Faces[0].Score = 1001 },
		"empty":    func(t *Track) { t.Samples[0].Faces[0].W = 0 },
		"too many": func(t *Track) { t.Samples[0].Faces = make([]Face, MaxFacesPerShot+1) },
	} {
		tr := movingSpeaker()
		mut(tr)

		if tr.Validate() == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestAtInterpolatesInIntegers(t *testing.T) {
	kfs := []Keyframe{{T: 0, X: 1000, Y: 0}, {T: 1000, X: 2000, Y: 500}}

	if x, y := At(kfs, 500); x != 1500 || y != 250 {
		t.Errorf("At(500) = %d,%d", x, y)
	}

	if x, _ := At(kfs, -5); x != 1000 {
		t.Error("before the first keyframe it is the first")
	}

	if x, _ := At(kfs, 9999); x != 2000 {
		t.Error("after the last keyframe it is the last")
	}

	if x, y := At(nil, 0); x != Unit/2 || y != Unit/2 {
		t.Error("no keyframes is the centre")
	}
}
