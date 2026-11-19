package testenv

import (
	"context"
	"time"

	"github.com/AlexGalhardo/stuarty/engine/transcribe"
)

// StubTranscriber is a transcription provider that needs no helper: it answers
// every audio with the same words.
type StubTranscriber struct {
	Lang  string
	Words []transcribe.RawWord
}

// Name implements transcribe.Provider.
func (*StubTranscriber) Name() string { return "stub" }

// Model implements transcribe.Provider.
func (*StubTranscriber) Model() string { return "tiny" }

// Check implements transcribe.Provider.
func (*StubTranscriber) Check(context.Context) error { return nil }

// Transcribe implements transcribe.Provider.
func (s *StubTranscriber) Transcribe(
	_ context.Context,
	_ transcribe.Input,
	progress func(done, total time.Duration),
) (transcribe.Raw, error) {
	progress(time.Second, 2*time.Second)

	lang := s.Lang
	if lang == "" {
		lang = "en"
	}

	return transcribe.Raw{Language: lang, Words: s.Words}, nil
}
