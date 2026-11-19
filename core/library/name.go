package library

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var (
	nameForm  = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	scopeForm = regexp.MustCompile(`^prj_[0-9a-hjkmnp-tv-z]{10}$`)
)

// reservedNames are the Windows device names: a library moves between systems,
// so no entry may be called one of them.
var reservedNames = map[string]bool{
	"con": true, "prn": true, "aux": true, "nul": true,
	"com1": true, "com2": true, "com3": true, "com4": true, "com5": true,
	"com6": true, "com7": true, "com8": true, "com9": true,
	"lpt1": true, "lpt2": true, "lpt3": true, "lpt4": true, "lpt5": true,
	"lpt6": true, "lpt7": true, "lpt8": true, "lpt9": true,
}

// ValidName checks the name of an entry: lower case letters, digits and single
// hyphens, at most 64 characters, and not a Windows device name (SDD-018
// section 4). A name is never a path.
func ValidName(name string) error {
	switch {
	case name == "":
		return errors.New("the name is empty")
	case len(name) > MaxNameLength:
		return fmt.Errorf("the name is longer than %d characters", MaxNameLength)
	case !nameForm.MatchString(name):
		return errors.New("the name must be lower case letters, digits and single hyphens (for example weekly-recap)")
	case reservedNames[name]:
		return errors.New("the name is reserved by the operating system")
	}

	return nil
}

// Slug turns free text into a valid name: lower case, every run of other
// characters becomes one hyphen, cut at 64 characters. It returns "" when no
// letter or digit is left.
func Slug(text string) string {
	var b strings.Builder

	pending := false

	for _, r := range strings.ToLower(text) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			if pending && b.Len() > 0 {
				b.WriteByte('-')
			}

			pending = false

			b.WriteRune(r)

			continue
		}

		pending = true
	}

	out := b.String()
	if len(out) > MaxNameLength {
		out = strings.TrimRight(out[:MaxNameLength], "-")
	}

	if reservedNames[out] {
		out += "-note"
	}

	return out
}

// ValidScope checks a memory scope: "workspace" or a project id.
func ValidScope(scope string) error {
	if scope == "workspace" || scopeForm.MatchString(scope) {
		return nil
	}

	return errors.New("the scope must be workspace or a project id (prj_...)")
}

// ValidTags checks the tag list of an entry: at most 10 tags, each a name.
func ValidTags(tags []string) error {
	if len(tags) > MaxTags {
		return fmt.Errorf("at most %d tags", MaxTags)
	}

	seen := map[string]bool{}

	for _, t := range tags {
		if err := ValidName(t); err != nil {
			return fmt.Errorf("the tag %q: %w", t, err)
		}

		if seen[t] {
			return fmt.Errorf("the tag %q is repeated", t)
		}

		seen[t] = true
	}

	return nil
}
