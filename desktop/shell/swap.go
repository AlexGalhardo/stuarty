package shell

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// The in-place update of macOS and Linux (SDD-009 section 10.5, launcher flow items 6 and 7): replace the
// application with a backup, start the new one, and put the backup back when it does not confirm.

// Limits of an archive of an update. The artifact is already verified, so these only bound a mistake.
const (
	maxArchiveEntries = 100_000
	maxArchiveBytes   = 2 << 30
	maxLinkBytes      = 4096
	swapLockStale     = 10 * time.Minute

	// watchdogStale is how long the heartbeat of the watchdog (and the age of a fresh pending.json, which
	// covers the moment before the watchdog is up) may be before the watchdog counts as dead.
	watchdogStale = 15 * time.Second

	// StartTimeout is how long the watchdog waits for the new version to confirm its start.
	StartTimeout = 60 * time.Second
)

// Errors of the swap.
var (
	ErrSwapBusy     = errors.New("another update is replacing the application")
	ErrBadArchive   = errors.New("the update archive is not valid")
	ErrNotInstalled = errors.New("this copy of the application is not an installed bundle or AppImage, " +
		"so it cannot replace itself; download the new version from " + ReleasePageURL)
	ErrDebInstall = errors.New("this copy was installed from a .deb package, which needs root to update; " +
		"download the new version from " + ReleasePageURL)
)

// UpdateStateDir is where the swap keeps its lock, its marker files and the record of a rollback. It is
// in the user config directory, never in the workspace.
func UpdateStateDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("update state: %w", err)
	}

	return filepath.Join(dir, "vukem-video", "update"), nil
}

// Target is the installed application: Path is the bundle directory (macOS) or the AppImage file
// (Linux); Exec is the program inside it that is running now.
type Target struct {
	Path string
	Exec string
}

// execRel is Exec relative to Path ("." for an AppImage, which is its own program).
func (t Target) execRel() string {
	rel, err := filepath.Rel(t.Path, t.Exec)
	if err != nil {
		return "."
	}

	return rel
}

func (t Target) isBundle() bool { return strings.HasSuffix(t.Path, ".app") }

// Pending is the marker of a swap that is not confirmed yet (update/pending.json).
type Pending struct {
	Version    string `json:"version"`
	Target     string `json:"target"`
	Backup     string `json:"backup"`
	TargetExec string `json:"target_exec"`
	BackupExec string `json:"backup_exec"`
}

func pendingPath(stateDir string) string { return filepath.Join(stateDir, "pending.json") }
func confirmedPath(stateDir, version string) string {
	return filepath.Join(stateDir, "confirmed-"+version)
}
func failedPath(stateDir string) string    { return filepath.Join(stateDir, "failed.json") }
func watchLockPath(stateDir string) string { return filepath.Join(stateDir, "watch.lock") }

// LocateTarget finds what the running executable is, for goos. A copy that is not an installed bundle or
// an AppImage (a build run from a folder, a .deb) cannot replace itself.
func LocateTarget(goos, exe string, getenv func(string) string) (Target, error) {
	switch goos {
	case "darwin":
		if app := bundleOf(exe); app != "" {
			return Target{Path: app, Exec: exe}, nil
		}
	case "linux":
		if image := getenv("APPIMAGE"); image != "" {
			return Target{Path: image, Exec: image}, nil
		}

		if strings.HasPrefix(filepath.ToSlash(exe), "/usr/") {
			return Target{}, ErrDebInstall
		}
	}

	return Target{}, ErrNotInstalled
}

// SwapRequest is one replacement.
type SwapRequest struct {
	StateDir string
	Target   Target
	// Artifact is the verified file: a zip with one .app bundle (macOS) or the AppImage (Linux).
	Artifact string
	Version  string
	Now      func() time.Time
}

