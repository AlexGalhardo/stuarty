package auth

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/AlexGalhardo/vukem/auth"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/service"
)

// ErrLastAdmin refuses a change that would leave network mode without an enabled administrator.
var ErrLastAdmin = errors.New("the last administrator cannot be demoted or disabled")

// UserView is a user as an administrator sees them: never the password hash.
type UserView struct {
	ID        string `json:"user_id"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	Disabled  bool   `json:"disabled"`
	CreatedMS int64  `json:"created_ms"`
}

func viewOf(u User) UserView {
	return UserView{ID: u.ID, Email: u.Email, Role: u.Role, Disabled: u.Disabled, CreatedMS: u.CreatedMS}
}

// List returns every user, oldest first, disabled ones included.
func (u *Users) List(ctx context.Context) ([]User, error) {
	rows, err := u.ex.QueryContext(ctx,
		`SELECT id, email, password, role, session_version, created_ms, disabled_ms IS NOT NULL
		 FROM users ORDER BY created_ms, id`)
	if err != nil {
		return nil, err
	}

	defer func() { _ = rows.Close() }()

	var out []User

	for rows.Next() {
		var user User
		if err := rows.Scan(&user.ID, &user.Email, &user.Password, &user.Role, &user.Version, &user.CreatedMS,
			&user.Disabled); err != nil {
			return nil, err
		}

		out = append(out, user)
	}

	return out, rows.Err()
}

// otherAdmins counts the enabled administrators besides one user, inside the statement that changes
// that user, so two concurrent demotions cannot both pass the check.
const otherAdmins = `(SELECT COUNT(*) FROM users WHERE role = 'admin' AND disabled_ms IS NULL AND id <> ?)`

// SetRole changes the role of a user and moves the session version, so their sessions end at the
// next request; their keys narrow at the next request because a key's scopes are the intersection
// with the owner's present role. The last enabled administrator cannot be demoted.
func (u *Users) SetRole(ctx context.Context, id, role string) (User, error) {
	if role != service.RoleAdmin && role != service.RoleMember {
		return User{}, ErrBadRole
	}

	res, err := u.ex.ExecContext(ctx,
		`UPDATE users SET role = ?, session_version = session_version + 1, updated_ms = ?
		 WHERE id = ? AND (? = 'admin' OR role <> 'admin' OR disabled_ms IS NOT NULL OR `+otherAdmins+` > 0)`,
		role, u.Now().UnixMilli(), id, role, id)

	return u.changed(ctx, res, err, id)
}

// SetDisabled disables or re-enables a user. Disabling moves the session version and makes login,
// sessions and keys of the user fail at their next request. The last enabled administrator cannot
// be disabled.
func (u *Users) SetDisabled(ctx context.Context, id string, disabled bool) (User, error) {
	now := u.Now().UnixMilli()

	var (
		res interface{ RowsAffected() (int64, error) }
		err error
	)

	if disabled {
		res, err = u.ex.ExecContext(ctx,
			`UPDATE users SET disabled_ms = ?, session_version = session_version + 1, updated_ms = ?
			 WHERE id = ? AND (role <> 'admin' OR disabled_ms IS NOT NULL OR `+otherAdmins+` > 0)`,
			now, now, id, id)
	} else {
		res, err = u.ex.ExecContext(ctx,
			`UPDATE users SET disabled_ms = NULL, session_version = session_version + 1, updated_ms = ? WHERE id = ?`,
			now, id)
	}

	return u.changed(ctx, res, err, id)
}

// Update changes the role and the disabled state of a user in one statement, so a call that sets both
// either applies both or neither, and the last-administrator guard sees the final state. A nil member
// is left alone. Sessions end at the next request, as with SetRole and SetDisabled.
func (u *Users) Update(ctx context.Context, id string, role *string, disabled *bool) (User, error) {
	if role != nil && *role != service.RoleAdmin && *role != service.RoleMember {
		return User{}, ErrBadRole
	}

	now := u.Now().UnixMilli()

	setRole, roleValue := role != nil, ""
	if role != nil {
		roleValue = *role
	}

	setDisabled := disabled != nil

	var disabledMS any
	if disabled != nil && *disabled {
		disabledMS = now
	}

	const newRole = `(CASE WHEN ? THEN ? ELSE role END)`
	const newDisabled = `(CASE WHEN ? THEN ? ELSE disabled_ms END)`

	res, err := u.ex.ExecContext(ctx,
		`UPDATE users SET role = `+newRole+`, disabled_ms = `+newDisabled+`,
		 session_version = session_version + 1, updated_ms = ?
		 WHERE id = ? AND (NOT (role = 'admin' AND disabled_ms IS NULL)
		  OR (`+newRole+` = 'admin' AND `+newDisabled+` IS NULL) OR `+otherAdmins+` > 0)`,
		setRole, roleValue, setDisabled, disabledMS, now, id,
		setRole, roleValue, setDisabled, disabledMS, id)

	return u.changed(ctx, res, err, id)
}

// changed turns the outcome of a guarded UPDATE into the user, ErrNoSuchUser or ErrLastAdmin.
func (u *Users) changed(
	ctx context.Context,
	res interface{ RowsAffected() (int64, error) },
	err error,
	id string,
) (User, error) {
	if err != nil {
		return User{}, err
	}

	if n, _ := res.RowsAffected(); n == 0 {
		if _, gerr := u.one(ctx, `WHERE id = ?`, id); errors.Is(gerr, auth.ErrUserNotFound) {
			return User{}, ErrNoSuchUser
		}

		return User{}, ErrLastAdmin
	}

	return u.one(ctx, `WHERE id = ?`, id)
}

// Admin is the user management of an administrator (SDD-007 section 4.1 item 2): list, create, change
// the role of and disable users. It is for network mode only and for a browser session only; a key
// cannot manage users, whatever its scopes.
type Admin struct {
	Users *Users
}

func forbidden(message, hint string) *service.Error {
	return &service.Error{Code: service.CodeForbidden, Message: message, Hint: hint}
}

func (a Admin) authorize(ctx context.Context) error {
	c, ok := service.CallerFrom(ctx)
	if !ok {
		return &service.Error{
			Code: "E_UNAUTHORIZED", Message: "the request carries no credentials.",
			Hint: "log in",
		}
	}

	if strings.HasPrefix(c.Actor, "key:") || !c.Browser {
		return forbidden("only a logged-in session can manage users.", "log in as an administrator in the browser")
	}

	if slices.Contains(c.Scopes, service.ScopeAdmin) {
		return nil
	}

	return forbidden("the caller lacks the \"admin\" scope.", "ask an administrator")
}

// List is UserList.
func (a Admin) List(ctx context.Context) ([]UserView, error) {
	if err := a.authorize(ctx); err != nil {
		return nil, err
	}

	users, err := a.Users.List(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]UserView, 0, len(users))
	for _, u := range users {
		out = append(out, viewOf(u))
	}

	return out, nil
}

// Create is UserCreate.
func (a Admin) Create(ctx context.Context, email, password, role string) (UserView, error) {
	if err := a.authorize(ctx); err != nil {
		return UserView{}, err
	}

	u, err := a.Users.Create(ctx, email, password, role)
	if err != nil {
		return UserView{}, userError(err)
	}

	return viewOf(u), nil
}

// UserPatch is what UserUpdate changes; a nil member is left alone.
type UserPatch struct {
	Role     *string
	Disabled *bool
}

// Update is UserUpdate.
func (a Admin) Update(ctx context.Context, id string, p UserPatch) (UserView, error) {
	if err := a.authorize(ctx); err != nil {
		return UserView{}, err
	}

	if p.Role == nil && p.Disabled == nil {
		return UserView{}, &service.Error{
			Code: command.CodeValidation, Message: "nothing to change.",
			Hint: "send a role or disabled",
		}
	}

	u, err := a.Users.Update(ctx, id, p.Role, p.Disabled)
	if err != nil {
		return UserView{}, userError(err)
	}

	return viewOf(u), nil
}

// userError maps the errors of the user store to the service error document.
func userError(err error) error {
	switch {
	case errors.Is(err, ErrLastAdmin):
		return &service.Error{
			Code: command.CodeInvalidState, Message: err.Error(),
			Hint: "make another user an administrator first",
		}
	case errors.Is(err, ErrNoSuchUser):
		return &service.Error{Code: command.CodeNotFound, Message: "there is no such user."}
	case errors.Is(err, ErrEmailTaken), errors.Is(err, ErrBadEmail), errors.Is(err, ErrBadRole),
		errors.Is(err, ErrPasswordLen):
		return &service.Error{Code: command.CodeValidation, Message: err.Error()}
	}

	return err
}
