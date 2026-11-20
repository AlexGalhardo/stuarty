package export

import (
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

var fps30 = timeline.Rational{Num: 30, Den: 1}

// Characters built from their code points, so that the source holds no invisible character: the Unicode
// line and paragraph separators, the next line control, a no-break space and the right-to-left mark.
var (
	lineSep = string(rune(0x2028))
	paraSep = string(rune(0x2029))
	nel     = string(rune(0x85))
	nbsp    = string(rune(0xa0))
	rlm     = string(rune(0x200f))
)

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
	marks := []timeline.Marker{
		{Frame: 10, Label: "before"},
		{Frame: 60, Label: "in"},
		{Frame: 120, Label: "end"},
		{Frame: 200, Label: "after"},
	}

	got := Chapters(marks, fps30, 30, 120)
	if len(got) != 2 || got[0] != (Chapter{StartMS: 0, EndMS: 1000}) ||
		got[1] != (Chapter{StartMS: 1000, EndMS: 3000, Title: "in"}) {
		t.Fatalf("got %v", got)
	}
}

// A first labelled marker after the first frame gets an untitled chapter before it, so its chapter
// starts where the marker is in every reader (an MP4 chapter list starts at 0).
func TestChaptersLeadBeforeTheFirstMarker(t *testing.T) {
	got := Chapters([]timeline.Marker{{Frame: 13, Label: "Chorus"}, {Frame: 33, Label: "Verse"}}, fps30, 0, 43)
	want := []Chapter{{0, 433, ""}, {433, 1100, "Chorus"}, {1100, 1433, "Verse"}}

	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("chapter %d: got %v, want %v", i, got[i], want[i])
		}
	}
}

// A title never ends with a backslash, which the FFmpeg reader takes as the escape of the end of the line.
func TestChapterTitleHasNoTrailingBackslash(t *testing.T) {
	for label, want := range map[string]string{
		`C:\`:                      "C:",
		"a \\ \\\t":                "a",
		`\`:                        "",
		`back\slash`:               `back\slash`,
		"  x" + nbsp + "\\  ":      "x",
		"\\ lead":                  `\ lead`,
		"a\\" + lineSep + "\\\r\n": "a",
	} {
		if got := chapterTitle(label); got != want {
			t.Errorf("chapterTitle(%q) = %q, want %q", label, got, want)
		}
	}

	if got := FFMetadata([]Chapter{{Title: `C:\`}}); !strings.HasSuffix(got, "title=C:\n") {
		t.Fatalf("FFMetadata wrote %q", got)
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
	if len(got) != 2 || got[0].EndMS != 1001 || got[1].StartMS != 1001 || got[1].EndMS != 2002 {
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

// FFMetadata keeps a title on its line even when its caller did not clean it: the FFmpeg reader ends a
// line at a NUL and at a CR too, and a title that ended its line could add a chapter or a key (T-102).
func TestFFMetadataTitleNeverEndsItsLine(t *testing.T) {
	inject := "[CHAPTER]%sTIMEBASE=1/1000%sSTART=1%sEND=9%stitle=evil"

	for _, sep := range []string{"\n", "\r", "\r\n", "\x00", lineSep, paraSep, nel, "\x0b", "\x0c"} {
		title := "a" + sep + strings.ReplaceAll(inject, "%s", sep)
		got := FFMetadata([]Chapter{{StartMS: 0, EndMS: 1000, Title: title}})

		lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
		if len(lines) != 6 || strings.ContainsAny(got, "\r\x00\x0b\x0c"+lineSep+paraSep+nel) {
			t.Fatalf("separator %q: the file is %q", sep, got)
		}

		if !strings.HasPrefix(lines[5], "title=a ") || strings.Count(got, "\n[CHAPTER]") != 1 {
			t.Fatalf("separator %q: the title line is %q", sep, lines[5])
		}
	}

	// A right-to-left mark is part of a title, not a separator.
	if got := FFMetadata([]Chapter{{Title: rlm + "shalom"}}); !strings.Contains(got, "title="+rlm+"shalom\n") {
		t.Fatalf("the bidirectional mark was lost: %q", got)
	}
}
