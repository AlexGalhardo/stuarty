package account

import (
	"bytes"
)

// The envelope of a sealed library file (ADR-0053, SDD-005 section 5.17.4): a magic, a version byte, a 12-byte
// random nonce, then the AES-256-GCM ciphertext and tag. The associated data are the label, a zero byte and the path
// relative to library/, so a sealed file copied to another name does not open.
const (
	fileMagic   = "STFL"
	fileVersion = byte(1)
	fileLabel   = "stuarty/file/v1"
)

var fileHeader = append([]byte(fileMagic), fileVersion)

// FileSealer seals and opens the files of the library. It is the one place that writes the envelope.
type FileSealer interface {
	// SealFile returns the file as it is stored: sealed with an account, as it is with none.
	SealFile(rel string, plain []byte) ([]byte, error)
	// OpenFile returns the content of a stored file that SealFile made. A clear file is not opened by it: the caller
	// asks IsSealedFile first.
	OpenFile(rel string, stored []byte) ([]byte, error)
	// Ready is nil when files can be read and written now, and ErrLocked while the studio is locked.
	Ready() error
	// Sealing reports whether SealFile seals, as opposed to returning clear text (no account).
	Sealing() bool
}

// IsSealedFile reports whether stored begins with the magic of a sealed file, whatever its version byte: a file whose
// version was changed is a sealed file that does not open, never clear text to seal again or to read as it is.
func IsSealedFile(stored []byte) bool { return bytes.HasPrefix(stored, []byte(fileMagic)) }

func fileAAD(rel string) []byte { return []byte(fileLabel + "\x00" + rel) }

// PassthroughFiles is the FileSealer of a workspace with no account: files are clear text, as before.
type PassthroughFiles struct{}

// SealFile implements FileSealer.
func (PassthroughFiles) SealFile(_ string, plain []byte) ([]byte, error) { return plain, nil }

// OpenFile implements FileSealer.
func (PassthroughFiles) OpenFile(_ string, stored []byte) ([]byte, error) { return stored, nil }

// Ready implements FileSealer.
func (PassthroughFiles) Ready() error { return nil }

// Sealing implements FileSealer.
func (PassthroughFiles) Sealing() bool { return false }

// SealFile implements FileSealer.
func (s *KeySealer) SealFile(rel string, plain []byte) ([]byte, error) {
	sealed, err := wrap(s.key, fileAAD(rel), plain)
	if err != nil {
		return nil, err
	}

	return append(append([]byte(nil), fileHeader...), sealed...), nil
}

// OpenFile implements FileSealer. Any failure is ErrUnsealed.
func (s *KeySealer) OpenFile(rel string, stored []byte) ([]byte, error) {
	body, ok := bytes.CutPrefix(stored, fileHeader)
	if !ok {
		return nil, ErrUnsealed
	}

	plain, err := unwrap(s.key, fileAAD(rel), body)
	if err != nil {
		return nil, ErrUnsealed
	}

	return plain, nil
}

// Ready implements FileSealer.
func (s *KeySealer) Ready() error { return nil }

// Sealing implements FileSealer.
func (s *KeySealer) Sealing() bool { return true }

// Files is the FileSealer of the library: clear text with no account, ErrLocked while locked, the data key while
// unlocked.
func (m *Manager) Files() FileSealer { return managerFiles{m} }

type managerFiles struct{ m *Manager }

func (f managerFiles) with(fn func(FileSealer) ([]byte, error)) ([]byte, error) {
	f.m.mu.Lock()
	defer f.m.mu.Unlock()

	if f.m.creating {
		return nil, ErrLocked
	}

	switch f.m.state {
	case StateNone:
		return fn(PassthroughFiles{})
	case StateLocked:
		return nil, ErrLocked
	case StateUnlocked:
	}

	return fn(f.m.sealer)
}

func (f managerFiles) SealFile(rel string, plain []byte) ([]byte, error) {
	return f.with(func(k FileSealer) ([]byte, error) { return k.SealFile(rel, plain) })
}

func (f managerFiles) OpenFile(rel string, stored []byte) ([]byte, error) {
	return f.with(func(k FileSealer) ([]byte, error) { return k.OpenFile(rel, stored) })
}

func (f managerFiles) Ready() error {
	_, err := f.with(func(FileSealer) ([]byte, error) { return nil, nil })

	return err
}

func (f managerFiles) Sealing() bool { return managerSealer(f).Sealing() }
