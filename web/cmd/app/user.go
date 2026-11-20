package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/workspace"
	"github.com/AlexGalhardo/stuarty/web/app/auth"
	"github.com/AlexGalhardo/stuarty/web/app/store"
)

// isUserCommand tells the two commands that manage the users of network mode
// (SDD-000 section 1, "Other CLI commands").
func isUserCommand(name string) bool { return name == "user:create" || name == "user:passwd" }

// userIO is where the user commands read and write. In is the standard input;
// Terminal is the same input as a file when it is one, so that a person at a
// terminal is asked for the password without echo.
type userIO struct {
	In       io.Reader
	Terminal *os.File
	Out, Err io.Writer
	Getenv   func(string) string
}

func processUserIO() userIO {
	return userIO{In: os.Stdin, Terminal: os.Stdin, Out: os.Stdout, Err: os.Stderr, Getenv: os.Getenv}
}

// runUser is `user:create` and `user:passwd`. They run in process under the
// workspace lock, so no server may own the workspace (E_WORKSPACE_LOCKED), and
// the password comes from a terminal prompt or from standard input, never from
// the arguments, where the process list would show it (SDD-007 section 4.1).
func runUser(ctx context.Context, name string, args []string, u userIO) int {
	email, admin, err := parseUserArgs(name, args)
	if err != nil {
		return userFail(u, err.Error())
	}

	sess, err := workspace.Open(ctx, workspace.Options{Force: workspace.InProcess, Getenv: u.Getenv})
	if err != nil {
		return userFailWith(u, err)
	}

	defer func() { _ = sess.Close() }()

	// With STUARTY_URL set, Open answers with a remote session that has no folder and no lock: the
	// store would then be created in the current directory and the user would never reach the server.
	if sess.Mode != workspace.ModeInProcess || sess.Dir == "" {
		return userFail(u, fmt.Sprintf(
			"%s writes the local workspace and %s names a server: unset it and run the command on the "+
				"machine of the workspace, with no server running", name, workspace.EnvURL))
	}

	if err := sess.Dir.EnsureLayout(); err != nil {
		return userFailWith(u, err)
	}

	password, err := readPassword(u)
	if err != nil {
		return userFail(u, err.Error())
	}

	st, err := store.Open(ctx, sess.Dir, store.Options{SchemaVersion: timeline.CurrentSchemaVersion})
	if err != nil {
		return userFailWith(u, err)
	}

	defer func() { _ = st.Close() }()

	net, err := auth.NewNetwork(st.DB(), auth.NetworkOptions{})
	if err != nil {
		return userFailWith(u, err)
	}

	switch name {
	case "user:create":
		role := service.RoleMember
		if admin {
			role = service.RoleAdmin
		}

		usr, err := net.Users.Create(ctx, email, password, role)
		if err != nil {
			return userFailWith(u, err)
		}

		_, _ = fmt.Fprintf(u.Out, "created the %s %s\n", usr.Role, usr.Email)
	default:
		usr, err := net.ChangePassword(ctx, email, password)
		if err != nil {
			return userFailWith(u, err)
		}

		_, _ = fmt.Fprintf(u.Out, "changed the password of %s; its sessions and keys are revoked\n", usr.Email)
	}

	return 0
}

const (
	createUsage = "usage: stuarty user:create <email> [--admin]"
	passwdUsage = "usage: stuarty user:passwd <email>" //nolint:gosec // a usage line, not a credential
)

func parseUserArgs(name string, args []string) (email string, admin bool, err error) {
	usage := passwdUsage
	if name == "user:create" {
		usage = createUsage
	}

	for _, a := range args {
		switch {
		case a == "--admin" && name == "user:create":
			admin = true
		case strings.HasPrefix(a, "-"):
			// The argument is not echoed: it may be a password typed in the wrong place.
			return "", false, fmt.Errorf("an argument starting with - is not an option of this command; the "+
				"password is never an argument: it is read from the terminal or from standard input. %s", usage)
		case email == "":
			email = a
		default:
			return "", false, errors.New(usage)
		}
	}

	if email == "" {
		return "", false, errors.New(usage)
	}

	return email, admin, nil
}

// readPassword asks a terminal twice, without echo, or takes the first line of
// standard input.
func readPassword(u userIO) (string, error) {
	if u.Terminal == nil || !isTerminal(u.Terminal) {
		return readLine(u.In)
	}

	var first, second string

	err := withoutEcho(u.Terminal, func() error {
		var err error

		_, _ = fmt.Fprint(u.Err, "Password: ")

		if first, err = readLine(u.Terminal); err != nil {
			return err
		}

		_, _ = fmt.Fprint(u.Err, "\nPassword again: ")

		second, err = readLine(u.Terminal)

		_, _ = fmt.Fprintln(u.Err)

		return err
	})
	if err != nil {
		return "", err
	}

	if first != second {
		return "", errors.New("the two passwords differ")
	}

	return first, nil
}

// readLine reads one line a byte at a time, so that nothing past it is consumed.
func readLine(r io.Reader) (string, error) {
	line, err := bufio.NewReaderSize(&oneByte{r}, 1).ReadString('\n')
	if err != nil && (!errors.Is(err, io.EOF) || line == "") {
		return "", errors.New("no password was given: type it at the prompt or send it on standard input")
	}

	return strings.TrimRight(line, "\r\n"), nil
}

// oneByte hands out one byte per read.
type oneByte struct{ r io.Reader }

func (o *oneByte) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	return o.r.Read(p[:1])
}

func userFailWith(u userIO, err error) int {
	if we, ok := errors.AsType[*workspace.Error](err); ok {
		_, _ = fmt.Fprintf(u.Err, "stuarty: %s: %s\n", we.Code, we.Message)

		if we.Retryable {
			return 2
		}

		return 1
	}

	return userFail(u, err.Error())
}

func userFail(u userIO, msg string) int {
	_, _ = fmt.Fprintln(u.Err, "stuarty:", msg)

	return 1
}
