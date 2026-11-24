package main

import "runtime/pprof"

// osThreads is the number of OS threads the Go runtime holds: the runtime starts one under load and keeps it.
func osThreads() int {
	return pprof.Lookup("threadcreate").Count()
}
