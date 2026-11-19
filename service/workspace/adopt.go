package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The workspace folder of the product before the rename to Stuarty (ADR-0044, SDD-005 section 6.6).
const (
	formerDir = ".vukem-video"

	// AdoptionRecord is the file in the adopted workspace that says where it came from and how.
	AdoptionRecord = "adopted.json"
	// MovedMarker is left in the former folder when the workspace was copied instead of renamed.
	MovedMarker = "MOVED-TO-STUARTY.txt"

	adoptingSuffix = ".adopting"
)

// Adoption methods.
const (
	AdoptedByRename = "rename"
	AdoptedByCopy   = "copy"
)

// ErrFormerInUse is returned when a process still owns the former workspace; it is closed first.
var ErrFormerInUse = errors.New("workspace: a process still owns the workspace of the former name")

// ErrAdoptionRefused wraps the reason a Check refused to adopt the former workspace.
var ErrAdoptionRefused = errors.New("workspace: the workspace of the former name cannot be adopted")

// AdoptOptions are the inputs of AdoptFormer. Only Former and Current are required.
type AdoptOptions struct {
	// Former and Current are the absolute paths of the former and of the new default workspace.
	Former, Current string
	// Rename moves a folder; os.Rename when nil. A test replaces it to simulate another volume.
	Rename func(oldPath, newPath string) error
	// Now gives the time of the backup and of the record; time.Now when nil.
	Now func() time.Time
	// Check, when set, may refuse the former workspace before anything is written (for example because a
	// newer version wrote it); its error is returned wrapped in ErrAdoptionRefused.
	Check func(former string) error

	// afterCopy runs on the staging folder of a copy before it is verified; a test uses it to damage the copy.
	afterCopy func(staging string) error
}

// Adoption is what AdoptFormer did.
type Adoption struct {
	Method string // AdoptedByRename or AdoptedByCopy
	Backup string // the folder inside the adopted workspace that holds the backup of the database
	Former string
	Dir    Dir
}

// AdoptionRecordFile is the content of AdoptionRecord.
type AdoptionRecordFile struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Method    string `json:"method"`
	Backup    string `json:"backup"`
	AdoptedAt string `json:"adopted_at"`
}

// FormerExists reports whether a workspace of the former name is waiting to be adopted next to current:
// the former folder exists, has not been moved already and current does not exist.
type formerState int

const (
	formerAbsent formerState = iota
	formerWaiting
	formerAndCurrent
)

func stateOf(former, current string) (formerState, error) {
	fi, err := os.Stat(former)
	if errors.Is(err, fs.ErrNotExist) {
		return formerAbsent, nil
	}

	if err != nil {
		return formerAbsent, fmt.Errorf("workspace: %w", err)
	}

	if !fi.IsDir() {
		return formerAbsent, nil
	}

	if _, err := os.Stat(filepath.Join(former, MovedMarker)); err == nil {
		return formerAbsent, nil
	}

	if _, err := os.Lstat(current); err == nil {
		return formerAndCurrent, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return formerAbsent, fmt.Errorf("workspace: %w", err)
	}

	return formerWaiting, nil
}

// HasFormerAlongside reports that both the former folder and the new one exist and the former one holds a
// workspace that was not moved: the new one is used and the former one is left alone.
func HasFormerAlongside(former, current string) bool {
	s, err := stateOf(former, current)

	return err == nil && s == formerAndCurrent
}

// AdoptFormer adopts the workspace of the former name as the new one. Nothing happens, and (nil, nil) is
// returned, when the former folder is absent, was moved already, or the new folder exists (then the new one
// is used and the former one is left as it is).
//
// The steps, in this order, each of which leaves the former folder complete if the next one fails:
//
//  1. the lock of the former workspace is taken to be sure no process owns it, then released;
//  2. Check may refuse it;
//  3. the database is copied to backups/adopt-<UTC time>/ inside the former folder;
//  4. the folder is renamed to the new name (the same volume); when the rename crosses volumes it is
//     copied to <new>.adopting, verified file by file (size, and SHA-256 for the database), renamed into
//     place and MovedMarker is left in the former folder, whose files stay;
//  5. AdoptionRecord is written in the adopted workspace.
//
// The former folder is never deleted.
func AdoptFormer(opts AdoptOptions) (*Adoption, error) {
	if opts.Rename == nil {
		opts.Rename = os.Rename
	}

	if opts.Now == nil {
		opts.Now = time.Now
	}

	state, err := stateOf(opts.Former, opts.Current)
	if err != nil || state != formerWaiting {
		return nil, err
	}

	if err := ensureUnowned(Dir(opts.Former)); err != nil {
		return nil, err
	}

	if opts.Check != nil {
		if err := opts.Check(opts.Former); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrAdoptionRefused, err)
		}
	}

	now := opts.Now().UTC()

	backup, err := backupDatabase(opts.Former, now)
	if err != nil {
		return nil, err
	}

	a := &Adoption{Method: AdoptedByRename, Former: opts.Former, Dir: Dir(opts.Current)}
	if backup != "" {
		rel, _ := filepath.Rel(opts.Former, backup)
		a.Backup = filepath.Join(opts.Current, rel)
	}

	if err := opts.Rename(opts.Former, opts.Current); err != nil {
		if !isCrossDevice(err) {
			return nil, fmt.Errorf("workspace: adopt %s: %w", opts.Former, err)
		}

		a.Method = AdoptedByCopy

		if err := copyAndVerify(opts.Former, opts.Current, opts.afterCopy); err != nil {
			return nil, err
		}

		// The verified copy is in place and is the workspace now: a failure from here on is reported
		// together with the adoption, never as "nothing was moved".
		if err := writeMovedMarker(opts.Former, opts.Current, now); err != nil {
			return a, err
		}
	}

	if err := writeRecord(a, now); err != nil {
		return a, err
	}

	return a, nil
}

