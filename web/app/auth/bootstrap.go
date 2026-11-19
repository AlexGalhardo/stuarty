package auth

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/AlexGalhardo/stuarty/service"
)

// Variables of the first administrator (SDD-000 section 10).
const (
	EnvAdminEmail    = "STUARTY_ADMIN_EMAIL"
	EnvAdminPassword = "STUARTY_ADMIN_PASSWORD"
	filePrefix       = "@file:"
)

// BootstrapAdmin creates the first administrator at start-up (SDD-007 section
// 4.1 item 2): when the user table is empty and both variables are set. The
// password may be an @file: reference, which keeps it out of the environment. It
// is created once; the variables are ignored when any user exists. The email is
// logged and the password never.
func BootstrapAdmin(ctx context.Context, users *Users, getenv func(string) string, log *slog.Logger) error {
	email, password := getenv(EnvAdminEmail), getenv(EnvAdminPassword)
	if email == "" || password == "" {
		return nil
	}

	n, err := users.Count(ctx)
	if err != nil || n > 0 {
		return err
	}

	if path, ok := strings.CutPrefix(password, filePrefix); ok {
		raw, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("%s: cannot read the password file: %w", EnvAdminPassword, err)
		}

		password = strings.TrimRight(string(raw), "\r\n")
	}

	user, err := users.Create(ctx, email, password, service.RoleAdmin)
	if err != nil {
		return fmt.Errorf(
			"%s and %s: the first administrator was not created: %w",
			EnvAdminEmail,
			EnvAdminPassword,
			err,
		)
	}

	log.InfoContext(ctx, "created the first administrator", "email", user.Email)

	return nil
}