// Swap stages the new application next to the target, moves the target to <target>.bak, puts the new one
// in its place and writes the marker. The rename that publishes the new version is atomic because the
// staged copy is in the same directory. If it fails the backup is put back.
func Swap(req SwapRequest) (Pending, error) {
	if err := os.MkdirAll(req.StateDir, 0o700); err != nil {
		return Pending{}, fmt.Errorf("update state: %w", err)
	}

	unlock, err := lockSwap(req.StateDir, req.Now)
	if err != nil {
		return Pending{}, err
	}

	defer unlock()

	parent, base := filepath.Dir(req.Target.Path), filepath.Base(req.Target.Path)
	stageRoot := filepath.Join(parent, "."+base+".staging")
	backup := req.Target.Path + ".bak"

	if err := os.RemoveAll(stageRoot); err != nil {
		return Pending{}, fmt.Errorf("clean %s: %w", stageRoot, err)
	}

	defer func() { _ = os.RemoveAll(stageRoot) }()

	staged, err := stage(req, stageRoot)
	if err != nil {
		return Pending{}, err
	}

	if err := os.RemoveAll(backup); err != nil {
		return Pending{}, fmt.Errorf("remove the old backup: %w", err)
	}

	if err := os.Rename(req.Target.Path, backup); err != nil {
		return Pending{}, fmt.Errorf("move the application aside: %w", err)
	}

	if err := os.Rename(staged, req.Target.Path); err != nil {
		if undo := os.Rename(backup, req.Target.Path); undo != nil {
			return Pending{}, fmt.Errorf("replace the application: %w (and restoring it failed: %w)", err, undo)
		}

		return Pending{}, fmt.Errorf("replace the application: %w", err)
	}

	rel := req.Target.execRel()
	pending := Pending{
		Version: req.Version, Target: req.Target.Path, Backup: backup,
		TargetExec: filepath.Join(req.Target.Path, rel), BackupExec: filepath.Join(backup, rel),
	}

	if !req.Target.isBundle() {
		pending.TargetExec, pending.BackupExec = req.Target.Path, backup
	}

	if err := writeJSON(pendingPath(req.StateDir), pending); err != nil {
		_ = rollbackFiles(pending)

		return Pending{}, err
	}

	return pending, nil
}

// stage prepares the new application under stageRoot and returns the path that becomes the target.
func stage(req SwapRequest, stageRoot string) (string, error) {
	if err := os.Mkdir(stageRoot, 0o700); err != nil {
		return "", fmt.Errorf("cannot write next to the application (%s): %w", filepath.Dir(stageRoot), err)
	}

	if !req.Target.isBundle() {
		out := filepath.Join(stageRoot, filepath.Base(req.Target.Path))
		if err := copyExecutable(req.Artifact, out); err != nil {
			return "", err
		}

		return out, nil
	}

	if err := extractZip(req.Artifact, stageRoot); err != nil {
		return "", err
	}

	entries, err := os.ReadDir(stageRoot)
	if err != nil || len(entries) != 1 || !entries[0].IsDir() || !strings.HasSuffix(entries[0].Name(), ".app") {
		return "", fmt.Errorf("%w: it must hold exactly one .app bundle", ErrBadArchive)
	}

	bundle := filepath.Join(stageRoot, entries[0].Name())

	if info, err := os.Stat(filepath.Join(bundle, req.Target.execRel())); err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("%w: the bundle has no program at %s", ErrBadArchive, req.Target.execRel())
	}

	return bundle, nil
}

func copyExecutable(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return fmt.Errorf("open the update: %w", err)
	}

	defer func() { _ = in.Close() }()

	// An AppImage is a program: it must be executable.
	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o755) //nolint:gosec // G302: see above
	if err != nil {
		return fmt.Errorf("stage the update: %w", err)
	}

	n, err := io.Copy(out, in)
	if err == nil && n == 0 {
		err = fmt.Errorf("%w: the file is empty", ErrBadArchive)
	}

	if err == nil {
		err = out.Sync()
	}

	if cerr := out.Close(); err == nil {
		err = cerr
	}

	if err != nil {
		return fmt.Errorf("stage the update: %w", err)
	}

	return os.Chmod(to, 0o755) //nolint:gosec // G302: an AppImage is a program
}

