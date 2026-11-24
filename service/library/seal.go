package library

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path"

	"github.com/AlexGalhardo/stuarty/service/account"
	"github.com/AlexGalhardo/stuarty/service/crashpoint"
)

// maxSealFile is the largest clear file the product seals in place; a larger one is no entry of the library (the
// limits of SDD-018 section 12 are far lower) and is left for the scan to list as invalid.
const maxSealFile = 64 << 20

// sealClearFile seals one clear file of the library: the sealed copy is written by an atomic rename, read back and
// compared with what the file held, and only then is the clear one gone (the rename replaces it). A sealed file, a
// link, a folder, a missing file and a folder with no account are left as they are. It reports whether it sealed.
func sealClearFile(root *vroot, rel string) (bool, error) {
	if !root.sealing() || root.ready() != nil {
		return false, nil
	}

	info, err := root.Lstat(rel)
	if err != nil {
		if isMissing(err) {
			return false, nil
		}

		return false, err
	}

	if !info.Mode().IsRegular() || info.Size() > maxSealFile {
		return false, nil
	}

	raw, err := root.ReadFile(rel)
	if err != nil {
		return false, err
	}

	if account.IsSealedFile(raw) {
		return false, nil
	}

	sealed, err := root.seal.SealFile(rel, raw)
	if err != nil {
		return false, err
	}

	if err := replaceFile(root, rel, sealed); err != nil {
		return false, err
	}

	stored, err := root.ReadFile(rel)
	if err != nil {
		return false, fmt.Errorf("library: %s did not read back after it was sealed: %w", printable(rel), err)
	}

	back, err := root.seal.OpenFile(rel, stored)
	if err != nil || !bytes.Equal(back, raw) {
		return false, fmt.Errorf("library: %s is not what it was after it was sealed", printable(rel))
	}

	// The file keeps the time it had: the date of a skill, or of a note that records none, is the date of its file.
	_ = root.Chtimes(rel, info.ModTime(), info.ModTime())

	// A kill here leaves some files sealed and some clear: every file is whole either way, and the next scan or unlock
	// seals the rest (SDD-005 section 5.17.4).
	crashpoint.Hit("library_seal.after_file")

	return true, nil
}

// sealClearTree seals every clear file under dir, a few at a time, and returns how many it sealed.
func sealClearTree(root *vroot, dir string) (int, error) {
	var files []string

	var walk func(rel string, depth int) error

	walk = func(rel string, depth int) error {
		if depth > maxWalkDepth {
			return errors.New("library: a folder is nested too deep to be sealed")
		}

		list, err := listDir(root, rel)
		if err != nil {
			return err
		}

		for _, e := range list {
			child := path.Join(rel, e.Name())

			switch {
			case e.IsDir():
				if err := walk(child, depth+1); err != nil {
					return err
				}
			case e.Type().IsRegular():
				files = append(files, child)
			}
		}

		return nil
	}

	if err := walk(dir, 0); err != nil {
		return 0, err
	}

	jobs, sealed := newPool(), counter{}

	for _, rel := range files {
		jobs.do(func() error {
			did, err := sealClearFile(root, rel)
			if did {
				sealed.add()
			}

			return err
		})
	}

	err := jobs.wait()

	return sealed.get(), err
}

// SealAll seals every clear file of library/, whatever it is (an entry, the review key, the record of the built-in
// skills, a file the scan does not index), and returns how many it sealed. It is the migration of a workspace that
// has files in clear text (SDD-005 section 5.17.4). With no account it does nothing, and while locked it answers
// account.ErrLocked.
func (m *Manager) SealAll(ctx context.Context) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	root, err := m.files.open()
	if err != nil {
		return 0, err
	}

	if root == nil {
		return 0, nil
	}

	defer func() { _ = root.Close() }()

	if err := root.ready(); err != nil {
		return 0, err
	}

	if err := ctx.Err(); err != nil {
		return 0, err
	}

	return sealClearTree(root, ".")
}
