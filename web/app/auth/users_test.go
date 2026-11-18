package auth_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	vauth "github.com/AlexGalhardo/vukem/auth"
	"github.com/AlexGalhardo/vukem/auth/hash"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
	"github.com/AlexGalhardo/vukem-video/service"
	"github.com/AlexGalhardo/vukem-video/service/workspace"
	"github.com/AlexGalhardo/vukem-video/web/app/auth"
	"github.com/AlexGalhardo/vukem-video/web/app/store"
)

const canary = "c4n4ry-admin-password-0123456789"

func newUsers(t *testing.T) *auth.Users {
	t.Helper()

	dir, err := workspace.Resolve(func(string) string { return filepath.Join(t.TempDir(), "ws") })
	if err != nil {
		t.Fatal(err)
	}

	if err := dir.EnsureLayout(); err != nil {
		t.Fatal(err)
	}

	st, err := store.Open(context.Background(), dir, store.Options{SchemaVersion: timeline.CurrentSchemaVersion})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = st.Close() })

	u := auth.NewUsers(st.DB())
	u.Hasher = hash.Fake()

	return u
}

func TestUsersCreateStoresAnArgon2idHash(t *testing.T) {
	users := newUsers(t)

	u, err := users.Create(t.Context(), "  Ana@Example.COM ", "correct horse battery", service.RoleMember)
	if err != nil {
		t.Fatal(err)
	}

	if u.Email != "ana@example.com" || !strings.HasPrefix(u.ID, "usr_") {
		t.Errorf("user = %+v", u)
	}

	if !strings.HasPrefix(u.Password, "$argon2id$") || strings.Contains(u.Password, "horse") {
		t.Errorf("stored password = %q, want an argon2id hash", u.Password)
	}

	got, err := users.ByEmail(t.Context(), "ANA@example.com")
	if err != nil || got.ID != u.ID {
		t.Errorf("ByEmail = %+v, %v", got, err)
	}

	if _, err := users.ByEmail(t.Context(), "nobody@example.com"); !errors.Is(err, vauth.ErrUserNotFound) {
		t.Errorf("an unknown email = %v, want ErrUserNotFound", err)
	}
}

func TestUsersRefuseWhatIsNotAUser(t *testing.T) {
	users := newUsers(t)

	for _, c := range []struct {
		email, password, role string
		want                  error
	}{
		{"not an email", "correct horse battery", service.RoleMember, auth.ErrBadEmail},
		{"Ana <ana@example.com>", "correct horse battery", service.RoleMember, auth.ErrBadEmail},
		{"ana@example.com", "short", service.RoleMember, auth.ErrPasswordLen},
		{"ana@example.com", strings.Repeat("x", 1025), service.RoleMember, auth.ErrPasswordLen},
		{"ana@example.com", "correct horse battery", "root", auth.ErrBadRole},
	} {
		if _, err := users.Create(t.Context(), c.email, c.password, c.role); !errors.Is(err, c.want) {
			t.Errorf("Create(%q, %d chars, %q) = %v, want %v", c.email, len(c.password), c.role, err, c.want)
		}
	}

	if _, err := users.Create(t.Context(), "ana@example.com", "correct horse battery", service.RoleAdmin); err != nil {
		t.Fatal(err)
	}

	if _, err := users.Create(
		t.Context(),
		"ANA@example.com",
		"correct horse battery",
		service.RoleMember,
	); !errors.Is(
		err,
		auth.ErrEmailTaken,
	) {
		t.Errorf("a second user with the same email = %v, want ErrEmailTaken", err)
	}

	if n, _ := users.Count(t.Context()); n != 1 {
		t.Errorf("count = %d, want 1", n)
	}
}

func TestRoleScopes(t *testing.T) {
	t.Parallel()

	if got := service.RoleScopes(service.RoleAdmin); len(got) != 5 {
		t.Errorf("admin scopes = %v, want the five", got)
	}

	member := service.RoleScopes(service.RoleMember)
	want := []string{service.ScopeRead, service.ScopeEdit, service.ScopeMedia, service.ScopeRender}

	if strings.Join(member, ",") != strings.Join(want, ",") {
		t.Errorf("member scopes = %v, want %v", member, want)
	}

	if got := service.RoleScopes("root"); len(got) != 0 {
		t.Errorf("an unknown role holds %v", got)
	}
}