// extractZip writes a zip under dest, confined to it. It refuses names that leave dest, absolute and
// escaping symbolic links, special files, and archives past the limits.
func extractZip(zipPath, dest string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrBadArchive, err)
	}

	defer func() { _ = zr.Close() }()

	if len(zr.File) > maxArchiveEntries {
		return fmt.Errorf("%w: more than %d entries", ErrBadArchive, maxArchiveEntries)
	}

	root, err := os.OpenRoot(dest)
	if err != nil {
		return fmt.Errorf("stage the update: %w", err)
	}

	defer func() { _ = root.Close() }()

	budget := int64(maxArchiveBytes)
	links := map[string]bool{}

	for _, f := range zr.File {
		name, err := safeName(f.Name)
		if err != nil {
			return err
		}

		// A link is checked as if its directory were a real one. An entry written through an earlier
		// link would make that check wrong (a link to ".." created inside a link to ".." leaves the
		// bundle), so nothing may be extracted below a link.
		for dir := path.Dir(name); dir != "."; dir = path.Dir(dir) {
			if links[dir] {
				return fmt.Errorf("%w: %q is below a link", ErrBadArchive, f.Name)
			}
		}

		if err := extractEntry(root, f, name, &budget); err != nil {
			return err
		}

		if f.Mode()&fs.ModeSymlink != 0 {
			links[name] = true
		}
	}

	return nil
}

func safeName(raw string) (string, error) {
	name := path.Clean(raw)
	if raw == "" || strings.ContainsAny(raw, "\\\x00") || path.IsAbs(name) || name == "." ||
		name == ".." || strings.HasPrefix(name, "../") || strings.Contains(name, ":") {
		return "", fmt.Errorf("%w: entry name %q", ErrBadArchive, raw)
	}

	return name, nil
}

func extractEntry(root *os.Root, f *zip.File, name string, budget *int64) error {
	mode := f.Mode()

	switch {
	case mode.IsDir():
		return root.MkdirAll(name, 0o755)
	case mode&fs.ModeSymlink != 0:
		return extractLink(root, f, name)
	case !mode.IsRegular():
		return fmt.Errorf("%w: %q is not a file, a directory or a link", ErrBadArchive, f.Name)
	}

	if dir := path.Dir(name); dir != "." {
		if err := root.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("stage the update: %w", err)
		}
	}

	src, err := f.Open()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrBadArchive, err)
	}

	defer func() { _ = src.Close() }()

	out, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("stage the update: %w", err)
	}

	n, err := io.Copy(out, io.LimitReader(src, *budget+1))
	*budget -= n

	if cerr := out.Close(); err == nil {
		err = cerr
	}

	switch {
	case err != nil:
		return fmt.Errorf("stage the update: %w", err)
	case *budget < 0:
		return fmt.Errorf("%w: more than %d bytes", ErrBadArchive, int64(maxArchiveBytes))
	}

	// Only the permission bits: a setuid or setgid bit in an archive is never applied.
	return root.Chmod(name, mode.Perm())
}

func extractLink(root *os.Root, f *zip.File, name string) error {
	src, err := f.Open()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrBadArchive, err)
	}

	defer func() { _ = src.Close() }()

	raw, err := io.ReadAll(io.LimitReader(src, maxLinkBytes+1))
	if err != nil || len(raw) > maxLinkBytes {
		return fmt.Errorf("%w: link %q", ErrBadArchive, f.Name)
	}

	target := string(raw)
	if target == "" || path.IsAbs(target) || strings.ContainsAny(target, "\\\x00") ||
		strings.HasPrefix(path.Join(path.Dir(name), target), "..") {
		return fmt.Errorf("%w: link %q points outside the bundle", ErrBadArchive, f.Name)
	}

	if dir := path.Dir(name); dir != "." {
		if err := root.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("stage the update: %w", err)
		}
	}

	return root.Symlink(target, name)
}

func lockSwap(stateDir string, now func() time.Time) (func(), error) {
	lock := filepath.Join(stateDir, "swap.lock")

	for range 2 {
		f, err := os.OpenFile(lock, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			_ = f.Close()

			return func() { _ = os.Remove(lock) }, nil
		}

		info, statErr := os.Stat(lock)
		if statErr != nil || now().Sub(info.ModTime()) < swapLockStale {
			return nil, ErrSwapBusy
		}

		// A lock that a dead process left behind.
		_ = os.Remove(lock)
	}

	return nil, ErrSwapBusy
}

func writeJSON(file string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}

	tmp := file + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("update state: %w", err)
	}

	if err := os.Rename(tmp, file); err != nil {
		_ = os.Remove(tmp)

		return fmt.Errorf("update state: %w", err)
	}

	return nil
}

