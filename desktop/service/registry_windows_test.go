package service

import (
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/windows/registry"
)

// scratchKey is a key of this test alone under HKEY_CURRENT_USER, deleted when the test ends: the test of
// the registration never writes the Run key of the machine it runs on, which would start a program at the
// next sign-in of whoever runs the tests.
func scratchKey(t *testing.T) string {
	t.Helper()

	sub := fmt.Sprintf(`Software\StuartyTest\run-%d-%d`, os.Getpid(), time.Now().UnixNano())
	if sub == RunSubKey {
		t.Fatal("the scratch key is the Run key")
	}

	t.Cleanup(func() {
		_ = registry.DeleteKey(registry.CURRENT_USER, sub)
		_ = registry.DeleteKey(registry.CURRENT_USER, `Software\StuartyTest`)
	})

	return sub
}

func TestRegistryRegistrarRegistersChecksAndCleans(t *testing.T) {
	sub := scratchKey(t)
	reg := NewRegistryRegistrar(sub, RunValueName, `C:\Program Files\Stuarty\stuarty-desktop.exe`)

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

	key, err := registry.OpenKey(registry.CURRENT_USER, sub, registry.QUERY_VALUE)
	if err != nil {
		t.Fatal(err)
	}

	got, _, err := key.GetStringValue(RunValueName)
	_ = key.Close()

	want := `"C:\Program Files\Stuarty\stuarty-desktop.exe" --background`
	if err != nil || got != want {
		t.Fatalf("value = %q, %v; want %q", got, err, want)
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

	key, err = registry.OpenKey(registry.CURRENT_USER, sub, registry.QUERY_VALUE)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = key.Close() }()

	if _, _, err := key.GetStringValue(RunValueName); !errors.Is(err, registry.ErrNotExist) {
		t.Errorf("the value is still there after disable: %v", err)
	}
}

func TestSystemRegistrarTargetsTheRunKeyOfTheUserOnly(t *testing.T) {
	t.Parallel()

	if SystemRegistrar() == nil {
		t.Fatal("no registrar on Windows")
	}

	// Never exercised here: only the constants the uninstaller must agree with are checked.
	if RunSubKey != `Software\Microsoft\Windows\CurrentVersion\Run` || RunValueName != "StuartyDesktop" {
		t.Errorf("run key %q value %q", RunSubKey, RunValueName)
	}
}
