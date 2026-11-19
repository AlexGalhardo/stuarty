package shell

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/desktop/update"
)

type zipEntry struct {
	name string
	body string
	mode os.FileMode // 0 means a regular file 0o755
}

func makeZip(t *testing.T, entries ...zipEntry) string {
	t.Helper()

	var buf bytes.Buffer

	zw := zip.NewWriter(&buf)

	for _, e := range entries {
		h := &zip.FileHeader{Name: e.name, Method: zip.Deflate}

		if e.mode == 0 {
			h.SetMode(0o755)
		} else {
			h.SetMode(e.mode)
		}

		w, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}

		if _, err := w.Write([]byte(e.body)); err != nil {
			t.Fatal(err)
		}
	}

	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "update.zip")
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	return path
}

func write(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	return string(b)
}

func gone(path string) bool {
	_, err := os.Lstat(path)

	return os.IsNotExist(err)
}

// bundle is an installed old application: Apps/Stuarty.app with its program.
func bundle(t *testing.T) (Target, string) {
	t.Helper()

	apps := t.TempDir()
	app := filepath.Join(apps, "Stuarty.app")
	exe := filepath.Join(app, "Contents", "MacOS", "stuarty-desktop")
	write(t, exe, "old")

	return Target{Path: app, Exec: exe}, apps
}

func newBundleZip(t *testing.T) string {
	t.Helper()

	return makeZip(t,
		zipEntry{name: "Stuarty.app/", mode: os.ModeDir | 0o755},
		zipEntry{name: "Stuarty.app/Contents/MacOS/stuarty-desktop", body: "new"},
		zipEntry{name: "Stuarty.app/Contents/Info.plist", body: "plist", mode: 0o644},
	)
}

func swapReq(t *testing.T, target Target, artifact string) SwapRequest {
	t.Helper()

	return SwapRequest{
		StateDir: filepath.Join(t.TempDir(), "state"), Target: target, Artifact: artifact,
		Version: "0.21.0", Now: time.Now,
	}
}

func TestSwapReplacesTheBundleAndKeepsABackup(t *testing.T) {
	target, apps := bundle(t)
	req := swapReq(t, target, newBundleZip(t))

	p, err := Swap(req)
	if err != nil {
		t.Fatal(err)
	}

	if got := read(t, target.Exec); got != "new" {
		t.Errorf("target program %q", got)
	}

	if got := read(t, p.BackupExec); got != "old" {
		t.Errorf("backup program %q", got)
	}

	if p.TargetExec != target.Exec || p.Version != "0.21.0" {
		t.Errorf("pending %+v", p)
	}

	var stored Pending
	if err := readJSON(pendingPath(req.StateDir), &stored); err != nil || stored != p {
		t.Errorf("marker %+v, %v", stored, err)
	}

	if !gone(filepath.Join(apps, ".Stuarty.app.staging")) {
		t.Error("the staging directory must be removed")
	}

	if !gone(filepath.Join(req.StateDir, "swap.lock")) {
		t.Error("the lock must be released")
	}
}

func TestSwapReplacesAnAppImage(t *testing.T) {
	image := filepath.Join(t.TempDir(), "Vukem.AppImage")
	write(t, image, "old")

	artifact := filepath.Join(t.TempDir(), "new.AppImage")
	write(t, artifact, "new")

	p, err := Swap(swapReq(t, Target{Path: image, Exec: image}, artifact))
	if err != nil {
		t.Fatal(err)
	}

	if read(t, image) != "new" || read(t, p.Backup) != "old" || p.BackupExec != p.Backup || p.TargetExec != image {
		t.Errorf("pending %+v", p)
	}
}

