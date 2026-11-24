package account

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// The limits of SDD-007 section 3.9 item 1.
const (
	MinUsername = 4
	MaxUsername = 16
	MinEmail    = 12
	MaxEmail    = 48
	MinPassword = 8
	MaxPassword = 32
	// MaxIdleMinutes is the longest idle lock a person may set: one day. Zero turns it off.
	MaxIdleMinutes = 1440
)

// The fields a Problem names.
const (
	FieldUsername     = "username"
	FieldEmail        = "email"
	FieldPassword     = "password"
	FieldConfirmation = "confirmation"
	FieldIdleLock     = "idle_lock_minutes"
)

// The reasons a field is refused (SDD-007 section 3.9 item 1).
const (
	ReasonTooShort            = "too_short"
	ReasonTooLong             = "too_long"
	ReasonNotLowercaseLetters = "not_lowercase_letters"
	ReasonInvalidFormat       = "invalid_format"
	ReasonNoUppercase         = "no_uppercase"
	ReasonNoLowercase         = "no_lowercase"
	ReasonNoDigit             = "no_digit"
	ReasonNoSpecial           = "no_special"
	ReasonSpaceOrControl      = "has_space_or_control"
	ReasonMismatch            = "mismatch"
	ReasonOutOfRange          = "out_of_range"
)

// Normalize is the composed Unicode form (NFC) of a password. The same typed text arrives composed from one
// system and decomposed from another (an é is one character or two), and the password must open the account
// whichever it is (VV-380). The rules and the key derivation both read the password through it.
func Normalize(password string) string { return norm.NFC.String(password) }

// Problem is one refused field and why. It never holds the value.
type Problem struct {
	Field  string `json:"field"`
	Reason string `json:"reason"`
}

// Input is what a person types to create an account.
type Input struct {
	Username     string
	Email        string
	Password     string
	Confirmation string
}

// Validate returns every problem of the input, in the order username, e-mail,
// password, confirmation, with every reason of the password; nil when it is fine.
func Validate(in Input) []Problem {
	var out []Problem

	out = append(out, Username(in.Username)...)
	out = append(out, Email(in.Email)...)
	out = append(out, Password(in.Password)...)
	out = append(out, Confirmation(in.Password, in.Confirmation)...)

	return out
}

// Username checks a username: 4 to 16 lowercase letters a to z.
func Username(s string) []Problem {
	n := utf8.RuneCountInString(s)

	switch {
	case n < MinUsername:
		return []Problem{{FieldUsername, ReasonTooShort}}
	case n > MaxUsername:
		return []Problem{{FieldUsername, ReasonTooLong}}
	}

	for _, r := range s {
		if r < 'a' || r > 'z' {
			return []Problem{{FieldUsername, ReasonNotLowercaseLetters}}
		}
	}

	return nil
}

// Email checks an e-mail address: 12 to 48 characters, a local part, one @, a
// domain with a dot and a final label of two letters or more. Nothing is sent
// and no address is looked up.
func Email(s string) []Problem {
	n := utf8.RuneCountInString(s)

	switch {
	case n < MinEmail:
		return []Problem{{FieldEmail, ReasonTooShort}}
	case n > MaxEmail:
		return []Problem{{FieldEmail, ReasonTooLong}}
	case !emailFormat(s):
		return []Problem{{FieldEmail, ReasonInvalidFormat}}
	}

	return nil
}

func emailFormat(s string) bool {
	local, domain, found := strings.Cut(s, "@")
	if !found || strings.Contains(domain, "@") {
		return false
	}

	if local == "" || strings.HasPrefix(local, ".") || strings.HasSuffix(local, ".") || strings.Contains(local, "..") {
		return false
	}

	for _, r := range local {
		if !isASCIIAlnum(r) && !strings.ContainsRune("._%+-", r) {
			return false
		}
	}

	labels := strings.Split(domain, ".")
	if len(labels) < 2 {
		return false
	}

	for _, label := range labels {
		if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}

		for _, r := range label {
			if !isASCIIAlnum(r) && r != '-' {
				return false
			}
		}
	}

	last := labels[len(labels)-1]
	if len(last) < 2 {
		return false
	}

	for _, r := range last {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') {
			return false
		}
	}

	return true
}

func isASCIIAlnum(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

// Password checks a password: 8 to 32 characters, no space and no control
// character, with at least one uppercase letter, one lowercase letter, one digit
// and one special character. Every missing class is reported.
func Password(s string) []Problem {
	s = Normalize(s)
	n := utf8.RuneCountInString(s)

	switch {
	case n < MinPassword:
		return []Problem{{FieldPassword, ReasonTooShort}}
	case n > MaxPassword:
		return []Problem{{FieldPassword, ReasonTooLong}}
	}

	var upper, lower, digit, special, bad bool

	for _, r := range s {
		switch {
		case unicode.IsSpace(r) || unicode.IsControl(r):
			bad = true
		case unicode.IsUpper(r):
			upper = true
		case unicode.IsLower(r):
			lower = true
		case unicode.IsDigit(r):
			digit = true
		case unicode.IsLetter(r):
			// A letter of a script with no case is neither an uppercase nor a lowercase letter, and it is
			// not special either.
		default:
			special = true
		}
	}

	var out []Problem

	add := func(missing bool, reason string) {
		if missing {
			out = append(out, Problem{FieldPassword, reason})
		}
	}

	add(bad, ReasonSpaceOrControl)
	add(!upper, ReasonNoUppercase)
	add(!lower, ReasonNoLowercase)
	add(!digit, ReasonNoDigit)
	add(!special, ReasonNoSpecial)

	return out
}

// Confirmation checks that the second typing of the password equals the first.
func Confirmation(password, confirmation string) []Problem {
	if Normalize(password) != Normalize(confirmation) {
		return []Problem{{FieldConfirmation, ReasonMismatch}}
	}

	return nil
}

// IdleMinutes checks the idle lock a person sets: 0 (off) to 1440 minutes.
func IdleMinutes(n int) []Problem {
	if n < 0 || n > MaxIdleMinutes {
		return []Problem{{FieldIdleLock, ReasonOutOfRange}}
	}

	return nil
}