// Notice is the one line printed the first time the former workspace is adopted.
func (a *Adoption) Notice() string {
	how := "moved"
	if a.Method == AdoptedByCopy {
		how = "copied (the former folder is left as it was, with a note in it)"
	}

	backup := "there was no database to back up"
	if a.Backup != "" {
		backup = "a backup of its database is in " + a.Backup
	}

	return fmt.Sprintf("stuarty: the workspace of Vukem Video, %s, was %s to %s; %s.\n", a.Former, how, a.Dir, backup)
}

func ensureUnowned(d Dir) error {
	if _, err := os.Stat(d.Path("lock")); err != nil {
		return nil //nolint:nilerr // no lock file: no process ever owned it
	}

	l, err := d.TryLock()
	if errors.Is(err, ErrLocked) {
		// An *Error, so that every surface answers E_WORKSPACE_LOCKED with this request instead of
		// E_INTERNAL; the message names no folder.
		return &Error{
			Code: CodeWorkspaceLocked,
			Message: "Vukem Video is still running and owns the workspace that Stuarty adopts at its first start: " +
				"close Vukem Video (or stop its server) and start again",
			Retryable: true,
			Err:       ErrFormerInUse,
		}
	}

	if err != nil {
		return fmt.Errorf("workspace: %w", err)
	}

	return l.Release()
}

// backupDatabase copies db.sqlite and its write-ahead files into backups/adopt-<UTC> of the former folder
// and returns that folder. A workspace with no database has nothing to back up and no folder is made.
func backupDatabase(former string, now time.Time) (string, error) {
	dest := filepath.Join(former, "backups", "adopt-"+now.Format("20060102T150405Z"))

	found := false

	for _, name := range []string{"db.sqlite", "db.sqlite-wal", "db.sqlite-shm"} {
		src := filepath.Join(former, name)
		if _, err := os.Stat(src); err != nil {
			continue
		}

		if err := os.MkdirAll(dest, 0o700); err != nil {
			return "", fmt.Errorf("workspace: backup: %w", err)
		}

		if err := copyFile(src, filepath.Join(dest, name)); err != nil {
			return "", fmt.Errorf("workspace: backup of %s: %w", name, err)
		}

		found = true
	}

	if !found {
		return "", nil
	}

	return dest, nil
}

// copyAndVerify copies the former folder to <current>.adopting, checks it and renames it to current.
func copyAndVerify(former, current string, afterCopy func(staging string) error) (err error) {
	staging := current + adoptingSuffix

	if err := os.RemoveAll(staging); err != nil {
		return fmt.Errorf("workspace: %w", err)
	}

	defer func() {
		if err != nil {
			_ = os.RemoveAll(staging)
		}
	}()

	if err := copyTree(former, staging); err != nil {
		return err
	}

	if afterCopy != nil {
		if err := afterCopy(staging); err != nil {
			return err
		}
	}

	if err := verifyTree(former, staging); err != nil {
		return err
	}

	// The staging folder and the new folder share a volume, so this rename does not cross one.
	if err := os.Rename(staging, current); err != nil {
		return fmt.Errorf("workspace: %w", err)
	}

	return nil
}

// runtimeFiles are not copied: they describe a running process and are stale in a folder that is adopted.
var runtimeFiles = map[string]bool{"runtime.json": true, "runtime.secret": true}

