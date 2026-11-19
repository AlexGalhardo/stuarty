//go:build crashtest

// Package crashpoint marks the places of the commit path where the crash
// tests kill the process (SDD-005 section 18 check 6). With the crashtest
// build tag Hit ends the process with exit code 137 at the point named by the
// environment variable CrashEnv, as a SIGKILL would.
package crashpoint

import "os"

// CrashEnv names the crash point the process dies at.
const CrashEnv = "STUARTY_CRASH_AT"

// Hit is a named crash point.
func Hit(name string) {
	if os.Getenv(CrashEnv) == name {
		os.Exit(137)
	}
}
