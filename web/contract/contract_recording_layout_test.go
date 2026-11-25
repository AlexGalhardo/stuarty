package contract

import (
	"strings"
	"testing"
)

// The two commands that frame a recording (SDD-002 C.96 and C.97) give the same results, errors, texts and final
// document through the Service, HTTP, the CLI and the MCP tools: a layout of a screen and a webcam clip, the
// background behind it, a second layout that replaces the first, the refusals (a camera the layout does not take,
// an item that is not there) and the help of the two commands.
func TestRecordingLayoutScenarioIsTheSameThroughEverySurface(t *testing.T) {
	docs := sameThroughEverySurface(t, "recording-layout", "prj_7k2m9x4q1b")

	doc := string(docs["prj_7k2m9x4q1b"])
	for _, want := range []string{`"fit":"contain"`, `"scale":`, `"crop":{`} {
		if !strings.Contains(doc, want) {
			t.Errorf("the final document has no %s: %s", want, doc)
		}
	}

	if strings.Contains(doc, `"name":"Background`) {
		t.Errorf("the background was removed by the last edit, but the document still has one: %s", doc)
	}
}
