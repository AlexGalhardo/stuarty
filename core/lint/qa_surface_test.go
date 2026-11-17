package lint

import "testing"

// L_AUDIO_CLIP reads peaks, so the fast lint of an edit omits it and a full lint reports it.
// The edit result is labelled lint_scope fast for that reason; the two totals are not the same.
func TestFastLintOmitsAudioClip(t *testing.T) {
	t.Parallel()

	d := newDoc()
	put(d, clip("itm_loud", 0, 90))
	peaks := make([]float64, 200)

	for i := range peaks {
		peaks[i] = 1.1
	}

	env := Env{Artifacts: fakeArtifacts{envSpec{Peaks: map[string][]float64{astVideo: peaks}}}}
	if got := has(Lint(d, env, Fast), CodeAudioClip); len(got) != 0 {
		t.Fatalf("fast lint reported %s: %+v", CodeAudioClip, got)
	}

	if got := has(Lint(d, env, Full), CodeAudioClip); len(got) != 1 {
		t.Fatalf("full lint %s findings: %+v", CodeAudioClip, got)
	}
}
