package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/workspace"
	"github.com/AlexGalhardo/stuarty/web/app/agentdocs"
	"github.com/AlexGalhardo/stuarty/web/app/owner"
	"github.com/AlexGalhardo/stuarty/web/app/store"
)

// isWorkspaceCommand tells the two commands that back up and restore the workspace (SDD-009 section 21.4).
func isWorkspaceCommand(name string) bool {
	return name == "workspace:backup" || name == "workspace:restore"
}

const (
	backupUsage  = "usage: stuarty workspace:backup <folder>"
	restoreUsage = "usage: stuarty workspace:restore <folder> [--replace] [--without-vault]"
)

// workspaceIO is where the workspace commands read and write.
type workspaceIO struct {
	Out, Err io.Writer
	Getenv   func(string) string
}

func processWorkspaceIO() workspaceIO {
	return workspaceIO{Out: os.Stdout, Err: os.Stderr, Getenv: os.Getenv}
}

// runWorkspace is `workspace:backup` and `workspace:restore`. A backup takes no lock: it snapshots the
// database through the database itself, so it runs beside a running server. A restore runs in process
// under the workspace lock, so no server may own the workspace (E_WORKSPACE_LOCKED).
func runWorkspace(ctx context.Context, name string, args []string, w workspaceIO) int {
	folder, replace, withoutVault, err := parseWorkspaceArgs(name, args)
	if err != nil {
		return workspaceFail(w, err)
	}

	if name == "workspace:backup" {
		return runBackup(ctx, folder, w)
	}

	return runRestore(ctx, folder, replace, withoutVault, w)
}

func parseWorkspaceArgs(name string, args []string) (folder string, replace, withoutVault bool, err error) {
	usage := backupUsage
	if name == "workspace:restore" {
		usage = restoreUsage
	}

	for _, a := range args {
		switch {
		case a == "--replace" && name == "workspace:restore":
			replace = true
		case a == "--without-vault" && name == "workspace:restore":
			withoutVault = true
		case strings.HasPrefix(a, "-"):
			return "", false, false, fmt.Errorf("unknown option %q. %s", a, usage)
		case folder == "":
			folder = a
		default:
			return "", false, false, fmt.Errorf("one folder only, got a second argument. %s", usage)
		}
	}

	if folder == "" {
		return "", false, false, errors.New(usage)
	}

	return folder, replace, withoutVault, nil
}

func runBackup(ctx context.Context, folder string, w workspaceIO) int {
	dir, err := workspace.Resolve(w.Getenv)
	if err != nil {
		return workspaceFail(w, err)
	}

	out, err := filepath.Abs(folder)
	if err != nil {
		return workspaceFail(w, err)
	}

	// The command is a process with no password: a workspace that has an account is locked for it, and nothing is
	// copied out of a locked studio (SDD-007 section 3.9 item 6).
	if hasAccount, err := store.DatabaseHasAccount(ctx, dir.Path("db.sqlite")); err != nil {
		return workspaceFail(w, err)
	} else if hasAccount {
		le := service.NewLockedError()
		_, _ = fmt.Fprintf(w.Err, "stuarty: %s: %s %s\n", le.Code, le.Message, le.Hint)

		return 1
	}

	m, err := workspace.Backup(ctx, workspace.BackupOptions{
		Dir: dir, Out: out, Version: agentdocs.Version, Snapshot: store.SnapshotDatabase,
	})
	if err != nil {
		return workspaceFail(w, err)
	}

	var size int64
	for _, f := range m.Files {
		size += f.Size
	}

	_, _ = fmt.Fprintf(w.Out, "backup of %s written to %s: %d files, %d bytes, checksums in %s and %s\n",
		dir, out, len(m.Files), size, workspace.ManifestFile, workspace.SumsFile)

	hasAccount, err := store.DatabaseHasAccount(ctx, filepath.Join(out, "db.sqlite"))
	if err != nil {
		return workspaceFail(w, err)
	}

	if hasAccount {
		_, _ = fmt.Fprintln(w.Out, "This workspace has a local account: its texts and its vault stay encrypted in "+
			"this backup and open again only with your password or your recovery key. Keep the recovery key. "+
			"The files of memory, prompts and skills are not encrypted, and neither are media files.")

		return 0
	}

	_, _ = fmt.Fprintln(w.Out, "The master key is NOT in this backup: keep it where you keep your other secrets "+
		"(the file named by STUARTY_MASTER_KEY_FILE, or STUARTY_MASTER_KEY). Without it the connected accounts "+
		"of a restored vault cannot be opened.")

	return 0
}

