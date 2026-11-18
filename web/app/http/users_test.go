package httpapi_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/service"
)

type userBody struct {
	UserID   string `json:"user_id"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	Disabled bool   `json:"disabled"`
}

func TestUsersAreManagedByAnAdministratorSession(t *testing.T) {
	h := newNetworkHarness(t, resolved(), true)
	h.user(t, "root@example.com", service.RoleAdmin)
	root := h.session(t, "root@example.com")

	res := h.send(t, http.MethodPost, "/api/v1/users",
		`{"email":"Ana@Example.com","password":"`+goodPass+`"}`, root, nil)
	if res.status != http.StatusCreated {
		t.Fatalf("POST /users = %d %s", res.status, res.body)
	}

	var ana userBody
	if err := json.Unmarshal(res.body, &ana); err != nil {
		t.Fatal(err)
	}

	if ana.Email != "ana@example.com" || ana.Role != service.RoleMember || ana.Disabled {
		t.Errorf("created user = %+v", ana)
	}

	if strings.Contains(string(res.body), "password") || strings.Contains(string(res.body), goodPass) {
		t.Errorf("the answer leaks the password: %s", res.body)
	}

	// The new user logs in; a duplicate email is refused.
	h.session(t, "ana@example.com")

	if res := h.send(t, http.MethodPost, "/api/v1/users",
		`{"email":"ana@example.com","password":"`+goodPass+`"}`, root, nil); res.status != http.StatusBadRequest &&
		res.status != http.StatusUnprocessableEntity {
		t.Errorf("a duplicate email = %d %s", res.status, res.body)
	}

	res = h.send(t, http.MethodGet, "/api/v1/users", "", root, nil)

	var list struct {
		Users []userBody `json:"users"`
	}
	if err := json.Unmarshal(res.body, &list); err != nil || res.status != http.StatusOK || len(list.Users) != 2 {
		t.Fatalf("GET /users = %d %s", res.status, res.body)
	}

	// Role and disabled in one request.
	res = h.send(t, http.MethodPatch, "/api/v1/users/"+ana.UserID, `{"role":"admin","disabled":true}`, root, nil)

	var upd userBody
	if err := json.Unmarshal(
		res.body,
		&upd,
	); err != nil || res.status != http.StatusOK ||
		upd.Role != service.RoleAdmin ||
		!upd.Disabled {
		t.Fatalf("PATCH /users = %d %s", res.status, res.body)
	}

	if res := h.send(t, http.MethodPatch, "/api/v1/users/usr_nobody0000", `{"disabled":true}`, root, nil); res.status !=
		http.StatusNotFound {
		t.Errorf("an unknown user = %d %s, want 404", res.status, res.body)
	}

	if res := h.send(t, http.MethodPatch, "/api/v1/users/"+ana.UserID, `{}`, root, nil); res.status < 400 {
		t.Errorf("an empty patch = %d %s, want an error", res.status, res.body)
	}
}

// A member session and a key, even one with the admin scope, never manage users.
func TestSecurity_UsersRefuseMembersAndKeys(t *testing.T) {
	h := newNetworkHarness(t, resolved(), true)
	h.user(t, "root@example.com", service.RoleAdmin)
	h.user(t, "ana@example.com", service.RoleMember)
	root, ana := h.session(t, "root@example.com"), h.session(t, "ana@example.com")
	key := h.makeKey(t, root, `{"name":"ops","scopes":["read","admin"],"expires_in_days":30}`)

	calls := []struct{ method, path, body string }{
		{http.MethodGet, "/api/v1/users", ""},
		{http.MethodPost, "/api/v1/users", `{"email":"x@example.com","password":"` + goodPass + `"}`},
		{http.MethodPatch, "/api/v1/users/usr_nobody0000", `{"disabled":true}`},
	}

	for _, c := range calls {
		res := h.send(t, c.method, c.path, c.body, ana, nil)
		if res.status != http.StatusForbidden || res.code() != service.CodeForbidden {
			t.Errorf("a member calling %s %s = %d %s, want E_FORBIDDEN", c.method, c.path, res.status, res.body)
		}

		res = h.asKey(t, key.Secret, c.method, c.path, c.body)
		if res.status != http.StatusForbidden || res.code() != service.CodeForbidden {
			t.Errorf("a key calling %s %s = %d %s, want E_FORBIDDEN", c.method, c.path, res.status, res.body)
		}

		if res := h.send(t, c.method, c.path, c.body, "", nil); res.status != http.StatusUnauthorized {
			t.Errorf("no credential calling %s %s = %d, want 401", c.method, c.path, res.status)
		}
	}
}

func TestUsersKeepTheLastAdministrator(t *testing.T) {
	h := newNetworkHarness(t, resolved(), true)
	root := h.user(t, "root@example.com", service.RoleAdmin)
	session := h.session(t, "root@example.com")

	for _, body := range []string{`{"disabled":true}`, `{"role":"member"}`, `{"role":"member","disabled":false}`} {
		res := h.send(t, http.MethodPatch, "/api/v1/users/"+root.ID, body, session, nil)
		if res.status != http.StatusConflict || res.code() != "E_INVALID_STATE" {
			t.Errorf("PATCH %s on the only administrator = %d %s, want 409 E_INVALID_STATE", body, res.status, res.body)
		}
	}
}
