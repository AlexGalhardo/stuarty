package oauth

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var hostIDRe = regexp.MustCompile(`^urn:uuid:[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// Reasons of a RefusedError: a sign-in the provider finished but that Stuarty does not accept.
const (
	// ReasonRegistration is a first sign-in whose redirect carried no issued client id.
	ReasonRegistration = "registration_incomplete"
	// ReasonClientMismatch is a redirect that names another client than the one kept for the account.
	ReasonClientMismatch = "client_mismatch"
	// ReasonIdentity is an identity token that does not belong to this sign-in.
	ReasonIdentity = "identity"
	// ReasonNoAccess is a sign-in that was not granted what Stuarty asked for.
	ReasonNoAccess = "no_access"
)

// RefusedError is a sign-in that the provider answered and that is not accepted. Reason is one of the
// constants above, so the service can say what happened in a fixed sentence.
type RefusedError struct{ Reason string }

func (e *RefusedError) Error() string { return "oauth: the sign-in was not accepted: " + e.Reason }

// HostID returns the stable identifier of this host that a provider asks for (SDD-016 section 12.1), a
// random UUID kept in the file at path: it is made the first time and read afterwards. It is not a secret
// and says nothing about the person.
func HostID(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err == nil {
		if id := strings.TrimSpace(string(raw)); hostIDRe.MatchString(id) {
			return id, nil
		}

		return "", errors.New("oauth: the host identifier file is not a UUID; remove it to make a new one")
	}

	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	b := make([]byte, 16)
	if _, rerr := rand.Read(b); rerr != nil {
		return "", rerr
	}

	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	id := fmt.Sprintf("urn:uuid:%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(id+"\n"), 0o600); err != nil {
		return "", err
	}

	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)

		return "", err
	}

	return id, nil
}
