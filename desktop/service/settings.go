// Package service holds the settings and the registration of the background mode of the desktop
// shell (SDD-012 section 21.3 item 7, epic 21): whether Stuarty stays resident when its window is
// closed, and whether it starts, hidden, when the person signs in. It has no window and no framework:
// the registration with the operating system is behind the Registrar interface, so the logic is tested
// with a fake and nothing in a test touches the registration of the machine.
package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// BackgroundFlag is the argument the registration passes to the shell: start with the window hidden.
const BackgroundFlag = "--background"

// settingsFile is the name of the file in the state folder of the shell.
const settingsFile = "background.json"

// Settings are the choices of the person about the background mode. They describe this computer, so
// they live in the user config folder and not in the workspace.
type Settings struct {
	// KeepRunning keeps the shell, with the server and the scheduler, running behind a tray icon when
	// the window is closed while a post is scheduled. Default off.
	KeepRunning bool `json:"keep_running"`
	// StartAtSignIn registers the shell to start, hidden, when the person signs in. It needs KeepRunning.
	StartAtSignIn bool `json:"start_at_sign_in"`
	// Asked says that the first-time dialog was answered, so it is never shown again.
	Asked bool `json:"asked"`
}

// Normalize returns the settings with the rule that starting at sign-in needs staying resident.
func (s Settings) Normalize() Settings {
	if !s.KeepRunning {
		s.StartAtSignIn = false
	}

	return s
}

// Path is the file of the settings inside the state folder dir.
func Path(dir string) string { return filepath.Join(dir, settingsFile) }

// Load reads the settings of dir. A missing or unreadable file is the default (everything off, not
// asked): a damaged file must never turn a resident process on.
func Load(dir string) Settings {
	raw, err := os.ReadFile(Path(dir))
	if err != nil {
		return Settings{}
	}

	var s Settings
	if json.Unmarshal(raw, &s) != nil {
		return Settings{}
	}

	return s.Normalize()
}

// Save writes the settings of dir, through a temporary file so a crash never leaves half of one.
func Save(dir string, s Settings) error {
	s = s.Normalize()

	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("background settings: %w", err)
	}

	tmp, err := os.CreateTemp(dir, settingsFile+".*.tmp")
	if err != nil {
		return fmt.Errorf("background settings: %w", err)
	}

	name := tmp.Name()
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)

		return fmt.Errorf("background settings: %w", err)
	}

	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)

		return fmt.Errorf("background settings: %w", err)
	}

	if err := os.Rename(name, Path(dir)); err != nil {
		_ = os.Remove(name)

		return fmt.Errorf("background settings: %w", err)
	}

	return nil
}

// Dir is the folder of the background settings: the user config folder of the shell, never the workspace.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("background settings: %w", err)
	}

	return filepath.Join(base, "stuarty"), nil
}
