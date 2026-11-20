package service

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/windows/registry"
)

type registryRegistrar struct{ subKey, name, exe string }

// NewRegistryRegistrar is a Registrar over a value of a key of HKEY_CURRENT_USER. The tests give it a key
// of their own, so none of them touches the Run key of the machine they run on.
func NewRegistryRegistrar(subKey, name, exe string) Registrar {
	return &registryRegistrar{subKey: subKey, name: name, exe: exe}
}

// SystemRegistrar is the registration of this executable in the Run key of the user.
func SystemRegistrar() Registrar {
	exe, err := os.Executable()
	if err != nil {
		return nil
	}

	return NewRegistryRegistrar(RunSubKey, RunValueName, exe)
}

// command is the line Windows runs at sign-in: the quoted executable and its arguments.
func (r *registryRegistrar) command(args []string) string {
	return strings.Join(append([]string{`"` + r.exe + `"`}, args...), " ")
}

func (r *registryRegistrar) Enable(args []string) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, r.subKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open %s: %w", r.subKey, err)
	}

	defer func() { _ = key.Close() }()

	if err := key.SetStringValue(r.name, r.command(args)); err != nil {
		return fmt.Errorf("write %s: %w", r.name, err)
	}

	return nil
}

func (r *registryRegistrar) Disable() error {
	key, err := registry.OpenKey(registry.CURRENT_USER, r.subKey, registry.SET_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}

		return fmt.Errorf("open %s: %w", r.subKey, err)
	}

	defer func() { _ = key.Close() }()

	if err := key.DeleteValue(r.name); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return fmt.Errorf("delete %s: %w", r.name, err)
	}

	return nil
}

func (r *registryRegistrar) Enabled() (bool, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, r.subKey, registry.QUERY_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return false, nil
		}

		return false, fmt.Errorf("open %s: %w", r.subKey, err)
	}

	defer func() { _ = key.Close() }()

	if _, _, err := key.GetStringValue(r.name); err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return false, nil
		}

		return false, fmt.Errorf("read %s: %w", r.name, err)
	}

	return true, nil
}
