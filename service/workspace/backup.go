package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// The format of a workspace backup (SDD-009 section 21.4).
const (
	BackupFormat   = "stuarty-backup"
	BackupVersion  = 1
	ManifestFile   = "manifest.json"
	SumsFile       = "SHA256SUMS"
	backupDBFile   = "db.sqlite"
	backupVaultRel = "vault.json"
)

// backupTrees are the directories of the workspace that hold data a backup keeps. Everything else
// (artifacts, frames, bitmaps, exports, tmp, logs, tools, chat, publish, backups) is rebuilt or is not data.
var backupTrees = []string{"store", "library", "fonts", "providers"}

// ErrBackupInvalid is a backup that cannot be trusted: a missing manifest, a file that is not in it,
// a path outside the allowed trees, or a checksum that does not match.
var ErrBackupInvalid = errors.New("workspace: the backup is not valid")

// ErrNotEmpty is a restore into a workspace that already has data.
var ErrNotEmpty = errors.New("workspace: the workspace already has data")

// ErrVaultKey is a restore whose master key does not open the vault of the backup.
var ErrVaultKey = errors.New("workspace: the master key does not open the vault of this backup")

// BackupFileEntry is one file of a backup.
type BackupFileEntry struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// Manifest is manifest.json of a backup. MasterKeyIncluded is always false: a backup never holds the
// master key (ADR-0033).
type Manifest struct {
	Format            string            `json:"format"`
	Version           int               `json:"version"`
	StuartyVersion    string            `json:"stuarty_version"`
	CreatedAt         string            `json:"created_at"`
	MasterKeyIncluded bool              `json:"master_key_included"`
	Files             []BackupFileEntry `json:"files"`
}

// BackupOptions configures Backup.
type BackupOptions struct {
	Dir     Dir    // the workspace; it is not locked, a server may be running
	Out     string // the folder to write: it must not exist or be empty
	Version string // the version of Stuarty that makes the backup
	Now     func() time.Time
	// Snapshot writes a consistent copy of the database at dbPath to dest (store.SnapshotDatabase).
	Snapshot func(ctx context.Context, dbPath, dest string) error
}

// Backup writes a consistent copy of the data of a workspace to o.Out and returns its manifest. The
// database goes first, as a snapshot made through the database itself, then the files, so every file
// the snapshot refers to is there. Files that never change once written (the originals under store/)
// are hard linked when the file system allows it, and copied otherwise.
func Backup(ctx context.Context, o BackupOptions) (*Manifest, error) {
	if o.Now == nil {
		o.Now = time.Now
	}

	if o.Snapshot == nil {
		return nil, errors.New("workspace: backup needs a database snapshot function")
	}

	if err := o.prepareOut(); err != nil {
		return nil, err
	}

	var entries []BackupFileEntry

	db := filepath.Join(o.Out, backupDBFile)
	if err := o.Snapshot(ctx, o.Dir.Path(backupDBFile), db); err != nil {
		return nil, err
	}

	e, err := hashFile(db, backupDBFile)
	if err != nil {
		return nil, err
	}

	entries = append(entries, e)

	if src := o.Dir.Path(backupVaultRel); fileExists(src) {
		e, err := copyHashed(src, filepath.Join(o.Out, backupVaultRel), backupVaultRel)
		if err != nil {
			return nil, err
		}

		entries = append(entries, e)
	}

	for _, tree := range backupTrees {
		got, err := backupTree(ctx, o.Dir.Path(tree), filepath.Join(o.Out, tree), tree, tree == "store")
		if err != nil {
			return nil, err
		}

		entries = append(entries, got...)
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })

	m := &Manifest{
		Format: BackupFormat, Version: BackupVersion, StuartyVersion: o.Version,
		CreatedAt: o.Now().UTC().Format(time.RFC3339), Files: entries,
	}

	if err := writeManifest(o.Out, m); err != nil {
		return nil, err
	}

	return m, nil
}

