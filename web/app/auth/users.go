package auth

import (
	"context"
	"database/sql"
	"errors"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service"

	"github.com/AlexGalhardo/vukem/auth"
	"github.com/AlexGalhardo/vukem/auth/hash"
	"github.com/AlexGalhardo/vukem/database"
)

// Password length bounds: the floor keeps a guessable password out, the ceiling
// keeps a request from making argon2id hash megabytes.
const (
	MinPasswordLength = 12
	MaxPasswordLength = 1024
	maxEmailLength    = 254
	// UserPrefix is the prefix of a user id (SDD-000 section 2).
	UserPrefix = "usr"
)

// Errors of the user store; the callers turn them into their own surface.
var (
	ErrEmailTaken  = errors.New("a user with that email already exists")
	ErrNoSuchUser  = errors.New("there is no user with that email")
	ErrBadEmail    = errors.New("the email is not a valid address")
	ErrBadRole     = errors.New("the role must be admin or member")
	ErrPasswordLen = errors.New("the password must have 12 to 1024 characters")
)

// User is a row of the users table. It satisfies vukem's auth.Authenticatable
// and auth.SessionVersioner, so the session guard ends a session when the
// version moves.
type User struct {
	ID        string
	Email     string
	Password  string // the argon2id hash
	Role      string
	Version   int64
	CreatedMS int64
	// Disabled users keep their row; login, sessions and keys treat them as not found.
	Disabled bool
}

// AuthID implements auth.Authenticatable.
func (u User) AuthID() string { return u.ID }

// AuthPassword implements auth.Authenticatable.
func (u User) AuthPassword() string { return u.Password }

// AuthSessionVersion implements auth.SessionVersioner.
func (u User) AuthSessionVersion() int64 { return u.Version }

// Scopes are the scopes of the role.
func (u User) Scopes() []string { return service.RoleScopes(u.Role) }

// Users is the user table of network mode over vukem's hasher and guard.
type Users struct {
	ex     database.Executor
	Hasher hash.Hasher
	Now    func() time.Time
}

// NewUsers uses argon2id with vukem's default parameters.
func NewUsers(ex database.Executor) *Users {
	return &Users{ex: ex, Hasher: hash.Default(), Now: time.Now}
}

// NormalizeEmail trims and lower-cases an address and refuses what is not a
// plain mailbox: login keys on it, so the stored and the typed form agree.
func NormalizeEmail(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))
	if email == "" || len(email) > maxEmailLength {
		return "", ErrBadEmail
	}

	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email || addr.Name != "" {
		return "", ErrBadEmail
	}

	return email, nil
}

// CheckPassword applies the length bounds.
func CheckPassword(password string) error {
	if n := utf8.RuneCountInString(password); n < MinPasswordLength || len(password) > MaxPasswordLength {
		return ErrPasswordLen
	}

	return nil
}

// Count is how many users exist.
func (u *Users) Count(ctx context.Context) (int, error) {
	var n int
	err := u.ex.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)

	return n, err
}

// Create adds a user with a hashed password.
func (u *Users) Create(ctx context.Context, email, password, role string) (User, error) {
	email, err := NormalizeEmail(email)
	if err != nil {
		return User{}, err
	}

	if role != service.RoleAdmin && role != service.RoleMember {
		return User{}, ErrBadRole
	}

	if err := CheckPassword(password); err != nil {
		return User{}, err
	}

	h, err := u.Hasher.Make(password)
	if err != nil {
		return User{}, err
	}

	now := u.Now().UnixMilli()
	user := User{ID: timeline.NewID(UserPrefix), Email: email, Password: h, Role: role, CreatedMS: now}

	_, err = u.ex.ExecContext(
		ctx,
		`INSERT INTO users (id, email, password, role, session_version, created_ms, updated_ms) VALUES (?, ?, ?, ?, 0, ?, ?)`,
		user.ID,
		user.Email,
		user.Password,
		user.Role,
		now,
		now,
	)
	if err != nil {
		if uc, ok := u.ex.(interface{ UniqueViolation(error) bool }); ok && uc.UniqueViolation(err) {
			return User{}, ErrEmailTaken
		}

		return User{}, err
	}

	return user, nil
}

// SetPassword replaces the password of the user with that email and moves the
// session version, so every browser session of the user ends at its next
// request (SDD-007 section 4.1 item 4). The caller revokes the keys.
func (u *Users) SetPassword(ctx context.Context, email, password string) (User, error) {
	email, err := NormalizeEmail(email)
	if err != nil {
		return User{}, err
	}

	if err := CheckPassword(password); err != nil {
		return User{}, err
	}

	h, err := u.Hasher.Make(password)
	if err != nil {
		return User{}, err
	}

	res, err := u.ex.ExecContext(ctx,
		`UPDATE users SET password = ?, session_version = session_version + 1, updated_ms = ? WHERE email = ?`,
		h, u.Now().UnixMilli(), email)
	if err != nil {
		return User{}, err
	}

	if n, _ := res.RowsAffected(); n == 0 {
		return User{}, ErrNoSuchUser
	}

	return u.ByEmail(ctx, email)
}

// ByEmail finds a user; auth.ErrUserNotFound when there is none.
func (u *Users) ByEmail(ctx context.Context, email string) (User, error) {
	return u.one(ctx, `WHERE email = ?`, strings.ToLower(strings.TrimSpace(email)))
}

// ByID implements auth.UserProvider.
func (u *Users) ByID(ctx context.Context, id string) (User, error) {
	user, err := u.one(ctx, `WHERE id = ?`, id)
	if err == nil && user.Disabled {
		return User{}, auth.ErrUserNotFound
	}

	return user, err
}

// ByCredentials implements auth.UserProvider: it looks the user up by the
// "email" credential and never checks the password.
func (u *Users) ByCredentials(ctx context.Context, creds map[string]string) (User, error) {
	user, err := u.ByEmail(ctx, creds["email"])
	if err == nil && user.Disabled {
		return User{}, auth.ErrUserNotFound
	}

	return user, err
}

// UpdatePassword implements auth.PasswordUpdater: the guard stores a hash made
// with newer parameters after a login. It does not move the session version.
func (u *Users) UpdatePassword(ctx context.Context, user User, hashed string) error {
	_, err := u.ex.ExecContext(ctx, `UPDATE users SET password = ? WHERE id = ?`, hashed, user.ID)

	return err
}

func (u *Users) one(ctx context.Context, where string, args ...any) (User, error) {
	var user User

	err := u.ex.QueryRowContext(ctx,
		`SELECT id, email, password, role, session_version, created_ms, disabled_ms IS NOT NULL FROM users `+where,
		args...).
		Scan(&user.ID, &user.Email, &user.Password, &user.Role, &user.Version, &user.CreatedMS, &user.Disabled)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, auth.ErrUserNotFound
	}

	return user, err
}
