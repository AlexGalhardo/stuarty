package contract

import (
	"strings"
	"testing"
)

// TestImageScenarioIsTheSameThroughEverySurface (SDD-021 section 11): the image operations give the same
// results, errors and texts through the in-process Service, the HTTP client, the CLI and the MCP tool, byte for
// byte: a document made blank and from a template, edited in a batch, dry run, replayed by key, undone and redone.
func TestImageScenarioIsTheSameThroughEverySurface(t *testing.T) {
	sameThroughEverySurface(t, "image")
}

// TestAssetLibraryScenarioIsTheSameThroughEverySurface (SDD-011 section 14, epic 08): an import with an origin,
// the filters of the library, the reuse by hash, the tags and the rights note give the same results, errors and
// texts through the Service, HTTP, the CLI and the media tools.
func TestAssetLibraryScenarioIsTheSameThroughEverySurface(t *testing.T) {
	sameThroughEverySurface(t, "assets", "prj_7k2m9x4q1b", "prj_8m3q6w9e2r")
}

// The scenario above would pass if every surface returned nothing: this reads the oracle for what each step
// must hold (epic 08 acceptance 2 and 3).
func TestAssetLibraryScenarioSaysWhatItShould(t *testing.T) {
	steps, fixtures := loadScenario(t, "assets")
	got := run(t, newDirect(t), steps, fixtures)

	// step 2: fetched_by_agent without a rights note
	if got[2].fail == "" || !strings.Contains(got[2].fail, "rights_note_required") {
		t.Errorf("a fetched file without a rights note: %q", got[2].fail)
	}

	// step 4: the tag and the reuse mark find the file; step 5: so does the text
	for _, i := range []int{4, 5} {
		if !strings.Contains(got[i].result, `"name":"clip.mov"`) ||
			!strings.Contains(got[i].result, `"reusable":true`) {
			t.Errorf("step %d (%s) did not find the file: %s", i, steps[i].Call, got[i].result)
		}
	}

	if strings.Contains(got[6].result, `"name":`) {
		t.Errorf("the visual section holds a video: %s", got[6].result)
	}

	// step 11: the detail names both projects
	if !strings.Contains(got[11].result, "Teaser") || !strings.Contains(got[11].result, "Trailer") {
		t.Errorf("the detail does not name both projects: %s", got[11].result)
	}

	if !strings.Contains(got[11].text, `<untrusted_media_text source="metadata" field="rights_note"`) {
		t.Errorf("the rights note is not delimited: %s", got[11].text)
	}
}
