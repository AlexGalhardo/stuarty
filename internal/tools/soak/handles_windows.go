//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

var getProcessHandleCount = syscall.NewLazyDLL("kernel32.dll").NewProc("GetProcessHandleCount")

// handlesPerThread is what a new OS thread of the Go runtime holds on Windows (its thread handle, its timer and
// the events of its parking): 20 locked threads raised the count by 94 handles, about 5 each, and the leaps seen
// in a loaded run were steps of 6 (VV-363).
const handlesPerThread = 6

// handleCount is the number of handles the process holds open, or -1 when Windows does not say.
func handleCount() int {
	h, err := syscall.GetCurrentProcess()
	if err != nil {
		return -1
	}

	var n uint32

	if r, _, _ := getProcessHandleCount.Call(
		uintptr(h),
		uintptr(unsafe.Pointer(&n)),
	); r == 0 {
		return -1
	}

	return int(n)
}