func TestSwapRefusesABadArchiveAndTouchesNothing(t *testing.T) {
	cases := map[string]string{
		"two bundles": makeZip(t,
			zipEntry{name: "A.app/Contents/MacOS/stuarty-desktop", body: "x"},
			zipEntry{name: "B.app/Contents/MacOS/stuarty-desktop", body: "x"}),
		"no program": makeZip(t, zipEntry{name: "A.app/Contents/Info.plist", body: "x"}),
		"not a zip":  filepath.Join(t.TempDir(), "missing.zip"),
		"not an app": makeZip(t, zipEntry{name: "tool/bin", body: "x"}),
	}

	for name, artifact := range cases {
		target, _ := bundle(t)

		if _, err := Swap(swapReq(t, target, artifact)); err == nil {
			t.Errorf("%s: want an error", name)
		}

		if read(t, target.Exec) != "old" {
			t.Errorf("%s: the application was changed", name)
		}

		if !gone(target.Path + ".bak") {
			t.Errorf("%s: a backup was made", name)
		}
	}
}

func TestExtractRefusesNamesThatLeaveTheDirectory(t *testing.T) {
	for _, name := range []string{"../evil", "A.app/../../evil", "/abs/evil", "a\\..\\evil", "C:/evil"} {
		target, apps := bundle(t)
		art := makeZip(t, zipEntry{name: name, body: "x"})

		if _, err := Swap(swapReq(t, target, art)); !errors.Is(err, ErrBadArchive) {
			t.Errorf("%q: %v", name, err)
		}

		if !gone(filepath.Join(filepath.Dir(apps), "evil")) {
			t.Errorf("%q: a file was written outside", name)
		}
	}
}

func TestExtractRefusesLinksOutsideTheBundle(t *testing.T) {
	for _, link := range []string{"/etc/passwd", "../../../outside", "../../..", ""} {
		tgt, _ := bundle(t)
		art := makeZip(t,
			zipEntry{name: "A.app/Contents/MacOS/stuarty-desktop", body: "x"},
			zipEntry{name: "A.app/Contents/link", body: link, mode: os.ModeSymlink | 0o777},
		)

		if _, err := Swap(swapReq(t, tgt, art)); !errors.Is(err, ErrBadArchive) {
			t.Errorf("link to %q: %v", link, err)
		}
	}
}

func TestExtractKeepsASafeLink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating a symbolic link needs a privilege on Windows")
	}

	tgt, _ := bundle(t)
	art := makeZip(t,
		zipEntry{name: "A.app/Contents/MacOS/stuarty-desktop", body: "new"},
		zipEntry{name: "A.app/Contents/Frameworks/Current", body: "../MacOS", mode: os.ModeSymlink | 0o777},
	)

	if _, err := Swap(swapReq(t, tgt, art)); err != nil {
		t.Fatal(err)
	}

	got, err := os.Readlink(filepath.Join(tgt.Path, "Contents", "Frameworks", "Current"))
	if err != nil || got != "../MacOS" {
		t.Errorf("link %q, %v", got, err)
	}
}

func TestExtractDropsSetuidBits(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no permission bits on Windows")
	}

	tgt, _ := bundle(t)
	art := makeZip(t,
		zipEntry{name: "A.app/Contents/MacOS/stuarty-desktop", body: "new", mode: 0o755 | os.ModeSetuid},
	)

	if _, err := Swap(swapReq(t, tgt, art)); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(tgt.Exec)
	if err != nil || info.Mode()&(os.ModeSetuid|os.ModeSetgid) != 0 {
		t.Errorf("mode %v, %v", info.Mode(), err)
	}
}

func TestOnlyOneSwapAtATime(t *testing.T) {
	target, _ := bundle(t)
	req := swapReq(t, target, newBundleZip(t))

	if err := os.MkdirAll(req.StateDir, 0o700); err != nil {
		t.Fatal(err)
	}

	lock := filepath.Join(req.StateDir, "swap.lock")
	write(t, lock, "")

	if _, err := Swap(req); !errors.Is(err, ErrSwapBusy) || read(t, target.Exec) != "old" {
		t.Fatalf("a held lock: %v", err)
	}

	// A lock left by a dead process does not block forever.
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(lock, old, old); err != nil {
		t.Fatal(err)
	}

	if _, err := Swap(req); err != nil {
		t.Fatalf("a stale lock: %v", err)
	}
}

