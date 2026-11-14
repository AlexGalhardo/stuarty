//go:build !windows

package main

func peakWorkingSet() uint64 { return 0 }
