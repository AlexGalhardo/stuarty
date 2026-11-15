//go:build !windows

package config

import "os"

func privileged() bool { return os.Geteuid() == 0 }
