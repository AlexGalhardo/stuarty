package recording

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

const hdr = `{"k":"header","v":1,"width":1920,"height":1080,"clock":"active_ms"}`

func file(lines ...string) string { return strings.Join(append([]string{hdr}, lines...), "\n") + "\n" }

func TestReadEventsKeepsClicksAndMoves(t *testing.T) {
	data := file(
		`{"k":"move","t":10,"x":5,"y":6}`,
		`{"k":"down","t":20,"b":"left","x":100,"y":200}`,
		`{"k":"up","t":120,"b":"left","x":101,"y":200}`,
		`{"k":"move","t":130,"x":1919,"y":1079}`,
	)

	all, err := ReadEvents(strings.NewReader(data), EventsQuery{})
	if err != nil || all.Total != 4 || len(all.Events) != 4 || all.Truncated || all.Discarded != 0 {
		t.Fatalf("all: %+v, %v", all, err)
	}

	if all.Width != 1920 || all.Height != 1080 {
		t.Errorf("size %dx%d", all.Width, all.Height)
	}

	clicks, err := ReadEvents(strings.NewReader(data), EventsQuery{Clicks: true})
	if err != nil || clicks.Total != 2 || clicks.Events[0].Kind != EventDown || clicks.Events[0].Button != ButtonLeft ||
		clicks.Events[0].X != 100 || clicks.Events[0].Y != 200 || clicks.Events[0].TMS != 20 {
		t.Fatalf("clicks: %+v, %v", clicks, err)
	}

	from, err := ReadEvents(strings.NewReader(data), EventsQuery{FromMS: 120, Clicks: true})
	if err != nil || from.Total != 1 || from.Events[0].Kind != EventUp {
		t.Fatalf("from: %+v, %v", from, err)
	}
}

func TestReadEventsTruncatesAtTheLimit(t *testing.T) {
	var b strings.Builder

	b.WriteString(hdr + "\n")

	for i := range 50 {
		fmt.Fprintf(&b, `{"k":"move","t":%d,"x":1,"y":1}`+"\n", i)
	}

	res, err := ReadEvents(strings.NewReader(b.String()), EventsQuery{Limit: 10})
	if err != nil || len(res.Events) != 10 || res.Total != 50 || !res.Truncated {
		t.Fatalf("%+v, %v", res, err)
	}
}

// A file that holds anything but the four kinds, or a member the format does not have, never reaches a caller
// (SDD-025 section 16.6): the privacy rule is that no key is in the file, and the reader is the second lock.
func TestReadEventsDiscardsEverythingThatIsNotThePointer(t *testing.T) {
	bad := []string{
		`{"k":"key","t":10,"code":65}`,
		`{"k":"key","t":10,"x":1,"y":1}`,
		`{"k":"down","t":10,"b":"left","x":1,"y":1,"key":"a"}`,
		`{"k":"down","t":10,"b":"keyboard","x":1,"y":1}`,
		`{"k":"down","t":10,"x":1,"y":1}`,
		`{"k":"move","t":10,"b":"left","x":1,"y":1}`,
		`{"k":"move","t":-1,"x":1,"y":1}`,
		`{"k":"move","t":10,"x":-1,"y":1}`,
		`{"k":"move","t":10,"x":1921,"y":1}`,
		`{"k":"move","t":10,"x":1920,"y":1}`,
		`{"k":"up","t":10,"b":"left","x":1,"y":1080}`,
		`{"k":"move","t":99999999999,"x":1,"y":1}`,
		`{"k":"move","x":1,"y":1}`,
		`{"k":"move","t":10,"x":1}`,
		`{"k":"header","v":1,"width":1,"height":1,"clock":"active_ms"}`,
		`{"k":"move","t":10,"x":1,"y":1} {"k":"key"}`,
		`not json`,
		`{"k":"move","t":10,"x":1,"y":1,"pad":"` + strings.Repeat("a", 100) + `"}`,
		`{"k":"move","t":5,"x":1,"y":1}`,
	}

	lines := append([]string{`{"k":"move","t":10,"x":1,"y":1}`}, bad...)

	res, err := ReadEvents(strings.NewReader(file(lines...)), EventsQuery{})
	if err != nil {
		t.Fatal(err)
	}

	if len(res.Events) != 1 || res.Total != 1 || res.Discarded != len(bad) {
		t.Fatalf("kept %d, discarded %d of %d: %+v", len(res.Events), res.Discarded, len(bad), res.Events)
	}
}

func TestReadEventsNeedsAHeader(t *testing.T) {
	for name, data := range map[string]string{
		"empty":        "",
		"no header":    `{"k":"move","t":1,"x":1,"y":1}` + "\n",
		"bad header":   `{"k":"header","v":2,"width":10,"height":10,"clock":"active_ms"}` + "\n",
		"huge header":  `{"k":"header","v":1,"width":99999,"height":10,"clock":"active_ms"}` + "\n",
		"wrong clock":  `{"k":"header","v":1,"width":10,"height":10,"clock":"wall"}` + "\n",
		"header + key": `{"k":"header","v":1,"width":10,"height":10,"clock":"active_ms","key":"a"}` + "\n",
	} {
		if _, err := ReadEvents(strings.NewReader(data), EventsQuery{}); !errors.Is(err, ErrEvents) {
			t.Errorf("%s: %v; want ErrEvents", name, err)
		}
	}
}

func TestReadEventsStopsAtALineThatIsTooLong(t *testing.T) {
	long := `{"k":"move","t":1,"x":1,"y":1,"pad":"` + strings.Repeat("a", 400) + `"}`

	if _, err := ReadEvents(strings.NewReader(file(long)), EventsQuery{}); !errors.Is(err, ErrEvents) {
		t.Errorf("a long line: %v", err)
	}
}

func TestNormalizeQuery(t *testing.T) {
	if q, err := NormalizeQuery(EventsQuery{}); err != nil || q.Limit != DefaultEventsLimit {
		t.Errorf("default: %+v, %v", q, err)
	}

	for _, q := range []EventsQuery{{Limit: -1}, {Limit: MaxEventsLimit + 1}, {FromMS: -5}} {
		if _, err := NormalizeQuery(q); err == nil {
			t.Errorf("%+v was accepted", q)
		}
	}

	if _, err := NormalizeQuery(EventsQuery{Limit: MaxEventsLimit}); err != nil {
		t.Error("the largest limit was refused")
	}
}
