package transcribe

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

const (
	maxMemberBytes  = 512 << 20
	maxArchiveBytes = 2 << 30
)

// ExtractHelper unpacks a helper archive whose SHA-256 has already matched the manifest into dir, which
// must be a new, empty folder. It takes the members the manifest lists, by the exact names the manifest
// gives and never by the names the archive carries (SDD-007 section 6.8 item 5); an entry the manifest does
// not list is never written, whatever its name says. The program is written 0700, the other files 0600,
// and the links of the manifest are made beside them (not on Windows). It returns the SHA-256 of the
// program, which is what tools/installed.json records and each start verifies.
func ExtractHelper(archive, dir string, e *ManifestEntry) (programSHA256 string, err error) {
	switch e.Format {
	case "zip":
		err = unzip(archive, dir, e.Members)
	case "tar.gz":
		err = untar(archive, dir, e.Members)
	default:
		err = fmt.Errorf("unknown archive format %q", e.Format)
	}

	if err != nil {
		return "", unavailable(ReasonBadArchive, "The archive does not hold what the manifest lists.", err, nil)
	}

	if runtime.GOOS != "windows" {
		for _, l := range e.Links {
			if err := os.Symlink(l.Target, filepath.Join(dir, l.Name)); err != nil {
				return "", unavailable(ReasonBadArchive, "A link of the helper could not be made.", err, nil)
			}
		}
	}

	sum, err := fileSHA256(filepath.Join(dir, e.File))
	if err != nil {
		return "", unavailable(ReasonBadArchive, "The helper program is missing.", err, nil)
	}

	return sum, nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}

	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// memberTable maps the name an archive entry must have to the member the manifest names for it.
func memberTable(members []Member) map[string]Member {
	t := make(map[string]Member, len(members))
	for _, m := range members {
		t[m.From] = m
	}

	return t
}

func writeMember(dir string, m Member, r io.Reader) error {
	mode := os.FileMode(0o600)
	if m.Executable {
		mode = 0o700
	}

	f, err := os.OpenFile(filepath.Join(dir, m.To), os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}

	n, err := io.Copy(f, io.LimitReader(r, maxMemberBytes+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}

	if err == nil && n > maxMemberBytes {
		err = fmt.Errorf("member %s is over %d bytes", m.To, maxMemberBytes)
	}

	return err
}

func unzip(archive, dir string, members []Member) error {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}

	defer func() { _ = zr.Close() }()

	want := memberTable(members)
	done := map[string]bool{}

	var total uint64

	for _, f := range zr.File {
		m, ok := want[f.Name]
		if !ok || !f.Mode().IsRegular() || done[f.Name] {
			continue
		}

		if total += f.UncompressedSize64; total > maxArchiveBytes {
			return errors.New("the archive unpacks to too much")
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		err = writeMember(dir, m, rc)
		_ = rc.Close()

		if err != nil {
			return err
		}

		done[f.Name] = true
	}

	return allDone(members, done)
}

func untar(archive, dir string, members []Member) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}

	defer func() { _ = f.Close() }()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}

	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	want := memberTable(members)
	done := map[string]bool{}

	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return err
		}

		m, ok := want[h.Name]
		if !ok || h.Typeflag != tar.TypeReg || done[h.Name] {
			continue
		}

		if err := writeMember(dir, m, tr); err != nil {
			return err
		}

		done[h.Name] = true
	}

	return allDone(members, done)
}

func allDone(members []Member, done map[string]bool) error {
	for _, m := range members {
		if !done[m.From] {
			return fmt.Errorf("the member %q is not in the archive", m.From)
		}
	}

	return nil
}
