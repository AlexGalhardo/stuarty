package workspace

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"
)

// EnvAllowDefaultHome lets a build that is not a release, and a test, open the default workspace
// (~/.stuarty). Only the value 1 counts (SDD-000 section 10, SDD-007 section 13).
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

// guardDefaultHome refuses the default workspace to a build that is not a release and to every test, so
// that a development binary never migrates the workspace of the installed application (VV-421). The
// refusal names the way out: another folder in STUARTY_HOME.
func guardDefaultHome(getenv func(string) string, abs, def string) error {
	if getenv(EnvAllowDefaultHome) == "1" || ReleasedBuild() || !samePath(abs, def) {
		return nil
	}
	return &Error{
		Code: CodeInvalidState,
		Message: fmt.Sprintf("this build is not a release, so it does not open the default workspace %s; "+
			"set %s to another folder (a test or a throw-away run), or %s=1 to open the default one on purpose",
			def, EnvHome, EnvAllowDefaultHome),
	}
}

func samePath(a, b string) bool {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return strings.EqualFold(a, b)
	}
	if a == b {
		return true
	}
	ai, aerr := os.Stat(a)
	bi, berr := os.Stat(b)
	return aerr == nil && berr == nil && os.SameFile(ai, bi)
}