func TestLocateTarget(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

	appDir := filepath.Join("Applications", "Stuarty.app")
	app := filepath.Join(appDir, "Contents", "MacOS", "stuarty-desktop")

	if got, err := LocateTarget("darwin", app, env(nil)); err != nil || got.Path != appDir || got.Exec != app {
		t.Errorf("darwin bundle: %+v, %v", got, err)
	}

	loose := filepath.Join("dev", "stuarty-desktop")
	if _, err := LocateTarget("darwin", loose, env(nil)); !errors.Is(err, ErrNotInstalled) {
		t.Errorf("darwin loose binary: %v", err)
	}

	got, err := LocateTarget(
		"linux",
		"/tmp/.mount_x/usr/bin/app",
		env(map[string]string{"APPIMAGE": "/home/a/V.AppImage"}),
	)
	if err != nil || got.Path != "/home/a/V.AppImage" || got.Exec != got.Path {
		t.Errorf("appimage: %+v, %v", got, err)
	}

	_, err = LocateTarget("linux", "/usr/lib/stuarty-desktop/stuarty-desktop", env(nil))
	if !errors.Is(err, ErrDebInstall) || !strings.Contains(err.Error(), ReleasePageURL) {
		t.Errorf("deb: %v", err)
	}

	if _, err := LocateTarget("linux", "/opt/x/app", env(nil)); !errors.Is(err, ErrNotInstalled) {
		t.Errorf("loose: %v", err)
	}
}

type starter struct {
	calls  []string
	exited chan struct{} // returned for the first start
	err    error         // returned for the first start
}

func (s *starter) start(execPath string) (<-chan struct{}, error) {
	s.calls = append(s.calls, execPath)

	if len(s.calls) == 1 {
		if s.err != nil {
			return nil, s.err
		}

		if s.exited != nil {
			return s.exited, nil
		}
	}

	return nil, nil
}

func swapped(t *testing.T) (Pending, SwapRequest, Target) {
	t.Helper()

	target, _ := bundle(t)
	req := swapReq(t, target, newBundleZip(t))

	p, err := Swap(req)
	if err != nil {
		t.Fatal(err)
	}

	return p, req, target
}

func watchCfg(req SwapRequest, s *starter) WatchConfig {
	return WatchConfig{
		StateDir: req.StateDir, Start: s.start, Timeout: 400 * time.Millisecond, Poll: 5 * time.Millisecond,
	}
}

func TestWatchKeepsTheNewVersionWhenItConfirms(t *testing.T) {
	p, req, target := swapped(t)
	s := &starter{}

	go func() {
		time.Sleep(30 * time.Millisecond)

		if err := ConfirmStart(req.StateDir, "0.21.0"); err != nil {
			t.Error(err)
		}
	}()

	if err := Watch(context.Background(), watchCfg(req, s)); err != nil {
		t.Fatal(err)
	}

	if read(t, target.Exec) != "new" || len(s.calls) != 1 || s.calls[0] != p.TargetExec {
		t.Errorf("program %q, starts %v", read(t, target.Exec), s.calls)
	}

	for _, path := range []string{p.Backup, pendingPath(req.StateDir), confirmedPath(req.StateDir, "0.21.0")} {
		if !gone(path) {
			t.Errorf("%s must be deleted", path)
		}
	}
}

func assertRolledBack(t *testing.T, req SwapRequest, target Target, s *starter) {
	t.Helper()

	if read(t, target.Exec) != "old" {
		t.Errorf("the previous version was not restored: %q", read(t, target.Exec))
	}

	if !gone(target.Path + ".bak") {
		t.Error("the backup must be consumed by the rollback")
	}

	if f, ok := LoadFailed(req.StateDir); !ok || f.Version != "0.21.0" || f.Notified {
		t.Errorf("failed record %+v %v", f, ok)
	}

	if !gone(pendingPath(req.StateDir)) {
		t.Error("pending must be cleared")
	}

	if len(s.calls) != 2 || s.calls[1] != target.Exec {
		t.Errorf("the previous version must be started again: %v", s.calls)
	}
}

func TestWatchRollsBackWhenTheNewVersionExits(t *testing.T) {
	_, req, target := swapped(t)
	s := &starter{exited: make(chan struct{})}
	close(s.exited)

	if err := Watch(context.Background(), watchCfg(req, s)); err == nil || !strings.Contains(err.Error(), "undone") {
		t.Fatalf("want the update undone, got %v", err)
	}

	assertRolledBack(t, req, target, s)
}

