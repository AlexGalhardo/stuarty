package crashharness

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/AlexGalhardo/stuarty/service/vault"
)

const (
	roleEnv = "STUARTY_CRASH_ROLE"
	dirEnv  = "STUARTY_CRASH_DIR"
	// crashEnv is the variable crashpoint.Hit reads (service/crashpoint, build tag crashtest).
	crashEnv = "STUARTY_CRASH_AT"
)

// masterKey is the key of the vaults of these tests; a fixed value, since nothing here is a secret.
var masterKey = bytes.Repeat([]byte{0x42}, vault.KeySize)

var (
	buildOnce sync.Once
	childBin  string
	errBuild  error
)

// child builds the test binary of this package with the crashtest tag, where crashpoint.Hit ends the process.
// It is built once.
func child(t *testing.T) string {
	t.Helper()

	buildOnce.Do(func() {
		childBin = filepath.Join(t.TempDir(), "child.test")
		if runtime.GOOS == "windows" {
			childBin += ".exe"
		}

		build := exec.CommandContext(
			t.Context(), "go", "test", "-c", "-tags", "crashtest", "-o", childBin, ".",
		)

		if out, err := build.CombinedOutput(); err != nil {
			errBuild = errors.New("go test -c: " + err.Error() + "\n" + string(out))
		}
	})

	if errBuild != nil {
		t.Fatal(errBuild)
	}

	return childBin
}

// TestVaultWriteChild is the body of the child: it stores one more secret and is ended by the crash point.
func TestVaultWriteChild(t *testing.T) {
	if os.Getenv(roleEnv) != "vault-write" {
		t.Skip("only runs as the child of a crash test") // verify:env-skip the role is set by the parent test
	}

	v := vault.Open(filepath.Join(os.Getenv(dirEnv), "vault.json"), vault.NewMemoryProvider(masterKey))
	err := v.Put("con_new", []byte("secret-new"))

	// Reaching this line means the crash point was never hit.
	t.Fatalf("the child finished (%v) without reaching %s", err, os.Getenv(crashEnv))
}

// T-101: a vault write killed before and after the rename: the vault opens and verifies, every secret that was
// acknowledged before is still there, and the new one is there only when the rename happened.
func TestSecurity_T101_VaultKilledMidWriteKeepsEveryAcknowledgedSecret(t *testing.T) {
	t.Parallel()

	bin := child(t)

	for point, durable := range map[string]bool{"vault.before_rename": false, "vault.after_rename": true} {
		t.Run(point, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			path := filepath.Join(dir, "vault.json")
			acknowledged := map[string]string{"con_a": "secret-a", "con_b": "secret-b"}

			before := vault.Open(path, vault.NewMemoryProvider(masterKey))
			for id, secret := range acknowledged {
				if err := before.Put(id, []byte(secret)); err != nil {
					t.Fatal(err)
				}
			}

			cmd := exec.CommandContext(
				t.Context(),
				bin,
				"-test.run",
				"^TestVaultWriteChild$",
			)
			cmd.Env = append(os.Environ(), roleEnv+"=vault-write", dirEnv+"="+dir, crashEnv+"="+point)

			out, err := cmd.CombinedOutput()

			exit, ok := errors.AsType[*exec.ExitError](err)
			if !ok || exit.ExitCode() != 137 {
				t.Fatalf("the child ended with %v, want exit code 137\n%s", err, out)
			}

			after := vault.Open(path, vault.NewMemoryProvider(masterKey))
			if err := after.Verify(); err != nil {
				t.Fatalf("the vault does not verify after the kill: %v", err)
			}

			for id, want := range acknowledged {
				got, err := after.Get(id)
				if err != nil || string(got) != want {
					t.Errorf("%s = %q, %v; an acknowledged secret was lost", id, got, err)
				}
			}

			got, err := after.Get("con_new")

			switch {
			case durable && (err != nil || string(got) != "secret-new"):
				t.Errorf("con_new = %q, %v; the rename happened, so the write must be there", got, err)
			case !durable && !errors.Is(err, vault.ErrNotFound):
				t.Errorf("con_new = %q, %v; the rename did not happen, so the write must not be there", got, err)
			}

			// The kill may leave the temporary file of the write; it holds only sealed data, and a later
			// write still works.
			if err := after.Put("con_c", []byte("secret-c")); err != nil {
				t.Errorf("the vault does not take a write after the kill: %v", err)
			}
		})
	}
}
