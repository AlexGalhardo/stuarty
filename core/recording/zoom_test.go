package recording

import (
	"reflect"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// click is a press at the share (x, y) of the source, in tenths of a percent.
func click(ms int64, x, y int) Click { return Click{TMS: ms, X: x * 10, Y: y * 10} }

func TestPlanZoomsFiveSeparatedClicks(t *testing.T) {
	var clicks []Click
	for i := range 5 {
		clicks = append(clicks, click(int64(2000+i*10000), 100+i*200, 300+i*100))
	}

	zooms, dropped := PlanZooms(clicks, DefaultZoomOptions(), Unit, Unit, 0, 60000)
	if len(zooms) != 5 || dropped != 0 {
		t.Fatalf("%d zooms, %d dropped: %+v", len(zooms), dropped, zooms)
	}

	o := DefaultZoomOptions()

	for i, z := range zooms {
		if z.FromPrev || z.ToNext || z.Clicks != 1 {
			t.Errorf("zoom %d chained or merged: %+v", i, z)
		}

		first := clicks[i].TMS
		if z.FullMS != first-int64(o.LeadMS) || z.StartMS != z.FullMS-int64(o.RampMS) {
			t.Errorf("zoom %d ease in %+v", i, z)
		}

		if hold := z.ReleaseMS - first; hold < int64(o.HoldMS) {
			t.Errorf("zoom %d holds %d ms after the click, least %d", i, hold, o.HoldMS)
		}

		if z.EndMS-z.ReleaseMS != int64(o.RampMS) || z.FX != clicks[i].X || z.FY != clicks[i].Y {
			t.Errorf("zoom %d ease out or focus %+v", i, z)
		}

		if i > 0 && z.StartMS < zooms[i-1].EndMS {
			t.Errorf("zoom %d starts before the previous one ends", i)
		}
	}
}

func TestPlanZoomsIsDeterministic(t *testing.T) {
	clicks := []Click{click(3000, 100, 100), click(3900, 150, 120), click(20000, 800, 700)}
	a, da := PlanZooms(clicks, DefaultZoomOptions(), 7000, Unit, 0, 40000)
	b, db := PlanZooms(clicks, DefaultZoomOptions(), 7000, Unit, 0, 40000)

	if !reflect.DeepEqual(a, b) || da != db {
		t.Fatalf("two runs differ: %+v and %+v", a, b)
	}
}

func TestPlanZoomsGroupsNearClicks(t *testing.T) {
	clicks := []Click{click(3000, 400, 400), click(3800, 440, 420), click(5200, 480, 400)}

	zooms, _ := PlanZooms(clicks, DefaultZoomOptions(), Unit, Unit, 0, 30000)
	if len(zooms) != 1 || zooms[0].Clicks != 3 {
		t.Fatalf("want one zoom of three clicks: %+v", zooms)
	}

	if zooms[0].FX != 4400 || zooms[0].FY != 4100 {
		t.Errorf("the focus is the centre of the box of the clicks: %d, %d", zooms[0].FX, zooms[0].FY)
	}

	if zooms[0].ReleaseMS != 5200+1200 {
		t.Errorf("held until the hold after the last click: %d", zooms[0].ReleaseMS)
	}
}

func TestPlanZoomsSplitsClicksThatDoNotFitOneWindow(t *testing.T) {
	// At 1.6x the window is 62.5 % of the source and a zoom may span 60 % of it: 375 per thousand.
	clicks := []Click{click(3000, 100, 500), click(3500, 800, 500)}

	zooms, _ := PlanZooms(clicks, DefaultZoomOptions(), Unit, Unit, 0, 30000)
	if len(zooms) != 2 {
		t.Fatalf("clicks too far apart for one window: %+v", zooms)
	}
}

func TestPlanZoomsChainsClosePairsIntoAPan(t *testing.T) {
	clicks := []Click{click(3000, 100, 100), click(6500, 800, 800)}

	zooms, _ := PlanZooms(clicks, DefaultZoomOptions(), Unit, Unit, 0, 30000)
	if len(zooms) != 2 || !zooms[0].ToNext || !zooms[1].FromPrev {
		t.Fatalf("want a chained pair: %+v", zooms)
	}

	if zooms[0].EndMS != zooms[0].ReleaseMS || zooms[1].StartMS != zooms[0].ReleaseMS {
		t.Errorf("the view moves from the release of the first: %+v", zooms)
	}

	if zooms[1].FullMS-zooms[1].StartMS < minMoveMS {
		t.Errorf("pan shorter than %d ms: %+v", minMoveMS, zooms[1])
	}
}

func TestPlanZoomsStaysInsideTheClip(t *testing.T) {
	// A click 100 ms in cannot ease in from before the clip, and a click at the end cannot ease out after it.
	zooms, _ := PlanZooms(
		[]Click{click(100, 500, 500), click(9800, 500, 500)},
		DefaultZoomOptions(),
		Unit,
		Unit,
		0,
		10000,
	)
	if len(zooms) != 2 {
		t.Fatalf("%+v", zooms)
	}

	if zooms[0].StartMS != 0 || zooms[0].FullMS < MinRampMS {
		t.Errorf("first zoom %+v", zooms[0])
	}

	if last := zooms[1]; last.EndMS > 10000 || last.ReleaseMS > 10000 {
		t.Errorf("last zoom ends after the clip: %+v", last)
	}
}

func TestPlanZoomsDropsWhatDoesNotFit(t *testing.T) {
	var clicks []Click
	for i := range 30 {
		clicks = append(clicks, click(int64(2000+i*10000), 100, 100))
	}

	o := DefaultZoomOptions()
	o.MaxZooms = 24

	zooms, dropped := PlanZooms(clicks, o, Unit, Unit, 0, 400000)
	if len(zooms) != 24 || dropped != 6 {
		t.Fatalf("%d zooms, %d dropped", len(zooms), dropped)
	}
}

func TestPlanZoomsIgnoresClicksOutsideTheRange(t *testing.T) {
	zooms, _ := PlanZooms(
		[]Click{click(500, 100, 100), click(50000, 100, 100)},
		DefaultZoomOptions(),
		Unit,
		Unit,
		1000,
		20000,
	)
	if len(zooms) != 0 {
		t.Fatalf("%+v", zooms)
	}
}

func TestZoomOptionsValidate(t *testing.T) {
	if err := DefaultZoomOptions().Validate(); err != nil {
		t.Fatal(err)
	}

	for name, mut := range map[string]func(*ZoomOptions){
		"level":     func(o *ZoomOptions) { o.Level = 3001 },
		"level low": func(o *ZoomOptions) { o.Level = 1099 },
		"lead_ms":   func(o *ZoomOptions) { o.LeadMS = -1 },
		"ramp_ms":   func(o *ZoomOptions) { o.RampMS = 199 },
		"hold_ms":   func(o *ZoomOptions) { o.HoldMS = 499 },
		"merge_ms":  func(o *ZoomOptions) { o.MergeMS = 10001 },
		"max_zooms": func(o *ZoomOptions) { o.MaxZooms = 25 },
	} {
		o := DefaultZoomOptions()
		mut(&o)

		if err := o.Validate(); err == nil || !strings.Contains(err.Error(), strings.Fields(name)[0]) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// A pan is over before the click it goes to, so the view is on the second place when the click happens, and the
// first place is held the least time after its last click.
func TestPlanZoomsPanEndsBeforeTheNextClick(t *testing.T) {
	o := DefaultZoomOptions()
	o.LeadMS = 200

	zooms, _ := PlanZooms([]Click{click(800, 800, 300), click(1800, 100, 700)}, o, Unit, Unit, 0, 10000)
	if len(zooms) != 2 || !zooms[0].ToNext {
		t.Fatalf("want a chained pair: %+v", zooms)
	}

	if zooms[1].FullMS > 1800 {
		t.Errorf("the pan ends at %d ms, after the click at 1800", zooms[1].FullMS)
	}

	if hold := zooms[0].ReleaseMS - 800; hold < MinHoldMS {
		t.Errorf("the first place is held %d ms after its click, least %d", hold, MinHoldMS)
	}

	if zooms[1].StartMS != zooms[0].ReleaseMS {
		t.Errorf("the pan starts at the release: %+v", zooms)
	}
}

func TestZoomSpans(t *testing.T) {
	kf := func(frame int, v float64) timeline.Keyframe { return timeline.NewKeyframe(frame, v) }

	t.Run("two separated zooms", func(t *testing.T) {
		got := ZoomSpans([]timeline.Keyframe{
			kf(0, 1), kf(10, 1.6), kf(40, 1.6), kf(60, 1), kf(100, 1), kf(110, 1.6), kf(150, 1.6), kf(170, 1),
		}, 1)
		want := []ZoomSpan{
			{StartFrame: 0, FullFrame: 10, ReleaseFrame: 40, EndFrame: 60},
			{StartFrame: 100, FullFrame: 110, ReleaseFrame: 150, EndFrame: 170},
		}

		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %+v, want %+v", got, want)
		}
	})

	t.Run("a pan between two zooms keeps both", func(t *testing.T) {
		got := ZoomSpans([]timeline.Keyframe{kf(0, 1), kf(10, 1.6), kf(40, 1.6), kf(60, 2), kf(90, 2), kf(110, 1)}, 1)
		if len(got) != 2 || got[0].EndFrame != 40 || got[1].StartFrame != 40 {
			t.Fatalf("got %+v", got)
		}
	})

	t.Run("no zoom at rest", func(t *testing.T) {
		if got := ZoomSpans([]timeline.Keyframe{kf(0, 1), kf(50, 1)}, 1); len(got) != 0 {
			t.Fatalf("got %+v, want none", got)
		}

		if got := ZoomSpans(nil, 1); len(got) != 0 {
			t.Fatalf("got %+v, want none", got)
		}
	})
}
