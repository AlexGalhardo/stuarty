package config

import (
	"fmt"
	"io"
	"os"
)

// FormerVariable pairs the name an environment variable had before the rename to Stuarty (ADR-0044)
// with its name now.
type FormerVariable struct {
	Former  string
	Current string
}

// formerPrefix and currentPrefix are the two spellings of the same variable (SDD-000 section 13.1).
const (
	formerPrefix  = "VUKEM_VIDEO_"
	currentPrefix = "STUARTY_"
)

// FormerVariables is the one table of the variables the program reads under their former name
// (SDD-000 section 13.1). They are read until release 1.0.0. A variable that only tests and tools read
// is not here: it was renamed with no fallback (SDD-000 section 13.2).
var FormerVariables = pairs(
	"HOME", "ADDR", "MODE", "FFMPEG", "FFPROBE", "ENCODER", "ALLOW_USER_X264", "EXPORT_RENDERER",
	"IMPORT_ROOTS", "MAX_EXPORTS", "MAX_IMPORT_BYTES", "TRANSCRIBE_URL", "TRANSCRIBE_KEY",
	"TRANSCRIBE_MODEL", "WHISPER_MODEL", "TEST_TRANSCRIBER", "MAX_EXPORTS_PER_KEY", "MAX_INGESTS",
	"MAX_TRANSCRIBES", "MAX_TRANSCRIBES_PER_KEY", "MAX_IMPORT_SECONDS", "MAX_IMPORT_PIXELS",
	"MAX_TRANSCRIBE_SECONDS", "MAX_EXPORT_BYTES", "MIN_FREE_BYTES", "CACHE_MAX_BYTES",
	"RATE_LIMIT_PER_MINUTE", "RATE_LIMIT_SESSION_PER_MINUTE", "RATE_LIMIT_HEAVY_PER_MINUTE", "SANDBOX",
	"INSECURE_ALLOW_ROOT", "LAUNCHER_SECRET", "ADMIN_EMAIL", "ADMIN_PASSWORD", "URL", "KEY", "PROJECT",
	"NO_BROWSER",
)

func pairs(suffixes ...string) []FormerVariable {
	out := make([]FormerVariable, len(suffixes))
	for i, s := range suffixes {
		out[i] = FormerVariable{Former: formerPrefix + s, Current: currentPrefix + s}
	}

	return out
}

// Environ is the process environment as a test or a caller sees it.
type Environ struct {
	Get   func(string) string
	Set   func(name, value string) error
	Unset func(name string) error
}

// ProcessEnviron is the environment of this process.
func ProcessEnviron() Environ {
	return Environ{Get: os.Getenv, Set: os.Setenv, Unset: os.Unsetenv}
}

// ApplyFormerVariables makes a variable set under its former name visible under its current one, so that
// the rest of the program reads one name. For each pair: the current name wins when it is set; otherwise a
// value under the former name is copied to the current one and the former name is unset (a secret then
// lives under one name only). It writes one deprecation warning per variable that is used, and one per
// variable that is ignored because the current name is also set, to warn, and returns the pairs it copied.
// Call it once, at the start of the process, before anything reads the environment.
func ApplyFormerVariables(env Environ, warn io.Writer) ([]FormerVariable, error) {
	var applied []FormerVariable

	for _, p := range FormerVariables {
		former := env.Get(p.Former)
		if former == "" {
			continue
		}

		if env.Get(p.Current) != "" {
			_, _ = fmt.Fprintf(
				warn,
				"stuarty: %s is ignored because %s is set; set the value you want under %s and remove %s\n",
				p.Former,
				p.Current,
				p.Current,
				p.Former,
			)

			continue
		}

		if err := env.Set(p.Current, former); err != nil {
			return applied, fmt.Errorf("config: %s: %w", p.Current, err)
		}

		if err := env.Unset(p.Former); err != nil {
			return applied, fmt.Errorf("config: %s: %w", p.Former, err)
		}

		applied = append(applied, p)

		_, _ = fmt.Fprintf(warn,
			"stuarty: %s is deprecated and is read only until release 1.0.0: set %s instead\n", p.Former, p.Current)
	}

	return applied, nil
}
