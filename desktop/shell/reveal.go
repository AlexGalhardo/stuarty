package shell

import (
	"errors"
	"os"
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
