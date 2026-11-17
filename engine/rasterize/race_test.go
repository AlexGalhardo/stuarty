//go:build race

package rasterize

// raceEnabled is true when the tests run with the race detector, which slows
// the code by about an order of magnitude.
const raceEnabled = true