func TestSetPasswordMovesTheSessionVersion(t *testing.T) {
	users := newUsers(t)

	u, err := users.Create(t.Context(), "ana@example.com", "correct horse battery", service.RoleMember)
	if err != nil {
		t.Fatal(err)
	}

	again, err := users.SetPassword(t.Context(), "ana@example.com", "another long password")
	if err != nil {
		t.Fatal(err)
	}

	if again.Version != u.Version+1 || again.Password == u.Password {
		t.Errorf("version %d -> %d, hash changed: %v", u.Version, again.Version, again.Password != u.Password)
	}

	if _, err := users.SetPassword(
		t.Context(),
		"nobody@example.com",
		"another long password",
	); !errors.Is(
		err,
		auth.ErrNoSuchUser,
	) {
		t.Errorf("an unknown user = %v, want ErrNoSuchUser", err)
	}
}

func bootstrapEnv(vars map[string]string) func(string) string {
	return func(k string) string { return vars[k] }
}

// The canary password never reaches the log, and the email does (SDD-007
// section 4.1 item 2; feeds TestSecurity_T04_SecretsNeverAppear).
func TestAdminBootstrapNoSecretInLogs(t *testing.T) {
	users := newUsers(t)
	logs := &bytes.Buffer{}
	log := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

	env := bootstrapEnv(map[string]string{
		auth.EnvAdminEmail: "Root@Example.com", auth.EnvAdminPassword: canary,
	})

	if err := auth.BootstrapAdmin(t.Context(), users, env, log); err != nil {
		t.Fatal(err)
	}

	u, err := users.ByEmail(t.Context(), "root@example.com")
	if err != nil || u.Role != service.RoleAdmin {
		t.Fatalf("admin = %+v, %v", u, err)
	}

	if ok, _ := users.Hasher.Verify(u.Password, canary); !ok {
		t.Error("the stored hash does not verify the password")
	}

	if strings.Contains(logs.String(), canary) || strings.Contains(u.Password, canary) {
		t.Errorf("the password is in the log or the row:\n%s", logs)
	}

	if !strings.Contains(logs.String(), "root@example.com") {
		t.Errorf("the log does not name the administrator:\n%s", logs)
	}

	// Created once: a later start with other values changes nothing.
	other := bootstrapEnv(map[string]string{auth.EnvAdminEmail: "other@example.com", auth.EnvAdminPassword: canary})
	if err := auth.BootstrapAdmin(t.Context(), users, other, log); err != nil {
		t.Fatal(err)
	}

	if n, _ := users.Count(t.Context()); n != 1 {
		t.Errorf("users = %d, want the variables ignored once a user exists", n)
	}
}

func TestAdminBootstrapFromAFileAndWithOneVariable(t *testing.T) {
	users := newUsers(t)
	log := slog.New(slog.DiscardHandler)

	if err := auth.BootstrapAdmin(t.Context(), users,
		bootstrapEnv(map[string]string{auth.EnvAdminEmail: "root@example.com"}), log); err != nil {
		t.Fatal(err)
	}

	if n, _ := users.Count(t.Context()); n != 0 {
		t.Fatalf("an email with no password created %d users", n)
	}

	file := filepath.Join(t.TempDir(), "admin-password")
	if err := os.WriteFile(file, []byte(canary+"\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	env := bootstrapEnv(
		map[string]string{auth.EnvAdminEmail: "root@example.com", auth.EnvAdminPassword: "@file:" + file},
	)
	if err := auth.BootstrapAdmin(t.Context(), users, env, log); err != nil {
		t.Fatal(err)
	}

	u, err := users.ByEmail(t.Context(), "root@example.com")
	if err != nil {
		t.Fatal(err)
	}

	if ok, _ := users.Hasher.Verify(u.Password, canary); !ok {
		t.Error("the password of the file was not used, or its line ending was kept")
	}
}

func TestAdminBootstrapRefusesAWeakPassword(t *testing.T) {
	users := newUsers(t)

	env := bootstrapEnv(map[string]string{auth.EnvAdminEmail: "root@example.com", auth.EnvAdminPassword: "short"})

	err := auth.BootstrapAdmin(t.Context(), users, env, slog.New(slog.DiscardHandler))
	if !errors.Is(err, auth.ErrPasswordLen) || strings.Contains(err.Error(), "short\n") {
		t.Errorf("err = %v, want ErrPasswordLen", err)
	}
}
