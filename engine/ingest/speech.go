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
