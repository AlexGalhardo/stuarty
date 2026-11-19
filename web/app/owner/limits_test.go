package owner

import (
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/config"
)

func envOf(env map[string]string) func(string) string { return func(k string) string { return env[k] } }

func TestLimitsFromEnvReadsTheTranscribeAndExportCeilings(t *testing.T) {
	env := map[string]string{
		"STUARTY_MAX_TRANSCRIBES":        "3",
		"STUARTY_MAX_TRANSCRIBE_SECONDS": "600",
		"STUARTY_MAX_EXPORT_BYTES":       "1048576",
	}

	l, err := limitsFromEnv(envOf(env), false)
	if err != nil || l.MaxTranscribes != 3 || l.MaxTranscribeSeconds != 600 || l.MaxExportBytes != 1<<20 {
		t.Errorf("limits = %+v, %v", l, err)
	}
}

// SDD-007 section 9.1: zero or negative is the default in local mode and a boot
// error that names the variable in network mode, for every limit variable.
func TestLimitsZeroOrNegativeByMode(t *testing.T) {
	for _, name := range config.LimitVariables() {
		for _, raw := range []string{"0", "-1"} {
			env := envOf(map[string]string{name: raw})

			l, err := limitsFromEnv(env, false)
			if err != nil || l.WithDefaults() != (service.Limits{}).WithDefaults() {
				t.Errorf("local %s=%s: %+v, %v", name, raw, l, err)
			}

			if n, err := cacheLimit(env, false); err != nil || n != 0 {
				t.Errorf("local cache %s=%s: %d, %v", name, raw, n, err)
			}
		}
	}

	for _, name := range config.LimitVariables() {
		if strings.Contains(name, "RATE_LIMIT") {
			continue // read by the limiter; config.Validate refuses them at boot
		}

		for _, raw := range []string{"0", "-1"} {
			env := envOf(map[string]string{name: raw})

			_, err := limitsFromEnv(env, true)
			if name == "STUARTY_CACHE_MAX_BYTES" {
				_, err = cacheLimit(env, true)
			}

			if err == nil || !strings.Contains(err.Error(), "invalid limit") || !strings.Contains(err.Error(), name) {
				t.Errorf("network %s=%s: %v", name, raw, err)
			}
		}
	}
}

func TestLimitsNotANumberIsRefusedInBothModes(t *testing.T) {
	for _, network := range []bool{false, true} {
		for _, raw := range []string{"many", "1.5"} {
			if _, err := limitsFromEnv(envOf(map[string]string{"STUARTY_MAX_EXPORTS": raw}), network); err == nil ||
				!strings.Contains(err.Error(), "STUARTY_MAX_EXPORTS") {
				t.Errorf("network=%v %q: %v", network, raw, err)
			}
		}
	}
}
