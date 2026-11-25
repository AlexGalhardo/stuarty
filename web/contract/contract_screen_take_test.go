package contract

import (
	"strings"
	"testing"
)

// The commands that finish a screen take (SDD-002 C.93 to C.95) give the same results, errors, texts and final
// document through the Service, HTTP, the CLI and the MCP tools: the zoom on the clicks of a seeded recording, the
// smoothed cursor and the click highlights, the refusals of the service (a clip with no recording, an item that is
// not there) and the help of the three commands.
func TestScreenTakeEffectsScenarioIsTheSameThroughEverySurface(t *testing.T) {
	docs := sameThroughEverySurface(t, "screen-take-effects", "prj_7k2m9x4q1b")

	doc := string(docs["prj_7k2m9x4q1b"])
	for _, want := range []string{
		`"name":"Cursor itm_m1n2p3q4r5"`,
		`"name":"Click itm_m1n2p3q4r5 1"`,
		`"name":"Click itm_m1n2p3q4r5 2"`,
		`"name":"Cursor"`,
		`"name":"Click highlight"`,
		`"keyframes":{`,
		`"scale":[`,
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("the final document has no %s: %s", want, doc)
		}
	}
}
