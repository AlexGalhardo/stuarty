//go:build !windows

package shell

func runSetup(string) error { return ErrInstallUnsupported }
