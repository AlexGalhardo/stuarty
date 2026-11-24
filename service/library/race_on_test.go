//go:build race

package library

// raceEnabled reports a -race build: timing limits do not hold under it.
const raceEnabled = true
