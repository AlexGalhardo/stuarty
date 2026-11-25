package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// EnvAllowDefaultHome lets a build that is not a release, and a test, open the default workspace
// (~/.stuarty). Only the value 1 counts (SDD-000 section 10, SDD-007 section 3.10).
const EnvAllowDefaultHome = "STUARTY_ALLOW_DEFAULT_HOME"

// Release is the version a release build stamps with
// -X github.com/AlexGalhardo/stuarty/service/workspace.Release=<X.Y.Z> (goreleaser; the desktop shell sets it
// from its own stamped version, main.version). A binary from `go build`, `go run`, `go install` or
// `go test` leaves it empty, and so does one stamped with a development version ("dev", "0.56.0-dev"):
// it is not a release, and it does not open the default workspace (VV-421).
var Release string

// underTest is the seam of the test binary check, replaced only by tests.
var underTest = testing.Testing

// ReleasedBuild reports whether this binary was built as a release.
func ReleasedBuild() bool {
	return !underTest() && releaseVersion(Release)
}

// releaseVersion accepts a version that starts with a digit and names no development build: "dev",
// "devel", "0.56.0-dev" and the empty string are not releases.
func releaseVersion(v string) bool {
	if v == "" || v[0] < '0' || v[0] > '9' {
		return false
	}
	return !strings.Contains(strings.ToLower(v), "dev")
}

// guardDefaultHome refuses the default workspace of home, and the folder of the former name, to a build that
// is not a release and to every test, so that a development binary never migrates the workspace of the
// installed application (VV-421). The refusal names the way out: another folder in STUARTY_HOME.
func guardDefaultHome(getenv func(string) string, abs, home string) error {
	if getenv(EnvAllowDefaultHome) == "1" || ReleasedBuild() {
		return nil
	}
	def := filepath.Join(home, defaultDir)
	if !samePath(abs, def) && !samePath(abs, filepath.Join(home, formerDir)) {
		return nil
	}
	return &Error{
		Code: CodeInvalidState,
		Message: fmt.Sprintf("this build is not a release, so it does not open the default workspace %s; "+
			"set %s to another folder (a test or a throw-away run), or %s=1 to open the default one on purpose",
			def, EnvHome, EnvAllowDefaultHome),
	}
}

// samePath reports whether the folder a is the folder b. The names are compared first; then the folders
// themselves, so that a symbolic link, a junction, an 8.3 short name or another spelling of b is caught;
// then, for a b that does not exist yet, their parents and their last names.
func samePath(a, b string) bool {
	if sameName(a, b) || sameFile(a, b) {
		return true
	}
	return sameName(filepath.Base(a), filepath.Base(b)) && sameFile(filepath.Dir(a), filepath.Dir(b))
}

// sameName compares two names the way the file system does: Windows and macOS ignore the case, and
// Windows drops the dots and spaces that end a name.
func sameName(a, b string) bool {
	switch runtime.GOOS {
	case "windows":
		return strings.EqualFold(strings.TrimRight(a, ". "), strings.TrimRight(b, ". "))
	case "darwin":
		return strings.EqualFold(a, b)
	default:
		return a == b
	}
}

func sameFile(a, b string) bool {
	ai, aerr := os.Stat(a)
	bi, berr := os.Stat(b)
	return aerr == nil && berr == nil && os.SameFile(ai, bi)
}