func TestWatchRollsBackWhenTheNewVersionNeverConfirms(t *testing.T) {
	_, req, target := swapped(t)
	s := &starter{}

	if err := Watch(context.Background(), watchCfg(req, s)); err == nil {
		t.Fatal("want an error")
	}

	assertRolledBack(t, req, target, s)
}

func TestWatchRollsBackWhenTheNewVersionCannotBeStarted(t *testing.T) {
	_, req, target := swapped(t)
	s := &starter{err: errors.New("exec format error")}

	if err := Watch(context.Background(), watchCfg(req, s)); err == nil {
		t.Fatal("want an error")
	}

	assertRolledBack(t, req, target, s)
}

// TestAnUpdateFromVukemVideoIsConfirmedByStuarty: an installed Vukem Video replaces itself with Stuarty and
// its watchdog waits in the update folder of the former name (ADR-0044). The start of Stuarty confirms
// there too, so the update is kept instead of undone.
func TestAnUpdateFromVukemVideoIsConfirmedByStuarty(t *testing.T) {
	config := t.TempDir()
	t.Setenv("APPDATA", config)
	t.Setenv("XDG_CONFIG_HOME", config)
	t.Setenv("HOME", config)

	base, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}

	target, _ := bundle(t)
	req := swapReq(t, target, newBundleZip(t))
	req.StateDir = filepath.Join(base, "vukem-video", "update")

	p, err := Swap(req)
	if err != nil {
		t.Fatal(err)
	}

	var confirmErr error

	// The watchdog starts the new version, which confirms as the application does once it has started.
	start := func(string) (<-chan struct{}, error) {
		confirmErr = ConfirmUpdates("0.21.0")

		return nil, nil
	}

	// The confirmation is written before the watchdog first looks, so the timeout is never reached.
	cfg := WatchConfig{
		StateDir: req.StateDir, Start: start, Timeout: 400 * time.Millisecond, Poll: 5 * time.Millisecond,
	}
	if err := Watch(context.Background(), cfg); err != nil {
		t.Fatalf("Watch = %v, want the update kept", err)
	}

	if confirmErr != nil {
		t.Fatal(confirmErr)
	}

	if read(t, target.Exec) != "new" || !gone(p.Backup) {
		t.Errorf("program %q, backup gone %v: want the new version kept", read(t, target.Exec), gone(p.Backup))
	}

	if _, failed := LoadFailed(req.StateDir); failed {
		t.Error("the update was recorded as failed")
	}
}

func TestConfirmIsIgnoredForAnotherVersion(t *testing.T) {
	_, req, _ := swapped(t)

	if err := ConfirmStart(req.StateDir, "0.20.0"); err != nil {
		t.Fatal(err)
	}

	if !gone(confirmedPath(req.StateDir, "0.20.0")) {
		t.Error("only the pending version confirms")
	}
}

func TestRunningTheVersionAfterAFailedTryClearsTheRecord(t *testing.T) {
	dir := t.TempDir()
	if err := writeJSON(failedPath(dir), Failed{Version: "0.21.0"}); err != nil {
		t.Fatal(err)
	}

	if err := MarkFailedNotified(dir); err != nil {
		t.Fatal(err)
	}

	if f, _ := LoadFailed(dir); !f.Notified {
		t.Error("notified must be recorded")
	}

	if err := ConfirmStart(dir, "0.21.0"); err != nil {
		t.Fatal(err)
	}

	if _, ok := LoadFailed(dir); ok {
		t.Error("a version that started is no longer failed")
	}
}

func installerFor(t *testing.T, target Target, start func(string, string) error, quits *atomic.Int32) swapInstaller {
	t.Helper()

	state := filepath.Join(t.TempDir(), "state")

	return swapInstaller{
		goos:   "darwin",
		quit:   func() { quits.Add(1) },
		locate: func(string) (Target, error) { return target, nil },
		start:  start,
		state:  func() (string, error) { return state, nil },
		now:    time.Now,
	}
}

