package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// iss is the Inno Setup script of the Windows installer, which this package cannot run (the compiler is not
// on every machine): the test reads it, so the key and the value that the uninstaller removes are the ones
// the application writes.
func iss(t *testing.T) string {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join("..", "packaging", "windows", "stuarty-desktop.iss"))
	if err != nil {
		t.Fatal(err)
	}

	return strings.ReplaceAll(string(raw), "\r\n", "\n")
}

// The uninstaller of Windows removes the registration of the start at sign-in (SDD-012 section 21.3 item 7,
// epic 21 acceptance 4): by the [Registry] section and by a command of [UninstallRun], for the same key and
// the same value the shell writes, and the installer itself never creates it.
func TestInstallerRemovesTheStartAtSignIn(t *testing.T) {
	t.Parallel()

	src := iss(t)

	// The key and the value the application writes are the ones the script removes.
	for _, want := range []string{
		`Root: HKCU; Subkey: "` + RunSubKey + `"; ValueName: "` + RunValueName +
			`"; ValueType: none; Flags: dontcreatekey uninsdeletevalue`,
		`delete ""HKCU\` + RunSubKey + `"" /v ` + RunValueName + ` /f`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("the installer script lacks %q", want)
		}
	}

	// Per user, and no administrator rights: nothing of the installer registers a machine-wide start.
	for _, banned := range []string{"HKLM", "HKCR", "Root: HKA"} {
		if strings.Contains(src, banned+";") {
			t.Errorf("the installer script writes %s, the start at sign-in is for the user alone", banned)
		}
	}

	if !strings.Contains(src, "PrivilegesRequired=lowest") {
		t.Error("the installer must stay per user")
	}

	// The resident application, which may have a hidden window, is stopped before its files go.
	stop := `taskkill.exe"; Parameters: "/F /IM {#AppExe}"; Flags: runhidden; RunOnceId: "StopDesktop"`
	if !strings.Contains(src, stop) {
		t.Error("the uninstaller does not stop the resident application")
	}
}
