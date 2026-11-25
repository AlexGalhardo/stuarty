package workspace

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// allowDefaultHome is the environment of a test whose HOME is a temporary folder: opening that default
// is safe, so it says so.
func allowDefaultHome(k string) string {
	if k == EnvAllowDefaultHome {
		return "1"
	}

	return ""
}

// asBuild makes the barrier see a binary outside a test, stamped with the given release version.
func asBuild(t *testing.T, release string) {
	t.Helper()

	oldRelease, oldTest := Release, underTest
	t.Cleanup(func() { Release, underTest = oldRelease, oldTest })

	Release = release
	underTest = func() bool { return false }
}

func tempHome(t *testing.T) string {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	return home
}

func noEnv(string) string { return "" }

func TestResolveRefusesTheDefaultHomeToABuildThatIsNotARelease(t *testing.T) {
	for name, release := range map[string]string{
		"no stamp":        "",
		"the desktop dev": "dev",
		"a dev stamp":     "0.56.0-dev",
		"a not-a-version": "(devel)",
		"a leading v":     "v0.55.0",
	} {
		t.Run(name, func(t *testing.T) {
			home := tempHome(t)
			asBuild(t, release)

			_, err := Resolve(noEnv)

			var we *Error
			if !errors.As(err, &we) || we.Code != CodeInvalidState {
				t.Fatalf("Resolve error = %v, want %s", err, CodeInvalidState)
			}

			if !strings.Contains(err.Error(), EnvHome) || !strings.Contains(err.Error(), EnvAllowDefaultHome) {
				t.Errorf("the message does not say how to choose a folder: %v", err)
			}

			if _, statErr := os.Stat(filepath.Join(home, defaultDir)); statErr == nil {
				t.Error("the refused default home was created")
			}
		})
	}
}

func TestResolveRefusesAnExplicitHomeThatIsTheDefault(t *testing.T) {
	home := tempHome(t)
	asBuild(t, "")

	_, err := Resolve(func(k string) string {
		if k == EnvHome {
			return filepath.Join(home, defaultDir)
		}

		return ""
	})
	if err == nil {
		t.Fatal("Resolve opened the default home through an explicit path")
	}
}

// linkTo makes link name the folder target: a junction on Windows, which needs no privilege, and a
// symbolic link elsewhere.
func linkTo(t *testing.T, target, link string) {
	t.Helper()

	if runtime.GOOS == "windows" {
		if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
			t.Fatalf("mklink /J: %v: %s", err, out)
		}

		return
	}

	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

func homeAt(path string) func(string) string {
	return func(k string) string {
		if k == EnvHome {
			return path
		}

		return ""
	}
}

// TestResolveRefusesTheDefaultHomeUnderAnotherName: the barrier compares folders, not strings, so another
// spelling of the default workspace, a link to it or a link to the folder that holds it is refused too.
func TestResolveRefusesTheDefaultHomeUnderAnotherName(t *testing.T) {
	home := tempHome(t)
	asBuild(t, "")

	def := filepath.Join(home, defaultDir)
	parentLink := filepath.Join(t.TempDir(), "home-link")
	linkTo(t, home, parentLink)

	// The default does not exist yet: only the folder that holds it can be compared.
	for _, p := range []string{def + string(filepath.Separator), filepath.Join(parentLink, defaultDir)} {
		if _, err := Resolve(homeAt(p)); err == nil {
			t.Errorf("STUARTY_HOME=%s opened the default home before it existed", p)
		}
	}

	if err := os.Mkdir(def, 0o700); err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(t.TempDir(), "ws-link")
	linkTo(t, def, link)
	t.Chdir(home) // for the relative name

	names := []string{link, filepath.Join(parentLink, defaultDir), "." + string(filepath.Separator) + defaultDir}
	if runtime.GOOS == "windows" {
		names = append(names, strings.ToUpper(def), def+".")
	}

	for _, p := range names {
		if _, err := Resolve(homeAt(p)); err == nil {
			t.Errorf("STUARTY_HOME=%s opened the default home", p)
		}
	}
}

// TestResolveRefusesTheFormerFolderNamedExplicitly: the folder of the former name is the workspace of an
// installed Vukem Video; a build that is not a release does not migrate it through STUARTY_HOME either.
func TestResolveRefusesTheFormerFolderNamedExplicitly(t *testing.T) {
	home := tempHome(t)
	asBuild(t, "")
	former := makeFormer(t, home)

	if _, err := Resolve(homeAt(former)); err == nil {
		t.Fatal("STUARTY_HOME naming the former folder opened it")
	}
}

