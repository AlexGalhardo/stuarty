package recording

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestReadTrackClicksOnlyInTheRange(t *testing.T) {
	data := file(
		`{"k":"move","t":10,"x":5,"y":6}`,
		`{"k":"down","t":500,"b":"left","x":100,"y":200}`,
		`{"k":"up","t":600,"b":"left","x":101,"y":200}`,
		`{"k":"down","t":2000,"b":"right","x":1919,"y":1079}`,
		`{"k":"down","t":9000,"b":"left","x":7,"y":8}`,
	)

	tr, err := ReadTrack(strings.NewReader(data), TrackQuery{FromMS: 400, ToMS: 5000, ClicksOnly: true})
	if err != nil || tr.Width != 1920 || tr.Height != 1080 || len(tr.Events) != 2 {
		t.Fatalf("%+v, %v", tr, err)
	}

	if tr.Events[0].TMS != 500 || tr.Events[1].X != 1919 || tr.Validate() != nil {
		t.Errorf("%+v", tr.Events)
	}

	clicks := tr.Clicks()
	if len(clicks) != 2 || clicks[0].X != shareOf(100, 1920) || clicks[0].Y != shareOf(200, 1080) {
		t.Errorf("%+v", clicks)
	}
}

func TestReadTrackKeepsThePositionAtTheStart(t *testing.T) {
	data := file(
		`{"k":"move","t":10,"x":5,"y":6}`,
		`{"k":"move","t":900,"x":50,"y":60}`,
		`{"k":"move","t":1500,"x":70,"y":80}`,
	)

	tr, err := ReadTrack(strings.NewReader(data), TrackQuery{FromMS: 1000, ToMS: 3000})
	if err != nil || len(tr.Events) != 2 {
		t.Fatalf("%+v, %v", tr, err)
	}

	if first := tr.Events[0]; first.TMS != 1000 || first.Kind != EventMove || first.X != 50 || first.Y != 60 {
		t.Errorf("the pointer is where it was last seen: %+v", first)
	}
}

func TestReadTrackThinsMoves(t *testing.T) {
	lines := make([]string, 0, 400)
	for i := range 400 {
		lines = append(lines, fmt.Sprintf(`{"k":"move","t":%d,"x":%d,"y":10}`, i*16, i))
	}

	tr, err := ReadTrack(strings.NewReader(file(lines...)), TrackQuery{FromMS: 0, ToMS: 6400})
	if err != nil {
		t.Fatal(err)
	}

	if len(tr.Events) > 6400/MinTrackStepMS+1 || len(tr.Events) < 100 {
		t.Fatalf("%d positions", len(tr.Events))
	}

	for i := 1; i < len(tr.Events); i++ {
		if tr.Events[i].TMS-tr.Events[i-1].TMS < MinTrackStepMS {
			t.Fatalf("positions %d ms apart", tr.Events[i].TMS-tr.Events[i-1].TMS)
		}
	}
}

func TestReadTrackRefusesWhatIsNotAnEventFile(t *testing.T) {
	if _, err := ReadTrack(strings.NewReader(`{"k":"move","t":1,"x":1,"y":1}`+"\n"), TrackQuery{ToMS: 10}); err == nil {
		t.Error("a file without a header was read")
	}

	if _, err := ReadTrack(strings.NewReader(file()), TrackQuery{FromMS: 5, ToMS: 5}); err == nil {
		t.Error("an empty range was read")
	}
}

func TestPointerTrackValidate(t *testing.T) {
	ok := PointerTrack{Width: 100, Height: 100, Events: []PointerEvent{{TMS: 1, Kind: EventMove, X: 99, Y: 0}}}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}

	for name, tr := range map[string]PointerTrack{
		"no size":    {Events: ok.Events},
		"too big":    {Width: MaxSide + 1, Height: 10},
		"outside":    {Width: 100, Height: 100, Events: []PointerEvent{{Kind: EventMove, X: 100}}},
		"kind":       {Width: 100, Height: 100, Events: []PointerEvent{{Kind: "key"}}},
		"time order": {Width: 100, Height: 100, Events: []PointerEvent{{TMS: 5, Kind: EventMove}, {TMS: 4, Kind: EventMove}}},
		"too many":   {Width: 100, Height: 100, Events: make([]PointerEvent, MaxTrackEvents+1)},
	} {
		if tr.Validate() == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

func TestSmoothPathFollowsAndLags(t *testing.T) {
	ev := []PointerEvent{{TMS: 0, Kind: EventMove, X: 0, Y: 0}, {TMS: 1000, Kind: EventMove, X: 1000, Y: 500}}

	raw := SmoothPath(ev, 0, 3000, 100, 0)
	if raw[0].XM != 0 || raw[len(raw)-1].XM != 1_000_000 || raw[len(raw)-1].TMS != 3000 {
		t.Fatalf("tau 0 follows the raw path: %+v ... %+v", raw[0], raw[len(raw)-1])
	}

	soft := SmoothPath(ev, 0, 3000, 100, 300)
	if len(soft) != len(raw) {
		t.Fatalf("%d and %d samples", len(soft), len(raw))
	}

	for i := range soft {
		if soft[i].TMS != raw[i].TMS {
			t.Fatalf("sample %d at %d and %d", i, soft[i].TMS, raw[i].TMS)
		}

		if soft[i].XM > raw[i].XM || (i > 0 && soft[i].XM < soft[i-1].XM) {
			t.Fatalf("sample %d: smoothed %d, raw %d, previous %d", i, soft[i].XM, raw[i].XM, soft[i-1].XM)
		}
	}

	if last := soft[len(soft)-1]; last.XM < 990_000 {
		t.Errorf("after 2 s the pointer has caught up: %d", last.XM)
	}

	if mid := soft[12]; mid.XM >= raw[12].XM || mid.XM <= 0 {
		t.Errorf("shortly after the move the pointer lags: %d against %d", mid.XM, raw[12].XM)
	}
}

func TestSmoothPathIsDeterministicAndHoldsTheLastPosition(t *testing.T) {
	ev := []PointerEvent{{TMS: 0, X: 10, Y: 20}, {TMS: 400, X: 700, Y: 300}, {TMS: 450, X: 710, Y: 310}}

	a, b := SmoothPath(ev, 0, 2000, 50, 120), SmoothPath(ev, 0, 2000, 50, 120)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("two runs differ")
	}

	if got := SmoothPath(ev, 0, 0, 50, 120); len(got) != 1 {
		t.Errorf("an empty range has one sample: %d", len(got))
	}

	if SmoothPath(nil, 0, 10, 50, 0) != nil {
		t.Error("no events, no path")
	}
}
