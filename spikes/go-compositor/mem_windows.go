//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

type procMemCounters struct {
	cb                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
}

// peakWorkingSet returns the peak resident (working set) size of this process in bytes.
func peakWorkingSet() uint64 {
	h, err := syscall.GetCurrentProcess()
	if err != nil {
		return 0
	}
	var c procMemCounters
	c.cb = uint32(unsafe.Sizeof(c))
	p := syscall.NewLazyDLL("psapi.dll").NewProc("GetProcessMemoryInfo")
	if r, _, _ := p.Call(uintptr(h), uintptr(unsafe.Pointer(&c)), uintptr(c.cb)); r == 0 {
		return 0
	}
	return uint64(c.PeakWorkingSetSize)
}
