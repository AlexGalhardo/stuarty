package contract

import (
	"encoding/json"
	"math"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/service"
)

// refDoc is the part of a project document the checks of reference scenario 2 read.
type refDoc struct {
	Items map[string]struct {
		Type       string `json:"type"`
		Start      int    `json:"start"`
		Duration   int    `json:"duration"`
		SourceIn   int    `json:"source_in"`
		LinkedItem string `json:"linked_item"`
		Words      []struct {
			Text     string `json:"text"`
			Start    int    `json:"start"`
			Duration int    `json:"duration"`
		} `json:"words"`
	} `json:"items"`
	Settings struct {
		Width  int `json:"width"`
		Height int `json:"height"`
		FPS    struct {
			Num int `json:"num"`
			Den int `json:"den"`
		} `json:"fps"`
	} `json:"settings"`
}

func decodeRefDoc(t *testing.T, raw []byte) refDoc {
	t.Helper()

	var doc refDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}

	return doc
}

// seconds is the end of the last item of the timeline.
func (d *refDoc) seconds() float64 {
	end := 0

	for _, it := range d.Items {
		end = max(end, it.Start+it.Duration)
	}

	return float64(end) * float64(d.Settings.FPS.Den) / float64(d.Settings.FPS.Num)
}

// verifyProjectExport is SDD-004 section 8 on the file the export job of the project
// wrote: the checks the job reports passed, and ffprobe finds H.264, AAC when the
// file has audio, the size of the project, a duration within one frame of the
// timeline and the moov atom before the media data.
func verifyProjectExport(t *testing.T, p *procEnv, label, project string, doc *refDoc, wantAudio bool) {
	t.Helper()

	jobs, err := p.service().JobList(callerCtx(), service.JobListRequest{ProjectID: project, State: "all"})
	if err != nil {
		t.Fatal(err)
	}

	var exp *service.ExportResult

	for i := range jobs.Jobs {
		j := &jobs.Jobs[i]
		if j.Kind == service.JobExport && j.State == service.JobSucceeded && j.Result != nil && j.Result.Export != nil {
			exp = j.Result.Export

			break
		}
	}

	if exp == nil {
		t.Fatalf("%s %s: no succeeded export job in %+v", label, project, jobs.Jobs)
	}

	if len(exp.Checks) == 0 {
		t.Errorf("%s %s: the export reports no verification checks", label, project)
	}

	for _, c := range exp.Checks {
		if !c.OK {
			t.Errorf("%s %s: verification %s failed: %+v", label, project, c.Name, c)
		}
	}

	out, err := exec.CommandContext(t.Context(), p.ffprobe,
		"-v", "error", "-print_format", "json", "-show_streams", "-show_format", exp.Path).Output()
	if err != nil {
		t.Fatalf("%s %s: ffprobe: %v", label, project, err)
	}

	var pr probed
	if err := json.Unmarshal(out, &pr); err != nil {
		t.Fatal(err)
	}

	var video, audio bool

	for _, s := range pr.Streams {
		switch s.CodecType {
		case "video":
			video = true

			if s.CodecName != "h264" || s.Width != doc.Settings.Width || s.Height != doc.Settings.Height ||
				s.PixFmt != "yuv420p" {
				t.Errorf("%s %s: video stream is %+v, want h264 %dx%d yuv420p",
					label, project, s, doc.Settings.Width, doc.Settings.Height)
			}
		case "audio":
			audio = true

			if s.CodecName != "aac" {
				t.Errorf("%s %s: audio codec is %s, want aac", label, project, s.CodecName)
			}
		}
	}

	if !video || (wantAudio && !audio) {
		t.Errorf("%s %s: video=%v audio=%v, want video and audio=%v", label, project, video, audio, wantAudio)
	}

	got, err := strconv.ParseFloat(pr.Format.Duration, 64)
	if want := doc.seconds(); err != nil || math.Abs(got-want) > 1.0/30+0.001 {
		t.Errorf("%s %s: duration %v, want %vs within one frame (%v)", label, project, pr.Format.Duration, want, err)
	}

	if !moovBeforeMdat(t, exp.Path) {
		t.Errorf("%s %s: the moov atom is not before mdat: the file is not faststart", label, project)
	}
}