func TestResolveDoesNotAdoptTheFormerWorkspaceWhenItRefuses(t *testing.T) {
	home := tempHome(t)
	asBuild(t, "")
	former := makeFormer(t, home)

	if _, err := Resolve(noEnv); err == nil {
		t.Fatal("Resolve opened the default home")
	}

	if _, err := os.Stat(former); err != nil {
		t.Errorf("the former workspace was touched: %v", err)
	}

	if _, err := os.Stat(filepath.Join(home, defaultDir)); err == nil {
		t.Error("a refused build created the default home")
	}
}

func TestResolveOpensAnExplicitHomeInABuildThatIsNotARelease(t *testing.T) {
	tempHome(t)
	asBuild(t, "")

	want := filepath.Join(t.TempDir(), "mine")

	d, err := Resolve(func(k string) string {
		if k == EnvHome {
			return want
		}

		return ""
	})
	if err != nil || string(d) != want {
		t.Fatalf("Resolve = %q, %v, want %q", d, err, want)
	}
}

func TestResolveOpensTheDefaultHomeWhenTheAllowVariableIsOne(t *testing.T) {
	home := tempHome(t)
	asBuild(t, "")

	for _, v := range []string{"0", "true", "yes", ""} {
		_, err := Resolve(func(k string) string {
			if k == EnvAllowDefaultHome {
				return v
			}

			return ""
		})
		if err == nil {
			t.Errorf("%s=%q opened the default home", EnvAllowDefaultHome, v)
		}
	}

	d, err := Resolve(allowDefaultHome)
	if err != nil || string(d) != filepath.Join(home, defaultDir) {
		t.Fatalf("Resolve = %q, %v, want the default home", d, err)
	}
}

func TestResolveOpensTheDefaultHomeInAReleasedBuild(t *testing.T) {
	home := tempHome(t)
	asBuild(t, "0.55.0")

	d, err := Resolve(noEnv)
	if err != nil || string(d) != filepath.Join(home, defaultDir) {
		t.Fatalf("Resolve = %q, %v, want the default home", d, err)
	}
}

// TestTheTestBinaryIsNeverARelease: a `go test` run refuses the default home even when a release version
// is stamped, so that no test migrates the workspace of the person who runs it.
func TestTheTestBinaryIsNeverARelease(t *testing.T) {
	tempHome(t)

	old := Release
	t.Cleanup(func() { Release = old })

	Release = "0.55.0"

	if ReleasedBuild() {
		t.Fatal("the test binary reads as a release")
	}

	if _, err := Resolve(noEnv); err == nil {
		t.Fatal("a test opened the default home")
	}
}

// TestTheReleaseBuildStampsWhatTheBarrierReads keeps the barrier and goreleaser in step.
func TestTheReleaseBuildStampsWhatTheBarrierReads(t *testing.T) {
	b, err := os.ReadFile("../../.config/goreleaser.yaml")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(b), "github.com/AlexGalhardo/stuarty/service/workspace.Release={{ .Version }}") {
		t.Error("goreleaser.yaml does not stamp workspace.Release")
	}
}

func TestReleaseVersion(t *testing.T) {
	for v, want := range map[string]bool{
		"0.55.0": true, "1.0.0-rc.1": true, "": false, "dev": false, "devel": false,
		"0.56.0-dev": false, "v1.0.0": false, "(devel)": false,
	} {
		if got := releaseVersion(v); got != want {
			t.Errorf("releaseVersion(%q) = %v, want %v", v, got, want)
		}
	}
}

// TestSecurity_T164_ADevelopmentBuildNeverOpensTheDefaultWorkspace: neither a build without a release stamp
// nor a test binary opens (or creates, or adopts into) the default workspace unless the allow variable is 1.
func TestSecurity_T164_ADevelopmentBuildNeverOpensTheDefaultWorkspace(t *testing.T) {
	home := tempHome(t)
	former := makeFormer(t, home)

	asBuild(t, "")

	if _, err := Resolve(noEnv); err == nil {
		t.Fatal("a build with no release stamp opened the default workspace")
	}

	Release, underTest = "0.55.0", func() bool { return true }

	if _, err := Resolve(noEnv); err == nil {
		t.Fatal("a test binary opened the default workspace")
	}

	if _, err := os.Stat(filepath.Join(home, defaultDir)); err == nil {
		t.Error("a refusal created the default workspace")
	}

	if _, err := os.Stat(former); err != nil {
		t.Errorf("a refusal moved the former workspace: %v", err)
	}
}
