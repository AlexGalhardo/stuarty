//go:build windows

package shell

import "errors"

// startDetached is not used on Windows: the setup program installs the update there.
func startDetached(string, ...string) (<-chan struct{}, error) {
	return nil, errors.New("not supported on Windows")
}
