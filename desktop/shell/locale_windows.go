//go:build windows

package shell

import (
	"syscall"
	"unsafe"
)

// systemLocale is the default locale name of the user ("pt-BR"); the
// environment is not consulted, Windows does not set LANG.
func systemLocale(_ func(string) string) string {
	proc := syscall.NewLazyDLL("kernel32.dll").NewProc("GetUserDefaultLocaleName")
	buf := make([]uint16, 85)
	n, _, _ := proc.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf)
}