func (o BackupOptions) prepareOut() error {
	if o.Out == "" {
		return errors.New("workspace: backup needs a folder to write to")
	}

	if rel, err := filepath.Rel(
		string(o.Dir),
		o.Out,
	); err == nil && rel != ".." &&
		!strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		for _, tree := range backupTrees {
			if rel == tree || strings.HasPrefix(rel, tree+string(filepath.Separator)) {
				return fmt.Errorf("workspace: the backup folder %s is inside %s/ of the workspace", o.Out, tree)
			}
		}
	}

	if left, err := os.ReadDir(o.Out); err == nil && len(left) > 0 {
		return fmt.Errorf("workspace: the backup folder %s is not empty", o.Out)
	}

	return os.MkdirAll(o.Out, 0o700)
}

func writeManifest(out string, m *Manifest) error {
	raw, err := json.MarshalIndent(m, "", "\t")
	if err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(out, ManifestFile), append(raw, '\n'), 0o600); err != nil {
		return err
	}

	var sums strings.Builder
	for _, f := range m.Files {
		fmt.Fprintf(&sums, "%s  %s\n", f.SHA256, f.Path)
	}

	return os.WriteFile(filepath.Join(out, SumsFile), []byte(sums.String()), 0o600)
}

// backupTree copies the regular files under src to dst (symbolic links and the in-progress folder of the
// store are skipped). With link set a file is hard linked when it can be.
func backupTree(ctx context.Context, src, dst, rel string, link bool) ([]BackupFileEntry, error) {
	var out []BackupFileEntry

	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) && p == src {
				return fs.SkipAll
			}

			return err
		}

		if err := ctx.Err(); err != nil {
			return err
		}

		sub, _ := filepath.Rel(src, p)
		if d.IsDir() {
			if d.Name() == ".incoming" {
				return filepath.SkipDir
			}

			return os.MkdirAll(filepath.Join(dst, sub), 0o700)
		}

		if !d.Type().IsRegular() {
			return nil
		}

		name := path.Join(rel, filepath.ToSlash(sub))
		target := filepath.Join(dst, sub)

		var e BackupFileEntry

		// The workspace is the owner's own folder, not a place an attacker writes to.
		if link && os.Link(p, target) == nil { //nolint:gosec // G122: see above
			e, err = hashFile(target, name)
		} else {
			e, err = copyHashed(p, target, name)
		}

		if err != nil {
			return err
		}

		out = append(out, e)

		return nil
	})

	return out, err
}

func fileExists(p string) bool {
	info, err := os.Stat(p)

	return err == nil && info.Mode().IsRegular()
}

func hashFile(p, name string) (BackupFileEntry, error) {
	f, err := os.Open(p)
	if err != nil {
		return BackupFileEntry{}, err
	}

	defer func() { _ = f.Close() }()

	h := sha256.New()

	n, err := io.Copy(h, f)
	if err != nil {
		return BackupFileEntry{}, err
	}

	return BackupFileEntry{Path: name, Size: n, SHA256: hex.EncodeToString(h.Sum(nil))}, nil
}

// copyHashed copies src to dst, mode 0600, and hashes what it wrote in the same pass, so the checksum is
// the one of the bytes of the copy even when the source changes meanwhile.
func copyHashed(src, dst, name string) (BackupFileEntry, error) {
	in, err := os.Open(src)
	if err != nil {
		return BackupFileEntry{}, err
	}

	defer func() { _ = in.Close() }()

	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return BackupFileEntry{}, err
	}

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return BackupFileEntry{}, err
	}

	h := sha256.New()

	n, err := io.Copy(io.MultiWriter(out, h), in)
	if err != nil {
		_ = out.Close()

		return BackupFileEntry{}, err
	}

	if err := out.Sync(); err != nil {
		_ = out.Close()

		return BackupFileEntry{}, err
	}

	if err := out.Close(); err != nil {
		return BackupFileEntry{}, err
	}

	return BackupFileEntry{Path: name, Size: n, SHA256: hex.EncodeToString(h.Sum(nil))}, nil
}

