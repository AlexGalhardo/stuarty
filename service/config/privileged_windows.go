//go:build windows

package config

import "golang.org/x/sys/windows"

func privileged() bool { return windows.GetCurrentProcessToken().IsElevated() }
