package export

import "syscall"

// residentBytes is the highest resident set this process has reached, from getrusage: macOS has no
// cheap call for the current one without cgo, and a peak is what the sampler wants anyway. ru_maxrss is
// in bytes there.
func residentBytes() (uint64, bool) {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return 0, false
	}

	return uint64(ru.Maxrss), true
}
