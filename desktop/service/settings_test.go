package service

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/AlexGalhardo/stuarty/desktop/shell"
)

func TestSettingsDefaultToEverythingOff(t *testing.T) {
	t.Parallel()

	if got := Load(t.TempDir()); got != (Settings{}) {
		t.Errorf("a missing file = %+v, want the zero settings", got)
	}

	dir := t.TempDir()
	if err := os.WriteFile(Path(dir), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	if got := Load(dir); got != (Settings{}) {
		t.Errorf("a damaged file = %+v, want the zero settings", got)
	}
}

func TestSettingsRoundTripAndStartNeedsKeepRunning(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "stuarty")

	want := Settings{KeepRunning: true, StartAtSignIn: true, Asked: true}
	if err := Save(dir, want); err != nil {
		t.Fatal(err)
	}

	if got := Load(dir); got != want {
		t.Errorf("round trip = %+v, want %+v", got, want)
	}

	if err := Save(dir, Settings{StartAtSignIn: true, Asked: true}); err != nil {
		t.Fatal(err)
	}

	if got := Load(dir); got.StartAtSignIn || !got.Asked {
		t.Errorf("start at sign-in without keep running was kept: %+v", got)
	}

	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Errorf("the folder holds %d entries (%v), want only background.json", len(entries), err)
	}
}

func TestBackgroundFlagAgreesWithTheShell(t *testing.T) {
	t.Parallel()

	if BackgroundFlag != shell.BackgroundFlag {
		t.Errorf("registration passes %q, the shell reads %q", BackgroundFlag, shell.BackgroundFlag)
	}
}

func TestApplyRegistersAndRemoves(t *testing.T) {
	t.Parallel()

	reg := &MemoryRegistrar{}

	got, err := Apply(Settings{KeepRunning: true, StartAtSignIn: true}, reg)
	if err != nil || !got.StartAtSignIn || !reg.On || len(reg.Args) != 1 || reg.Args[0] != BackgroundFlag {
		t.Fatalf("on: %+v, %v, registration %+v", got, err, reg)
	}

	got, err = Apply(Settings{KeepRunning: true}, reg)
	if err != nil || got.StartAtSignIn || reg.On {
		t.Fatalf("off: %+v, %v, registration %+v", got, err, reg)
	}

	// Keep running off takes the registration with it.
	_, _ = Apply(Settings{KeepRunning: true, StartAtSignIn: true}, reg)

	if got, err = Apply(Settings{StartAtSignIn: true}, reg); err != nil || got.StartAtSignIn || reg.On {
		t.Fatalf("keep running off: %+v, %v, registration %+v", got, err, reg)
	}
}

func TestApplyRefusalLeavesTheOptionOff(t *testing.T) {
	t.Parallel()

	reg := &MemoryRegistrar{FailEnable: errors.New("access is denied")}

	got, err := Apply(Settings{KeepRunning: true, StartAtSignIn: true}, reg)
	if !errors.Is(err, ErrRegistration) || got.StartAtSignIn || reg.On {
		t.Fatalf("refusal: %+v, %v, registration %+v", got, err, reg)
	}
}

func TestReconcileFollowsTheSystem(t *testing.T) {
	t.Parallel()

	reg := &MemoryRegistrar{}

	if got := Reconcile(Settings{KeepRunning: true, StartAtSignIn: true}, reg); got.StartAtSignIn {
		t.Error("the entry was removed by hand and the setting still says on")
	}

	reg.On = true

	if got := Reconcile(Settings{KeepRunning: true, StartAtSignIn: true}, reg); !got.StartAtSignIn {
		t.Error("a registered entry was turned off")
	}
}
