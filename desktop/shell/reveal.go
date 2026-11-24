package shell

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// RevealCommand is the file manager command that shows path in its folder with
// the file selected where the system can (Explorer, Finder); the Linux fallback
// opens the folder, because there is no common way to select a file. The path is
// passed as one argument, never through a shell.
func RevealCommand(goos, path string) (name string, args []string) {
	switch goos {
	case "windows":
		return "explorer.exe", []string{"/select," + path}
	case "darwin":
		return "open", []string{"-R", path}
	default:
		return "xdg-open", []string{filepath.Dir(path)}
	}
}

// CheckRevealPath accepts only an absolute path to an existing file: the page
// must not be able to make the shell open anything it cannot name in full.
//
// On Windows a network, device or extended path is refused from its text, before the disk is asked: a stat of
// \\host\share\... opens an SMB connection to that host and offers it the person's credentials, and the product
// never writes an export to such a path (service/pathguard refuses them).
func CheckRevealPath(path string) error {
	if strings.ContainsRune(path, 0) || !filepath.IsAbs(path) {
		return errors.New("the path is not absolute")
	}
	if runtime.GOOS == "windows" && remoteOrDevice(path) {
		return errors.New("a network or device path is not opened")
	}
	info, err := os.Stat(path)
	if err != nil {
		return errors.New("the file does not exist")
	}
	if !info.Mode().IsRegular() {
		return errors.New("the path is not a file")
	}
	return nil
}

// remoteOrDevice is whether a Windows path names another host or a device: \\host\share, \\?\, \\.\, \??\ and
// their forward-slash spellings, the prefixes service/pathguard refuses.
func remoteOrDevice(path string) bool {
	for _, p := range []string{`\\`, "//", `\??\`, `/\`, `\/`} {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// MediaRoot is the media folder of a workspace: where the content store keeps the original files
// (engine/store, SDD-000 section 9). The app module cannot import engine/store (it would pull the renderer into the
// shell), so the folder name is here and a test keeps the two equal.
func MediaRoot(workspace string) string { return filepath.Join(workspace, "store") }

// CheckRevealUnder is CheckRevealPath for a file of the library (SDD-011 section 14.10, SDD-012 section 13.1): the
// path must also lie, with its symbolic links resolved, strictly inside root, the media folder of the workspace the
// shell started the server on. The shell asks it by itself and never trusts the page, which can send any text; a link
// it cannot resolve is refused rather than guessed. No message carries a path.
//
// The text of the path is checked before the disk is touched: a page could otherwise make the shell stat a UNC
// path (\\host\share\...), which on Windows opens an SMB connection to that host and offers it the person's
// credentials. Only a path that already lies, as written, under the media folder (or under the folder it resolves
// to, which is what the server returns) is ever looked at; a device path (\\?\, \\.\) has another volume and is
// refused the same way.
func CheckRevealUnder(root, path string) error {
	if strings.ContainsRune(path, 0) || !filepath.IsAbs(path) {
		return errors.New("the path is not absolute")
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return errors.New("the media folder of the workspace cannot be checked")
	}
	outside := errors.New("the file is outside the media folder of the workspace")
	if clean := filepath.Clean(path); !inside(filepath.Clean(root), clean) && !inside(realRoot, clean) {
		return outside
	}
	if err := CheckRevealPath(path); err != nil {
		return err
	}
	target, err := filepath.EvalSymlinks(path)
	if err != nil {
		return errors.New("the file is reached through a link that cannot be checked")
	}
	if !inside(realRoot, target) {
		return outside
	}
	// The store names every original by its SHA-256 (store/<aa>/<64 hex>): any other name under the folder (a
	// partial import in .incoming, an alternate data stream, a file someone dropped there, perhaps a program) is
	// not a stored file, so the file manager is never pointed at it.
	if !storedName(filepath.Base(target)) || !storedName(filepath.Base(path)) {
		return errors.New("the file is not a stored file of the library")
	}
	return nil
}

// inside is whether target lies strictly inside root, by the relative path and never by a text prefix, so
// /ws/store-evil is not inside /ws/store. On Windows the volume and the names compare without case.
func inside(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	return err == nil && !filepath.IsAbs(rel) && rel != "." && rel != ".." &&
		!strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// storedName is whether name has the shape of a stored original: 64 lowercase hexadecimal characters, no
// extension and no stream.
func storedName(name string) bool {
	if len(name) != 64 {
		return false
	}
	for _, r := range name {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

// Revealer opens the file manager on a stored file of the library. Every member is replaceable so that a test runs
// it with a fake command and never opens the file manager of the person at the keyboard.
type Revealer struct {
	GOOS string
	// Root is the media folder of the workspace.
	Root string
	// LookPath finds the file manager; exec.LookPath when nil.
	LookPath func(name string) (string, error)
	// Start runs the file manager without waiting for it: Explorer exits with 1 even when it worked, so only a
	// failure to start counts.
	Start func(name string, args []string) error
}

// Reveal checks path against the media folder and starts the file manager on it. The error is the sentence the
// page shows.
func (r Revealer) Reveal(path string) error {
	if err := CheckRevealUnder(r.Root, path); err != nil {
		return err
	}
	name, args := RevealCommand(r.GOOS, path)
	look := r.LookPath
	if look == nil {
		look = exec.LookPath
	}
	bin, err := look(name)
	if err != nil {
		return errors.New("no file manager was found on this computer")
	}
	if err := r.Start(bin, args); err != nil {
		return errors.New("the file manager could not be started")
	}
	return nil
}
