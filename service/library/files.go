package library

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"

	core "github.com/AlexGalhardo/stuarty/core/library"
)

// The folders of library/ (SDD-018 section 2).
const (
	memoryDir = "memory"
	promptDir = "prompts"
	skillDir  = "skills"

	maxEntryFile  = 128 << 10 // a note or a prompt file, far above their body limits
	maxWalkDepth  = 8
	maxWalkFiles  = 400 // a folder with more is not read at all
	maxWalkBytes  = 8 << 20
	maxSearchText = 256 << 10 // the part of a skill that is searched
)

// files is the library folder of a workspace. Every access goes through os.Root,
// so no name and no link can lead out of it.
type files struct{ dir string }

// open returns the root of the library folder, or nil when it does not exist.
func (f files) open() (*os.Root, error) {
	r, err := os.OpenRoot(f.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil //nolint:nilnil // no library folder yet is an empty library
	}

	return r, err
}

// create returns the root of the library folder, making it first.
func (f files) create() (*os.Root, error) {
	if err := os.MkdirAll(f.dir, 0o700); err != nil {
		return nil, err
	}

	return os.OpenRoot(f.dir)
}

func entryPath(kind core.Kind, name string) string {
	switch kind {
	case core.KindMemory:
		return memoryDir + "/" + name + ".md"
	case core.KindPrompt:
		return promptDir + "/" + name + ".md"
	case core.KindSkill:
		return skillDir + "/" + name
	}

	return ""
}

// listDir returns the entries of a folder under root; a missing folder is empty.
func listDir(root *os.Root, dir string) ([]fs.DirEntry, error) {
	d, err := root.Open(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	defer func() { _ = d.Close() }()

	list, err := d.ReadDir(-1)
	if err != nil {
		return nil, err
	}

	sort.Slice(list, func(i, j int) bool { return list[i].Name() < list[j].Name() })

	return list, nil
}

// signature is what the scan compares: the number of files, their bytes and the
// latest modification time.
func signature(count int, bytes, mtime int64) string {
	return fmt.Sprintf("%d:%d:%d", count, bytes, mtime)
}

func fileSig(info fs.FileInfo) string {
	return signature(1, info.Size(), info.ModTime().UnixNano())
}

// readSmall reads a regular file under root, refusing a link and a file over the
// limit.
func readSmall(root *os.Root, rel string, limit int64) ([]byte, fs.FileInfo, error) {
	info, err := root.Lstat(rel)
	if err != nil {
		return nil, nil, err
	}

	if !info.Mode().IsRegular() {
		return nil, nil, errors.New("it is not a regular file")
	}

	if info.Size() > limit {
		return nil, nil, fmt.Errorf("the file is %d bytes; the limit is %d", info.Size(), limit)
	}

	f, err := root.Open(rel)
	if err != nil {
		return nil, nil, err
	}

	defer func() { _ = f.Close() }()

	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, nil, err
	}

	if int64(len(data)) > limit {
		return nil, nil, errors.New("the file grew while it was read")
	}

	return data, info, nil
}

// walkSkill walks the folder of a skill under root (the folder itself is dir).
// visit gets the slash path inside the folder and its info for every regular
// file; a link, a device or a folder too deep or too full makes the walk fail,
// so a skill is never read in part.
func walkSkill(root *os.Root, dir string, visit func(rel string, info fs.FileInfo) error) error {
	count, total := 0, int64(0)

	var walk func(rel string, depth int) error

	walk = func(rel string, depth int) error {
		if depth > maxWalkDepth {
			return errors.New("the folder is nested too deep")
		}

		list, err := listDir(root, path.Join(dir, rel))
		if err != nil {
			return err
		}

		for _, e := range list {
			child := path.Join(rel, e.Name())

			info, err := root.Lstat(path.Join(dir, child))
			if err != nil {
				return err
			}

			switch {
			case info.IsDir():
				if err := walk(child, depth+1); err != nil {
					return err
				}
			case info.Mode().IsRegular():
				count++

				total += info.Size()

				if count > maxWalkFiles || total > maxWalkBytes {
					return errors.New("the folder is far over the size limits of a skill")
				}

				if err := visit(child, info); err != nil {
					return err
				}
			default:
				return fmt.Errorf("%s is a link or a special file", child)
			}
		}

		return nil
	}

	return walk(".", 0)
}

