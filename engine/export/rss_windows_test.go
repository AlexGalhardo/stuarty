package export

import (
	"syscall"
	"unsafe"
)

var getProcessMemoryInfo = syscall.NewLazyDLL("psapi.dll").NewProc("GetProcessMemoryInfo")

// processMemoryCounters is PROCESS_MEMORY_COUNTERS.
type processMemoryCounters struct {
	cb                         uint32
	pageFaultCount             uint32
	peakWorkingSetSize         uintptr
	workingSetSize             uintptr
	quotaPeakPagedPoolUsage    uintptr
	quotaPagedPoolUsage        uintptr
	quotaPeakNonPagedPoolUsage uintptr
	quotaNonPagedPoolUsage     uintptr
	pagefileUsage              uintptr
	peakPagefileUsage          uintptr
}

// residentBytes is the working set of this process, which is its resident memory on Windows.
func residentBytes() (uint64, bool) {
	var c processMemoryCounters

	c.cb = uint32(unsafe.Sizeof(c))

	h, err := syscall.GetCurrentProcess()
	if err != nil {
		return 0, false
	}

	r, _, _ := getProcessMemoryInfo.Call(
		uintptr(h),
		uintptr(unsafe.Pointer(&c)),
		uintptr(c.cb),
	)
	if r == 0 {
		return 0, false
	}

	return uint64(c.workingSetSize), true
}
