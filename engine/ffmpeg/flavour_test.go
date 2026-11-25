package ffmpeg

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func fakeBuild(t *testing.T, name string) VersionInfo {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "builds", name, "version.txt"))
	if err != nil {
		t.Fatal(err)
	}
	v, ok := ParseVersion("ffmpeg", string(b))
	if !ok {
		t.Fatalf("%s: version output not parsed", name)
	}
	return v
}

func TestFlavourRecognisesTheBuilds(t *testing.T) {
	cases := []struct {
		build string
		want  FlavourName
	}{
		{"own", FlavourOwn},
		{"btbn", FlavourBtbN},
	}
	for _, c := range cases {
		f := fakeBuild(t, c.build).Flavour()
		if f.Name != c.want || f.License.Name != "lgpl" || !f.License.Version3 || !f.LGPLOnly() {
			t.Errorf("%s: %+v", c.build, f)
		}
	}

	own := fakeBuild(t, "own").Flavour()
	if own.ExtraVersion != "vukem1" || own.LinksOpenH264 {
		t.Errorf("own build: %+v", own)
	}
	if !fakeBuild(t, "btbn").Flavour().LinksOpenH264 {
		t.Error("the BtbN build has OpenH264 compiled in")
	}

	user, _ := ParseVersion("ffmpeg", "ffmpeg version 9.0.2 Copyright\nconfiguration: --enable-gpl --enable-libx264")
	if f := user.Flavour(); f.Name != FlavourOther || !f.GPL || f.LGPLOnly() || len(f.Problems()) != 0 {
		t.Errorf("a user's FFmpeg: %+v", f)
	}
}

// TestOwnBuildHasNoGPLFlags is NFR-043 over the fake of the own build: the configure line has neither
// --enable-gpl, --enable-nonfree nor --enable-libopenh264, and a copy that does is flagged.
func ownBuildHasNoGPLFlags(t *testing.T) {
	t.Helper()

	v := fakeBuild(t, "own")
	for _, bad := range []string{"--enable-gpl", "--enable-nonfree", "--enable-libopenh264"} {
		if slices.Contains(v.Flags, bad) {
			t.Errorf("the fake of the own build has %s", bad)
		}
	}
	if p := v.Flavour().Problems(); len(p) != 0 {
		t.Errorf("problems: %v", p)
	}

	for _, bad := range []string{"--enable-gpl", "--enable-nonfree", "--enable-libopenh264"} {
		out := "ffmpeg version n9.0.2-vukem1 Copyright\nconfiguration: --enable-version3 " + bad + " --extra-version=vukem1\n"
		tampered, _ := ParseVersion("ffmpeg", out)
		if p := tampered.Flavour().Problems(); !slices.Equal(p, []string{bad}) {
			t.Errorf("%s: problems = %v", bad, p)
		}
	}
}

func TestPinnedBuildIsRecognised(t *testing.T) {
	f := pinned(t).Flavour()
	// The pin is the BtbN build until the own build is adopted (epic 46); either one must be clean.
	if (f.Name != FlavourBtbN && f.Name != FlavourOwn) || !f.LGPLOnly() || len(f.Problems()) != 0 {
		t.Errorf("the pinned build: %+v", f)
	}
}

func TestOwnBuildHasNoGPLFlags(t *testing.T)              { ownBuildHasNoGPLFlags(t) }
func TestSecurity_T27_OwnBuildHasNoGPLFlags(t *testing.T) { ownBuildHasNoGPLFlags(t) }

// TestFlavourAcceptsBothMarkersOfTheOwnBuild pins VV-231: the build ffmpeg-9.0.2-3 says stuarty<N>, the
// builds installed before it say vukem<N>, and both are the own build; any other marker is not.
func TestFlavourAcceptsBothMarkersOfTheOwnBuild(t *testing.T) {
	for marker, want := range map[string]FlavourName{
		"stuarty3": FlavourOwn, "vukem2": FlavourOwn, "other1": FlavourOther, "": FlavourOther,
	} {
		line := "configuration: --enable-version3 --disable-libx264"
		if marker != "" {
			line += " --extra-version=" + marker
		}
		v, ok := ParseVersion("ffmpeg", "ffmpeg version n9.0.2 Copyright\n"+line+"\n")
		if !ok {
			t.Fatalf("%q: not parsed", marker)
		}
		if got := v.Flavour().Name; got != want {
			t.Errorf("marker %q: flavour %s, want %s", marker, got, want)
		}
	}
}
