package owner

import "testing"

func TestLimitsFromEnvReadsTheTranscribeAndExportCeilings(t *testing.T) {
	env := map[string]string{
		"VUKEM_VIDEO_MAX_TRANSCRIBES":        "3",
		"VUKEM_VIDEO_MAX_TRANSCRIBE_SECONDS": "600",
		"VUKEM_VIDEO_MAX_EXPORT_BYTES":       "1048576",
	}

	l, err := limitsFromEnv(func(k string) string { return env[k] })
	if err != nil || l.MaxTranscribes != 3 || l.MaxTranscribeSeconds != 600 || l.MaxExportBytes != 1<<20 {
		t.Errorf("limits = %+v, %v", l, err)
	}

	if _, err := limitsFromEnv(func(k string) string {
		if k == "VUKEM_VIDEO_MAX_EXPORT_BYTES" {
			return "-1"
		}

		return ""
	}); err == nil {
		t.Error("a negative ceiling was accepted")
	}
}