// rollbackFiles deletes the new application and puts the backup back.
func rollbackFiles(p Pending) error {
	if err := os.RemoveAll(p.Target); err != nil {
		return fmt.Errorf("remove the new version: %w", err)
	}

	if err := os.Rename(p.Backup, p.Target); err != nil {
		return fmt.Errorf("restore the previous version: %w", err)
	}

	return nil
}

// ConfirmStart is called by a running application once its server is up and its window exists. When a
// swap to this version is pending it writes the confirmation the watchdog waits for, and it clears the
// record of an earlier failed attempt at this version.
func ConfirmStart(stateDir, running string) error {
	if rec, ok := LoadFailed(stateDir); ok && rec.Version == running {
		_ = os.Remove(failedPath(stateDir))
	}

	// No marker, or one of another version, means this start is not one to confirm.
	if !pendingIs(stateDir, running) {
		return nil
	}

	if err := os.WriteFile(confirmedPath(stateDir, running), []byte("ok\n"), 0o600); err != nil {
		return err
	}

	// A watchdog that was killed cannot delete the backup, so the confirmed start does it.
	finishOrphan(stateDir, running, time.Now)

	return nil
}

// fresh reports whether file was written less than watchdogStale ago.
func fresh(file string, now func() time.Time) bool {
	info, err := os.Stat(file)

	return err == nil && now().Sub(info.ModTime()) < watchdogStale
}

// watchdogAlive reports whether a watchdog is working on the pending update: its heartbeat file is
// fresh, or the marker was written so recently that the watchdog has not started yet.
func watchdogAlive(stateDir string, now func() time.Time) bool {
	return fresh(watchLockPath(stateDir), now) || fresh(pendingPath(stateDir), now)
}

// finish deletes the backup and the markers of an update that started well.
func finish(stateDir string, p Pending) {
	_ = os.RemoveAll(p.Backup)
	_ = os.Remove(pendingPath(stateDir))
	_ = os.Remove(confirmedPath(stateDir, p.Version))
}

// finishOrphan completes an update whose new version is running and healthy but whose watchdog is gone.
func finishOrphan(stateDir, running string, now func() time.Time) {
	if !pendingIs(stateDir, running) || watchdogAlive(stateDir, now) {
		return
	}

	unlock, err := lockSwap(stateDir, now)
	if err != nil {
		return
	}

	defer unlock()

	var p Pending
	if readJSON(pendingPath(stateDir), &p) == nil && p.Version == running {
		finish(stateDir, p)
	}
}

// RecoverPending is called first thing at every start of the application. When an earlier update left
// pending.json, its watchdog is gone (it died mid-update) and the running copy is not the new version, the
// backup is put back exactly as the watchdog would and failed.json tells the person once. A running copy
// that is the new version is left alone: ConfirmStart finishes the update when it starts well. It returns
// whether it undid an update. It takes the swap lock, so two starts at once recover once.
func RecoverPending(stateDir, running string, now func() time.Time) (bool, error) {
	var p Pending
	if readJSON(pendingPath(stateDir), &p) != nil || p.Version == "" || p.Version == running ||
		watchdogAlive(stateDir, now) {
		return false, nil
	}

	unlock, err := lockSwap(stateDir, now)
	if err != nil {
		// Another start is recovering, or a swap is running: it owns the update.
		return false, nil
	}

	defer unlock()

	// Read again under the lock: the other start may have finished already.
	if readJSON(pendingPath(stateDir), &p) != nil || p.Version == running {
		return false, nil
	}

	_ = os.RemoveAll(filepath.Join(filepath.Dir(p.Target), "."+filepath.Base(p.Target)+".staging"))

	if _, err := os.Stat(confirmedPath(stateDir, p.Version)); err == nil {
		finish(stateDir, p)

		return false, nil
	}

	if _, err := os.Lstat(p.Backup); err != nil {
		// Nothing to restore: never delete the target without a backup to put in its place.
		_ = os.Remove(pendingPath(stateDir))

		return false, nil
	}

	if err := rollbackFiles(p); err != nil {
		return false, err
	}

	_ = os.Remove(pendingPath(stateDir))
	_ = os.Remove(confirmedPath(stateDir, p.Version))

	if err := writeJSON(failedPath(stateDir), Failed{Version: p.Version}); err != nil {
		return true, err
	}

	return true, nil
}

