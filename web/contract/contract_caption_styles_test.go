package contract

import (
	"strings"
	"testing"
)

// The commands of epic 71 (SDD-002 C.88 to C.91) give the same results, errors, texts and final document through
// the Service, HTTP, the CLI and the MCP tools: a caption style applied to a track, the blur of a region of a clip
// on a Blur track, the studio voice presets on an item and a track, and the refusals of the core (an unknown style,
// a blur too weak, a region outside the picture, a voice preset on a caption).
func TestCaptionStylesBlurVoiceScenarioIsTheSameThroughEverySurface(t *testing.T) {
	docs := sameThroughEverySurface(t, "caption-styles-blur-voice", "prj_7k2m9x4q1b")

	doc := string(docs["prj_7k2m9x4q1b"])
	for _, want := range []string{
		`"caption_style"`,
		`"font_family":"Inter"`,
		`"highlight_background":"#2563EB"`,
		`"name":"Blur region"`,
		`"name":"Blur"`,
		`"blur":44`,
		`"mask":{`,
		`"kind":"noise_reduction"`,
		`"kind":"limiter"`,
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("the final document has no %s: %s", want, doc)
		}
	}
}
