package service

import (
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/windows/registry"
)

// memKey is a registry key in memory: the logic of the registrar is tested on it, so no test of this
// package writes the registry of the machine it runs on.
type memKey struct {
	values  map[string]string
	failSet error
}

func (m *memKey) set(name, value string) error {
	if m.failSet != nil {
		return m.failSet
	}

	if m.values == nil {
		m.values = map[string]string{}
	}

	m.values[name] = value

	return nil
}

func (m *memKey) remove(name string) error {
	delete(m.values, name)

	return nil
}

func (m *memKey) has(name string) (bool, error) {
	_, ok := m.values[name]

	return ok, nil
}

// exerciseRegistrar registers, checks and removes the value through reg and reads what the key holds
// with read, which reports false when the value is not there.
func exerciseRegistrar(t *testing.T, reg Registrar, read func() (string, bool)) {
	t.Helper()

	if on, err := reg.Enabled(); err != nil || on {
		t.Fatalf("before: enabled=%v, %v", on, err)
	}

	// Removing what is not there is no error: the uninstaller and the settings both do it.
	if err := reg.Disable(); err != nil {
		t.Fatalf("disable of nothing: %v", err)
	}

	if err := reg.Enable([]string{BackgroundFlag}); err != nil {
		t.Fatal(err)
	}

	if on, err := reg.Enabled(); err != nil || !on {
		t.Fatalf("after enable: enabled=%v, %v", on, err)
	}

	want := `"C:\Program Files\Stuarty\stuarty-desktop.exe" --background`
	if got, ok := read(); !ok || got != want {
		t.Fatalf("value = %q (%v); want %q", got, ok, want)
	}

	if err := reg.Enable([]string{BackgroundFlag}); err != nil {
		t.Fatalf("enable twice: %v", err)
	}

	if err := reg.Disable(); err != nil {
		t.Fatal(err)
	}

	if on, err := reg.Enabled(); err != nil || on {
		t.Fatalf("after disable: enabled=%v, %v", on, err)
	}

	if got, ok := read(); ok {
		t.Errorf("the value %q is still there after disable", got)
	}
}

func TestRegistryRegistrarRegistersChecksAndCleans(t *testing.T) {
	t.Parallel()

	key := &memKey{}
	reg := &registryRegistrar{key: key, name: RunValueName, exe: `C:\Program Files\Stuarty\stuarty-desktop.exe`}

	exerciseRegistrar(t, reg, func() (string, bool) {
		v, ok := key.values[RunValueName]

		return v, ok
	})
}

func TestRegistryRegistrarQuotesThePathAsOneArgument(t *testing.T) {
	t.Parallel()

	key := &memKey{}
	exe := `C:\Users\Ana Maria\AppData\Local\Programs\Stuarty Desktop\stuarty-desktop.exe`
	reg := &registryRegistrar{key: key, name: RunValueName, exe: exe}

	if err := reg.Enable([]string{BackgroundFlag}); err != nil {
		t.Fatal(err)
	}

	if got, want := key.values[RunValueName], `"`+exe+`" --background`; got != want {
		t.Errorf("value = %q, want %q", got, want)
	}
}

func TestRegistryRegistrarRefusalIsAnError(t *testing.T) {
	t.Parallel()

	key := &memKey{failSet: errors.New("access is denied")}
	reg := &registryRegistrar{key: key, name: RunValueName, exe: `C:\x\stuarty-desktop.exe`}

	got, err := Apply(Settings{KeepRunning: true, StartAtSignIn: true}, reg)
	if !errors.Is(err, ErrRegistration) || got.StartAtSignIn {
		t.Fatalf("refusal: %+v, %v", got, err)
	}
}

// The real registry is exercised only on request (STUARTY_TEST_REGISTRY=1), on a scratch key of this test
// alone and never on the Run key: a test must not write the registry of a person's machine.
func TestRegistryRegistrarOnTheRealRegistry(t *testing.T) {
	if os.Getenv("STUARTY_TEST_REGISTRY") != "1" {
		t.Skip( // verify:env-skip writes HKEY_CURRENT_USER, only on a disposable machine
			"set STUARTY_TEST_REGISTRY=1 on a disposable machine to run it on a scratch key",
		)
	}

	sub := fmt.Sprintf(`Software\StuartyTest\run-%d-%d`, os.Getpid(), time.Now().UnixNano())
	if sub == RunSubKey {
		t.Fatal("the scratch key is the Run key")
	}

	t.Cleanup(func() {
		_ = registry.DeleteKey(registry.CURRENT_USER, sub)
		_ = registry.DeleteKey(registry.CURRENT_USER, `Software\StuartyTest`)
	})

	reg := NewRegistryRegistrar(sub, RunValueName, `C:\Program Files\Stuarty\stuarty-desktop.exe`)

	exerciseRegistrar(t, reg, func() (string, bool) {
		key, err := registry.OpenKey(registry.CURRENT_USER, sub, registry.QUERY_VALUE)
		if err != nil {
			return "", false
		}

		defer func() { _ = key.Close() }()

		v, _, err := key.GetStringValue(RunValueName)

		return v, err == nil
	})
}

func TestSystemRegistrarTargetsTheRunKeyOfTheUserOnly(t *testing.T) {
	t.Parallel()

	reg, ok := SystemRegistrar().(*registryRegistrar)
	if !ok {
		t.Fatal("no registry registrar on Windows")
	}

	// Never exercised here: only the key and the value the uninstaller must agree with are checked.
	if k, ok := reg.key.(hkcuKey); !ok || k.subKey != RunSubKey || reg.name != RunValueName {
		t.Errorf("registrar = %+v", reg)
	}

	if RunSubKey != `Software\Microsoft\Windows\CurrentVersion\Run` || RunValueName != "StuartyDesktop" {
		t.Errorf("run key %q value %q", RunSubKey, RunValueName)
	}
}