// ReadManifest reads and checks manifest.json of a backup folder: the format, the version and that every
// path is a relative path inside one of the allowed trees.
func ReadManifest(from string) (*Manifest, error) {
	raw, err := os.ReadFile(filepath.Join(from, ManifestFile))
	if err != nil {
		return nil, fmt.Errorf("%w: %s has no %s", ErrBackupInvalid, from, ManifestFile)
	}

	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("%w: %s is not readable: %w", ErrBackupInvalid, ManifestFile, err)
	}

	if m.Format != BackupFormat {
		return nil, fmt.Errorf("%w: %s is not a Stuarty backup", ErrBackupInvalid, from)
	}

	if m.Version != BackupVersion {
		return nil, fmt.Errorf(
			"%w: the backup is version %d and this build reads version %d",
			ErrBackupInvalid,
			m.Version,
			BackupVersion,
		)
	}

	if m.MasterKeyIncluded {
		return nil, fmt.Errorf("%w: a backup never holds the master key", ErrBackupInvalid)
	}

	for _, f := range m.Files {
		if !allowedBackupPath(f.Path) {
			return nil, fmt.Errorf(
				"%w: %s lists the path %q, which a backup never holds",
				ErrBackupInvalid,
				ManifestFile,
				f.Path,
			)
		}
	}

	return &m, nil
}

func allowedBackupPath(p string) bool {
	if p == "" || path.IsAbs(p) || p != path.Clean(p) || strings.ContainsAny(p, "\\:") {
		return false
	}

	if p == backupDBFile || p == backupVaultRel {
		return true
	}

	for _, tree := range backupTrees {
		if strings.HasPrefix(p, tree+"/") && !strings.Contains(p, "..") {
			return true
		}
	}

	return false
}

