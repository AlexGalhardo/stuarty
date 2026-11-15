package export

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// idRe is the shape of a project or job identifier: letters, digits, '_' and '-', never a separator
// or a dot, so a name cannot leave the exports folder (SDD-007 T-13).
var idRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

// exportPath is exports/<project id>/<job id>.mp4 under dir, from identifiers that passed idRe. The
// name of a project is never part of it.
func exportPath(dir, projectID, jobID string) (string, error) {
	if dir == "" || !idRe.MatchString(projectID) || !idRe.MatchString(jobID) {
		return "", errors.New("an export path is made of a folder and two identifiers")
	}

	return filepath.Join(dir, projectID, jobID+".mp4"), nil
}

// publishUnder copies the verified file into <dir>/<project id>/ and renames it to <job id>.mp4, with
// every step through os.Root so no name can leave dir, and the folder of the project is mode 0700.
// The file appears under its final name only whole. It returns the path of the published file.
func publishUnder(dir, projectID, jobID, ext, partial string) (string, error) {
	final, err := exportPath(dir, projectID, jobID)
	if err != nil {
		return "", err
	}
	final = strings.TrimSuffix(final, ".mp4") + ext

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}

	root, err := os.OpenRoot(dir)
	if err != nil {
		return "", err
	}
	defer func() { _ = root.Close() }()

	if err := root.Mkdir(projectID, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return "", err
	}

	tmp := filepath.Join(projectID, "."+jobID+".part")
	name := filepath.Join(projectID, jobID+ext)

	if err := copyInto(root, partial, tmp); err != nil {
		_ = root.Remove(tmp)

		return "", err
	}

	if err := root.Rename(tmp, name); err != nil {
		_ = root.Remove(tmp)

		return "", fmt.Errorf("renaming the export into place: %w", err)
	}

	return final, nil
}

func copyInto(root *os.Root, src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := root.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}

	defer func() {
		if cerr := out.Close(); err == nil {
			err = cerr
		}
	}()

	if _, err = io.Copy(out, in); err != nil {
		return err
	}

	return out.Sync()
}
