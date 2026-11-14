package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
)

// ArtifactName is <kind>-<profile>[-<variant>].<ext> (SDD-011 6.1).
func ArtifactName(kind string, profile int, variant, ext string) string {
	n := kind + "-" + strconv.Itoa(profile)
	if variant != "" {
		n += "-" + variant
	}
	return n + "." + ext
}

// publish is step 7 (SDD-011 6.2): each file is renamed into artifacts/<hash>/ under its final name,
// read-only. The caller registers the returned rows after this returns. When the final name already
// exists (a concurrent job won) the new file is deleted: both were verified.
func (j *job) publish(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dir := filepath.Join(j.in.ArtifactsDir, j.in.Hash)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return ingestErr(
			ReasonDecodeFailed,
			"The artifacts folder could not be written.",
			map[string]any{"step": StepPublish},
			err,
		)
	}
	files := append([]workFile(nil), j.files...)
	sort.SliceStable(files, func(a, b int) bool {
		return ArtifactName(
			files[a].kind,
			files[a].profile,
			files[a].variant,
			files[a].ext,
		) < ArtifactName(
			files[b].kind,
			files[b].profile,
			files[b].variant,
			files[b].ext,
		)
	})
	for _, f := range files {
		if f.kind == "normalized" && f.meta == nil {
			f.meta = j.res.Facts
		}
		name := ArtifactName(f.kind, f.profile, f.variant, f.ext)
		src, dst := j.path(f.name), filepath.Join(dir, name)
		sum, size, err := hashFile(src)
		if err != nil {
			return ingestErr(
				ReasonDecodeFailed,
				"A produced file could not be read back.",
				map[string]any{"step": StepPublish},
				err,
			)
		}
		if err := publishFile(src, dst); err != nil {
			return ingestErr(
				ReasonDecodeFailed,
				"A produced file could not be saved.",
				map[string]any{"step": StepPublish},
				err,
			)
		}
		rel, _ := filepath.Rel(filepath.Dir(j.in.ArtifactsDir), dst)
		j.res.Artifacts = append(j.res.Artifacts, Artifact{
			Kind: f.kind, Profile: f.profile, Variant: f.variant, Ext: f.ext,
			RelPath: filepath.ToSlash(rel), Size: size, SHA256: sum, Meta: f.meta,
		})
	}
	return nil
}

func hashFile(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	n, err := io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), n, err
}

// publishFile makes src read-only and moves it to dst with an atomic rename. When the rename fails
// across volumes the file is copied to .<name>.part, flushed and renamed.
func publishFile(src, dst string) error {
	if _, err := os.Stat(dst); err == nil {
		return nil // a concurrent job published it first
	}
	if err := os.Chmod(src, 0o400); err != nil {
		return err
	}
	err := os.Rename(src, dst)
	if err == nil {
		return nil
	}
	if _, serr := os.Stat(dst); serr == nil {
		return nil
	}
	// Different volumes: copy, flush, rename.
	part := filepath.Join(filepath.Dir(dst), "."+filepath.Base(dst)+".part")
	in, oerr := os.Open(src)
	if oerr != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, cerr := os.OpenFile(part, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if cerr != nil {
		return cerr
	}
	if _, cerr = io.Copy(out, in); cerr == nil {
		cerr = out.Sync()
	}
	if closeErr := out.Close(); cerr == nil {
		cerr = closeErr
	}
	if cerr != nil {
		_ = os.Remove(part)
		return cerr
	}
	if cerr = os.Chmod(part, 0o400); cerr == nil {
		cerr = os.Rename(part, dst)
	}
	if cerr != nil {
		_ = os.Chmod(part, 0o600)
		_ = os.Remove(part)
	}
	return cerr
}

// RemoveArtifacts deletes the artifact folder of a hash, clearing the read-only attribute that
// Windows needs first. The folder is flat. A folder that is not there is not an error.
func RemoveArtifacts(artifactsDir, hash string) error {
	dir := filepath.Join(artifactsDir, hash)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		_ = os.Chmod(filepath.Join(dir, e.Name()), 0o600)
	}
	return os.RemoveAll(dir)
}
