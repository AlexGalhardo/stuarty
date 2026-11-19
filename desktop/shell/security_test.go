package shell

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/desktop/update"
)

// The launcher part of the threat T-35 (SDD-007): the update channel stays closed until the person
// consents, a bad archive cannot write outside its staging directory, and a version that does not start
// is replaced by the previous one. The client part (forged feed, digest, signature, downgrade, cut
// download) is in desktop/update.

type countingClock struct{ calls int }

func (c *countingClock) After(time.Duration) <-chan time.Time {
	c.calls++

	return make(chan time.Time)
}

func TestSecurity_T35_NothingConnectsBeforeTheAnswerAndNeverAfterNo(t *testing.T) {
	// Not asked yet, and a damaged settings file (which loads as not asked), then an answer of No.
	damaged := filepath.Join(t.TempDir(), "desktop.json")
	if err := os.WriteFile(damaged, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	fromDamaged, loadErr := LoadSettings(damaged)
	if loadErr == nil {
		t.Fatal("the damaged file must report an error")
	}

	for name, settings := range map[string]Settings{
		"not asked": {}, "damaged file": fromDamaged, "answered no": Settings{}.WithAutoCheck(false),
	} {
		client := &fakeClient{results: []error{update.ErrNotNewer}}
		clock := &countingClock{}
		ui := &fakeUI{}
		l := NewLauncher(
			LauncherConfig{Settings: settings, UI: ui, Client: client, Current: "0.20.0", After: clock.After},
		)

		l.Start(context.Background())

		if client.count() != 0 || clock.calls != 0 {
			t.Errorf("%s: %d check(s), %d scheduled wait(s) at start", name, client.count(), clock.calls)
		}

		// Answering the first-start question with No keeps it closed.
		if !settings.Asked() {
			ui.consent(false)
		}

		l.Close()
		l.Wait()

		if client.count() != 0 || clock.calls != 0 {
			t.Errorf("%s: %d check(s), %d wait(s) after the answer", name, client.count(), clock.calls)
		}
	}
}

func TestSecurity_T35_ArchiveCannotEscapeItsStagingDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		// skip-env: symlink-privilege
		t.Skip("the link cases need symbolic links, which Windows creates only with a privilege")
	}

	exe := zipEntry{name: "A.app/Contents/MacOS/stuarty-desktop", body: "x"}
	link := func(name, to string) zipEntry { return zipEntry{name: name, body: to, mode: os.ModeSymlink | 0o777} }

	cases := map[string][]zipEntry{
		"parent traversal":   {exe, {name: "A.app/../../escape", body: "x"}},
		"absolute name":      {exe, {name: "/tmp/escape", body: "x"}},
		"absolute link":      {exe, link("A.app/Contents/l", "/etc")},
		"link out of bundle": {exe, link("A.app/Contents/l", "../../..")},
		// The second link looks inside the bundle on paper, but it sits below the first one, so it
		// would point outside once extracted. Nothing may be written below a link.
		"link below a link": {exe, link("A.app/Contents/up", ".."), link("A.app/Contents/up/out", "..")},
		"file below a link": {exe, link("A.app/Contents/up", ".."), {name: "A.app/Contents/up/file", body: "x"}},
		"device file":       {exe, {name: "A.app/dev", mode: os.ModeDevice | 0o666}},
	}

	for name, entries := range cases {
		target, apps := bundle(t)

		if _, err := Swap(swapReq(t, target, makeZip(t, entries...))); !errors.Is(err, ErrBadArchive) {
			t.Errorf("%s: got %v, want ErrBadArchive", name, err)
		}

		if read(t, target.Exec) != "old" || !gone(target.Path+".bak") {
			t.Errorf("%s: the installed application was touched", name)
		}

		for _, outside := range []string{filepath.Join(apps, "escape"), filepath.Join(filepath.Dir(apps), "escape")} {
			if !gone(outside) {
				t.Errorf("%s: %s was written outside the staging directory", name, outside)
			}
		}
	}
}

func TestSecurity_T35_FailedStartRestoresThePreviousVersion(t *testing.T) {
	target, _ := bundle(t)

	var (
		pending string
		backup  string
	)

	inst := installerFor(t, target, func(backupExec, pendingFile string) error {
		backup, pending = backupExec, pendingFile

		return nil
	}, new(atomic.Int32))

	artifact := newBundleZip(t)
	if err := inst.Install(context.Background(), update.Artifact{Path: artifact, Version: "0.21.0"}); err != nil {
		t.Fatal(err)
	}

	if read(t, target.Exec) != "new" || read(t, backup) != "old" {
		t.Fatal("the swap did not happen")
	}

	// The new version starts and exits at once, as a broken build would.
	crashed := make(chan struct{})
	close(crashed)

	starts := 0
	err := Watch(context.Background(), WatchConfig{
		StateDir: filepath.Dir(pending),
		Start: func(string) (<-chan struct{}, error) {
			starts++

			if starts == 1 {
				return crashed, nil
			}

			return nil, nil
		},
		Timeout: time.Second, Poll: time.Millisecond,
	})
	if err == nil {
		t.Fatal("the failed start must be reported")
	}

	if read(t, target.Exec) != "old" || !gone(target.Path+".bak") || starts != 2 {
		t.Fatalf("not restored: program %q, starts %d", read(t, target.Exec), starts)
	}

	// The schedule never offers the version that failed again.
	failed, ok := LoadFailed(filepath.Dir(pending))
	if !ok || failed.Version != "0.21.0" {
		t.Fatalf("failed record %+v %v", failed, ok)
	}

	client := &fakeClient{offer: &update.Offer{Version: "0.21.0"}, results: []error{nil}}
	ui := &fakeUI{}
	l := NewLauncher(LauncherConfig{
		Settings: Settings{}.WithAutoCheck(true), UI: ui, Client: client, Current: "0.20.0",
		After: new(countingClock).After, RolledBack: failed.Version,
	})

	l.Start(context.Background())
	eventually(t, "the check", func() bool { return client.count() == 1 })
	l.Close()
	l.Wait()

	if len(ui.offered()) != 0 {
		t.Error("a version that failed to start was offered again by the schedule")
	}
}

func TestSecurity_T35_InstallerOnlyTakesTheFileOfItsSystem(t *testing.T) {
	// A setup program offered to the macOS installer, or a zip to the Linux one, never reaches the swap.
	cases := map[string]string{"darwin": "update.AppImage", "linux": "update.zip"}

	for goos, file := range cases {
		inst := swapInstaller{
			goos: goos, quit: func() { t.Error("quit") },
			locate: func(string) (Target, error) { t.Error("located"); return Target{}, nil },
		}

		if err := inst.Install(
			context.Background(),
			update.Artifact{Path: file},
		); !errors.Is(
			err,
			ErrInstallUnsupported,
		) {
			t.Errorf("%s: %v", goos, err)
		}
	}
}