// VerifyBackup checks every file of a backup folder against its manifest: present, the size and the
// SHA-256 as listed, and no file that the manifest does not list.
func VerifyBackup(from string) (*Manifest, error) {
	m, err := ReadManifest(from)
	if err != nil {
		return nil, err
	}

	if err := verifyFiles(from, m); err != nil {
		return nil, err
	}

	listed := map[string]bool{ManifestFile: true, SumsFile: true}
	for _, f := range m.Files {
		listed[f.Path] = true
	}

	err = filepath.WalkDir(from, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		rel, _ := filepath.Rel(from, p)

		// A backup holds regular files only. A symbolic link (or a device, a pipe) would be followed
		// when hashed and linked as a link into the workspace, pointing anywhere.
		if !d.Type().IsRegular() {
			return fmt.Errorf(
				"%w: %s is not a regular file (a link or a special file)",
				ErrBackupInvalid,
				filepath.ToSlash(rel),
			)
		}

		if !listed[filepath.ToSlash(rel)] {
			return fmt.Errorf(
				"%w: %s is in the backup but not in its manifest",
				ErrBackupInvalid,
				filepath.ToSlash(rel),
			)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return m, nil
}

func verifyFiles(root string, m *Manifest) error {
	for _, f := range m.Files {
		got, err := hashFile(filepath.Join(root, filepath.FromSlash(f.Path)), f.Path)
		if err != nil {
			return fmt.Errorf("%w: %s cannot be read: %w", ErrBackupInvalid, f.Path, err)
		}

		if got.Size != f.Size || got.SHA256 != f.SHA256 {
			return fmt.Errorf("%w: the checksum of %s does not match the manifest", ErrBackupInvalid, f.Path)
		}
	}

	return nil
}

// RestoreOptions configures Restore.
type RestoreOptions struct {
	Dir          Dir    // the workspace; the caller holds its lock
	From         string // a backup folder
	Replace      bool   // move what the workspace holds to replaced-<time>/ instead of refusing
	WithoutVault bool   // leave the vault out: the accounts are connected again
	Now          func() time.Time
	// VerifyVault opens the vault file at the path with the master key at hand and reports whether
	// it can be read (the key is not in the backup). It is required when the backup has a vault and
	// WithoutVault is false.
	VerifyVault func(vaultPath string) error
}

// RestoreResult is what Restore did.
type RestoreResult struct {
	Manifest   *Manifest
	ReplacedTo string // the folder the old data was moved to, empty when there was none
	VaultDrop  bool   // the vault of the backup was left out
}

// Restore puts a backup into a workspace. In this order: every file of the backup is checked against its
// manifest; the vault of the backup is opened with the key at hand and a key that does not open it
// refuses the restore before anything is written; a workspace that has data is refused, or with
// Replace moved aside, never deleted; the files are written; the written files are checked again.
func Restore(ctx context.Context, o RestoreOptions) (*RestoreResult, error) {
	if o.Now == nil {
		o.Now = time.Now
	}

	m, err := VerifyBackup(o.From)
	if err != nil {
		return nil, err
	}

	res := &RestoreResult{Manifest: m}

	hasVault := false

	for _, f := range m.Files {
		hasVault = hasVault || f.Path == backupVaultRel
	}

	if hasVault && o.WithoutVault {
		res.VaultDrop = true
	} else if hasVault {
		if o.VerifyVault == nil {
			return nil, ErrVaultKey
		}

		if err := o.VerifyVault(filepath.Join(o.From, backupVaultRel)); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrVaultKey, err)
		}
	}

	moved, err := o.clearWorkspace()
	if err != nil {
		return nil, err
	}

	res.ReplacedTo = moved

	written := make([]BackupFileEntry, 0, len(m.Files))

	for _, f := range m.Files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		if f.Path == backupVaultRel && res.VaultDrop {
			continue
		}

		src := filepath.Join(o.From, filepath.FromSlash(f.Path))
		dst := o.Dir.Path(filepath.FromSlash(f.Path))

		if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
			return nil, err
		}

		if strings.HasPrefix(f.Path, "store/") && os.Link(src, dst) == nil {
			written = append(written, f)

			continue
		}

		if _, err := copyHashed(src, dst, f.Path); err != nil {
			return nil, fmt.Errorf("workspace: restore %s: %w", f.Path, err)
		}

		written = append(written, f)
	}

	if err := verifyFiles(string(o.Dir), &Manifest{Files: written}); err != nil {
		return nil, fmt.Errorf("workspace: the restored files do not match the manifest: %w", err)
	}

	return res, nil
}

// workspaceData are the entries of a workspace that hold what a backup restores.
var workspaceData = []string{
	backupDBFile, backupDBFile + "-wal", backupDBFile + "-shm", backupVaultRel,
	"store", "library", "fonts", "providers",
}

// clearWorkspace refuses a workspace that has data, or moves it to replaced-<time>/. An empty database
// file (the workspace was opened and nothing was saved) is not data.
func (o RestoreOptions) clearWorkspace() (string, error) {
	var present []string

	for _, name := range workspaceData {
		info, err := os.Stat(o.Dir.Path(name))
		if err != nil {
			continue
		}

		if info.IsDir() {
			if left, _ := os.ReadDir(o.Dir.Path(name)); len(left) == 0 {
				continue
			}
		} else if info.Size() == 0 {
			_ = os.Remove(o.Dir.Path(name))

			continue
		}

		present = append(present, name)
	}

	if len(present) == 0 {
		return "", nil
	}

	if !o.Replace {
		return "", fmt.Errorf(
			"%w (%s): restore into an empty workspace (a new server, or an empty volume), or add --replace "+
				"to move the data aside to replaced-<time>/ first",
			ErrNotEmpty, strings.Join(present, ", "),
		)
	}

	aside := o.Dir.Path("replaced-" + o.Now().UTC().Format("20060102T150405Z"))
	if err := os.MkdirAll(aside, 0o700); err != nil {
		return "", err
	}

	for _, name := range present {
		if err := os.Rename(o.Dir.Path(name), filepath.Join(aside, name)); err != nil {
			return "", fmt.Errorf("workspace: move %s aside: %w", name, err)
		}
	}

	return aside, nil
}
