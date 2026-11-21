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

// TestImageImportScenarioIsTheSameThroughEverySurface (SDD-021 section 10.1, epic 48): an image file imported from
// a path and from a hash, into a new document and into an existing one, a replay by key, a dry run and the
// refusals (a source over the canvas, a file that is not an image, nothing to import, an unknown hash and an
// unknown image) give the same results, errors and texts through the Service, HTTP, the CLI and the MCP tool.
func TestImageImportScenarioIsTheSameThroughEverySurface(t *testing.T) {
	sameThroughEverySurface(t, "image-import")
}

// The scenario above would pass if every surface answered nothing: this reads what each step must hold.
func TestImageImportScenarioSaysWhatItShould(t *testing.T) {
	steps, fixtures := loadScenario(t, "image-import")
	got := run(t, newDirect(t), steps, fixtures)

	for i, want := range map[int][]string{
		0:  {`"created":true`, `"width":640`, `"height":360`, `"image_id":"img_0000000002"`},
		1:  {`"width":1080`, `"height":1080`},
		2:  {`"asset":"sha256:63c0b55ce26237dfbd4c411ab124bba6377524fc879c96b61418b559cfdfec92"`, `"name":"photo"`},
		3:  {`"revision":2`, `"layer_id"`},
		4:  {`"replayed":true`, `"revision":2`},
		5:  {`"dry_run":true`, `"applied":false`},
		7:  {`"width":2000`, `"height":1200`, `"image_id":"img_0000000009"`},
		12: {`img_0000000002`, `img_0000000004`, `img_0000000009`},
	} {
		for _, frag := range want {
			if !strings.Contains(got[i].result, frag) {
				t.Errorf("step %d (%s) lacks %s: %s", i, steps[i].Call, frag, got[i].result)
			}
		}
	}

	for _, i := range []int{6, 8, 9, 10, 11} {
		if got[i].fail == "" {
			t.Errorf("step %d (%s) was accepted", i, steps[i].Call)
		}
	}
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

	// step 10: the PUT body reaches the Service with every member it names, the reuse mark included
	if !strings.Contains(got[10].result, `"reusable":true`) || !strings.Contains(got[10].result, "checked") {
		t.Errorf("AssetSetInfo did not apply its members: %s", got[10].result)
	}

	// the last step lists the reusable files: the mark set in step 10 keeps the file in
	if !strings.Contains(got[len(got)-1].result, `"name":"clip.mov"`) {
		t.Errorf("the reusable filter lost the file: %s", got[len(got)-1].result)
	}

	if strings.Contains(got[6].result, `"name":`) {
		t.Errorf("the visual section holds a video: %s", got[6].result)
	}

	// the clip is a video: the audio section, listed on every surface, holds no entry of it
	audio := len(got) - 3
	if strings.Contains(got[audio].result, `"name":`) {
		t.Errorf("the audio section holds a video: %s", got[audio].result)
	}

	// step 11: the detail names both projects
	if !strings.Contains(got[11].result, "Teaser") || !strings.Contains(got[11].result, "Trailer") {
		t.Errorf("the detail does not name both projects: %s", got[11].result)
	}

	if !strings.Contains(got[11].text, `<untrusted_media_text source="metadata" field="rights_note"`) {
		t.Errorf("the rights note is not delimited: %s", got[11].text)
	}
}
