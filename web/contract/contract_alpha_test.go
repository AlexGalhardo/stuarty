package contract

import (
	"bytes"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/web/contract/testenv"
)

// TestAlphaOperationsAreTheSameInProcessAndOverHTTP (ADR-0008, NFR-025, SDD-005 section 18 check
// 24): trash, restore, template, job listing and doctor give the same results, errors, texts and
// final document through the in-process Service and through the HTTP client, byte for byte.
func TestAlphaOperationsAreTheSameInProcessAndOverHTTP(t *testing.T) {
	steps, fixtures := loadScenario(t, "alpha")

	oracle := newDirectWith(t, testenv.Options{})
	want := run(t, oracle, steps, fixtures)
	wantDoc := finalDocument(t, oracle, "prj_7k2m9x4q1b")

	got := newHTTPWith(t, testenv.Options{})
	have := run(t, got, steps, fixtures)

	for i := range steps {
		if settled(have[i], want[i]) && have[i].result != want[i].result {
			t.Errorf(
				"step %d (%s): result differs\n got: %s\nwant: %s",
				i,
				steps[i].Call,
				have[i].result,
				want[i].result,
			)
		}

		if have[i].fail != want[i].fail {
			t.Errorf("step %d (%s): error differs\n got: %s\nwant: %s", i, steps[i].Call, have[i].fail, want[i].fail)
		}

		if settled(have[i], want[i]) && have[i].text != want[i].text {
			t.Errorf("step %d (%s): text differs\n got: %q\nwant: %q", i, steps[i].Call, have[i].text, want[i].text)
		}
	}

	if doc := finalDocument(t, got, "prj_7k2m9x4q1b"); !bytes.Equal(doc, wantDoc) {
		t.Errorf("the final documents differ\n got: %s\nwant: %s", doc, wantDoc)
	}
}

// TestMotionScenarioIsTheSameThroughEverySurface (epic 31, RF-080 and RF-082): the bezier easing,
// keyframe.set_easing, animation.apply_preset and animation.list_presets give the same results,
// errors, texts and final document through the Service, HTTP, the CLI and the MCP tools, and
// command_help describes the two new commands on each of them.
func TestMotionScenarioIsTheSameThroughEverySurface(t *testing.T) {
	docs := sameThroughEverySurface(t, "motion", "prj_7k2m9x4q1b")

	doc := string(docs["prj_7k2m9x4q1b"])
	for _, want := range []string{`"bezier":[0.36,0,0.66,-0.56]`, `"easing":"bezier"`, `"opacity"`} {
		if !strings.Contains(doc, want) {
			t.Errorf("the final document has no %s: %s", want, doc)
		}
	}
}
