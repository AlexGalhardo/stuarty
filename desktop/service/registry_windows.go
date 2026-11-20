package service

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// runKey is the part of the registry the registrar uses: the string values of one key of
// HKEY_CURRENT_USER. Tests give the registrar an in-memory key, so no test writes the registry of the
// machine it runs on.
type runKey interface {
	// set writes a string value, creating the key when it is missing.
	set(name, value string) error
	// remove deletes a value; a value or a key that is not there is no error.
	remove(name string) error
	// has says whether the value exists.
	has(name string) (bool, error)
}

type registryRegistrar struct {
	key       runKey
	name, exe string
}

// NewRegistryRegistrar is a Registrar over a value of a key of HKEY_CURRENT_USER.
func NewRegistryRegistrar(subKey, name, exe string) Registrar {
	return &registryRegistrar{key: hkcuKey{subKey: subKey}, name: name, exe: exe}
}

// SystemRegistrar is the registration of this executable in the Run key of the user.
func SystemRegistrar() Registrar {
	exe, err := os.Executable()
	if err != nil {
		return nil
	}

	return NewRegistryRegistrar(RunSubKey, RunValueName, exe)
}

// command is the line Windows runs at sign-in: the quoted executable and its arguments. A Windows path
// cannot hold a double quote, so the quotes always enclose the whole path, spaces included.
func (r *registryRegistrar) command(args []string) string {
	return strings.Join(append([]string{`"` + r.exe + `"`}, args...), " ")
}

func (r *registryRegistrar) Enable(args []string) error {
	return r.key.set(r.name, r.command(args))
}

func (r *registryRegistrar) Disable() error { return r.key.remove(r.name) }

func (r *registryRegistrar) Enabled() (bool, error) { return r.key.has(r.name) }

// hkcuKey is a key of HKEY_CURRENT_USER.
type hkcuKey struct{ subKey string }

func (k hkcuKey) set(name, value string) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, k.subKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open %s: %w", k.subKey, err)
	}

	defer func() { _ = key.Close() }()

	if err := key.SetStringValue(name, value); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}

	return nil
}

func (k hkcuKey) remove(name string) error {
	key, err := registry.OpenKey(registry.CURRENT_USER, k.subKey, registry.SET_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}

		return fmt.Errorf("open %s: %w", k.subKey, err)
	}

	defer func() { _ = key.Close() }()

	if err := key.DeleteValue(name); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return fmt.Errorf("delete %s: %w", name, err)
	}

	return nil
}

func (k hkcuKey) has(name string) (bool, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, k.subKey, registry.QUERY_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return false, nil
		}

		return false, fmt.Errorf("open %s: %w", k.subKey, err)
	}

	defer func() { _ = key.Close() }()

	if _, _, err := key.GetStringValue(name); err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return false, nil
		}

		return false, fmt.Errorf("read %s: %w", name, err)
	}

	return true, nil
}
