package main

import (
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/AlexGalhardo/stuarty/desktop/service"
	"github.com/AlexGalhardo/stuarty/desktop/shell"
)

// A quit that does not come through the File menu or the tray (the installer of an update, the system) runs
// the shutdown tasks and then closes the window: the close must let the window go, not hide it behind a tray.
// No window and no server exist here: the close must decide before it touches either.
func TestEveryShutdownClosesInsteadOfHiding(t *testing.T) {
	t.Parallel()

	b := &background{settings: service.Settings{KeepRunning: true}, scheduled: 2, known: true}
	if shell.DecideClose(b.state()) != shell.CloseHide {
		t.Fatal("precondition: with the option on and two posts waiting, a close hides")
	}

	b.shuttingDown()

	if got := shell.DecideClose(b.state()); got != shell.CloseExit {
		t.Fatalf("a close during the shutdown = %v, want exit", got)
	}

	e := application.NewWindowEvent()
	b.onClosing(e)

	if e.IsCancelled() {
		t.Error("the close during the shutdown was cancelled")
	}
}