func TestSwapInstallerSwapsStartsTheWatchdogAndQuits(t *testing.T) {
	target, _ := bundle(t)

	var (
		quits atomic.Int32
		args  []string
	)

	inst := installerFor(t, target, func(backupExec, pending string) error {
		args = []string{backupExec, pending}

		return nil
	}, &quits)

	if err := inst.Install(
		context.Background(),
		update.Artifact{Path: newBundleZip(t), Version: "0.21.0"},
	); err != nil {
		t.Fatal(err)
	}

	if quits.Load() != 1 || read(t, target.Exec) != "new" || read(t, args[0]) != "old" ||
		filepath.Base(args[1]) != "pending.json" {
		t.Errorf("quits %d, args %v", quits.Load(), args)
	}
}

func TestSwapInstallerUndoesTheSwapWhenTheWatchdogCannotStart(t *testing.T) {
	target, _ := bundle(t)

	var quits atomic.Int32

	inst := installerFor(t, target, func(string, string) error { return errors.New("boom") }, &quits)

	err := inst.Install(context.Background(), update.Artifact{Path: newBundleZip(t), Version: "0.21.0"})
	if err == nil || quits.Load() != 0 || read(t, target.Exec) != "old" {
		t.Fatalf("err %v, quits %d, program %q", err, quits.Load(), read(t, target.Exec))
	}

	if !gone(target.Path + ".bak") {
		t.Error("the backup must be put back")
	}
}

func TestSwapInstallerRefusesWhenNotInstalledOrWrongFile(t *testing.T) {
	var quits atomic.Int32

	inst := installerFor(t, Target{}, nil, &quits)
	inst.locate = func(string) (Target, error) { return Target{}, ErrDebInstall }

	if err := inst.Install(context.Background(), update.Artifact{Path: "x.zip"}); !errors.Is(err, ErrDebInstall) {
		t.Errorf("deb: %v", err)
	}

	if err := inst.Install(
		context.Background(),
		update.Artifact{Path: "x.exe"},
	); !errors.Is(
		err,
		ErrInstallUnsupported,
	) {
		t.Errorf("wrong file: %v", err)
	}

	if quits.Load() != 0 {
		t.Error("must not quit")
	}
}

// killedWatchdog is a swap whose watchdog died: pending.json and the backup stay, and no heartbeat is
// fresh any more.
func killedWatchdog(t *testing.T) (SwapRequest, Target) {
	t.Helper()

	_, req, target := swapped(t)
	age := time.Now().Add(-time.Hour)

	if err := os.Chtimes(pendingPath(req.StateDir), age, age); err != nil {
		t.Fatal(err)
	}

	return req, target
}

func TestSecurity_T35_KilledWatchdogStillRestoresThePreviousVersion(t *testing.T) {
	req, target := killedWatchdog(t)

	write(t, filepath.Join(filepath.Dir(target.Path), "."+filepath.Base(target.Path)+".staging", "x"), "x")

	done, err := RecoverPending(req.StateDir, "0.20.0", time.Now)
	if err != nil || !done {
		t.Fatalf("recover: %v %v", done, err)
	}

	if read(t, target.Exec) != "old" || !gone(target.Path+".bak") || !gone(pendingPath(req.StateDir)) {
		t.Error("the previous version must be back and the marker cleared")
	}

	if !gone(filepath.Join(filepath.Dir(target.Path), "."+filepath.Base(target.Path)+".staging")) {
		t.Error("the staging must be cleaned")
	}

	if f, ok := LoadFailed(req.StateDir); !ok || f.Version != "0.21.0" || f.Notified {
		t.Errorf("failed record %+v %v", f, ok)
	}

	// Idempotent: a second start finds nothing to do.
	if done, err := RecoverPending(req.StateDir, "0.20.0", time.Now); done || err != nil {
		t.Errorf("second recover: %v %v", done, err)
	}
}

