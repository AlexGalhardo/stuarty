package export

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/engine/audio"
)

// MaxMeasureSeconds is the longest stretch AudioMeasure renders (SDD-004 section 7.1).
const MaxMeasureSeconds = 600

// ErrMeasureTooLong is returned for a range longer than MaxMeasureSeconds.
var ErrMeasureTooLong = errors.New("export: the range to measure is longer than 600 seconds")

// AudioMeasure is what AudioMeasure says about a range: the same mix measured with the processors of the items and of
// their tracks (After) and without them (Before), both before the loudness gain of the export.
type AudioMeasure struct {
	Before audio.Measurement `json:"before"`
	After  audio.Measurement `json:"after"`
	// Start and End are the frames measured, the end exclusive, and Seconds their length.
	Start   int     `json:"start"`
	End     int     `json:"end"`
	Seconds float64 `json:"seconds"`
}

// MeasureAudio renders the audio of the frames [start, end) of the job's document twice through the mixer of the export,
// with and without the processors, and measures each in Go (SDD-004 section 7.1). With itemID only that item's source
// is measured. start and end of 0 and 0 are the whole timeline.
func MeasureAudio(ctx context.Context, job Job, itemID string, start, end int) (*AudioMeasure, error) {
	if job.Runner == nil || job.Doc == nil || job.ArtifactsDir == "" {
		return nil, errors.New("export: the measure job needs a runner, a document and the artifacts folder")
	}

	doc := job.Doc
	job.Start, job.End = start, end

	from, to, err := exportRange(&job, endFrame(doc))
	if err != nil {
		return nil, err
	}

	fps := doc.Settings.FPS
	secs := float64(to-from) * float64(fps.Den) / float64(fps.Num)

	if secs > MaxMeasureSeconds {
		return nil, ErrMeasureTooLong
	}

	dir, err := job.Runner.NewJobRunDir(job.JobID)
	if err != nil {
		return nil, wrapRun(err, ReasonRender, "A work folder for the measurement could not be made.", "")
	}

	defer func() { _ = dir.Remove() }()

	s := newState(ctx, &job, dir, to-from)
	s.offset = from
	s.deadline = max(s.deadline, time.Duration(secs*float64(time.Second))*4)

	first := samplesAt(fps, from)
	total := samplesAt(fps, to) - first

	var srcs []plan.AudioSource

	all := plan.Audio(doc, from, to)
	for i := range all {
		if itemID != "" && all[i].ItemID != itemID {
			continue
		}

		all[i].StartSample -= first
		srcs = append(srcs, all[i])
	}

	run := func(list []plan.AudioSource) (audio.Measurement, error) {
		m := audio.NewMeter()

		err := audio.Mix(ctx, list, total, &audioOpener{s: s}, func(block []float32) error {
			m.Write(block)

			return nil
		})
		if err != nil {
			return audio.Measurement{}, wrapAudio(err, "")
		}

		return m.Result(), nil
	}

	after, err := run(srcs)
	if err != nil {
		return nil, fmt.Errorf("measure with the processors: %w", err)
	}

	plain := make([]plan.AudioSource, len(srcs))
	for i := range srcs {
		plain[i] = srcs[i]
		plain[i].Chain, plain[i].Track, plain[i].BusChain = nil, "", nil
	}

	before, err := run(plain)
	if err != nil {
		return nil, fmt.Errorf("measure without the processors: %w", err)
	}

	return &AudioMeasure{Before: before, After: after, Start: from, End: to, Seconds: secs}, nil
}
