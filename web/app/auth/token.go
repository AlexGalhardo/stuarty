package auth

import (
	"crypto/subtle"
	"strings"
)

// Bearer returns the credential of an Authorization header and whether the
// header is a Bearer one.
func Bearer(header string) (string, bool) {
	scheme, cred, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(cred) == "" {
		return "", false
	}

	return strings.TrimSpace(cred), true
}

// TokenMatches compares in constant time (SDD-007 section 3.7 item 3).
func TokenMatches(want, got string) bool {
	return subtle.ConstantTimeCompare([]byte(want), []byte(got)) == 1
}
