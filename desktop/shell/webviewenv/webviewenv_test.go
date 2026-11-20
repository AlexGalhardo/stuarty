package webviewenv

import "testing"

func TestGetenvAnswersOtherVariablesFromTheEnvironment(t *testing.T) {
	t.Setenv("STUARTY_TEST_OTHER", "v")

	if got := Getenv("STUARTY_TEST_OTHER"); got != "v" {
		t.Errorf("other variable %q", got)
	}
}
