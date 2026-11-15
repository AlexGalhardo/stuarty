package auth_test

import (
	"context"
	"errors"
	"testing"

	vauth "github.com/AlexGalhardo/vukem/auth"

	"github.com/AlexGalhardo/vukem-video/service"
	"github.com/AlexGalhardo/vukem-video/web/app/auth"
)

const adminPassword = "correct horse battery staple"

func sessionOf(role string) context.Context {
	return service.WithCaller(context.Background(), service.Caller{
		Actor: "user:usr_test", Scopes: service.RoleScopes(role), Browser: true, OwnerUserID: "usr_test",
	})
}

func codeOf(err error) string {
	if se, ok := errors.AsType[*service.Error](err); ok {
		return se.Code
	}

	return ""
}

func TestAdminUsersAreForAnAdministratorSession(t *testing.T) {
	admin := auth.Admin{Users: newUsers(t)}

	if _, err := admin.Create(
		sessionOf(service.RoleMember),
		"a@example.com",
		adminPassword,
		service.RoleMember,
	); codeOf(
		err,
	) != service.CodeForbidden {
		t.Errorf("a member session: %v, want E_FORBIDDEN", err)
	}

	key := service.WithCaller(context.Background(), service.Caller{
		Actor: "key:key_test", Scopes: service.AllScopes, OwnerUserID: "usr_test",
	})
	if _, err := admin.Create(
		key,
		"a@example.com",
		adminPassword,
		service.RoleMember,
	); codeOf(
		err,
	) != service.CodeForbidden {
		t.Errorf("a key with every scope: %v, want it refused", err)
	}

	if _, err := admin.List(key); codeOf(err) != service.CodeForbidden {
		t.Errorf("a key listing users: %v, want it refused", err)
	}

	if _, err := admin.Create(
		sessionOf(service.RoleAdmin),
		"a@example.com",
		adminPassword,
		service.RoleMember,
	); err != nil {
		t.Fatal(err)
	}

	list, err := admin.List(sessionOf(service.RoleAdmin))
	if err != nil || len(list) != 1 || list[0].Email != "a@example.com" || list[0].Role != service.RoleMember {
		t.Errorf("list = %+v, %v", list, err)
	}
}

func TestAdminUsersKeepTheLastAdministrator(t *testing.T) {
	users := newUsers(t)
	admin := auth.Admin{Users: users}
	ctx := sessionOf(service.RoleAdmin)
	only, err := users.Create(ctx, "root@example.com", adminPassword, service.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}

	disable, member := true, service.RoleMember

	if _, err := admin.Update(ctx, only.ID, auth.UserPatch{Disabled: &disable}); codeOf(err) != "E_INVALID_STATE" {
		t.Errorf("disabling the only administrator: %v, want E_INVALID_STATE", err)
	}

	if _, err := admin.Update(ctx, only.ID, auth.UserPatch{Role: &member}); codeOf(err) != "E_INVALID_STATE" {
		t.Errorf("demoting the only administrator: %v, want E_INVALID_STATE", err)
	}

	second, err := users.Create(ctx, "second@example.com", adminPassword, service.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := admin.Update(ctx, only.ID, auth.UserPatch{Role: &member}); err != nil {
		t.Errorf("demoting one of two administrators: %v", err)
	}

	if _, err := admin.Update(ctx, second.ID, auth.UserPatch{Disabled: &disable}); codeOf(err) != "E_INVALID_STATE" {
		t.Errorf("disabling the remaining administrator: %v, want E_INVALID_STATE", err)
	}

	if _, err := admin.Update(ctx, "usr_nobody", auth.UserPatch{Disabled: &disable}); codeOf(err) != "E_NOT_FOUND" {
		t.Errorf("an unknown user: %v, want E_NOT_FOUND", err)
	}
}

func TestAdminUsersRoleChangeAndDisableEndSessions(t *testing.T) {
	users := newUsers(t)
	admin := auth.Admin{Users: users}
	ctx := sessionOf(service.RoleAdmin)

	if _, err := users.Create(ctx, "root@example.com", adminPassword, service.RoleAdmin); err != nil {
		t.Fatal(err)
	}

	m, err := users.Create(ctx, "member@example.com", adminPassword, service.RoleMember)
	if err != nil {
		t.Fatal(err)
	}

	promoted := service.RoleAdmin

	up, err := admin.Update(ctx, m.ID, auth.UserPatch{Role: &promoted})
	if err != nil || up.Role != service.RoleAdmin {
		t.Fatalf("promote = %+v, %v", up, err)
	}

	after, err := users.ByID(context.Background(), m.ID)
	if err != nil || after.Version != m.Version+1 || after.Role != service.RoleAdmin {
		t.Errorf("a role change must move the session version: %+v, %v", after, err)
	}

	disable := true

	if _, err := admin.Update(ctx, m.ID, auth.UserPatch{Disabled: &disable}); err != nil {
		t.Fatal(err)
	}

	if _, err := users.ByID(context.Background(), m.ID); !errors.Is(err, vauth.ErrUserNotFound) {
		t.Errorf("a disabled user by id (a session or a key): %v, want not found", err)
	}

	if _, err := users.ByCredentials(
		context.Background(),
		map[string]string{"email": "member@example.com"},
	); !errors.Is(
		err,
		vauth.ErrUserNotFound,
	) {
		t.Errorf("a disabled user logging in: %v, want not found", err)
	}

	enable := false

	if _, err := admin.Update(ctx, m.ID, auth.UserPatch{Disabled: &enable}); err != nil {
		t.Fatal(err)
	}

	if _, err := users.ByID(context.Background(), m.ID); err != nil {
		t.Errorf("an enabled user again: %v", err)
	}
}

// A call that sets the role and the disabled state is one statement: both apply or neither.
func TestAdminUsersUpdateIsAtomic(t *testing.T) {
	users := newUsers(t)
	admin := auth.Admin{Users: users}
	ctx := sessionOf(service.RoleAdmin)

	only, err := users.Create(ctx, "root@example.com", adminPassword, service.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}

	member, err := users.Create(ctx, "m@example.com", adminPassword, service.RoleMember)
	if err != nil {
		t.Fatal(err)
	}

	enabled, disabled, asMember, asAdmin := false, true, service.RoleMember, service.RoleAdmin

	// Refused as a whole: the role does not change although the enable alone would be fine.
	if _, err := admin.Update(
		ctx,
		only.ID,
		auth.UserPatch{Role: &asMember, Disabled: &enabled},
	); codeOf(
		err,
	) != "E_INVALID_STATE" {
		t.Fatalf("demoting the only administrator in a two-field update: %v", err)
	}

	got, err := users.List(ctx)
	if err != nil {
		t.Fatal(err)
	}

	for _, u := range got {
		if u.ID == only.ID && (u.Role != service.RoleAdmin || u.Disabled) {
			t.Errorf("the refused update changed the administrator: %+v", u)
		}
	}

	// Applied as a whole: promoted and disabled together; a disabled administrator does not count.
	v, err := admin.Update(ctx, member.ID, auth.UserPatch{Role: &asAdmin, Disabled: &disabled})
	if err != nil || v.Role != service.RoleAdmin || !v.Disabled {
		t.Fatalf("promote and disable = %+v, %v", v, err)
	}

	if _, err := admin.Update(ctx, only.ID, auth.UserPatch{Disabled: &disabled}); codeOf(err) != "E_INVALID_STATE" {
		t.Errorf("a disabled administrator counted as one: %v", err)
	}
}
