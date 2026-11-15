//go:build !linux && !darwin && !windows

package export

// residentBytes is not available on this system; the sampler then reports the Go runtime's own count.
func residentBytes() (uint64, bool) { return 0, false }