// errReplaceAccount refuses --replace over a workspace that has a local account: the command runs with no password,
// so it would let anyone who can run it swap a locked studio for another (ADR-0052).
var errReplaceAccount = errors.New("this workspace has a local account and --replace would change it without the " +
	"password: restore into an empty workspace, or move this folder away yourself first")

func runRestore(ctx context.Context, folder string, replace, withoutVault bool, w workspaceIO) int {
	sess, err := workspace.Open(ctx, workspace.Options{Force: workspace.InProcess, Getenv: w.Getenv})
	if err != nil {
		return workspaceFail(w, err)
	}

	defer func() { _ = sess.Close() }()

	from, err := filepath.Abs(folder)
	if err != nil {
		return workspaceFail(w, err)
	}

	if replace {
		hasAccount, aerr := store.DatabaseHasAccount(ctx, sess.Dir.Path("db.sqlite"))
		if aerr != nil {
			return workspaceFail(w, aerr)
		}

		if hasAccount {
			return workspaceFail(w, errReplaceAccount)
		}
	}

	// The vault of a backup that has an account is sealed by a key only the password or the recovery key opens,
	// and the key of this machine has no part in it: the checksums of the manifest are what can be checked now.
	backupHasAccount, err := store.DatabaseHasAccount(ctx, filepath.Join(from, "db.sqlite"))
	if err != nil {
		return workspaceFail(w, err)
	}

	res, err := workspace.Restore(ctx, workspace.RestoreOptions{
		Dir: sess.Dir, From: from, Replace: replace, WithoutVault: withoutVault,
		VerifyVault: func(vaultPath string) error {
			if backupHasAccount {
				return nil
			}

			return owner.VerifyVault(w.Getenv, string(sess.Dir), vaultPath)
		},
	})
	if err != nil {
		if errors.Is(err, workspace.ErrVaultKey) {
			_, _ = fmt.Fprintln(w.Err, "stuarty: "+err.Error())
			_, _ = fmt.Fprintln(
				w.Err,
				"The master key is not in a backup. Put the key that sealed this vault where this "+
					"command reads it (STUARTY_MASTER_KEY_FILE or STUARTY_MASTER_KEY) and run it again, or add "+
					"--without-vault to restore everything else and connect the accounts again.",
			)

			return 1
		}

		return workspaceFail(w, err)
	}

	if err := sess.Dir.EnsureLayout(); err != nil {
		return workspaceFail(w, err)
	}

	_, _ = fmt.Fprintf(
		w.Out,
		"restored %d files into %s (checked against the manifest of the backup)\n",
		len(res.Manifest.Files),
		sess.Dir,
	)
	if res.ReplacedTo != "" {
		_, _ = fmt.Fprintf(w.Out, "the data that was there is kept in %s\n", res.ReplacedTo)
	}

	if res.VaultDrop {
		_, _ = fmt.Fprintln(w.Out, "the vault was left out: connect the accounts again")
	}

	return 0
}

func workspaceFail(w workspaceIO, err error) int {
	if we, ok := errors.AsType[*workspace.Error](err); ok {
		_, _ = fmt.Fprintf(w.Err, "stuarty: %s: %s\n", we.Code, we.Message)

		if we.Retryable {
			return 2
		}

		return 1
	}

	_, _ = fmt.Fprintf(w.Err, "stuarty: %s\n", err)

	return 1
}
