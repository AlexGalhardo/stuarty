package ingest

import (
	"errors"
	"fmt"
)

// SpeechVersion is the version of speech-1.json.
const SpeechVersion = 1

// Sources of a speech map.
const (
	SpeechFromEnvelope   = "envelope"
	SpeechFromTranscript = "transcript"
)

// SpeechMergeGapMS is the pause between two words that still belongs to the same stretch of
// speech. It is the 300 ms the transcript repair uses for a pause (ADR-0021), so a breath
// between words does not let the music come back up.
const SpeechMergeGapMS = 300

// SpeechRange is a stretch of speech in samples at 48 kHz, end exclusive. It has the shape of
// a silence range so the same frame conversion applies to both.
type SpeechRange = SilenceRange

// Speech is speech-1.json: where an asset is spoken over, in samples (RF-043, SDD-002 A.9).
// The service fills `resolved` of the ducking command from it, never from a job started by an
// edit (SDD-007 5.4 rule 2). Source tells how the ranges were found: the transcript when one
// exists, else the loudness envelope of the audio (the inverse of the silence artifact).
type Speech struct {
	Version     int           `json:"version"`
	SampleRate  int           `json:"sample_rate"`
	SampleCount int64         `json:"sample_count"`
	Source      string        `json:"source"`
	ThresholdDB int           `json:"threshold_db,omitempty"`
	Ranges      []SpeechRange `json:"ranges"`
}

// SpeechFromSilence is the envelope fallback: the stretches of the audio that are not silent at
// threshold dB (one of SilenceThresholdsDB, negative). It is the exact inverse of that set of
// silence-1.json, so the two never disagree.
func SpeechFromSilence(s *Silence, thresholdDB int) (*Speech, error) {
	if s == nil {
		return nil, errors.New("speech: no silence artifact")
	}

	var set *SilenceSet

	for i := range s.Sets {
		if s.Sets[i].ThresholdDB == thresholdDB {
			set = &s.Sets[i]
		}
	}

	if set == nil {
		return nil, fmt.Errorf("speech: the silence artifact has no %d dB set", thresholdDB)
	}

	out := &Speech{
		Version: SpeechVersion, SampleRate: s.SampleRate, SampleCount: s.SampleCount,
		Source: SpeechFromEnvelope, ThresholdDB: thresholdDB, Ranges: []SpeechRange{},
	}
	pos := int64(0)

	for _, r := range set.Ranges {
		if r.Start > pos {
			out.Ranges = append(out.Ranges, SpeechRange{Start: pos, End: r.Start})
		}

		pos = max(pos, r.End)
	}

	if pos < s.SampleCount {
		out.Ranges = append(out.Ranges, SpeechRange{Start: pos, End: s.SampleCount})
	}

	return out, nil
}

// SpokenWord is the span of one transcript word in milliseconds.
type SpokenWord struct{ StartMS, EndMS int64 }

// SpeechFromWords is the transcript path: the spans of the words, in order, with a gap shorter
// than gapMS joined, clamped to the audio. A transcript with no words gives no ranges.
func SpeechFromWords(words []SpokenWord, sampleCount int64, gapMS int64) *Speech {
	out := &Speech{
		Version: SpeechVersion, SampleRate: SampleRate, SampleCount: sampleCount,
		Source: SpeechFromTranscript, Ranges: []SpeechRange{},
	}

	const perMS = SampleRate / 1000

	for _, w := range words {
		start := min(max(w.StartMS, 0)*perMS, sampleCount)
		end := min(max(w.EndMS, 0)*perMS, sampleCount)

		if end <= start {
			continue
		}

		if n := len(out.Ranges); n > 0 && start-out.Ranges[n-1].End < gapMS*perMS {
			out.Ranges[n-1].End = max(out.Ranges[n-1].End, end)

			continue
		}

		out.Ranges = append(out.Ranges, SpeechRange{Start: start, End: end})
	}

	return out
}

// SpeechFrames converts speech ranges in samples to asset frame ranges. Unlike FramesFromSamples,
// a frame that holds any speech belongs to the range (the start rounds down and the end up), so
// ducking never starts late or ends early. Ranges that touch after rounding are joined.
func SpeechFrames(ranges []SpeechRange, rate Rational) []FrameRange {
	out := make([]FrameRange, 0, len(ranges))

	if rate.Num <= 0 || rate.Den <= 0 {
		return out
	}

	per := int64(SampleRate) * rate.Den

	for _, r := range ranges {
		start := r.Start * rate.Num / per
		end := (r.End*rate.Num + per - 1) / per

		if start >= end {
			continue
		}

		if n := len(out); n > 0 && start <= out[n-1].End {
			out[n-1].End = max(out[n-1].End, end)

			continue
		}

		out = append(out, FrameRange{Start: start, End: end})
	}

	return out
}

// speechThresholdDB is the set the envelope map is read from: the default of
// clip.remove_silences, so the two commands agree on what a pause is.
const speechThresholdDB = -35

// writeSpeech derives the envelope speech map from the silence analysis of the same job.
func (j *job) writeSpeech(s *Silence) error {
	sp, err := SpeechFromSilence(s, speechThresholdDB)
	if err != nil {
		return err
	}

	return j.writeJSON("speech.json", sp)
}
