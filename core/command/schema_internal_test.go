package command

import (
	"encoding/json"
	"testing"
)

// VV-206: the `resolved` argument is filled by the service, so the published schema must not offer it.
func TestSchemaLeavesOutInternalArguments(t *testing.T) {
	for _, op := range []string{
		"clip.remove_silences", "caption.from_transcript", "caption.retime_from_clip", "audio.duck", "audio.mark_beats",
	} {
		raw, err := Default().Schema(op)
		if err != nil {
			t.Fatalf("%s: %v", op, err)
		}

		var s struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(raw, &s); err != nil {
			t.Fatalf("%s: %v", op, err)
		}

		if _, ok := s.Properties["resolved"]; ok {
			t.Errorf("%s: the schema lists the internal argument resolved", op)
		}
	}
}
