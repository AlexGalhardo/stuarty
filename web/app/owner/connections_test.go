package owner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/vault"
	"github.com/AlexGalhardo/stuarty/service/workspace"
)

func connect(t *testing.T, env map[string]string) *Owner {
	t.Helper()

	o, err := Open(context.Background(), Config{Getenv: envOf(env), Inline: true})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = o.Close() })

	return o
}

func admin() context.Context {
	return service.WithCaller(context.Background(), service.Caller{Actor: "local:cli", Scopes: service.AllScopes})
}

// The master key of a Docker secret generates on the first credential, stays outside the workspace,
// and opens the vault in the next process (SDD-014 section 4).
func TestOwnerKeepsTheMasterKeyInTheFileOfTheSecret(t *testing.T) {
	root := t.TempDir()
	ws := filepath.Join(root, "ws")
	keyFile := filepath.Join(root, "secrets", "master.key")

	if err := os.MkdirAll(filepath.Dir(keyFile), 0o700); err != nil {
		t.Fatal(err)
	}

	env := map[string]string{workspace.EnvHome: ws, vault.EnvMasterKeyFile: keyFile}

	first := connect(t, env)

	const secret = "tok-OWNER-CANARY"

	created, err := first.Service.ConnectionCreate(admin(), service.ConnectionCreateRequest{
		Kind: "test", Label: "Blog", Endpoint: "https://blog.example.com", Accepts: []string{"text"}, Secret: secret,
	})
	if err != nil || !created.SecretSet {
		t.Fatalf("create: %+v, %v", created, err)
	}

	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	key, err := os.ReadFile(keyFile)
	if err != nil || strings.TrimSpace(string(key)) == "" {
		t.Fatalf("the key file was not written: %v", err)
	}

	// Neither the key nor the secret is anywhere in the workspace.
	err = filepath.WalkDir(ws, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		raw, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}

		for _, needle := range []string{secret, strings.TrimSpace(string(key))} {
			if strings.Contains(string(raw), needle) {
				t.Errorf("%s holds a credential or the master key", path)
			}
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	second := connect(t, env)

	list, err := second.Service.ConnectionList(admin(), service.ConnectionListRequest{})
	if err != nil || len(list.Connections) != 1 || !list.Connections[0].SecretSet {
		t.Fatalf("the second start: %+v, %v", list, err)
	}

	if err := second.Close(); err != nil {
		t.Fatal(err)
	}

	// With the key file gone the vault says so.
	if err := os.Remove(keyFile); err != nil {
		t.Fatal(err)
	}

	third := connect(t, env)

	if _, err := third.Service.ConnectionTest(admin(), service.ConnectionRef{ID: created.ID}); err == nil ||
		!strings.Contains(err.Error(), service.CodeVaultLocked) {
		t.Errorf("a test without the key: %v, want E_VAULT_LOCKED", err)
	}
}

func TestOwnerRefusesAnAmbiguousMasterKey(t *testing.T) {
	root := t.TempDir()

	for name, env := range map[string]map[string]string{
		"both": {vault.EnvMasterKey: "x", vault.EnvMasterKeyFile: filepath.Join(root, "k")},
	} {
		env[workspace.EnvHome] = filepath.Join(root, name)

		o, err := Open(context.Background(), Config{Getenv: envOf(env), Inline: true})
		if err == nil {
			_ = o.Close()

			t.Errorf("%s: the start was accepted", name)

			continue
		}

		if strings.Contains(err.Error(), "CANARYKEY") {
			t.Errorf("%s: the error echoes the key: %v", name, err)
		}
	}
}

// VV-269: a master key that is named and is not a key does not stop the start: `doctor --full` reports it as
// a failed vault check with the hint of the error, a credential answers E_VAULT_LOCKED, and the bytes of the
// key are in neither (SDD-014 sections 4 and 8).
func TestOwnerReportsAMalformedMasterKeyInsteadOfStopping(t *testing.T) {
	for name, env := range map[string]map[string]string{
		"variable": {vault.EnvMasterKey: "not-a-key-CANARYKEY"},
		"file":     nil,
	} {
		root := t.TempDir()

		if env == nil {
			keyFile := filepath.Join(root, "wrong.key")
			if err := os.WriteFile(keyFile, []byte("not-a-key-CANARYKEY\n"), 0o600); err != nil {
				t.Fatal(err)
			}

			env = map[string]string{vault.EnvMasterKeyFile: keyFile}
		}

		env[workspace.EnvHome] = filepath.Join(root, "ws")

		o := connect(t, env)

		res, err := o.Service.Doctor(person(), service.DoctorRequest{Full: true})
		if err != nil {
			t.Fatalf("%s: doctor failed instead of reporting: %v", name, err)
		}

		var check *service.DoctorCheck

		for i := range res.Checks {
			if res.Checks[i].Name == "vault" {
				check = &res.Checks[i]
			}
		}

		if check == nil || check.OK || !strings.Contains(check.Hint, "32 bytes") {
			t.Errorf("%s: vault check = %+v", name, check)
		}

		_, err = o.Service.ConnectionCreate(admin(), service.ConnectionCreateRequest{
			Kind:     "test",
			Label:    "Blog",
			Endpoint: "https://blog.example.com",
			Accepts:  []string{"text"},
			Secret:   "tok-X",
		})
		if err == nil || !strings.Contains(err.Error(), service.CodeVaultLocked) {
			t.Errorf("%s: a credential with a bad key: %v, want E_VAULT_LOCKED", name, err)
		}

		if err == nil || check == nil {
			continue
		}

		if strings.Contains(err.Error(), "CANARYKEY") || strings.Contains(check.Hint+check.Detail, "CANARYKEY") {
			t.Errorf("%s: the key is echoed", name)
		}
	}
}

func person() context.Context {
	return service.WithCaller(context.Background(), service.Caller{
		Actor: "local:cli", Scopes: service.AllScopes, Operator: true,
	})
}
