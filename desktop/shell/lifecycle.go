package shell

import "slices"

// The decisions of the background mode (SDD-012 section 21.3 item 7) as pure functions: the code that
// calls the windowing framework only carries them out, so what closing a window does is tested with no
// window.

// CloseAction is what the shell does when the person closes the window.
type CloseAction int

const (
	// CloseExit ends the application, as it always did.
	CloseExit CloseAction = iota
	// CloseHide keeps the application, with its server and scheduler, behind a tray icon.
	CloseHide
)

// BackgroundState is what the close decision depends on.
type BackgroundState struct {
	// KeepRunning is the setting "keep running in the background".
	KeepRunning bool
	// Scheduled is the number of targets in the state scheduled or approved; Known is false when the
	// server could not be asked and no earlier answer exists.
	Scheduled int
	Known     bool
	// Paused says the person paused publishing: nothing would go out, so there is nothing to wait for.
	Paused bool
	// Quitting says the application is already quitting (File, Quit; the tray; the system; an update).
	Quitting bool
}

// DecideClose says what closing the window does. The window is hidden only when the person asked for
// it, something waits to be published, and publishing is not paused; in every other case it exits.
func DecideClose(s BackgroundState) CloseAction {
	if s.Quitting || !s.KeepRunning || s.Paused || !s.Known || s.Scheduled <= 0 {
		return CloseExit
	}

	return CloseHide
}

// StartHidden says whether the shell starts with the window hidden: only when it was started by the
// registration at sign-in (the BackgroundFlag argument) and the person keeps it running. With the
// option off the argument is ignored, so a stale registration never leaves Stuarty running unseen.
func StartHidden(args []string, keepRunning bool) bool {
	if !keepRunning {
		return false
	}

	return slices.Contains(args, BackgroundFlag)
}

// BackgroundFlag is the argument of the registration at sign-in; desktop/service says it too.
const BackgroundFlag = "--background"

// QuitPrompt says whether Quit from the tray asks first, and how many scheduled targets it names. It
// asks only when something waits.
func QuitPrompt(scheduled int, known bool) (ask bool, count int) {
	if !known || scheduled <= 0 {
		return false, 0
	}

	return true, scheduled
}
