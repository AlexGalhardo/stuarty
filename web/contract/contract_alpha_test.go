package contract

import (
	"bytes"
	"testing"

	"github.com/AlexGalhardo/vukem-video/web/contract/testenv"
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
		if have[i].result != want[i].result {
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

		if have[i].text != want[i].text {
			t.Errorf("step %d (%s): text differs\n got: %q\nwant: %q", i, steps[i].Call, have[i].text, want[i].text)
		}
	}

	if doc := finalDocument(t, got, "prj_7k2m9x4q1b"); !bytes.Equal(doc, wantDoc) {
		t.Errorf("the final documents differ\n got: %s\nwant: %s", doc, wantDoc)
	}
}
