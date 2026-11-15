//go:build race

package lint

// raceEnabled reports a -race build: timing limits do not hold under it.
const raceEnabled = true
