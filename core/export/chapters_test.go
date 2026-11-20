package export

import (
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

var fps30 = timeline.Rational{Num: 30, Den: 1}

func TestChaptersFromLabelledMarkers(t *testing.T) {
	marks := []timeline.Marker{
		{Frame: 0, Label: "Intro"},
		{Frame: 45},
		{Frame: 90, Label: "  Main\tpart "},
		{Frame: 150, Label: "   "},
		{Frame: 240, Label: "Outro"},
	}

	got := Chapters(marks, fps30, 0, 300)
	want := []Chapter{
		{0, 3000, "Intro"},
		{3000, 8000, "Main part"},
		{8000, 10000, "Outro"},
	}

	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("chapter %d: got %v, want %v", i, got[i], want[i])
		}
	}
}

func TestChaptersOfARangeAreShiftedAndClipped(t *testing.T) {
	marks := []timeline.Marker{{Frame: 10, Label: "before"}, {Frame: 60, Label: "in"}, {Frame: 120, Label: "end"}, {Frame: 200, Label: "after"}}

	got := Chapters(marks, fps30, 30, 120)
	if len(got) != 1 || got[0] != (Chapter{StartMS: 1000, EndMS: 3000, Title: "in"}) {
		t.Fatalf("got %v", got)
	}
}

func TestChaptersNone(t *testing.T) {
	if got := Chapters(nil, fps30, 0, 30); got != nil {
		t.Fatalf("got %v", got)
	}

	if got := Chapters([]timeline.Marker{{Frame: 1}}, fps30, 0, 30); got != nil {
		t.Fatalf("an unlabelled marker made %v", got)
	}
}

func TestChaptersRoundTheTimeOfNTSC(t *testing.T) {
	got := Chapters([]timeline.Marker{{Frame: 30, Label: "a"}}, timeline.Rational{Num: 30000, Den: 1001}, 0, 60)
	if len(got) != 1 || got[0].StartMS != 1001 {
		t.Fatalf("got %v", got)
	}
}

func TestFFMetadataEscapesTheTitle(t *testing.T) {
	got := FFMetadata([]Chapter{{StartMS: 0, EndMS: 1000, Title: `a=b;c#d\e`}})
	want := ";FFMETADATA1\n[CHAPTER]\nTIMEBASE=1/1000\nSTART=0\nEND=1000\n" + `title=a\=b\;c\#d\\e` + "\n"

	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	if strings.Count(FFMetadata([]Chapter{{Title: "x\ny"}}), "\n") != 6 {
		t.Fatal("a newline in a title must not add a line")
	}
}