func readJSON(file string, v any) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, v)
}

// Failed is the record of an update that was undone (update/failed.json).
type Failed struct {
	Version  string `json:"version"`
	Notified bool   `json:"notified"`
}

// LoadFailed reads the record of an undone update.
func LoadFailed(stateDir string) (Failed, bool) {
	var f Failed
	if err := readJSON(failedPath(stateDir), &f); err != nil || f.Version == "" {
		return Failed{}, false
	}

	return f, true
}

// MarkFailedNotified records that the person was told about the undone update.
func MarkFailedNotified(stateDir string) error {
	f, ok := LoadFailed(stateDir)
	if !ok {
		return nil
	}

	f.Notified = true

	return writeJSON(failedPath(stateDir), f)
}

// WatchConfig is what the watchdog needs; a test injects the starter and the clock.
type WatchConfig struct {
	StateDir string
	// Start runs the program at execPath detached and returns a channel closed when it exits, or nil
	// when it cannot tell.
	Start func(execPath string) (exited <-chan struct{}, err error)
	// After is the clock, default time.After.
	After   func(d time.Duration) <-chan time.Time
	Timeout time.Duration
	Poll    time.Duration
}

// Watch is the watchdog (SDD-009 section 10.5 item 7). It starts the new version and waits for its
// confirmation. When the new version confirms, the backup is deleted; when it exits first, cannot be
// started, or does not confirm in time, the backup is put back and started again.
func Watch(ctx context.Context, cfg WatchConfig) error {
	if cfg.After == nil {
		cfg.After = time.After
	}

	if cfg.Timeout == 0 {
		cfg.Timeout = StartTimeout
	}

	if cfg.Poll == 0 {
		cfg.Poll = 200 * time.Millisecond
	}

	var p Pending
	if err := readJSON(pendingPath(cfg.StateDir), &p); err != nil {
		return fmt.Errorf("read the pending update: %w", err)
	}

	// The heartbeat tells a later start that this watchdog is alive; a killed one stops beating.
	beat := watchLockPath(cfg.StateDir)
	if err := os.WriteFile(beat, nil, 0o600); err != nil {
		return fmt.Errorf("watchdog lock: %w", err)
	}

	defer func() { _ = os.Remove(beat) }()

	exited, err := cfg.Start(p.TargetExec)
	if err != nil {
		return failUpdate(cfg, p, fmt.Errorf("start the new version: %w", err))
	}

	deadline := cfg.After(cfg.Timeout)

	for {
		now := time.Now()
		_ = os.Chtimes(beat, now, now)

		if _, err := os.Stat(confirmedPath(cfg.StateDir, p.Version)); err == nil {
			finish(cfg.StateDir, p)

			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-exited:
			// One last look: the program may have confirmed and then been closed by the person.
			if _, err := os.Stat(confirmedPath(cfg.StateDir, p.Version)); err == nil {
				continue
			}

			return failUpdate(cfg, p, errors.New("the new version exited before it started"))
		case <-deadline:
			return failUpdate(cfg, p, errors.New("the new version did not confirm its start in time"))
		case <-cfg.After(cfg.Poll):
		}
	}
}

func failUpdate(cfg WatchConfig, p Pending, cause error) error {
	if err := rollbackFiles(p); err != nil {
		return fmt.Errorf("%w (%w)", err, cause)
	}

	_ = os.Remove(pendingPath(cfg.StateDir))
	_ = os.Remove(confirmedPath(cfg.StateDir, p.Version))

	if err := writeJSON(failedPath(cfg.StateDir), Failed{Version: p.Version}); err != nil {
		return err
	}

	if _, err := cfg.Start(p.TargetExec); err != nil {
		return fmt.Errorf("the update was undone but the previous version did not start: %w", err)
	}

	return fmt.Errorf("the update was undone: %w", cause)
}

func pendingIs(stateDir, version string) bool {
	var p Pending

	return readJSON(pendingPath(stateDir), &p) == nil && p.Version == version
}
