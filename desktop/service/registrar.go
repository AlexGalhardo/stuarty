package service

import (
	"errors"
	"fmt"
)

// Registrar is the registration of the shell with the operating system for the start at sign-in, for
// the signed-in user only and with no administrator rights. The shell implements it with the autostart
// of its framework; tests use MemoryRegistrar.
type Registrar interface {
	Enable(args []string) error
	Disable() error
	Enabled() (bool, error)
}

// ErrRegistration wraps a failure of the operating system registration.
var ErrRegistration = errors.New("the start at sign-in could not be changed")

// Apply carries the settings out on the registration: it registers the shell when StartAtSignIn is on and
// removes the registration when it is off. When the system refuses, the returned settings have the
// option off, so the stored state never says more than what the system holds, and the error says why.
func Apply(s Settings, reg Registrar) (Settings, error) {
	s = s.Normalize()

	if s.StartAtSignIn {
		if err := reg.Enable([]string{BackgroundFlag}); err != nil {
			s.StartAtSignIn = false

			return s, fmt.Errorf("%w: %w", ErrRegistration, err)
		}

		return s, nil
	}

	if err := reg.Disable(); err != nil {
		return s, fmt.Errorf("%w: %w", ErrRegistration, err)
	}

	return s, nil
}

// Reconcile reads the registration and corrects the settings to it: a person who removed the entry by
// hand (the Startup tab of the task manager, for instance) has start at sign-in off here too.
func Reconcile(s Settings, reg Registrar) Settings {
	on, err := reg.Enabled()
	if err == nil && !on {
		s.StartAtSignIn = false
	}

	return s
}

// MemoryRegistrar is a Registrar that keeps its state in memory, for tests of this and other packages.
type MemoryRegistrar struct {
	On   bool
	Args []string
	// FailEnable makes Enable fail, to test a refusal of the system.
	FailEnable error
}

// Enable implements Registrar.
func (m *MemoryRegistrar) Enable(args []string) error {
	if m.FailEnable != nil {
		return m.FailEnable
	}

	m.On, m.Args = true, append([]string(nil), args...)

	return nil
}

// Disable implements Registrar.
func (m *MemoryRegistrar) Disable() error {
	m.On, m.Args = false, nil

	return nil
}

// Enabled implements Registrar.
func (m *MemoryRegistrar) Enabled() (bool, error) { return m.On, nil }
