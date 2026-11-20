package shell

import (
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func TestSingleInstanceIDIsStablePerWorkspace(t *testing.T) {
	t.Parallel()

	a := InstanceID(`C:\Users\a\.stuarty`)
	if a != InstanceID(`C:\Users\a\.stuarty`) {
		t.Error("the id of one workspace changed between two calls")
	}

	if a == InstanceID(`C:\Users\b\.stuarty`) {
		t.Error("two workspaces share one instance")
	}

	if !regexp.MustCompile(`^stuarty-desktop-[0-9a-f]{16}$`).MatchString(a) {
		t.Errorf("id %q is not a safe mutex or socket name", a)
	}
}

func TestSingleInstanceIDReadsTwoSpellingsOfOneFolderAsOne(t *testing.T) {
	t.Parallel()

	if InstanceID("/ws/./a/../b") != InstanceID("/ws/b") {
		t.Error("an unclean path made another instance")
	}

	same := InstanceID(strings.ToUpper("/ws/Stuarty")) == InstanceID("/ws/stuarty")
	if caseBlind := runtime.GOOS == "windows" || runtime.GOOS == "darwin"; same != caseBlind {
		t.Errorf("case folding = %v on %s, want %v", same, runtime.GOOS, caseBlind)
	}
}

func TestSingleInstanceSecondLaunchFocusesUnlessItIsTheSignInStart(t *testing.T) {
	t.Parallel()

	if got := DecideSecondLaunch([]string{`C:\Stuarty\stuarty-desktop.exe`}); got != SecondFocus {
		t.Errorf("a plain second launch = %v, want focus", got)
	}

	if got := DecideSecondLaunch([]string{"stuarty-desktop.exe", BackgroundFlag}); got != SecondIgnore {
		t.Errorf("the sign-in start as a second launch = %v, want ignore", got)
	}
}
