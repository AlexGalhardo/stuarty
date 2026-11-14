package ingest

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
)

// SilenceRange is a silent stretch in samples at 48 kHz, end exclusive.
type SilenceRange struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
}

// SilenceSet is the ranges found at one threshold.
type SilenceSet struct {
	ThresholdDB int            `json:"threshold_db"`
	Ranges      []SilenceRange `json:"ranges"`
}

// Silence is silence-1.json (SDD-011 9.1).
type Silence struct {
	Version       int          `json:"version"`
	SampleRate    int          `json:"sample_rate"`
	SampleCount   int64        `json:"sample_count"`
	MinDurationMS int          `json:"min_duration_ms"`
	Sets          []SilenceSet `json:"sets"`
}

var (
	silenceStartRe = regexp.MustCompile(`\[Parsed_silencedetect_(\d+) @ [^\]]+\] silence_start: (-?[0-9.e+-]+)`)
	silenceEndRe   = regexp.MustCompile(`\[Parsed_silencedetect_(\d+) @ [^\]]+\] silence_end: (-?[0-9.e+-]+)`)
)

// ParseSilence reads the stderr of the silencedetect run. The instance number in
// Parsed_silencedetect_N is the filter's position in the graph: asplit is 0, so threshold
// i (0-based) is instance i+1. Each time is converted to samples, round(seconds * 48000),
// and a silence still open at the end of the file is closed at sampleCount.
func ParseSilence(stderr string, sampleCount int64) (*Silence, error) {
	s := &Silence{Version: 1, SampleRate: SampleRate, SampleCount: sampleCount, MinDurationMS: SilenceMinMS}
	open := map[int]int64{}
	ranges := map[int][]SilenceRange{}
	toSamples := func(sec string) int64 {
		f, _ := strconv.ParseFloat(sec, 64)
		return max(0, int64(math.Round(f*SampleRate)))
	}
	for _, line := range splitLines(stderr) {
		if m := silenceStartRe.FindStringSubmatch(line); m != nil {
			n, _ := strconv.Atoi(m[1])
			open[n] = toSamples(m[2])
		} else if m := silenceEndRe.FindStringSubmatch(line); m != nil {
			n, _ := strconv.Atoi(m[1])
			start, ok := open[n]
			if !ok {
				return nil, fmt.Errorf("silence: end without start for instance %d", n)
			}
			ranges[n] = append(ranges[n], SilenceRange{start, min(toSamples(m[2]), sampleCount)})
			delete(open, n)
		}
	}
	for n, start := range open {
		ranges[n] = append(ranges[n], SilenceRange{start, sampleCount})
	}
	for i, db := range SilenceThresholdsDB {
		r := ranges[i+1]
		if r == nil {
			r = []SilenceRange{}
		}
		s.Sets = append(s.Sets, SilenceSet{ThresholdDB: -db, Ranges: r})
	}
	return s, nil
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := range len(s) {
		if s[i] == '\n' || s[i] == '\r' {
			if i > start {
				out = append(out, s[start:i])
			}
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

// Loudness is loudness-1.json (SDD-011 9.2).
type Loudness struct {
	Version        int      `json:"version"`
	IntegratedLUFS *float64 `json:"integrated_lufs"`
	LRALU          float64  `json:"lra_lu"`
	TruePeakDBTP   *float64 `json:"true_peak_dbtp"`
	Silent         bool     `json:"silent"`
	SampleCount    int64    `json:"sample_count"`
}

var (
	lufsRe = regexp.MustCompile(`Integrated loudness:\s+I:\s+(-?\d+\.\d|-inf) LUFS`)
	lraRe  = regexp.MustCompile(`Loudness range:\s+LRA:\s+(\d+\.\d) LU`)
	peakRe = regexp.MustCompile(`True peak:\s+Peak:\s+(-?\d+\.\d|-inf) dBFS`)
)

// ParseLoudness reads the summary block that ebur128 prints on stderr. A silent file prints
// I: -70.0 LUFS and Peak: -inf dBFS; then the peak and the integrated value are null and silent is
// true (the -70.0 is a floor, not a measurement).
func ParseLoudness(stderr string, sampleCount int64) (*Loudness, error) {
	i, l, p := lufsRe.FindStringSubmatch(stderr), lraRe.FindStringSubmatch(stderr), peakRe.FindStringSubmatch(stderr)
	if i == nil || l == nil || p == nil {
		return nil, errors.New("loudness: the ebur128 summary is missing")
	}
	out := &Loudness{Version: 1, SampleCount: sampleCount}
	out.LRALU, _ = strconv.ParseFloat(l[1], 64)
	if p[1] == "-inf" {
		out.Silent = true
		return out, nil
	}
	v, _ := strconv.ParseFloat(p[1], 64)
	out.TruePeakDBTP = &v
	if i[1] != "-inf" {
		lu, _ := strconv.ParseFloat(i[1], 64)
		out.IntegratedLUFS = &lu
	}
	return out, nil
}
