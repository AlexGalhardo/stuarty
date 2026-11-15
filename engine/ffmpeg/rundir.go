package ffmpeg

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrSymlinkInStore is returned for a link inside store/: the store holds only regular files that
// this product wrote (SDD-007 6.4).
var ErrSymlinkInStore = errors.New("ffmpeg: a symbolic link inside the store")

// canonicalDir resolves the links of a workspace subdirectory once, at start. An operator may point
// store/, tmp/ or exports/ at another disk; from then on the resolved path is the one in use, so
// nothing is resolved again while a run is going. A directory that does not exist yet is returned
// as it is.
func canonicalDir(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if errors.Is(err, os.ErrNotExist) {
		return path, nil
	}

	if err != nil {
		return "", fmt.Errorf("ffmpeg: resolving %s: %w", path, err)
	}

	return resolved, nil
}

// OpenStoreFile opens a file of the store, named relative to it, through os.Root, so a name cannot
// leave the store, and refuses a link anywhere in the path of the file.
func (r *Runner) OpenStoreFile(name string) (*os.File, error) {
	root, err := os.OpenRoot(r.StoreDir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()

	fi, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}

	if fi.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%w: %s", ErrSymlinkInStore, filepath.Base(name))
	}

	return root.Open(name)
}
