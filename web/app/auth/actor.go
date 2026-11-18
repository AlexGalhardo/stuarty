package auth

import (
	"errors"
	"regexp"
)

// DefaultLabel is the label of a client that sends none (SDD-000 section 3).
const DefaultLabel = "api"

// UILabel is reserved for the browser session: a client may not claim it
// (SDD-003 section 2.3), or it would share the UI's undo chain.
const UILabel = "ui"

// ClientHeader names the label of a bearer client (SDD-007 section 3.7 item 4).
const ClientHeader = "Vukem-Video-Client"

var labelRe = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)

// ErrBadLabel is a client label that is malformed or reserved.
var ErrBadLabel = errors.New("the client label must be 1 to 32 characters from a-z, 0-9 and -, and not ui")

// Actor is the actor of a bearer client: "local:<label>".
func Actor(label string) (string, error) {
	if label == "" {
		label = DefaultLabel
	}

	if !labelRe.MatchString(label) || label == UILabel {
		return "", ErrBadLabel
	}

	return "local:" + label, nil
}
