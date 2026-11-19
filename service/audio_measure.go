package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// AudioMeasureRequest asks for the level of the sound of a project with and without its processors (SDD-004 section
// 7.1): of the whole timeline, of a range, or of one item.
type AudioMeasureRequest struct {
	ProjectID  string `json:"project_id"`
	TimelineID string `json:"timeline_id,omitempty"` // SDD-003 section 1.7
	// ItemID measures that item alone, with the chain of its track; its own range is the default range.
	ItemID string `json:"item_id,omitempty"`
	// Start and End are the range, as in an export; both empty is the item's range, or the whole timeline.
	Start string `json:"start,omitempty"`
	End   string `json:"end,omitempty"`
}

// AudioLevels is the measurement of one render of the range.
type AudioLevels struct {
	// IntegratedLUFS is the loudness of ITU-R BS.1770-4, nil for silence or less than 0.4 s.
	IntegratedLUFS *float64 `json:"integrated_lufs"`
	// TruePeakDBTP is the highest sample after oversampling, nil for digital silence.
	TruePeakDBTP *float64 `json:"true_peak_dbtp"`
	// NoiseFloorDBFS is the level of the quietest tenth of the 100 ms blocks, -120 for digital silence.
	NoiseFloorDBFS float64 `json:"noise_floor_dbfs"`
	// SpeechToNoiseDB is the integrated loudness minus the noise floor.
	SpeechToNoiseDB *float64 `json:"speech_to_noise_db"`
}

// AudioMeasureResult is the measurement of the range before the processors and after them, both before the loudness
// gain of the export.
type AudioMeasureResult struct {
	ProjectID  string      `json:"project_id"`
	Revision   int64       `json:"revision"`
	ItemID     string      `json:"item_id,omitempty"`
	StartFrame int64       `json:"start_frame"`
	EndFrame   int64       `json:"end_frame"`
	Seconds    float64     `json:"seconds"`
	Before     AudioLevels `json:"before"`
	After      AudioLevels `json:"after"`
	Text       string      `json:"text"`
}

// AudioMeasureInput is what the meter needs.
type AudioMeasureInput struct {
	Doc    *timeline.Document
	JobID  string
	ItemID string
	// Start and End are the frames to measure, the end exclusive.
	Start, End int64
}

// AudioMeasureOutput is what the meter measured.
type AudioMeasureOutput struct {
	Before, After AudioLevels
	Seconds       float64
}

// ErrAudioRangeTooLong is what a meter returns for a range longer than MaxAudioMeasureSeconds.
var ErrAudioRangeTooLong = errors.New("service: the range to measure is longer than the limit")

// MaxAudioMeasureSeconds is the longest range AudioMeasure measures.
const MaxAudioMeasureSeconds = 600

// AudioMeter is implemented by an Exporter that can render the audio of a project and measure it.
type AudioMeter interface {
	MeasureAudio(ctx context.Context, in AudioMeasureInput) (*AudioMeasureOutput, error)
}

// AudioMeasure renders the range through the mixer of the export, with and without the processors, and measures both.
func (s *svc) AudioMeasure(ctx context.Context, req AudioMeasureRequest) (*AudioMeasureResult, error) {
	c, err := authorize(ctx, "AudioMeasure", req.ProjectID)
	if err != nil {
		return nil, err
	}

	if err := s.useTimeline(ctx, c, &req.ProjectID, req.TimelineID); err != nil {
		return nil, err
	}

	row, err := s.store.Projects().Get(ctx, req.ProjectID)
	if err != nil || !visible(c, row.OwnerUserID) {
		return nil, projectNotFound(req.ProjectID, err)
	}

	doc, derr := decodeRow(&row)
	if derr != nil {
		return nil, derr
	}

	start, end, rerr := s.measureRange(doc, req)
	if rerr != nil {
		return nil, rerr
	}

	meter, ok := s.exporter.(AudioMeter)
	if !ok {
		if merr := s.exporterMissing(); merr != nil {
			return nil, merr
		}

		return nil, newError(command.CodeInternal, "measuring the audio is not available in this build.")
	}

	out, merr := meter.MeasureAudio(ctx, AudioMeasureInput{
		Doc: doc, JobID: s.newIDs("job"), ItemID: req.ItemID, Start: start, End: end,
	})
	if errors.Is(merr, ErrAudioRangeTooLong) {
		return nil, newError(command.CodeLimit, "the range is longer than %d seconds.", MaxAudioMeasureSeconds).
			withPath("args").withHint("measure a shorter range with start and end, or one item")
	}

	if merr != nil {
		return nil, asError(merr)
	}

	res := &AudioMeasureResult{
		ProjectID: row.ID, Revision: row.Revision, ItemID: req.ItemID, StartFrame: start, EndFrame: end,
		Seconds: out.Seconds, Before: out.Before, After: out.After,
	}
	res.Text = audioMeasureText(res)

	return res, nil
}

// measureRange is the range to measure and the check of the item: an item that is not a clip or an audio item has no
// sound to measure, and without a range the item's own range is the default.
func (s *svc) measureRange(doc *timeline.Document, req AudioMeasureRequest) (start, end int64, err error) {
	var it timeline.Item

	if req.ItemID != "" {
		var ok bool

		if it, ok = doc.Items[req.ItemID]; !ok {
			return 0, 0, itemNotFound(req.ItemID)
		}

		if it.Type != timeline.ItemClip && it.Type != timeline.ItemAudio {
			return 0, 0, newError(command.CodeTypeMismatch, "item %s is a %s item and has no audio.", it.ID, it.Type).
				withPath("item_id").withHint("measure a clip or an audio item")
		}
	}

	start, end, rerr := exportRange(doc, ExportArgs{Start: req.Start, End: req.End})
	if rerr != nil {
		return 0, 0, rerr
	}

	if req.ItemID != "" && req.Start == "" && req.End == "" {
		start, end = int64(it.Start), int64(it.Start+it.Duration)
	}

	return start, end, nil
}

func audioMeasureText(r *AudioMeasureResult) string {
	var b strings.Builder

	fmt.Fprintf(&b, "audio measure %s rev %d, frames %d to %d (%.3f s)", r.ProjectID, r.Revision, r.StartFrame,
		r.EndFrame, r.Seconds)

	if r.ItemID != "" {
		fmt.Fprintf(&b, ", item %s", r.ItemID)
	}

	for _, row := range []struct {
		name   string
		levels AudioLevels
	}{{"before", r.Before}, {"after ", r.After}} {
		fmt.Fprintf(&b, "\n%s: %s", row.name, row.levels.line())
	}

	return b.String()
}

func (l AudioLevels) line() string {
	num := func(v *float64, unit string) string {
		if v == nil {
			return "none"
		}

		return fmt.Sprintf("%.1f %s", *v, unit)
	}

	return fmt.Sprintf("loudness %s, true peak %s, noise floor %.1f dBFS, speech to noise %s",
		num(l.IntegratedLUFS, "LUFS"), num(l.TruePeakDBTP, "dBTP"), l.NoiseFloorDBFS, num(l.SpeechToNoiseDB, "dB"))
}
