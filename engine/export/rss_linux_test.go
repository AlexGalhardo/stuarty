package export

import (
	"os"
	"strconv"
	"strings"
)

// residentBytes is the resident set of this process, from /proc/self/statm (pages, second field).
func residentBytes() (uint64, bool) {
	raw, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return 0, false
	}

	fields := strings.Fields(string(raw))
	if len(fields) < 2 {
		return 0, false
	}

	pages, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0, false
	}

	return pages * uint64(os.Getpagesize()), true
}