func copyTree(from, to string) error {
	return filepath.WalkDir(from, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}

		target := filepath.Join(to, rel)

		switch {
		case d.IsDir():
			return os.MkdirAll(target, 0o700)
		case runtimeFiles[rel]:
			return nil
		case d.Type().IsRegular():
			return copyFile(path, target)
		default:
			return fmt.Errorf(
				"workspace: %s is not a regular file or a folder: it is not copied, adopt it by hand",
				path,
			)
		}
	})
}

func verifyTree(from, to string) error {
	return filepath.WalkDir(from, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}

		if rel == "." || runtimeFiles[rel] {
			return nil
		}

		got, err := os.Lstat(filepath.Join(to, rel))
		if err != nil {
			return fmt.Errorf("workspace: the copy lacks %s: %w", rel, err)
		}

		if d.IsDir() {
			if !got.IsDir() {
				return fmt.Errorf("workspace: %s was copied as a file", rel)
			}

			return nil
		}

		want, err := d.Info()
		if err != nil {
			return err
		}

		if got.Size() != want.Size() {
			return fmt.Errorf("workspace: the copy of %s has %d bytes, the original %d", rel, got.Size(), want.Size())
		}

		if strings.HasPrefix(rel, "db.sqlite") {
			a, err := sha256File(path)
			if err != nil {
				return err
			}

			b, err := sha256File(filepath.Join(to, rel))
			if err != nil {
				return err
			}

			if a != b {
				return fmt.Errorf("workspace: the copy of %s differs from the original", rel)
			}
		}

		return nil
	})
}

func copyFile(from, to string) (err error) {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(to), 0o700); err != nil {
		return err
	}

	out, err := os.OpenFile(to, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}

	defer func() {
		if cerr := out.Close(); err == nil {
			err = cerr
		}
	}()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}

	return out.Sync()
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

func writeMovedMarker(former, current string, now time.Time) error {
	text := fmt.Sprintf("Stuarty copied this workspace to %s on %s.\n"+
		"The copy is the one in use. The files here were not changed and not deleted; delete this folder\n"+
		"yourself once you are sure the copy is complete.\n", current, now.Format(time.RFC3339))

	if err := os.WriteFile(filepath.Join(former, MovedMarker), []byte(text), 0o600); err != nil {
		return fmt.Errorf("workspace: %w", err)
	}

	return nil
}

func writeRecord(a *Adoption, now time.Time) error {
	data, err := json.MarshalIndent(AdoptionRecordFile{
		From: a.Former, To: string(a.Dir), Method: a.Method, Backup: a.Backup, AdoptedAt: now.Format(time.RFC3339),
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("workspace: %w", err)
	}

	if err := os.WriteFile(a.Dir.Path(AdoptionRecord), append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("workspace: %w", err)
	}

	return nil
}

// adoptFormer is what Resolve does at the default location: adopt the former workspace when one waits,
// and say so on Notice; when both folders hold a workspace, use the new one and name the former one.
func adoptFormer(former, current string) error {
	a, err := AdoptFormer(AdoptOptions{Former: former, Current: current})
	if err != nil {
		// Already an *Error (the former workspace is in use: E_WORKSPACE_LOCKED): keep its code.
		if _, ok := errors.AsType[*Error](err); ok {
			return err
		}

		// The rename or the copy is done and only the note in the former folder or the record failed:
		// the workspace is adopted.
		if a != nil {
			return &Error{
				Code: CodeInvalidState,
				Message: fmt.Sprintf(
					"the workspace of Vukem Video was adopted into %s but its note or its record "+
						"could not be written: %s",
					a.Dir, strings.TrimPrefix(err.Error(), "workspace: "),
				),
				Err: err,
			}
		}

		// Any other failure answers E_INVALID_STATE with its reason, so that every surface shows it.
		return &Error{
			Code: CodeInvalidState,
			Message: fmt.Sprintf(
				"the workspace of Vukem Video cannot be adopted: %s. Nothing of it was moved or deleted; "+
					"at most a backup of its database was written inside it. Fix the cause and start again, "+
					"or set %s to choose another folder",
				strings.TrimPrefix(err.Error(), "workspace: "), EnvHome,
			),
			Err: err,
		}
	}

	if a != nil {
		_, _ = io.WriteString(Notice, a.Notice())

		return nil
	}

	if HasFormerAlongside(former, current) {
		if _, err := os.Stat(filepath.Join(former, "db.sqlite")); err == nil {
			_, _ = fmt.Fprintf(
				Notice,
				"stuarty: a workspace of Vukem Video is in %s and was not adopted because %s exists; "+
					"to use the old one, set %s=%s\n",
				former,
				current,
				EnvHome,
				former,
			)
		}
	}

	return nil
}
