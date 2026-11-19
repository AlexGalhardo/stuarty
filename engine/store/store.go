package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"time"
)

// Error codes of SDD-000 section 8 that this package raises.
const (
	CodeLimitExceeded    = "E_LIMIT_EXCEEDED"
	CodeUnsupportedMedia = "E_UNSUPPORTED_MEDIA"
	CodeInternal         = "E_INTERNAL"
	CodeNotFound         = "E_NOT_FOUND"
)

// MinFileSize is the smallest file the store accepts (SDD-007 7.1 step 11).
const MinFileSize = 12

// Error carries an SDD-000 error code and the details of SDD-007 9.1.
type Error struct {
	Code    string
	Reason  string
	Message string
	Details map[string]any
	Err     error
}

func (e *Error) Error() string {
	s := e.Code + ": " + e.Message
	if e.Err != nil {
		s += ": " + e.Err.Error()
	}
	return s
}

func (e *Error) Unwrap() error { return e.Err }

var hashRe = regexp.MustCompile(`^[0-9a-f]{64}$`)

// ValidHash reports whether s is a lowercase hexadecimal SHA-256 digest.
func ValidHash(s string) bool { return hashRe.MatchString(s) }

// Store is the content-addressed original files of a workspace: store/<aa>/<sha256>
// (SDD-000 section 9). Files are written once and never modified.
type Store struct {
	dir string
}

// New returns the store of a workspace directory. The folders are created on first use.
func New(workspace string) *Store {
	return &Store{dir: filepath.Join(workspace, "store")}
}

// Dir is the store directory.
func (s *Store) Dir() string { return s.dir }

func (s *Store) incoming() string { return filepath.Join(s.dir, ".incoming") }

// Path is store/<first 2 hex>/<sha256> for a valid digest.
func (s *Store) Path(hash string) (string, error) {
	if !ValidHash(hash) {
		return "", &Error{
			Code:    CodeInternal,
			Reason:  "bad_hash",
			Message: "a store key is 64 lowercase hexadecimal characters",
		}
	}
	return filepath.Join(s.dir, hash[:2], hash), nil
}

// Result is the outcome of Put.
type Result struct {
	Hash         string // lowercase hexadecimal SHA-256, without a prefix
	Size         int64
	Path         string
	Deduplicated bool // the hash was already in the store
}

// ctxReader stops a copy when the context ends.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

// Put streams r through SHA-256 into store/.incoming/<random>, counting bytes, then
// publishes it as store/<aa>/<sha256> (SDD-011 2.3 step 4). A stream longer than limit is
// E_LIMIT_EXCEEDED; the copy is deleted whatever the outcome unless it became the object.
// When the hash is already stored the copy is discarded and the existing file is reused.
// The published file is read-only (0400, the read-only attribute on Windows).
func (s *Store) Put(ctx context.Context, r io.Reader, limit int64) (Result, error) {
	if err := os.MkdirAll(s.incoming(), 0o700); err != nil {
		return Result{}, &Error{
			Code:    CodeInternal,
			Reason:  "store_unwritable",
			Message: "the store cannot be written",
			Err:     err,
		}
	}
	tmp, err := os.CreateTemp(s.incoming(), "put-*")
	if err != nil {
		return Result{}, &Error{
			Code:    CodeInternal,
			Reason:  "store_unwritable",
			Message: "the store cannot be written",
			Err:     err,
		}
	}
	published := false
	defer func() {
		if !published {
			_ = tmp.Close()
			_ = os.Chmod(tmp.Name(), 0o600)
			_ = os.Remove(tmp.Name())
		}
	}()

	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(ctxReader{ctx, r}, limit+1))
	if err != nil {
		return Result{}, fmt.Errorf("store: copy: %w", err)
	}
	if n > limit {
		return Result{}, &Error{
			Code:    CodeLimitExceeded,
			Reason:  "import_bytes",
			Message: "The file is larger than the import limit.",
			Details: map[string]any{
				"limit":    "import_bytes",
				"value":    n,
				"ceiling":  limit,
				"variable": "STUARTY_MAX_IMPORT_BYTES",
			},
		}
	}
	if n < MinFileSize {
		reason := "too_small"
		if n == 0 {
			reason = "empty_file"
		}
		return Result{}, &Error{
			Code:    CodeUnsupportedMedia,
			Reason:  reason,
			Message: "The file is empty or too small to be media.",
		}
	}
	if err := tmp.Sync(); err != nil {
		return Result{}, &Error{
			Code:    CodeInternal,
			Reason:  "store_unwritable",
			Message: "the copy could not be flushed",
			Err:     err,
		}
	}
	if err := tmp.Close(); err != nil {
		return Result{}, &Error{
			Code:    CodeInternal,
			Reason:  "store_unwritable",
			Message: "the copy could not be closed",
			Err:     err,
		}
	}

	hash := hex.EncodeToString(h.Sum(nil))
	dest, _ := s.Path(hash)
	res := Result{Hash: hash, Size: n, Path: dest}

	if st, err := os.Stat(dest); err == nil {
		if st.Size() != n {
			return Result{}, &Error{
				Code:    CodeInternal,
				Reason:  "store_corrupt",
				Message: "a stored file has the hash of another size",
			}
		}
		res.Deduplicated = true
		return res, nil
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return Result{}, &Error{
			Code:    CodeInternal,
			Reason:  "store_unwritable",
			Message: "the store cannot be written",
			Err:     err,
		}
	}
	if err := os.Chmod(tmp.Name(), 0o400); err != nil {
		return Result{}, &Error{
			Code:    CodeInternal,
			Reason:  "store_unwritable",
			Message: "the copy could not be made read-only",
			Err:     err,
		}
	}
	if err := os.Rename(tmp.Name(), dest); err != nil {
		// A concurrent Put of the same bytes won: reuse its file.
		if st, serr := os.Stat(dest); serr == nil && st.Size() == n {
			res.Deduplicated = true
			return res, nil
		}
		return Result{}, &Error{
			Code:    CodeInternal,
			Reason:  "store_unwritable",
			Message: "the copy could not be published",
			Err:     err,
		}
	}
	published = true
	syncDir(filepath.Dir(dest))
	return res, nil
}

