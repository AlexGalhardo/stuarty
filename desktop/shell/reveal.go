package shell

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
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
func CheckRevealPath(path string) error {
	if strings.ContainsRune(path, 0) || !filepath.IsAbs(path) {
		return errors.New("the path is not absolute")
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

// MediaRoot is the media folder of a workspace: where the content store keeps the original files
// (engine/store, SDD-000 section 9). The app module cannot import engine/store (it would pull the renderer into the
// shell), so the folder name is here and a test keeps the two equal.
func MediaRoot(workspace string) string { return filepath.Join(workspace, "store") }

// CheckRevealUnder is CheckRevealPath for a file of the library (SDD-011 section 14.10, SDD-012 section 13.1): the
// path must also lie, with its symbolic links resolved, strictly inside root, the media folder of the workspace the
// shell started the server on. The shell asks it by itself and never trusts the page, which can send any text; a link
// it cannot resolve is refused rather than guessed. No message carries a path.
func CheckRevealUnder(root, path string) error {
	if err := CheckRevealPath(path); err != nil {
		return err
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return errors.New("the media folder of the workspace cannot be checked")
	}
	target, err := filepath.EvalSymlinks(path)
	if err != nil {
		return errors.New("the file is reached through a link that cannot be checked")
	}
	rel, err := filepath.Rel(realRoot, target)
	if err != nil || filepath.IsAbs(rel) || rel == "." || rel == ".." ||
		strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errors.New("the file is outside the media folder of the workspace")
	}
	return nil
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
