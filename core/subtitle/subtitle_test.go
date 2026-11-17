package subtitle

import (
	"reflect"
	"strings"
	"testing"
)

const srt = "\ufeff1\r\n00:00:01,000 --> 00:00:02,500\r\nHello <i>there</i>\r\nsecond line\r\n\r\n2\r\n00:01:00,5 --> 01:00:00,000\r\n{\\an8}Bye\r\n"

func TestParseSRT(t *testing.T) {
	got, err := Parse(SRT, srt)
	if err != nil {
		t.Fatal(err)
	}
	want := []Cue{
		{StartMs: 1000, EndMs: 2500, Text: "Hello there\nsecond line"},
		{StartMs: 60500, EndMs: 3600000, Text: "Bye"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestParseVTTDropsHeaderNotesAndSettings(t *testing.T) {
	text := "WEBVTT - title\n\nNOTE a note\nover two lines\n\nintro\n00:01.000 --> 00:03.250 align:start position:10%\n<c.yellow>Hi</c>\n\n" +
		"STYLE\n::cue { color: red }\n\n00:00:04.000 --> 00:00:05.000\n\n"
	got, err := Parse(VTT, text)
	if err != nil {
		t.Fatal(err)
	}
	if want := []Cue{{StartMs: 1000, EndMs: 3250, Text: "Hi"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if Detect(text) != VTT || Detect(srt) != SRT {
		t.Error("Detect told the formats apart wrongly")
	}
}

func TestParseRefusesBadTimes(t *testing.T) {
	for name, text := range map[string]string{
		"not a time":   "1\n00:00:01 --> 00:00:02,000\nx\n",
		"ends first":   "1\n00:00:02,000 --> 00:00:01,000\nx\n",
		"minutes > 59": "1\n00:61:00,000 --> 00:62:00,000\nx\n",
	} {
		if _, err := Parse(SRT, text); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	if _, err := Parse("ass", srt); err == nil {
		t.Error("an unknown format must be refused")
	}
	if _, err := Parse(SRT, strings.Repeat("1\n00:00:01,000 --> 00:00:02,000\nx\n\n", MaxCues+1)); err == nil {
		t.Error("more than MaxCues cues must be refused")
	}
}

func TestFormatRoundTrips(t *testing.T) {
	cues := []Cue{{StartMs: 1000, EndMs: 2500, Text: "a\nb"}, {StartMs: 3723004, EndMs: 3725000, Text: "c"}}
	for _, f := range []string{SRT, VTT} {
		out, err := Format(f, cues)
		if err != nil {
			t.Fatal(err)
		}
		back, err := Parse(f, out)
		if err != nil || !reflect.DeepEqual(back, cues) {
			t.Errorf("%s: got %+v (%v) from\n%s", f, back, err, out)
		}
	}
	srtOut, _ := Format(SRT, cues)
	vttOut, _ := Format(VTT, cues)
	if !strings.Contains(srtOut, "00:00:01,000 --> 00:00:02,500") || !strings.HasPrefix(vttOut, "WEBVTT\n\n") ||
		!strings.Contains(vttOut, "01:02:03.004") {
		t.Errorf("unexpected text:\n%s\n%s", srtOut, vttOut)
	}
}
