package httpapi

import (
	"github.com/AlexGalhardo/vukem/web"

	"github.com/AlexGalhardo/vukem-video/web/app/auth"
)

type userCreateIn struct {
	Email    string `json:"email"          validate:"required" doc:"A plain mailbox; it is lower-cased and must be unused."`
	Password string `json:"password"       validate:"required" doc:"12 to 1,024 characters."`
	Role     string `json:"role,omitempty"                     doc:"admin or member; default member."`
}

type userUpdateIn struct {
	UserID   string  `json:"user_id"            validate:"required"`
	Role     *string `json:"role,omitempty"                         doc:"admin or member."`
	Disabled *bool   `json:"disabled,omitempty"                     doc:"True disables the user; false enables them again."`
}

type userListOut struct {
	Users []auth.UserView `json:"users"`
}

// admin is the user management of an administrator session; it exists in network mode only. The
// rule (a session, never a key; the last administrator stays) is in auth.Admin, not here.
func (h *apiHandler) admin() (auth.Admin, error) {
	if h.Network == nil || h.Auth == nil {
		return auth.Admin{}, notInThisMode()
	}

	return auth.Admin{Users: h.Auth.Users}, nil
}

// usersList is GET /users.
func (h *apiHandler) usersList(c *web.Ctx, _ noIn) (userListOut, error) {
	a, err := h.admin()
	if err != nil {
		return userListOut{}, err
	}

	users, err := a.List(c.Context())
	if err != nil {
		return userListOut{}, err
	}

	return userListOut{Users: users}, nil
}

// usersCreate is POST /users.
func (h *apiHandler) usersCreate(c *web.Ctx, in userCreateIn) (auth.UserView, error) {
	a, err := h.admin()
	if err != nil {
		return auth.UserView{}, err
	}

	role := in.Role
	if role == "" {
		role = "member"
	}

	c.SetHeader("Cache-Control", "no-store")

	return a.Create(c.Context(), in.Email, in.Password, role)
}

// usersUpdate is PATCH /users/{user_id}: a role change, a disable or an enable, in one statement.
func (h *apiHandler) usersUpdate(c *web.Ctx, in userUpdateIn) (auth.UserView, error) {
	a, err := h.admin()
	if err != nil {
		return auth.UserView{}, err
	}

	return a.Update(c.Context(), in.UserID, auth.UserPatch{Role: in.Role, Disabled: in.Disabled})
}
