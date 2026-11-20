package shell

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/AlexGalhardo/stuarty/desktop/update"
)

// ReleasePageURL is where a person downloads a version the shell cannot install by itself.
const ReleasePageURL = "https://github.com/AlexGalhardo/stuarty/releases"

// Settings are the preferences of the desktop shell. They are not workspace data: the update client
// never touches the workspace (SDD-009 section 10.5 item 7), so they live in the user config directory.
type Settings struct {
	// UpdateCheck is the setting update.check (SDD-009 section 10.5): nil means the person was not asked
	// yet, true means check at every start and every 24 hours, false means only on request.
	UpdateCheck *bool `json:"update.check,omitempty"`
}

// Asked reports whether the person already answered the first-start question.
func (s Settings) Asked() bool { return s.UpdateCheck != nil }

// AutoCheck reports whether the person agreed to the automatic checks. Not asked yet is false.
func (s Settings) AutoCheck() bool { return s.UpdateCheck != nil && *s.UpdateCheck }

// WithAutoCheck returns the settings with the answer recorded.
func (s Settings) WithAutoCheck(on bool) Settings {
	s.UpdateCheck = &on

	return s
}

// SettingsPath is the settings file of the shell, in the user config directory.
func SettingsPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("shell settings: %w", err)
	}

	path := filepath.Join(dir, "stuarty", "desktop.json")
	adoptFormerSettings(filepath.Join(dir, "vukem-video", "desktop.json"), path)

	return path, nil
}

// adoptFormerSettings copies the settings file that Vukem Video wrote (ADR-0044) to its new place, once, so
// that the answer to the first-start question about updates is not asked again. The former file is left
// where it is. A missing or damaged former file changes nothing: the defaults ask again and connect to nothing.
func adoptFormerSettings(former, current string) {
	if _, err := os.Stat(current); err == nil {
		return
	}

	s, err := LoadSettings(former)
	if err != nil || !s.Asked() {
		return
	}

	_ = s.Save(current)
}

// LoadSettings reads the settings file. A missing file gives the defaults: not asked yet, so nothing
// connects. A file that cannot be read or parsed also gives the defaults, together with the error, so
// that a damaged file never turns the network on.
func LoadSettings(path string) (Settings, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Settings{}, nil
	}

	if err != nil {
		return Settings{}, fmt.Errorf("shell settings: %w", err)
	}

	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		return Settings{}, fmt.Errorf("shell settings: %s: %w", path, err)
	}

	return s, nil
}

// Save writes the settings file, creating its directory.
func (s Settings) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("shell settings: %w", err)
	}

	data, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("shell settings: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("shell settings: %w", err)
	}

	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)

		return fmt.Errorf("shell settings: %w", err)
	}

	return nil
}

// UpdateChecker is the part of update.Client the menu action uses.
type UpdateChecker interface {
	CheckNow(ctx context.Context) (*update.Offer, error)
}

// UpdateOutcome is what the "Check for updates" action shows. Offer is set only when a newer, verified
// version is ready: the shell then asks whether to install it.
type UpdateOutcome struct {
	Title   string
	Message string
	Offer   *update.Offer
}

// CheckForUpdates runs the check the person asked for and turns every result into text for a dialog.
// client is nil when the build could not create one; buildErr says why. It never fails: a missing
// feed, a network error and a bad signature each become a message.
func CheckForUpdates(
	ctx context.Context,
	language string,
	client UpdateChecker,
	buildErr error,
	current string,
) UpdateOutcome {
	title := Text(language, TextCheckUpdates)

	if client == nil {
		return failedOutcome(language, buildErr)
	}

	offer, err := client.CheckNow(ctx)

	switch {
	case err == nil:
		return UpdateOutcome{
			Title:   title,
			Message: OfferMessage(language, offer, current),
			Offer:   offer,
		}
	case errors.Is(err, update.ErrNotNewer):
		return UpdateOutcome{Title: title, Message: fmt.Sprintf(Text(language, TextUpToDate), current)}
	default:
		return failedOutcome(language, err)
	}
}

// OfferMessage is the text of the dialog that offers a verified newer version, with its release notes
// when the feed carries them.
func OfferMessage(language string, offer *update.Offer, current string) string {
	msg := fmt.Sprintf(Text(language, TextOffer), offer.Version, current)
	if notes := strings.TrimSpace(offer.Notes); notes != "" {
		msg += "\n\n" + notes
	}

	return msg
}

func failedOutcome(language string, err error) UpdateOutcome {
	title := Text(language, TextCheckUpdates)

	if err == nil || errors.Is(err, update.ErrNotConfigured) {
		return UpdateOutcome{Title: title, Message: fmt.Sprintf(Text(language, TextNoFeed), ReleasePageURL)}
	}

	return UpdateOutcome{Title: title, Message: Text(language, TextCheckFailed) + err.Error()}
}

// InstallFailedMessage is the text of the dialog shown when an accepted update could not be applied.
func InstallFailedMessage(language string, err error) string {
	return Text(language, TextInstallFailed) + err.Error()
}
