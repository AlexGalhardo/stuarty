package export

import "testing"

// TestRenderersRefuseAnEmptyConfig: both the single-frame renderer (ViewFrame)
// and the sequence renderer (desktop playback) require a runner, a document and
// an artifacts directory.
func TestRenderersRefuseAnEmptyConfig(t *testing.T) {
	t.Parallel()

	if _, err := NewRenderer(t.Context(), RenderConfig{}); err == nil {
		t.Fatal("NewRenderer accepted an empty config")
	}

	if _, err := NewSequenceRenderer(t.Context(), RenderConfig{}); err == nil {
		t.Fatal("NewSequenceRenderer accepted an empty config")
	}
}
