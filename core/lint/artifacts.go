package lint

// Window is the length of the audio windows of the peaks artifact, in milliseconds.
const Window = 20

// Peaks is the peaks artifact of an asset: the largest absolute sample, from 0 to 1, of
// each 20 ms window from the start of the asset.
type Peaks []float64

// Loudness is the loudness artifact of an asset: its integrated loudness in LUFS.
type Loudness struct{ LUFS float64 }

// TranscriptWord is a word of the transcript of an asset, in frames of the asset own
// time axis, half-open.
type TranscriptWord struct {
	Text       string
	Start, End int
}

// ArtifactReader gives the rules that read artifacts what ingest and transcription
// produced: pure data in and out, no file and no job. The service implements it with
// the artifacts it stored. A missing artifact makes the rule silent, never an error:
// ok is false, and the rule skips what depends on it.
type ArtifactReader interface {
	Peaks(asset string) (p Peaks, ok bool)
	Loudness(asset string) (l Loudness, ok bool)
	Transcript(asset string) (words []TranscriptWord, ok bool)
}
