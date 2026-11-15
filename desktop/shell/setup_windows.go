//go:build windows

package shell

import (
	"context"
	"fmt"
	"os/exec"
)

// runSetup starts the installer and does not wait: it must run after this process has quit. The path
// is the staged file of the update client, whose name the client matched against a strict pattern.
// The context is Background on purpose: a cancelled context would kill the installer.
func runSetup(path string) error {
	if err := exec.CommandContext(context.Background(), path).Start(); err != nil {
		return fmt.Errorf("start the installer: %w", err)
	}

	return nil
}
