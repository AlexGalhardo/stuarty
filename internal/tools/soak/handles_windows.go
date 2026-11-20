//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

var getProcessHandleCount = syscall.NewLazyDLL("kernel32.dll").NewProc("GetProcessHandleCount")

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
