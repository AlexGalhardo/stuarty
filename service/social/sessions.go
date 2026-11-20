package social

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Session is the address of an upload session a network gave, and the size of the file it is for.
// The address is useless without the access token, so it holds no credential (SDD-020 section 7.4).
type Session struct {
	URI string `json:"uri"`
	// ID is what the network named the upload (a container or a video), for a network that needs it besides
	// the address.
	ID string `json:"id,omitempty"`
	// Uploaded is true when the whole file reached the network and only its processing is left.
	Uploaded bool  `json:"uploaded,omitempty"`
	Size     int64 `json:"size"`
	Created  int64 `json:"created_ms"`
}

// Sessions keeps the upload sessions of publications in progress, by key, so that a restart resumes an
// upload instead of starting again.
type Sessions interface {
	// Load returns the session of a key; false when there is none.
	Load(key string) (Session, bool, error)
	Save(key string, s Session) error
	// Clear removes the session of a key; a missing one is not an error.
	Clear(key string) error
}

// FileSessions keeps the sessions as files of one folder, mode 0600, written through a temporary file
// and a rename.
type FileSessions struct{ Dir string }

// UploadKey is the key of an upload: the same connection, file and post give the same key, a changed
// file or post another one.
func UploadKey(parts ...string) string {
	h := sha256.New()

	for _, p := range parts {
		_, _ = fmt.Fprintf(h, "%d:%s|", len(p), p)
	}

	return hex.EncodeToString(h.Sum(nil))[:32]
}

func (f FileSessions) path(key string) string { return filepath.Join(f.Dir, key+".json") }

// Load implements Sessions.
func (f FileSessions) Load(key string) (Session, bool, error) {
	raw, err := os.ReadFile(f.path(key))
	if errors.Is(err, fs.ErrNotExist) {
		return Session{}, false, nil
	}

	if err != nil {
		return Session{}, false, err
	}

	var s Session
	if json.Unmarshal(raw, &s) != nil || s.URI == "" {
		// A file that does not parse is a session that never was: start again.
		_ = os.Remove(f.path(key))

		return Session{}, false, nil //nolint:nilerr // a file that does not parse is no session
	}

	return s, true, nil
}

// Save implements Sessions.
func (f FileSessions) Save(key string, s Session) error {
	if s.Created == 0 {
		s.Created = time.Now().UnixMilli()
	}

	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(f.Dir, 0o750); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(f.Dir, ".session-*")
	if err != nil {
		return err
	}

	name := tmp.Name()

	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)

		return err
	}

	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)

		return err
	}

	if err := os.Chmod(name, 0o600); err != nil {
		_ = os.Remove(name)

		return err
	}

	return os.Rename(name, f.path(key))
}

// Clear implements Sessions.
func (f FileSessions) Clear(key string) error {
	if err := os.Remove(f.path(key)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	return nil
}

// MemorySessions is a Sessions in memory, for tests.
type MemorySessions struct {
	M map[string]Session
}

// Load implements Sessions.
func (m *MemorySessions) Load(key string) (Session, bool, error) {
	s, ok := m.M[key]

	return s, ok, nil
}

// Save implements Sessions.
func (m *MemorySessions) Save(key string, s Session) error {
	if m.M == nil {
		m.M = map[string]Session{}
	}

	m.M[key] = s

	return nil
}

// Clear implements Sessions.
func (m *MemorySessions) Clear(key string) error {
	delete(m.M, key)

	return nil
}
