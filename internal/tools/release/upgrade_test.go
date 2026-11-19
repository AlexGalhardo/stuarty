package main

import (
	"strings"
	"testing"
)

// TestInstallersUpgradeFormerName asserts that the installers upgrade an installed Vukem Video instead of
// leaving it running and installed (ADR-0044, SDD-009 section 10.5). ISCC, pkgbuild and dpkg do not run
// here, so the scripts are read.
func TestInstallersUpgradeFormerName(t *testing.T) {
	t.Parallel()

	has := func(t *testing.T, file string, wants ...string) {
		t.Helper()

		text := repoFile(t, file)
		for _, want := range wants {
			if !strings.Contains(text, want) {
				t.Errorf("%s does not contain %q", file, want)
			}
		}
	}

	t.Run("windows server installer", func(t *testing.T) {
		t.Parallel()
		has(
			t,
			".config/installers/windows/stuarty.iss",
			`/F /IM {#FormerExe}`,
			`DestName: "{#FormerExe}"`,
			"[InstallDelete]",
			`Name: "{app}\vukem-video.ico"`,
			`Name: "{autoprograms}\Vukem Video.lnk"`,
			`Name: "{autodesktop}\Vukem Video.lnk"`,
		)
	})

	t.Run("windows desktop installer", func(t *testing.T) {
		t.Parallel()
		has(t, "desktop/packaging/windows/stuarty-desktop.iss",
			`/F /IM {#FormerAppExe}`, `/F /IM {#FormerServerExe}`, "[InstallDelete]",
			`Name: "{app}\{#FormerAppExe}"`, `Name: "{app}\{#FormerServerExe}"`,
			`Name: "{autoprograms}\Vukem Video Desktop.lnk"`, `Name: "{autodesktop}\Vukem Video Desktop.lnk"`)
	})

	t.Run("macos package", func(t *testing.T) {
		t.Parallel()
		has(t, ".config/installers/macos/build-pkg.sh", "--scripts", "postinstall")
		has(
			t,
			".config/installers/macos/postinstall",
			"pkill -x vukem-video",
			"ln -sf stuarty /usr/local/bin/vukem-video",
			`rm -rf "/Applications/Vukem Video.app"`,
		)
	})

	t.Run("linux packages", func(t *testing.T) {
		t.Parallel()
		has(t, ".config/goreleaser.yaml", "dst: /usr/bin/vukem-video", "type: symlink")
		has(t, "desktop/packaging/linux/build-deb.sh", `"$root/usr/bin/vukem-video-desktop"`)
	})

	t.Run("no installer deletes a workspace", func(t *testing.T) {
		t.Parallel()

		for _, file := range []string{
			".config/installers/windows/stuarty.iss", "desktop/packaging/windows/stuarty-desktop.iss",
			".config/installers/macos/postinstall",
		} {
			var code []string

			for line := range strings.SplitSeq(repoFile(t, file), "\n") {
				if trimmed := strings.TrimSpace(
					line,
				); !strings.HasPrefix(trimmed, ";") &&
					!strings.HasPrefix(trimmed, "#") {
					code = append(code, strings.ToLower(line))
				}
			}

			text := strings.Join(code, "\n")
			for _, bad := range []string{".vukem-video", ".stuarty", "filesandordirs", "dirifempty"} {
				if strings.Contains(text, bad) {
					t.Errorf("%s has a line that names a workspace or deletes a folder: %q", file, bad)
				}
			}
		}
	})
}