// checkCaptions reads the captions of the edited recording from the document: every
// word starts within 100 ms (three frames) of the fixed timing of the recording, and
// no caption reaches outside the clip it is linked to, so none covers a silence the
// scenario removed.
func checkCaptions(t *testing.T, label string, doc *refDoc, rec recordingText) {
	t.Helper()

	const toleranceFrames = 3

	frames := func(ms int) int { return ms * doc.Settings.FPS.Num / (1000 * doc.Settings.FPS.Den) }

	words := map[string]recordingWord{}
	for _, w := range rec.Words {
		words[w.Text] = w
	}

	var seen int

	for id, c := range doc.Items {
		if c.Type != "caption" {
			continue
		}

		clip, ok := doc.Items[c.LinkedItem]
		if !ok {
			t.Errorf("%s: caption %s is linked to %q, which is not in the document", label, id, c.LinkedItem)

			continue
		}

		if c.Start < clip.Start || c.Start+c.Duration > clip.Start+clip.Duration {
			t.Errorf("%s: caption %s [%d, %d) reaches outside its clip [%d, %d)",
				label, id, c.Start, c.Start+c.Duration, clip.Start, clip.Start+clip.Duration)
		}

		for _, w := range c.Words {
			ref, ok := words[strings.ToLower(w.Text)]
			if !ok {
				t.Errorf("%s: caption %s has the word %q, which the recording does not say", label, id, w.Text)

				continue
			}

			seen++

			want := clip.Start + frames(ref.StartMS) - clip.SourceIn
			if got := c.Start + w.Start; got < want-toleranceFrames || got > want+toleranceFrames {
				t.Errorf("%s: word %q starts at frame %d, the fixed timing puts it at %d (+-%d)",
					label, w.Text, got, want, toleranceFrames)
			}
		}
	}

	if seen != len(rec.Words) {
		t.Errorf("%s: the captions carry %d words, the recording has %d", label, seen, len(rec.Words))
	}
}

var referenceTwoProjects = []string{"prj_7k2m9x4q1b", "prj_6t4y2w8j1m", "prj_3d5f7h9k2n", "prj_8c4v6x2z5b"}

// TestReferenceScenario2 (P2-QA-005, SDD-008 section 5): a generated talking-head
// recording is imported, transcribed with the test provider, cut, stripped of its
// silences and captioned from the transcript, exported and verified; then each of the
// three starter templates is instantiated with the same media, exported and verified.
// The CLI process, MCP over stdio and MCP over HTTP drive it against a real `serve`,
// and the four documents must be equal after the three runs.
func TestReferenceScenario2(t *testing.T) {
	rec := readRecording(t)

	var fixed []recordingWord
	for _, w := range stubWords() {
		fixed = append(fixed, recordingWord{Text: w.Text, StartMS: int(w.StartMS), EndMS: int(w.EndMS)})
	}

	if !slices.Equal(fixed, rec.Words) {
		t.Fatalf("the words of the test provider %+v are not the fixed timings of the recording %+v", fixed, rec.Words)
	}

	first := map[string]string{}

	driveRealServerWith(t, "reference-2", referenceProject, true, testTranscriberEnv,
		func(t *testing.T, d driver, p *procEnv) {
			t.Helper()

			for _, project := range referenceTwoProjects {
				raw := finalDocument(t, d, project)
				doc := decodeRefDoc(t, raw)

				verifyProjectExport(t, p, d.name(), project, &doc, project == referenceProject)

				if project == referenceProject {
					checkCaptions(t, d.name(), &doc, rec)

					// A pure tone is quieter than the loudness target; that finding is the
					// recording, not the edit, so it is the one a clean lint may carry.
					lint, err := d.service().Lint(callerCtx(), service.LintRequest{ProjectID: project})
					if err != nil || lint.Errors != 0 {
						t.Fatalf("%s: lint of the edited recording = %+v, %v", d.name(), lint, err)
					}

					for _, f := range lint.Findings {
						if f.Code != "L_LOUDNESS" {
							t.Errorf("%s: lint of the edited recording is not clean: %+v", d.name(), f)
						}
					}
				}

				// The recording is the same bytes in each run, so the whole document is compared.
				norm := scrub(normalize(t, d, raw))
				if want, ok := first[project]; !ok {
					first[project] = norm
				} else if norm != want {
					t.Errorf("%s: the document of %s differs\n got: %s\nwant: %s", d.name(), project, norm, want)
				}
			}
		})
}
