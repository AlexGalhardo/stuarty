package cli

import (
	"errors"
	"io/fs"
	"net/url"
	"regexp"
	"strings"
)

// readReason is why a file could not be read, in the language of the product and
// without the full stop that operating system messages carry (a localized
// Windows message ends in one and the sentence that wraps it adds another).
func readReason(err error) string {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "the file does not exist"
	case errors.Is(err, fs.ErrPermission):
		return "permission denied"
	}

	return strings.TrimRight(err.Error(), ". ")
}

var windowsDrive = regexp.MustCompile(`^/[A-Za-z]:`)

// expandPath turns what a person types into a path: "~" and "~/x" are the home
// directory (Windows shells do not expand it, VV-048 c) and a file:// URL is the
// path it names (d). Anything else is returned as typed.
func expandPath(path string, getenv func(string) string) string {
	if getenv == nil {
		return path
	}

	if rest, ok := strings.CutPrefix(path, "file://"); ok {
		u, err := url.Parse("file://" + rest)
		if err != nil || u.Path == "" {
			return path
		}

		p := u.Path
		if windowsDrive.MatchString(p) {
			p = p[1:]
		}

		return p
	}

	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		home := getenv("HOME")
		if home == "" {
			home = getenv("USERPROFILE")
		}

		if home != "" {
			return home + path[1:]
		}
	}

	return path
}
