package main

import (
	"context"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/desktop/shell"
	"github.com/AlexGalhardo/vukem-video/desktop/update"
)

// A build without the feed URL and key has no client; the checker must be a nil interface, not a
// typed nil, so that the menu action shows the not-configured message instead of calling a nil client.
func TestUnconfiguredBuildShowsTheNotConfiguredMessage(t *testing.T) {
	_, err := update.FromBuild("0.15.0", t.TempDir(), false, shell.InstallerFor("linux", func() {}))
	if err == nil {
		t.Fatal("the placeholder build must not be configured")
	}

	u := &updater{buildErr: err}
	if u.checker() != nil {
		t.Fatal("no client must give a nil checker")
	}

	out := shell.CheckForUpdates(context.Background(), u.checker(), u.buildErr, "0.15.0")
	if out.Offer != nil || !strings.Contains(out.Message, "no update feed or signing key") {
		t.Errorf("message %q", out.Message)
	}
}
