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
	"github.com/AlexGalhardo/stuarty/service/account"
)

// sealOverhead is what the envelope adds to a file: the header, the nonce and the tag. The size limits of the files
// are limits of their content.
const sealOverhead = 5 + 12 + 16

// errSealedFile is a sealed file that does not open: edited, cut or moved outside the product (SDD-018 section 2).
var errSealedFile = errors.New("the file is sealed and does not open with the key of this account: " +
	"it was edited, cut or moved outside the product")

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
// so no name and no link can lead out of it. seal says how the files are stored: sealed with an account
// (ADR-0053), clear text with none; nil is clear text.
type files struct {
	dir  string
	seal account.FileSealer
}

// vroot is an open library folder with the way its files are stored. Reads open a sealed file and also read a clear
// one; writes seal while an account exists.
type vroot struct {
	*os.Root
	seal account.FileSealer
}

// plainRoot is a folder whose files are never sealed: a folder a person chose to import from, the exports.
func plainRoot(r *os.Root) *vroot { return &vroot{Root: r} }

func (r *vroot) sealing() bool { return r.seal != nil && r.seal.Sealing() }

// ready is ErrLocked while the studio is locked and the folder holds sealed files.
func (r *vroot) ready() error {
	if r.seal == nil {
		return nil
	}

	return r.seal.Ready()
}

// ready is nil when the files can be read and written now.
func (f files) ready() error {
	if f.seal == nil {
		return nil
	}

	return f.seal.Ready()
}

// open returns the root of the library folder, or nil when it does not exist.
func (f files) open() (*vroot, error) {
	r, err := os.OpenRoot(f.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil //nolint:nilnil // no library folder yet is an empty library
	}

	if err != nil {
		return nil, err
	}

	return &vroot{Root: r, seal: f.seal}, nil
}

// create returns the root of the library folder, making it first.
func (f files) create() (*vroot, error) {
	if err := os.MkdirAll(f.dir, 0o700); err != nil {
		return nil, err
	}

	r, err := os.OpenRoot(f.dir)
	if err != nil {
		return nil, err
	}

	return &vroot{Root: r, seal: f.seal}, nil
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
func listDir(root *vroot, dir string) ([]fs.DirEntry, error) {
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
func readSmall(root *vroot, rel string, limit int64) ([]byte, fs.FileInfo, error) {
	data, info, _, err := readFile(root, rel, limit)

	return data, info, err
}

// readFile is readSmall that also says whether the file was clear text where the folder seals: the scan seals
// such a file. A sealed file is opened; one that does not open is errSealedFile. The limit is on the content.
func readFile(root *vroot, rel string, limit int64) (data []byte, info fs.FileInfo, clear bool, err error) {
	if err := root.ready(); err != nil {
		return nil, nil, false, err
	}

	info, err = root.Lstat(rel)
	if err != nil {
		return nil, nil, false, err
	}

	if !info.Mode().IsRegular() {
		return nil, nil, false, errors.New("it is not a regular file")
	}

	room := limit
	if root.seal != nil {
		room += sealOverhead
	}

	if info.Size() > room {
		return nil, nil, false, fmt.Errorf("the file is %d bytes; the limit is %d", info.Size(), limit)
	}

	f, err := root.Open(rel)
	if err != nil {
		return nil, nil, false, err
	}

	defer func() { _ = f.Close() }()

	data, err = io.ReadAll(io.LimitReader(f, room+1))
	if err != nil {
		return nil, nil, false, err
	}

	if int64(len(data)) > room {
		return nil, nil, false, errors.New("the file grew while it was read")
	}

	if root.seal != nil && account.IsSealedFile(data) {
		if data, err = root.seal.OpenFile(rel, data); err != nil {
			if errors.Is(err, account.ErrLocked) {
				return nil, nil, false, err
			}

			return nil, nil, false, errSealedFile
		}
	} else if root.sealing() {
		clear = true
	}

	if int64(len(data)) > limit {
		return nil, nil, false, fmt.Errorf("the file is %d bytes; the limit is %d", len(data), limit)
	}

	return data, info, clear, nil
}

// walkSkill walks the folder of a skill under root (the folder itself is dir).
// visit gets the slash path inside the folder and its info for every regular
// file; a link, a device or a folder too deep or too full makes the walk fail,
// so a skill is never read in part.
func walkSkill(root *vroot, dir string, visit func(rel string, info fs.FileInfo) error) error {
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
func skillSignature(root *vroot, dir string) (string, error) {
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
func readSkillFiles(root *vroot, dir string) ([]core.File, string, error) {
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
func writeFile(root *vroot, rel string, data []byte) error { return writeFileAs(root, rel, rel, data) }

// writeFileAs writes the file at rel as the file named final, which is what a sealed file is bound to: a folder is
// written under a temporary name and renamed, so its files are sealed for the path they will have.
func writeFileAs(root *vroot, rel, final string, data []byte) error {
	if err := root.ready(); err != nil {
		return err
	}

	if root.sealing() {
		sealed, err := root.seal.SealFile(final, data)
		if err != nil {
			return err
		}

		data = sealed
	}

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
func writeSkillDir(root *vroot, name string, all []core.File) error {
	return writeDirAtomic(root, skillDir, name, all)
}

// writeDirAtomic writes the whole folder parent/name under a temporary name and
// renames it into place, keeping the old folder until the new one is there. A
// failure leaves the old folder.
func writeDirAtomic(root *vroot, parent, name string, all []core.File) error {
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

		if err := writeFileAs(root, path.Join(tmp, f.Path), path.Join(parent, name, f.Path), f.Data); err != nil {
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
