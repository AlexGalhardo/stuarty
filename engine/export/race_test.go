//go:build race

package export

// raceEnabled is true when the tests run with the race detector, which multiplies the memory of the
// process and slows it by an order of magnitude: the measurements of NFR-006 and NFR-007 are not taken.
const raceEnabled = true
