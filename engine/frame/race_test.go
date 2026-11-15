//go:build race

package frame

// raceEnabled is true under the race detector, which slows every step: timing gates are skipped.
const raceEnabled = true
