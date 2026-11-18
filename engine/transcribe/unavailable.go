package transcribe

import (
	"fmt"
	"maps"
	"net/url"
	"strings"

	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg"
)

// Reasons of E_TRANSCRIBE_UNAVAILABLE (details.reason).
const (
	ReasonChecksumMismatch = "checksum_mismatch"
	ReasonSizeMismatch     = "size_mismatch"
	ReasonNotPublished     = "not_published"
	ReasonDownloadRefused  = "download_refused"
	ReasonDigestMismatch   = "digest_mismatch"
	ReasonBadArchive       = "bad_archive"
	ReasonNotInstalled     = "not_installed"
)

// HelperName is the name of the whisper.cpp helper in the manifest and under tools/.
const (
	HelperName   = "whisper"
	DefaultModel = "small"
)

// Models are the model names of the manifest, the only names `--model` and VUKEM_VIDEO_WHISPER_MODEL accept.
var Models = []string{"tiny", "base", "small", "large-v3-turbo"}

// SupportedPlatforms are the operating systems and architectures the helper is installed for; the
// manifest has an entry for each (SDD-007 section 6.8 item 7, TestManifestHasEntryForEverySupportedPlatform).
var SupportedPlatforms = [][2]string{
	{"linux", "amd64"},
	{"linux", "arm64"},
	{"darwin", "amd64"},
	{"darwin", "arm64"},
	{"windows", "amd64"},
	{"windows", "arm64"},
}

func unavailable(reason, msg string, cause error, details map[string]any) *ffmpeg.Error {
	d := map[string]any{"reason": reason}
	maps.Copy(d, details)

	return &ffmpeg.Error{Code: CodeUnavailable, Reason: reason, Message: msg, Details: d, Err: cause}
}

// Announce is what the install says before it downloads: the size and the source (SDD-011 section 10.2).
func Announce(entries ...ManifestEntry) string {
	var total int64

	parts := make([]string, 0, len(entries))
	for i := range entries {
		e := &entries[i]
		total += e.Bytes
		parts = append(parts, fmt.Sprintf("%s (%.1f MB) from %s", e.File, float64(e.Bytes)/1e6, hostOf(e.URL)))
	}

	return fmt.Sprintf("This downloads %s: %.1f MB in all.", strings.Join(parts, " and "), float64(total)/1e6)
}

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "an unknown host"
	}

	return u.Hostname()
}

// UnavailableError is the E_TRANSCRIBE_UNAVAILABLE of a missing helper or model: what is missing, its
// download size and the command that installs it (SDD-007 section 6.8 item 2).
func UnavailableError(m *MissingError, model string) *ffmpeg.Error {
	names := make([]string, 0, len(m.Items))
	needModel := false

	for _, it := range m.Items {
		names = append(names, it.Name)
		needModel = needModel || it.Name != HelperName
	}

	cmd := "vukem-video tools:install whisper"
	if needModel {
		cmd += " --model " + model
	}

	e := unavailable(ReasonNotInstalled,
		"Local transcription needs "+strings.Join(names, " and ")+", which is not installed.", m,
		map[string]any{
			"missing": names, "download_bytes": m.DownloadBytes, "install_command": cmd,
			"tool": HelperName, "model": model,
		})
	e.Hint = fmt.Sprintf(
		"Run %q, choose a smaller model with VUKEM_VIDEO_WHISPER_MODEL, or configure an API provider.",
		cmd,
	)

	return e
}
