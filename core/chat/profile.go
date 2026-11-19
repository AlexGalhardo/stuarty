package chat

import (
	"errors"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var profileNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

// The refusals of the profile rules; the service turns them into E_VALIDATION with the
// reason of SDD-017 section 11.
var (
	ErrProfileName = errors.New(
		"the name is 1 to 32 characters from a-z, 0-9 and -, and starts with a letter or a digit")
	ErrCommand = errors.New(
		"the command is one executable: an absolute path or a program name, with no control character")
	ErrArgs = errors.New("at most 32 arguments of at most 512 characters, with no control character")
)

// ValidProfileName reports whether name is a profile name.
func ValidProfileName(name string) bool { return profileNameRe.MatchString(name) }

// CheckProfile applies the syntactic rules of SDD-017 section 3 to a profile as a person
// typed it. That the program exists and its absolute path are the service's to check: this
// package reads no file.
func CheckProfile(name, command string, args []string) error {
	if !ValidProfileName(name) {
		return ErrProfileName
	}

	if command == "" || len(command) > MaxCommandLength || !utf8.ValidString(command) || hasControl(command) ||
		strings.TrimSpace(command) != command {
		return ErrCommand
	}

	// A relative path with a separator would depend on the directory of the server; a bare
	// name is looked up once, when the profile is written.
	if strings.ContainsAny(command, `/\`) && !strings.HasPrefix(command, "/") && !isWindowsAbs(command) {
		return ErrCommand
	}

	if len(args) > MaxArgs {
		return ErrArgs
	}

	for _, a := range args {
		if len(a) > MaxArgLength || !utf8.ValidString(a) || hasControl(a) {
			return ErrArgs
		}
	}

	return nil
}

// isWindowsAbs recognises a drive path or a UNC path on any host, so a profile written on
// one system is judged the same on another.
func isWindowsAbs(p string) bool {
	if len(p) >= 3 && p[1] == ':' && (p[2] == '\\' || p[2] == '/') &&
		(p[0] >= 'a' && p[0] <= 'z' || p[0] >= 'A' && p[0] <= 'Z') {
		return true
	}

	return strings.HasPrefix(p, `\\`)
}

func hasControl(s string) bool {
	return strings.IndexFunc(s, unicode.IsControl) >= 0
}

// Suggestion is a known agent: the names of its executable that a person would look for and
// the arguments that put it in the Agent Client Protocol mode. It is text; Stuarty installs
// and runs nothing to build it (SDD-017 section 3).
type Suggestion struct {
	Name        string   `json:"name"`
	Title       string   `json:"title"`
	Executables []string `json:"executables"`
	Args        []string `json:"args"`
	Install     string   `json:"install"`
}

// Suggestions is the fixed list of known agents. The vendors' names are plain text.
func Suggestions() []Suggestion {
	return []Suggestion{
		{
			Name: "claude-code", Title: "Claude Code",
			Executables: []string{"claude-agent-acp", "claude-code-acp"}, Args: []string{},
			Install: "Install the Agent Client Protocol adapter for Claude Code (for example with npm), then sign in " +
				"to Claude Code the way you always do.",
		},
		{
			Name:        "codex",
			Title:       "Codex",
			Executables: []string{"codex-acp"},
			Args:        []string{},
			Install:     "Install the Agent Client Protocol adapter for Codex, then sign in to Codex the way you always do.",
		},
		{
			Name: "opencode", Title: "OpenCode",
			Executables: []string{"opencode"}, Args: []string{"acp"},
			Install: "Install OpenCode, then sign in to it the way you always do.",
		},
		{
			Name: "gemini", Title: "Gemini CLI",
			Executables: []string{"gemini"}, Args: []string{"--experimental-acp"},
			Install: "Install Gemini CLI, then sign in to it the way you always do.",
		},
	}
}