// syncDir flushes a directory entry where the platform allows it; Windows does not.
func syncDir(dir string) {
	if runtime.GOOS == "windows" {
		return
	}
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
}

// Has reports whether the hash is stored.
func (s *Store) Has(hash string) bool {
	p, err := s.Path(hash)
	if err != nil {
		return false
	}
	st, err := os.Lstat(p)
	return err == nil && st.Mode().IsRegular()
}

// Open opens a stored file for reading.
func (s *Store) Open(hash string) (*os.File, error) {
	p, err := s.Path(hash)
	if err != nil {
		return nil, err
	}
	// Lstat does not follow links: a link inside store/ is refused (SDD-007 section 6.4 item 4).
	st, err := os.Lstat(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, &Error{Code: CodeNotFound, Reason: "not_stored", Message: "the file is not in the store", Err: err}
	}
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, &Error{Code: CodeNotFound, Reason: "not_regular", Message: "the stored file is not a regular file"}
	}
	f, err := os.Open(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, &Error{Code: CodeNotFound, Reason: "not_stored", Message: "the file is not in the store", Err: err}
	}
	return f, err
}

// Remove deletes a stored file and its folder when that is empty. It clears the read-only
// attribute first, which Windows needs. A file that is not there is not an error.
func (s *Store) Remove(hash string) error {
	p, err := s.Path(hash)
	if err != nil {
		return err
	}
	_ = os.Chmod(p, 0o600)
	if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	_ = os.Remove(filepath.Dir(p)) // only succeeds when empty
	return nil
}

// SweepIncoming deletes the copies in store/.incoming older than maxAge: the leftovers of
// a crash during Put (SDD-011 6.4 item 1). It returns the number removed.
func (s *Store) SweepIncoming(now time.Time, maxAge time.Duration) (int, error) {
	entries, err := os.ReadDir(s.incoming())
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || now.Sub(info.ModTime()) < maxAge {
			continue
		}
		p := filepath.Join(s.incoming(), e.Name())
		_ = os.Chmod(p, 0o600)
		if os.Remove(p) == nil {
			n++
		}
	}
	return n, nil
}

// SweepOrphans deletes files of store/<aa>/ that known does not list and that are older than
// maxAge: what emptying the trash leaves after a crash (SDD-011 6.4 item 4). known is asked
// for the hash of each file.
func (s *Store) SweepOrphans(now time.Time, maxAge time.Duration, known func(hash string) bool) (int, error) {
	dirs, err := os.ReadDir(s.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	n := 0
	for _, d := range dirs {
		if !d.IsDir() || len(d.Name()) != 2 {
			continue
		}
		files, err := os.ReadDir(filepath.Join(s.dir, d.Name()))
		if err != nil {
			continue
		}
		for _, f := range files {
			info, err := f.Info()
			if err != nil || !ValidHash(f.Name()) || now.Sub(info.ModTime()) < maxAge || known(f.Name()) {
				continue
			}
			if s.Remove(f.Name()) == nil {
				n++
			}
		}
	}
	return n, nil
}
