package subtitle

import (
	"slices"
	"testing"
)

// FuzzParse feeds any text to the subtitle parser as SRT and as WebVTT (SDD-008 section 13.2): no panic, no more
// than MaxCues cues, and a text it read is written and read back to the same cues.
func FuzzParse(f *testing.F) {
	f.Add("1\n00:00:01,000 --> 00:00:02,500\nHello\n\n2\n00:00:03,000 --> 00:00:04,000\nWorld\n")
	f.Add("WEBVTT\n\n00:01.000 --> 00:02.000 align:start\n<b>Hi</b>\n\nNOTE x\n")
	f.Add("\ufeff00:00:00,000 --> 99:99:99,999\nx")
	f.Add("-->\n\n-->\n")

	f.Fuzz(func(t *testing.T, text string) {
		for _, format := range []string{SRT, VTT} {
			cues, err := Parse(format, text)
			if err != nil {
				continue
			}

			if len(cues) > MaxCues {
				t.Fatalf("%s: %d cues, more than %d", format, len(cues), MaxCues)
			}

			out, err := Format(format, cues)
			if err != nil {
				t.Fatalf("%s: a parsed text does not format: %v", format, err)
			}

			again, err := Parse(format, out)
			if err != nil {
				t.Fatalf("%s: the written text does not parse: %v\n%s", format, err, out)
			}

			if !slices.Equal(cues, again) {
				t.Fatalf("%s: the cues change through a write and a read:\n%v\n%v", format, cues, again)
			}
		}
	})
}