func TestRecoverLeavesAnAliveWatchdogAlone(t *testing.T) {
	_, req, target := swapped(t)

	// A fresh pending.json: the watchdog has not started yet.
	if done, err := RecoverPending(
		req.StateDir,
		"0.20.0",
		time.Now,
	); done || err != nil ||
		read(t, target.Exec) != "new" {
		t.Fatalf("fresh marker: %v %v", done, err)
	}

	// A fresh heartbeat of a running watchdog.
	age := time.Now().Add(-time.Hour)
	if err := os.Chtimes(pendingPath(req.StateDir), age, age); err != nil {
		t.Fatal(err)
	}

	write(t, watchLockPath(req.StateDir), "")

	if done, err := RecoverPending(
		req.StateDir,
		"0.20.0",
		time.Now,
	); done || err != nil ||
		read(t, target.Exec) != "new" {
		t.Fatalf("heartbeat: %v %v", done, err)
	}
}

func TestRecoverWithTheNewCopyRunningConfirmsAndDeletesTheBackup(t *testing.T) {
	req, target := killedWatchdog(t)

	if done, err := RecoverPending(
		req.StateDir,
		"0.21.0",
		time.Now,
	); done || err != nil ||
		read(t, target.Exec) != "new" {
		t.Fatalf("the new copy is not undone: %v %v", done, err)
	}

	if err := ConfirmStart(req.StateDir, "0.21.0"); err != nil {
		t.Fatal(err)
	}

	if !gone(target.Path+".bak") || !gone(pendingPath(req.StateDir)) || !gone(confirmedPath(req.StateDir, "0.21.0")) {
		t.Error("a healthy new copy without a watchdog must finish the update")
	}

	if _, ok := LoadFailed(req.StateDir); ok || read(t, target.Exec) != "new" {
		t.Error("nothing failed")
	}
}

func TestRecoverIgnoresAStalePendingOfTheSameVersion(t *testing.T) {
	req, target := killedWatchdog(t)
	// The update was finished by hand: the backup is already gone.
	if err := os.RemoveAll(target.Path + ".bak"); err != nil {
		t.Fatal(err)
	}

	if done, err := RecoverPending(req.StateDir, "0.21.0", time.Now); done || err != nil {
		t.Fatalf("same version: %v %v", done, err)
	}

	// An old copy without a backup: the target is never deleted, the marker is dropped.
	if done, err := RecoverPending(
		req.StateDir,
		"0.20.0",
		time.Now,
	); done || err != nil ||
		read(t, target.Exec) != "new" {
		t.Fatalf("no backup: %v %v", done, err)
	}

	if !gone(pendingPath(req.StateDir)) {
		t.Error("a marker with no backup is dropped")
	}
}

func TestRecoverFinishesWhenTheNewVersionAlreadyConfirmed(t *testing.T) {
	req, target := killedWatchdog(t)
	write(t, confirmedPath(req.StateDir, "0.21.0"), "ok")

	if done, err := RecoverPending(
		req.StateDir,
		"0.20.0",
		time.Now,
	); done || err != nil || read(t, target.Exec) != "new" ||
		!gone(target.Path+".bak") ||
		!gone(pendingPath(req.StateDir)) {
		t.Fatalf("confirmed: %v %v", done, err)
	}
}

func TestConcurrentStartsRecoverOnce(t *testing.T) {
	req, target := killedWatchdog(t)

	// A start that holds the swap lock keeps the others out.
	unlock, err := lockSwap(req.StateDir, time.Now)
	if err != nil {
		t.Fatal(err)
	}

	if done, err := RecoverPending(
		req.StateDir,
		"0.20.0",
		time.Now,
	); done || err != nil ||
		read(t, target.Exec) != "new" {
		t.Fatalf("locked: %v %v", done, err)
	}

	unlock()

	var recovered atomic.Int32

	var wg sync.WaitGroup

	for range 8 {
		wg.Go(func() {
			if done, err := RecoverPending(req.StateDir, "0.20.0", time.Now); err == nil && done {
				recovered.Add(1)
			}
		})
	}

	wg.Wait()

	if recovered.Load() != 1 || read(t, target.Exec) != "old" || !gone(target.Path+".bak") {
		t.Errorf("recovered %d times, exec %q", recovered.Load(), read(t, target.Exec))
	}
}
