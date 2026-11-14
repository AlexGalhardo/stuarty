package ingest

import "testing"

func TestParseSilence(t *testing.T) {
	// The format FFmpeg 9.0.2 printed on the pinned build; instances 1 and 2 are thresholds -30 and -35.
	stderr := `[Parsed_silencedetect_2 @ 000001fd5147cd80] silence_start: 0.99991
[Parsed_silencedetect_1 @ 000001fd5147b980] silence_start: 1
[Parsed_silencedetect_1 @ 000001fd5147b980] silence_end: 1.5 | silence_duration: 0.5
[Parsed_silencedetect_1 @ 000001fd5147b980] silence_start: 2
[Parsed_silencedetect_2 @ 000001fd5147cd80] silence_end: 3 | silence_duration: 2.00009
`
	s, err := ParseSilence(stderr, 144000)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Sets) != 4 || s.Sets[0].ThresholdDB != -30 || s.Sets[3].ThresholdDB != -45 || s.MinDurationMS != 150 ||
		s.SampleRate != 48000 {
		t.Fatalf("%+v", s)
	}
	want0 := []SilenceRange{{48000, 72000}, {96000, 144000}} // the second is closed at the end of the file
	if got := s.Sets[0].Ranges; len(got) != 2 || got[0] != want0[0] || got[1] != want0[1] {
		t.Errorf("-30 dB: %v", got)
	}
	if got := s.Sets[1].Ranges; len(got) != 1 || got[0] != (SilenceRange{47996, 144000}) {
		t.Errorf("-35 dB: %v", got)
	}
	if s.Sets[2].Ranges == nil || len(s.Sets[2].Ranges) != 0 {
		t.Errorf("an empty set must be an empty list, not null: %#v", s.Sets[2].Ranges)
	}
	if _, err := ParseSilence("[Parsed_silencedetect_1 @ 00] silence_end: 2 | silence_duration: 1", 100); err == nil {
		t.Error("an end without a start parsed")
	}
}

func TestParseLoudness(t *testing.T) {
	speech := `[Parsed_ebur128_0 @ 0000021ea0b82ec0] Summary:

  Integrated loudness:
    I:         -14.9 LUFS
    Threshold: -24.9 LUFS

  Loudness range:
    LRA:         3.4 LU
    Threshold:  -34.9 LUFS

  True peak:
    Peak:        -2.4 dBFS
`
	l, err := ParseLoudness(speech, 2880000)
	if err != nil || l.IntegratedLUFS == nil || *l.IntegratedLUFS != -14.9 || l.LRALU != 3.4 || l.TruePeakDBTP == nil ||
		*l.TruePeakDBTP != -2.4 ||
		l.Silent {
		t.Fatalf("%+v %v", l, err)
	}
	silent := `  Integrated loudness:
    I:         -70.0 LUFS
  Loudness range:
    LRA:         0.0 LU
  True peak:
    Peak:       -inf dBFS
`
	l, err = ParseLoudness(silent, 48000)
	if err != nil || !l.Silent || l.IntegratedLUFS != nil || l.TruePeakDBTP != nil {
		t.Errorf("silent file: %+v %v", l, err)
	}
	if _, err := ParseLoudness("nothing here", 1); err == nil {
		t.Error("garbage parsed")
	}
}