// skillSignature is the signature of a skill folder, from stat calls only.
func skillSignature(root *os.Root, dir string) (string, error) {
	count, bytes, latest := 0, int64(0), int64(0)

	err := walkSkill(root, dir, func(_ string, info fs.FileInfo) error {
		count++
		bytes += info.Size()
		latest = max(latest, info.ModTime().UnixNano())

		return nil
	})
	if err != nil {
		return "", err
	}

	return signature(count, bytes, latest), nil
}

// readSkillFiles reads every file of a skill folder, with the signature.
func readSkillFiles(root *os.Root, dir string) ([]core.File, string, error) {
	var out []core.File

	count, bytes, latest := 0, int64(0), int64(0)

	err := walkSkill(root, dir, func(rel string, info fs.FileInfo) error {
		data, _, err := readSmall(root, path.Join(dir, rel), core.MaxSkillFile*2)
		if err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}

		out = append(out, core.File{Path: rel, Data: data})
		count++
		bytes += info.Size()
		latest = max(latest, info.ModTime().UnixNano())

		return nil
	})
	if err != nil {
		return nil, "", err
	}

	return out, signature(count, bytes, latest), nil
}

func randomSuffix() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)

	return hex.EncodeToString(b)
}

// writeFile writes a file by a temporary name and a rename, so a reader never
// sees half of it and a failure leaves the old file.
func writeFile(root *os.Root, rel string, data []byte) error {
	if err := root.MkdirAll(path.Dir(rel), 0o700); err != nil {
		return err
	}

	tmp := path.Join(path.Dir(rel), ".tmp-"+randomSuffix())
	if err := root.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}

	if err := root.Rename(tmp, rel); err != nil {
		_ = root.Remove(tmp)

		return err
	}

	return nil
}

// writeSkillDir writes the whole folder of a skill under skills/.
func writeSkillDir(root *os.Root, name string, all []core.File) error {
	return writeDirAtomic(root, skillDir, name, all)
}

// writeDirAtomic writes the whole folder parent/name under a temporary name and
// renames it into place, keeping the old folder until the new one is there. A
// failure leaves the old folder.
func writeDirAtomic(root *os.Root, parent, name string, all []core.File) error {
	if err := root.MkdirAll(parent, 0o700); err != nil {
		return err
	}

	tmp := path.Join(parent, ".tmp-"+randomSuffix())
	if err := root.Mkdir(tmp, 0o700); err != nil {
		return err
	}

	cleanup := func() { _ = root.RemoveAll(tmp) }

	for _, f := range all {
		if err := core.ValidFilePath(f.Path); err != nil {
			cleanup()

			return invalid("path", "%q: %v", f.Path, err)
		}

		if err := writeFile(root, path.Join(tmp, f.Path), f.Data); err != nil {
			cleanup()

			return err
		}
	}

	final := path.Join(parent, name)
	old := ""

	if _, err := root.Lstat(final); err == nil {
		old = path.Join(parent, ".old-"+randomSuffix())
		if err := root.Rename(final, old); err != nil {
			cleanup()

			return err
		}
	}

	if err := root.Rename(tmp, final); err != nil {
		if old != "" {
			_ = root.Rename(old, final)
		}

		cleanup()

		return err
	}

	if old != "" {
		_ = root.RemoveAll(old)
	}

	return nil
}

// stem returns the name of a file ending in .md, and false for anything else.
func stem(file string) (string, bool) {
	if strings.HasPrefix(file, ".") || !strings.HasSuffix(file, ".md") {
		return "", false
	}

	return strings.TrimSuffix(file, ".md"), true
}
