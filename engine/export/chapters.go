package export

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"

	exportcore "github.com/AlexGalhardo/stuarty/core/export"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg/ffargs"
)

// chapterPlan is what the export writes as chapters (SDD-004 section 8.1): the labelled markers of
// the timeline inside the exported range, planned by core/export. It is empty for a project with none.
func (s *state) chapterPlan() []exportcore.Chapter {
	return exportcore.Chapters(s.job.Doc.Markers, s.job.Doc.Settings.FPS, s.offset, s.offset+s.frames)
}

// chapterInput writes the ffmetadata file of the chapters into the run directory and returns it as
// an input, or ok false when there are no chapters and the command stays as it was.
func (s *state) chapterInput() (in ffargs.Input, ok bool, err error) {
	chapters := s.chapterPlan()
	if len(chapters) == 0 {
		return ffargs.Input{}, false, nil
	}

	path, err := s.dir.Path(s.job.JobID + ".chapters.ffmeta")
	if err != nil {
		return ffargs.Input{}, false, exportErr(ReasonEncoder, "The chapter file could not be placed.", err, "")
	}

	if err := os.WriteFile(path, []byte(exportcore.FFMetadata(chapters)), 0o600); err != nil {
		return ffargs.Input{}, false, exportErr(ReasonEncoder, "The chapter file could not be written.", err, "")
	}

	return ffargs.MetadataInput(path), true, nil
}

type probedChapters struct {
	Chapters []struct {
		Start string `json:"start_time"`
		End   string `json:"end_time"`
		Tags  struct {
			Title string `json:"title"`
		} `json:"tags"`
	} `json:"chapters"`
}

// verifyChapters reads the chapters back with ffprobe and returns the checks that fail: as many as
// planned, each at its planned time (within a millisecond and a half) with its title.
func (s *state) verifyChapters(ctx context.Context, partial string) ([]Check, error) {
	want := s.chapterPlan()

	argv, err := ffargs.ProbeChapters(s.job.Runner.Policy(s.dir), partial)
	if err != nil {
		return nil, exportErr(ReasonRender, "The verification command was refused.", err, "")
	}

	raw, res, err := s.job.Runner.RunFFprobe(ctx, s.dir, argv, verifyTimeout)
	if err != nil {
		return nil, wrapRun(err, ReasonRender, "The exported file could not be read back.", res.Stderr)
	}

	var got probedChapters
	if err := json.Unmarshal(raw, &got); err != nil {
		return []Check{{Name: "chapters", Expected: "ffprobe JSON", Got: err.Error()}}, nil
	}

	if len(got.Chapters) != len(want) {
		return []Check{{
			Name: "chapters", Expected: strconv.Itoa(len(want)) + " chapters", Got: strconv.Itoa(len(got.Chapters)) + " chapters",
		}}, nil
	}

	var failed []Check

	for i, w := range want {
		g := got.Chapters[i]
		start, _ := strconv.ParseFloat(g.Start, 64)
		end, _ := strconv.ParseFloat(g.End, 64)

		if math.Abs(start*1000-float64(w.StartMS)) > 1.5 || math.Abs(end*1000-float64(w.EndMS)) > 1.5 || g.Tags.Title != w.Title {
			failed = append(failed, Check{
				Name:     fmt.Sprintf("chapter_%d", i+1),
				Expected: fmt.Sprintf("%q from %d to %d ms", w.Title, w.StartMS, w.EndMS),
				Got:      fmt.Sprintf("%q from %.0f to %.0f ms", g.Tags.Title, start*1000, end*1000),
			})
		}
	}

	return failed, nil
}

// chapterTracks is the number of data streams an exported MP4 carries: the FFmpeg MP4 muxer writes
// the chapters twice, as a Nero `chpl` atom and as a QuickTime chapter track, and ffprobe lists the
// track as one `data` stream (an audio only file gets the atom and no track). A file with no chapters has none.
func (s *state) chapterTracks() int {
	if len(s.chapterPlan()) > 0 {
		return 1
	}

	return 0
}

// chapterTrackNote completes the expectation of the streams check when a chapter track is allowed.
func (s *state) chapterTrackNote() string {
	if s.chapterTracks() > 0 {
		return " and the data stream of the chapters"
	}

	return " only"
}
