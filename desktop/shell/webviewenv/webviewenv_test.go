package webviewenv

import "testing"

func TestGetenvAnswersOtherVariablesFromTheEnvironment(t *testing.T) {
	t.Setenv("VUKEM_VIDEO_TEST_OTHER", "v")

	if got := Getenv("VUKEM_VIDEO_TEST_OTHER"); got != "v" {
		t.Errorf("other variable %q", got)
	}
}
