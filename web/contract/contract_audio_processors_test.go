package contract

import (
	"strings"
	"testing"
)

// The audio processor commands of epic 34 (SDD-002 C.79 to C.83) give the same results, errors, texts and final
// document through the Service, HTTP, the CLI and the MCP tools: a processor added to a clip, to an audio item and to a
// track, changed, moved, removed, a preset applied, and the refusals of the core (a value out of its range, a marked
// noise profile on a track). Every kind is on the clip and on the track at some step, and the sound measure is asked
// through each surface.
func TestAudioProcessorsScenarioIsTheSameThroughEverySurface(t *testing.T) {
	docs := sameThroughEverySurface(t, "audio-processors", "prj_7k2m9x4q1b")

	doc := string(docs["prj_7k2m9x4q1b"])
	for _, want := range []string{
		`"processors"`,
		`"kind":"noise_reduction"`,
		`"kind":"limiter"`,
		`"kind":"deesser"`,
		`"hp_hz":80`,
		`"threshold_db":-48`,
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("the final document has no %s: %s", want, doc)
		}
	}
}
