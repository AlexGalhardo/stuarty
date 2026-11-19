package transcribe

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
)

// Error codes of SDD-000 section 8 that a transcription raises.
const (
	CodeUnavailable = "E_TRANSCRIBE_UNAVAILABLE"
	CodeFailed      = "E_TRANSCRIBE_FAILED"
)

// Reasons of E_TRANSCRIBE_FAILED (details.reason).
const (
	ReasonOutputMissing = "output_missing"
	ReasonExitCode      = "exit_code"
	ReasonOutputTooBig  = "output_too_large"
	ReasonPrepare       = "audio_prepare_failed"
	ReasonTimeout       = "timeout"
)

// Provider turns audio into timed words. It does not post-process (SDD-011 section 10.4): that is
// Postprocess, in one place, so every provider yields the same shape (ADR-0021 item 7).
type Provider interface {
	Name() string  // "whisper.cpp" or "openai-compatible"
	Model() string // part of the artifact key (ADR-0021 item 4)
	// Check says whether the provider can run now. It never downloads. It returns *MissingError when the
	// helper or the model is not installed.
	Check(ctx context.Context) error
	Transcribe(ctx context.Context, in Input, progress func(done, total time.Duration)) (Raw, error)
}

// Range is a stretch of the audio in milliseconds, end exclusive. SDD-011 gives the silences of the
// ingest in samples at 48 kHz; the caller converts them (SDD-011 section 10.4 works in milliseconds).
type Range struct {
	StartMS, EndMS int64
}

// Input is one transcription request.
type Input struct {
	// Audio is the path of the asset's normalized audio, inside one of InputDirs.
	Audio string
	// InputDirs are the artifact folders FFmpeg may read Audio from.
	InputDirs []string
	// Duration is the length of that audio.
	Duration time.Duration
	// Language is "" or "auto" to detect, else a BCP 47 tag; the provider uses its primary subtag.
	Language string
	// Silences are the -35 dB silences of the same audio (used by the API provider to chunk).
	Silences []Range
	// WorkDir is the run directory of the job, the only place the provider writes.
	WorkDir *ffmpeg.RunDir
}

// Raw is what a provider returns: words as the model timed them, before Postprocess.
type Raw struct {
	Language string
	Words    []RawWord
	Info     map[string]string // provider and helper versions, model hash
}

// RawWord is one word of a provider.
type RawWord struct {
	Text    string
	StartMS int64
	EndMS   int64
	// TokenProbs are the probabilities of the word-bearing tokens; empty when unknown.
	TokenProbs []float64
	// Confidence is set by the API provider from segment log-probabilities; NaN when unknown.
	Confidence float64
}

// Word is a word of the transcript, in integer milliseconds of the asset's audio.
type Word struct {
	Text       string   `json:"text"`
	StartMS    int64    `json:"start_ms"`
	EndMS      int64    `json:"end_ms"`
	Confidence *float64 `json:"confidence,omitempty"`
}

// Segment is a stretch of words for display (SDD-011 section 10.5).
type Segment struct {
	StartMS   int64 `json:"start_ms"`
	EndMS     int64 `json:"end_ms"`
	FirstWord int   `json:"first_word"`
	LastWord  int   `json:"last_word"`
}

// Transcript is the post-processed result.
type Transcript struct {
	Language string    `json:"language"`
	Words    []Word    `json:"words"`
	Segments []Segment `json:"segments"`
}

// Stats counts what Postprocess did (the postprocess object of the artifact).
type Stats struct {
	Version            int `json:"version"`
	Pauses             int `json:"pauses"`
	OnsetsSnapped      int `json:"onsets_snapped"`
	TailsTrimmed       int `json:"tails_trimmed"`
	ZeroLengthRepaired int `json:"zero_length_repaired"`
	MinDurationExtend  int `json:"min_duration_extended"`
	InvariantFixes     int `json:"invariant_fixes"`
}

// Item is one thing a provider needs that is not installed.
type Item struct {
	Name    string `json:"name"`
	URLHost string `json:"url_host"`
	Bytes   int64  `json:"bytes"`
}

// MissingError is returned by Check when the helper or the model is not installed (SDD-011 section 10.2).
// The service turns it into E_TRANSCRIBE_UNAVAILABLE with the install command.
type MissingError struct {
	Items         []Item
	DownloadBytes int64
}

func (m *MissingError) Error() string {
	names := make([]string, 0, len(m.Items))
	for _, it := range m.Items {
		names = append(names, it.Name)
	}
	return fmt.Sprintf("%s: not installed: %s", CodeUnavailable, strings.Join(names, ", "))
}

// ArtifactKey is the 12 hex characters of the transcript artifact name: asset hash, provider, model and
// requested language (ADR-0021 item 4). A request with the same four values finds the stored artifact;
// an empty language is "auto".
func ArtifactKey(assetHash, provider, model, language string) string {
	if language == "" {
		language = "auto"
	}
	h := sha256.New()
	for _, part := range []string{assetHash, provider, model, strings.ToLower(language)} {
		_, _ = fmt.Fprintf(h, "%d:%s;", len(part), part)
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}
