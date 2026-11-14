//go:build !crashtest

// Package crashpoint marks the places of the commit path where the crash
// tests kill the process (SDD-005 section 18 check 6). Without the crashtest
// build tag Hit does nothing and the compiler removes the call.
package crashpoint

// Hit is a named crash point.
func Hit(string) {}
