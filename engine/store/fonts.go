package store

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/AlexGalhardo/stuarty/engine/rasterize"
)

// Fonts is the user fonts of a workspace: fonts/<sha256>, mode 0600 (SDD-000
// section 9, SDD-007 section 7.7). A file is validated before it is stored; a
// font that fails is never written. The files are read only by the in-process
// rasterizer, never given to FFmpeg or whisper.cpp.
type Fonts struct {
	dir string
}

// NewFonts returns the user fonts of a workspace directory. The folder is
// created on first use.
func NewFonts(workspace string) *Fonts {
	return &Fonts{dir: filepath.Join(workspace, "fonts")}
}

// Dir is the fonts directory.
func (f *Fonts) Dir() string { return f.dir }

func (f *Fonts) path(hash string) (string, error) {
	if !ValidHash(hash) {
		return "", &Error{
			Code:    CodeInternal,
			Reason:  "bad_hash",
			Message: "a font key is 64 lowercase hexadecimal characters",
		}
	}

	return filepath.Join(f.dir, hash), nil
}

// Put validates the font and stores it by content hash. It returns the hex
// SHA-256 (the document writes it as "sha256:" and the digest). The error of
// a refused font is a *rasterize.InvalidFontError and nothing is written.
func (f *Fonts) Put(data []byte) (string, error) {
	if err := rasterize.ValidateFont(data); err != nil {
		return "", err
	}

	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])

	dest, err := f.path(hash)
	if err != nil {
		return "", err
	}

	if st, err := os.Stat(dest); err == nil && st.Mode().IsRegular() && st.Size() == int64(len(data)) {
		return hash, nil
	}

	if err := os.MkdirAll(f.dir, 0o700); err != nil {
		return "", fontsUnwritable(err)
	}

	tmp, err := os.CreateTemp(f.dir, ".put-*")
	if err != nil {
		return "", fontsUnwritable(err)
	}

	published := false

	defer func() {
		if !published {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}
	}()

	if err := tmp.Chmod(0o600); err != nil && !errors.Is(err, fs.ErrInvalid) {
		return "", fontsUnwritable(err)
	}

	if _, err := tmp.Write(data); err != nil {
		return "", fontsUnwritable(err)
	}

	if err := tmp.Sync(); err != nil {
		return "", fontsUnwritable(err)
	}

	if err := tmp.Close(); err != nil {
		return "", fontsUnwritable(err)
	}

	if err := os.Rename(tmp.Name(), dest); err != nil {
		// A concurrent Put of the same bytes won: reuse its file.
		if st, serr := os.Stat(dest); serr == nil && st.Size() == int64(len(data)) {
			return hash, nil
		}

		return "", fontsUnwritable(err)
	}

	published = true

	syncDir(f.dir)

	return hash, nil
}

// Has reports whether the font is stored.
func (f *Fonts) Has(hash string) bool {
	p, err := f.path(hash)
	if err != nil {
		return false
	}

	st, err := os.Stat(p)

	return err == nil && st.Mode().IsRegular()
}

// Read returns the bytes of a stored font. A font that is not stored is an
// error that wraps rasterize.ErrFontNotStored (and fs.ErrNotExist).
// Bytes whose hash differs from their name are refused as corrupt.
func (f *Fonts) Read(hash string) ([]byte, error) {
	p, err := f.path(hash)
	if err != nil {
		return nil, err
	}

	st, err := os.Stat(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w: %w", rasterize.ErrFontNotStored, err)
	}

	if err != nil {
		return nil, err
	}

	if st.Size() > rasterize.MaxFontBytes {
		return nil, &Error{
			Code:    CodeInternal,
			Reason:  "store_corrupt",
			Message: "a stored font is larger than the limit",
		}
	}

	data, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}

	if sum := sha256.Sum256(data); hex.EncodeToString(sum[:]) != hash {
		return nil, &Error{
			Code:    CodeInternal,
			Reason:  "store_corrupt",
			Message: "a stored font does not match its hash",
		}
	}

	return data, nil
}

// Source is Read as a rasterize.FontSource.
func (f *Fonts) Source() rasterize.FontSource { return f.Read }

func fontsUnwritable(err error) error {
	return &Error{
		Code:    CodeInternal,
		Reason:  "store_unwritable",
		Message: "the fonts folder cannot be written",
		Err:     err,
	}
}
