package shell

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/desktop/update"
)

func TestSettingsDefaultToOff(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "desktop.json")

	s, err := LoadSettings(path)
	if err != nil || s.Asked() || s.AutoCheck() {
		t.Fatalf("missing file: %+v, %v", s, err)
	}

	if (Settings{}).Asked() || (Settings{}).AutoCheck() {
		t.Error("the zero value of the setting must be not asked and off")
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "desktop.json")

	if err := (Settings{}).WithAutoCheck(true).Save(path); err != nil {
		t.Fatal(err)
	}

	s, err := LoadSettings(path)
	if err != nil || !s.Asked() || !s.AutoCheck() {
		t.Fatalf("after save: %+v, %v", s, err)
	}

	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), `"update.check":true`) {
		t.Errorf("file %q lacks the setting key update.check", raw)
	}
}

func TestAnsweredNoIsAskedButOff(t *testing.T) {
	path := filepath.Join(t.TempDir(), "desktop.json")

	if err := (Settings{}).WithAutoCheck(false).Save(path); err != nil {
		t.Fatal(err)
	}

	s, err := LoadSettings(path)
	if err != nil || !s.Asked() || s.AutoCheck() {
		t.Fatalf("after No: %+v, %v", s, err)
	}
}

func TestDamagedSettingsStayOff(t *testing.T) {
	path := filepath.Join(t.TempDir(), "desktop.json")
	if err := os.WriteFile(path, []byte(`{"update.check": tru`), 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := LoadSettings(path)
	if err == nil || s.AutoCheck() || s.Asked() {
		t.Errorf("damaged file: %+v, %v", s, err)
	}
}

type fakeChecker struct {
	offer *update.Offer
	err   error
	calls int
}

func (f *fakeChecker) CheckNow(context.Context) (*update.Offer, error) {
	f.calls++

	return f.offer, f.err
}

func TestCheckForUpdatesWithoutAClientSaysNotConfigured(t *testing.T) {
	// update.New returns ErrNotConfigured for the placeholder feed; the menu shows it as text.
	_, buildErr := update.New(update.Config{FeedURL: update.FeedURL})
	if !errors.Is(buildErr, update.ErrNotConfigured) {
		t.Fatalf("the placeholder build must be not configured, got %v", buildErr)
	}

	for _, err := range []error{buildErr, nil} {
		out := CheckForUpdates(context.Background(), "en", nil, err, "0.15.0")
		if out.Offer != nil || !strings.Contains(out.Message, "no update feed or signing key") ||
			!strings.Contains(out.Message, ReleasePageURL) {
			t.Errorf("message %q", out.Message)
		}
	}
}

func TestCheckForUpdatesOutcomes(t *testing.T) {
	offer := &update.Offer{Version: "0.16.0"}

	cases := []struct {
		name  string
		check *fakeChecker
		want  string
		offer bool
	}{
		{"new version", &fakeChecker{offer: offer}, "Version 0.16.0 is available", true},
		{
			"with notes",
			&fakeChecker{offer: &update.Offer{Version: "0.16.0", Notes: "Faster export"}},
			"install it?\n\nFaster export",
			true,
		},
		{"up to date", &fakeChecker{err: fmt.Errorf("%w: offered 0.15.0", update.ErrNotNewer)}, "is up to date", false},
		{
			"failure",
			&fakeChecker{err: errors.New("update: feed: 503 Service Unavailable")},
			"Could not check for updates: update: feed: 503",
			false,
		},
	}
	for _, c := range cases {
		out := CheckForUpdates(context.Background(), "en", c.check, nil, "0.15.0")
		if !strings.Contains(out.Message, c.want) || (out.Offer != nil) != c.offer || c.check.calls != 1 {
			t.Errorf("%s: %+v", c.name, out)
		}
	}
}

func TestInstallerIsChosenPerGOOS(t *testing.T) {
	quit := func() {}

	if _, ok := InstallerFor("windows", quit).(setupInstaller); !ok {
		t.Error("windows must run the setup program")
	}

	for _, goos := range []string{"darwin", "linux", "freebsd"} {
		inst := InstallerFor(goos, quit)

		err := inst.Install(context.Background(), update.Artifact{Path: "x", Version: "0.16.0"})
		if !errors.Is(err, ErrInstallUnsupported) || !strings.Contains(err.Error(), ReleasePageURL) {
			t.Errorf("%s: %v", goos, err)
		}
	}
}

func TestSetupInstallerQuitsOnlyAfterTheSetupStarted(t *testing.T) {
	quits := 0
	art := update.Artifact{Path: "setup.exe"}

	ok := setupInstaller{run: func(string) error { return nil }, quit: func() { quits++ }}
	if err := ok.Install(context.Background(), art); err != nil || quits != 1 {
		t.Errorf("started: %v, quits %d", err, quits)
	}

	boom := errors.New("access denied")
	bad := setupInstaller{run: func(string) error { return boom }, quit: func() { quits++ }}

	if err := bad.Install(context.Background(), art); !errors.Is(err, boom) || quits != 1 {
		t.Errorf("failed: %v, quits %d", err, quits)
	}
}

func TestUpdateMessagesFollowTheLanguage(t *testing.T) {
	out := CheckForUpdates(context.Background(), "pt-BR", nil, update.ErrNotConfigured, "0.15.0")
	if out.Title != "Verificar atualizações" || !strings.Contains(out.Message, "não tem endereço de atualizações") {
		t.Errorf("title %q message %q", out.Title, out.Message)
	}

	if got := InstallFailedMessage(
		"pt-BR",
		errors.New("disk"),
	); !strings.HasPrefix(
		got,
		"A atualização não foi instalada",
	) {
		t.Errorf("message %q", got)
	}
}

// TestSettingsOfTheFormerNameAreAdopted: the answer a person gave to Vukem Video about updates is read from
// its folder once and kept in the new one (ADR-0044); the former file is left in place, and an answer
// already recorded under the new name wins.
func TestSettingsOfTheFormerNameAreAdopted(t *testing.T) {
	config := t.TempDir()
	t.Setenv("APPDATA", config)
	t.Setenv("XDG_CONFIG_HOME", config)
	t.Setenv("HOME", config)

	base, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}

	former := filepath.Join(base, "vukem-video", "desktop.json")
	if err := (Settings{}).WithAutoCheck(true).Save(former); err != nil {
		t.Fatal(err)
	}

	path, err := SettingsPath()
	if err != nil {
		t.Fatal(err)
	}

	if want := filepath.Join(base, "stuarty", "desktop.json"); path != want {
		t.Fatalf("SettingsPath = %s, want %s", path, want)
	}

	if s, err := LoadSettings(path); err != nil || !s.AutoCheck() {
		t.Errorf("adopted settings = %+v, %v, want the answer yes", s, err)
	}

	if _, err := os.Stat(former); err != nil {
		t.Errorf("the former file was removed: %v", err)
	}

	// The person changes the answer under the new name; reading the path again must not bring the old one back.
	if err := (Settings{}).WithAutoCheck(false).Save(path); err != nil {
		t.Fatal(err)
	}

	if _, err := SettingsPath(); err != nil {
		t.Fatal(err)
	}

	if s, err := LoadSettings(path); err != nil || s.AutoCheck() || !s.Asked() {
		t.Errorf("settings = %+v, %v, want the new answer no", s, err)
	}
}

// TestSettingsOfTheFormerNameThatAreDamagedAreIgnored: a former file that cannot be read gives the defaults.
func TestSettingsOfTheFormerNameThatAreDamagedAreIgnored(t *testing.T) {
	config := t.TempDir()
	t.Setenv("APPDATA", config)
	t.Setenv("XDG_CONFIG_HOME", config)
	t.Setenv("HOME", config)

	base, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}

	former := filepath.Join(base, "vukem-video", "desktop.json")
	if err := os.MkdirAll(filepath.Dir(former), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(former, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	path, err := SettingsPath()
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(path); err == nil {
		t.Error("a damaged former file was copied")
	}

	if s, err := LoadSettings(path); err != nil || s.Asked() {
		t.Errorf("settings = %+v, %v, want the defaults", s, err)
	}
}
