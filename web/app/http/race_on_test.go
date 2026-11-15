//go:build race

package httpapi_test

// raceEnabled reports a -race build: timing comparisons do not hold under it.
const raceEnabled = true
