// Package pathguard turns the path a caller names for a media import into an
// open file handle, or refuses it (SDD-007 section 7.1). The service never
// opens an import path any other way, and never opens it twice: everything
// after the guard works on the returned handle.
package pathguard

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"unicode/utf8"
)

// Codes of SDD-000 section 8 that the guard raises.
const (
	CodePathNotAllowed = "E_PATH_NOT_ALLOWED"
	CodeFileNotFound   = "E_FILE_NOT_FOUND"
)

const (
	maxPathBytes = 4096
	hintPlain    = "Pass a plain absolute file path."
	hintInside   = "Copy the file outside the workspace, then import the copy."
)

// Error is a refusal. Its message never contains the path.
type Error struct {
	Code    string
	Message string
	Hint    string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func refuse(msg, hint string) *Error {
	return &Error{Code: CodePathNotAllowed, Message: msg, Hint: hint}
}

var schemeRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]+:`)

// reserved are the Windows device names, compared in upper case against the
// part of a component before its first dot.
var reserved = []string{
	"CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$",
	"COM0", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9", "COM¹", "COM²", "COM³",
	"LPT0", "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9", "LPT¹", "LPT²", "LPT³",
}

// Clean runs the syntactic steps 1 to 6 of SDD-007 section 7.1 for the given
// operating system ("windows" or anything else for Unix rules). It returns
// the path rewritten with single separators and without "." components, or
// an *Error. It touches no file, so it behaves the same on every host, which
// is what the table test and the fuzz test rely on.
func Clean(path, goos string) (string, error) {
	windows := goos == "windows"

	if path == "" || len(path) > maxPathBytes || !utf8.ValidString(path) {
		return "", refuse(
			"the path is empty, too long or not valid text.",
			"Pass a plain absolute file path of at most 4096 bytes.",
		)
	}

	for _, r := range path {
		if r < 0x20 || r == 0x7f {
			return "", refuse("the path has a control character.", hintPlain)
		}
	}

	if schemeRe.MatchString(path) {
		return "", refuse("the path looks like a URL; only files are imported.", hintPlain)
	}

	if windows {
		return cleanWindows(path)
	}

	return cleanUnix(path)
}

func cleanUnix(path string) (string, error) {
	if !strings.HasPrefix(path, "/") {
		return "", refuse(
			"the path is not absolute.",
			"Pass an absolute path; the service never resolves relative paths.",
		)
	}

	var out []string

	for c := range strings.SplitSeq(path, "/") {
		switch c {
		case "", ".":
		case "..":
			return "", refuse("the path has a \"..\" component.", "Pass the path without \"..\".")
		default:
			out = append(out, c)
		}
	}

	return "/" + strings.Join(out, "/"), nil
}

func cleanWindows(path string) (string, error) {
	if strings.HasPrefix(path, `\\`) || strings.HasPrefix(path, "//") || strings.HasPrefix(path, `\??\`) ||
		strings.HasPrefix(path, `/\`) || strings.HasPrefix(path, `\/`) {
		return "", refuse("network, device and extended paths are not accepted.", hintPlain)
	}

	if len(path) < 3 || !isLetter(path[0]) || path[1] != ':' || (path[2] != '\\' && path[2] != '/') {
		return "", refuse("the path is not absolute (it must start with a drive letter and a separator).", hintPlain)
	}

	drive := strings.ToUpper(path[:1])

	var out []string

	for _, c := range strings.FieldsFunc(path[3:], func(r rune) bool { return r == '\\' || r == '/' }) {
		switch {
		case c == ".":
			continue
		case c == "..":
			return "", refuse("the path has a \"..\" component.", "Pass the path without \"..\".")
		case strings.ContainsAny(c, `:<>"|?*`):
			return "", refuse("a component has a character Windows does not allow in a file name.", hintPlain)
		case strings.HasSuffix(c, ".") || strings.HasSuffix(c, " "):
			return "", refuse(
				"a component ends in a dot or a space.",
				"Rename the file so that no name ends in a dot or a space.",
			)
		case isReserved(c):
			return "", refuse("a component is a reserved device name.", "Rename the file.")
		}

		out = append(out, c)
	}

	return drive + `:\` + strings.Join(out, `\`), nil
}

func isLetter(b byte) bool { return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' }

func isReserved(component string) bool {
	stem, _, _ := strings.Cut(component, ".")

	return slices.Contains(reserved, strings.ToUpper(strings.TrimRight(stem, " ")))
}

// Config is what the guard needs from the process.
type Config struct {
	// Network is true in network mode (SDD-007 section 4): failures after
	// step 6 are all the same refusal, and an empty Roots refuses every path.
	Network bool
	// Roots is VUKEM_VIDEO_IMPORT_ROOTS. Each is canonicalised with steps 1
	// to 7 by New.
	Roots []string
	// Workspace is the workspace root: no root may lie inside it and no file
	// inside it is imported.
	Workspace string
	// GOOS is the operating system whose path syntax applies; empty is the host.
	GOOS string
}

// Guard opens import paths.
type Guard struct {
	network   bool
	goos      string
	roots     []string
	workspace string
	foldCase  bool
}

// New canonicalises the roots and the workspace. A root that cannot be
// resolved, or that lies inside the workspace, is an error: the operator
// must fix the configuration (SDD-007 section 7.1 step 8).
func New(cfg Config) (*Guard, error) {
	goos := cfg.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}

	g := &Guard{network: cfg.Network, goos: goos, foldCase: goos == "windows" || goos == "darwin"}

	if cfg.Workspace != "" {
		ws, err := filepath.Abs(cfg.Workspace)
		if err != nil {
			return nil, fmt.Errorf("pathguard: workspace: %w", err)
		}

		if resolved, rerr := filepath.EvalSymlinks(ws); rerr == nil {
			ws = resolved
		}

		g.workspace = ws
	}

	for _, r := range cfg.Roots {
		resolved, err := g.resolve(r)
		if err != nil {
			return nil, fmt.Errorf("pathguard: import root %q: %w", r, err)
		}

		if g.workspace != "" && g.within(g.workspace, resolved) {
			return nil, fmt.Errorf("pathguard: import root %q lies inside the workspace", r)
		}

		g.roots = append(g.roots, resolved)
	}

	return g, nil
}

// resolve is steps 1 to 7: Clean, then the real path.
func (g *Guard) resolve(path string) (string, error) {
	cleaned, err := Clean(path, g.goos)
	if err != nil {
		return "", err
	}

	resolved, err := filepath.EvalSymlinks(filepath.FromSlash(cleaned))
	if err != nil {
		return "", err
	}

	return resolved, nil
}

// within reports whether path is dir or lies under it, comparing component by
// component so that /media2 is not inside /media.
func (g *Guard) within(dir, path string) bool {
	d, p := splitParts(dir), splitParts(path)
	if len(p) < len(d) {
		return false
	}

	for i := range d {
		if g.foldCase && strings.EqualFold(d[i], p[i]) || d[i] == p[i] {
			continue
		}

		return false
	}

	return true
}

func splitParts(p string) []string {
	return strings.FieldsFunc(filepath.ToSlash(p), func(r rune) bool { return r == '/' })
}

// Roots are the canonical import roots. They are server paths: a caller that is
// not an administrator must never be shown them.
func (g *Guard) Roots() []string { return slices.Clone(g.roots) }

// OpenImport is steps 1 to 10: it returns the file opened for reading, which
// the caller must close. Every later step uses the handle, never the path.
func (g *Guard) OpenImport(path string) (*os.File, error) {
	return g.open(path, false)
}

// ownDirs are the folders of the workspace where the server itself writes the files it exports: project
// bundles and template files. They hold no secret, unlike the rest of the workspace.
var ownDirs = []string{"exports", "templates"}

// OpenExported is OpenImport for the import of a bundle or a template: it also accepts a file that the
// server wrote under exports/ or templates/ of the workspace, so the round trip needs no copy (VV-081).
// A medium, a font and every other path of the workspace stay refused. It applies in local mode only: a
// network server shows no path of its workspace, and its import roots are the rule.
func (g *Guard) OpenExported(path string) (*os.File, error) {
	return g.open(path, true)
}

func (g *Guard) open(path string, exported bool) (*os.File, error) {
	if _, err := Clean(path, g.goos); err != nil {
		return nil, err
	}

	if g.network && len(g.roots) == 0 {
		return nil, refuse("this server has no import roots.", "Ask the operator to set VUKEM_VIDEO_IMPORT_ROOTS.")
	}

	resolved, err := g.resolve(path)
	if err != nil {
		return nil, g.late(err)
	}

	ours := exported && g.exportedByUs(resolved)

	if g.workspace != "" && g.within(g.workspace, resolved) && !ours {
		return nil, g.late(refuse("the file lies inside the workspace.", hintInside))
	}

	var root string

	if len(g.roots) > 0 && !ours {
		i := slices.IndexFunc(g.roots, func(r string) bool { return g.within(r, resolved) })
		if i < 0 {
			return nil, g.late(refuse("the file is outside the import roots.", "Move the file under an import root."))
		}

		root = g.roots[i]
	}

	before, err := os.Lstat(resolved)
	if err != nil {
		return nil, g.late(err)
	}

	if !before.Mode().IsRegular() {
		return nil, g.late(refuse("the path is not a regular file.", "Import a file, not a directory or a device."))
	}

	f, err := openUnder(root, resolved)
	if err != nil {
		return nil, g.late(err)
	}

	after, err := f.Stat()
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(before, after) {
		_ = f.Close()

		return nil, g.late(refuse("the file changed while it was opened.", "Try again."))
	}

	return f, nil
}

// exportedByUs reports whether resolved lies under exports/ or templates/ of the workspace of a local server.
func (g *Guard) exportedByUs(resolved string) bool {
	if g.network || g.workspace == "" {
		return false
	}

	for _, d := range ownDirs {
		if g.within(filepath.Join(g.workspace, d), resolved) {
			return true
		}
	}

	return false
}

// openUnder opens real inside root with os.Root, so that a link swapped after
// the resolution cannot lead out of it, or with os.Open when there is no root.
func openUnder(root, resolved string) (*os.File, error) {
	if root == "" {
		return os.Open(resolved)
	}

	r, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	// within has matched root against resolved component by component (folding
	// case where the volume does), so the rest is the path under root; filepath.Rel
	// would compare the case and climb out with ".." on macOS.
	rel := filepath.Join(splitParts(resolved)[len(splitParts(root)):]...)
	if rel == "" {
		rel = "."
	}

	return r.Open(rel)
}

// late classifies a failure of steps 7 to 10 (SDD-007 section 7.3): in local
// mode a missing or unreadable file is E_FILE_NOT_FOUND and the rest keep their
// code; in network mode every one is the same E_PATH_NOT_ALLOWED, so that the
// answer does not map the server's file system.
func (g *Guard) late(err error) *Error {
	if g.network {
		return refuse("the file cannot be imported.", "Use a path under an import root of this server.")
	}

	if pe, ok := errors.AsType[*Error](err); ok {
		return pe
	}

	return &Error{
		Code: CodeFileNotFound, Message: "the file does not exist or cannot be read.",
		Hint: "Check the path and the permissions of the file.",
	}
}
