package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
)

// The service writes a file only into the workspace and never takes a path from the caller (SDD-007
// T-13). --out is therefore a step of the CLI after the job succeeded: it copies the file the
// service produced, or downloads it through the download URL when the server is remote.

// outFlags are the flags of a command whose result is a file the person may want elsewhere.
func outFlags() []flagDef {
	return []flagDef{
		{
			Name:  "out",
			Kind:  kindString,
			Usage: "copy the file to this path when the job succeeds (implies --wait; the workspace keeps its own copy)",
		},
		{Name: "force", Kind: kindBool, Default: "false", Usage: "replace the --out file when it exists"},
	}
}

// outPath is the destination of --out as an absolute path, or "" when the flag is absent. A file
// that exists is refused here, before any job starts, unless --force.
func (c *call) outPath() (string, error) {
	if !c.args.has("out") {
		return "", nil
	}

	raw := expandPath(c.args.str("out"), c.rt.Getenv)
	if raw == "" {
		return "", validation("out", "--out needs a file path.")
	}

	path := filepath.Clean(raw)

	if !filepath.IsAbs(path) {
		wd, err := c.rt.Getwd()
		if err != nil {
			return "", fmt.Errorf("read the current directory: %w", err)
		}

		path = filepath.Join(wd, path)
	}

	if info, err := os.Stat(path); err == nil {
		if info.IsDir() {
			return "", validation("out", "%s is a folder; give a file path.", path)
		}

		if !c.args.boolean("force") {
			return "", validation("out", "%s exists; give another --out or --force.", path)
		}
	}

	return path, nil
}

// savedJob is a job plus the place its file was copied to.
type savedJob struct {
	*service.Job

	SavedTo string `json:"saved_to"`
}

// savedFile is a file result plus the place it was copied to.
type savedFile struct {
	*service.FileResult

	SavedTo string `json:"saved_to"`
}

// saveJobFile copies the file of a succeeded job to dest and says so in the output. A job that has
// not succeeded is left as jobOutput reported it.
func (c *call) saveJobFile(ctx context.Context, out *output, dest string) (*output, error) {
	job, ok := out.value.(*service.Job)
	if !ok || dest == "" || job.State != service.JobSucceeded {
		return out, nil
	}

	if job.Result == nil || job.Result.Export == nil {
		return nil, &service.Error{Code: command.CodeInternal, Message: "the job succeeded without a file to copy."}
	}

	res := job.Result.Export
	if err := c.copyFile(ctx, res.Path, res.DownloadURL, dest); err != nil {
		return nil, err
	}

	text := fmt.Sprintf("%s\nsaved to %s", strings.TrimRight(out.text, "\r\n"), dest)

	return &output{value: savedJob{Job: job, SavedTo: dest}, text: text}, nil
}

// saveResultFile copies the file of an export result to dest.
func (c *call) saveResultFile(ctx context.Context, res *service.FileResult, dest string) (*output, error) {
	if err := c.copyFile(ctx, res.Path, res.DownloadURL, dest); err != nil {
		return nil, err
	}

	text := fmt.Sprintf("%s\nsaved to %s", strings.TrimRight(res.Text, "\r\n"), dest)

	return &output{value: savedFile{FileResult: res, SavedTo: dest}, text: text}, nil
}

// copyFile writes dest from the local file the service reported, or from the download URL when the
// service gave no path (a remote server). It never replaces a file unless --force, and removes a
// partial file when the copy fails.
func (c *call) copyFile(ctx context.Context, local, downloadURL, dest string) error {
	var src io.ReadCloser

	switch {
	case c.download != nil:
		// A remote server's Path names a file on its machine, not ours: never open it here,
		// or a server could make the CLI copy any local file the user can read.
		if downloadURL == "" {
			return &service.Error{
				Code: command.CodeInternal, Message: "the server reported no download URL to copy to --out.",
			}
		}

		body, err := c.download(ctx, downloadURL)
		if err != nil {
			return err
		}

		src = body
	case local != "":
		f, err := os.Open(local)
		if err != nil {
			return &service.Error{
				Code: command.CodeInternal,
				Message: fmt.Sprintf(
					"the file is ready but %s cannot be read: %s.", local, readReason(err),
				),
				Hint: "copy it by hand, or read it from the download URL of the job",
			}
		}

		src = f
	default:
		return &service.Error{
			Code: command.CodeInternal, Message: "the service reported no file to copy to --out.",
		}
	}

	defer src.Close()

	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
		return fmt.Errorf("create the folder of %s: %w", dest, err)
	}

	mode := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	if c.args.boolean("force") {
		mode = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}

	dst, err := os.OpenFile(dest, mode, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return validation("out", "%s exists; give another --out or --force.", dest)
		}

		return fmt.Errorf("write %s: %w", dest, err)
	}

	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		_ = os.Remove(dest)

		return fmt.Errorf("copy to %s: %w", dest, err)
	}

	if err := dst.Close(); err != nil {
		_ = os.Remove(dest)

		return fmt.Errorf("write %s: %w", dest, err)
	}

	return nil
}
