package shell

import "testing"

func TestDecideClose(t *testing.T) {
	t.Parallel()

	on := BackgroundState{KeepRunning: true, Scheduled: 2, Known: true}

	cases := []struct {
		name string
		s    BackgroundState
		want CloseAction
	}{
		{"option on and a target scheduled hides", on, CloseHide},
		{"option off exits as before", BackgroundState{Scheduled: 2, Known: true}, CloseExit},
		{"nothing scheduled exits", BackgroundState{KeepRunning: true, Known: true}, CloseExit},
		{"unknown count exits", BackgroundState{KeepRunning: true, Scheduled: 2}, CloseExit},
		{"paused exits", BackgroundState{KeepRunning: true, Scheduled: 2, Known: true, Paused: true}, CloseExit},
		{"quitting exits", BackgroundState{KeepRunning: true, Scheduled: 2, Known: true, Quitting: true}, CloseExit},
	}

	for _, c := range cases {
		if got := DecideClose(c.s); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestStartHidden(t *testing.T) {
	t.Parallel()

	if !StartHidden([]string{"--binary", "x", BackgroundFlag}, true) {
		t.Error("the sign-in start with the option on must start hidden")
	}

	if StartHidden([]string{BackgroundFlag}, false) {
		t.Error("with the option off a stale registration must show the window")
	}

	if StartHidden(nil, true) {
		t.Error("a normal start must show the window")
	}
}

func TestQuitPrompt(t *testing.T) {
	t.Parallel()

	if ask, n := QuitPrompt(3, true); !ask || n != 3 {
		t.Errorf("three waiting: ask=%v n=%d", ask, n)
	}

	for _, c := range []struct {
		n     int
		known bool
	}{{0, true}, {3, false}, {-1, true}} {
		if ask, _ := QuitPrompt(c.n, c.known); ask {
			t.Errorf("QuitPrompt(%d, %v) asks", c.n, c.known)
		}
	}
}

func TestQuitTextsNameTheCount(t *testing.T) {
	t.Parallel()

	for _, lang := range []string{"en", "pt-BR"} {
		if got := Text(lang, TextQuitWaitingOne); got == "" || got == TextQuitWaitingOne {
			t.Errorf("%s: no text for one waiting post: %q", lang, got)
		}

		if got := QuitWaitingMessage(lang, 3); got == "" || !containsDigit(got, '3') {
			t.Errorf("%s: the message does not name the count: %q", lang, got)
		}
	}
}

func containsDigit(s string, d rune) bool {
	for _, r := range s {
		if r == d {
			return true
		}
	}

	return false
}
