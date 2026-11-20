//go:build !windows

package service

// SystemRegistrar is nil off Windows: the shell registers the start at sign-in there with the autostart of
// its framework (a launch agent on macOS, an autostart entry on Linux), which lives in the module of the
// shell and not here.
func SystemRegistrar() Registrar { return nil }
