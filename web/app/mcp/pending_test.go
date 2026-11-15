package mcp

import "testing"

// The tests below belong to tools or behaviour whose parts are not in the build
// yet. Each names what it waits for; replace the skip with the test when it lands.

func TestSecurity_T07_TranscriptDelimitedAndTruncated(t *testing.T) {
	t.Skip(
		"transcription is not in the Prototype build: media_inspect answers \"transcript: none yet\" (SDD-007 section 8.2, P2)",
	)
}
