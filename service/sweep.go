package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	enginestore "github.com/AlexGalhardo/stuarty/engine/store"
)

// exportImagesDir is the folder of exports/ that holds the exports of images, in a folder for each image.
const exportImagesDir = "images"

// orphanAge protects a file that an import copied into store/ before its row
// existed (SDD-005 section 13.4).
const orphanAge = 24 * time.Hour

// SweepResult counts what a sweep removed.
type SweepResult struct {
	StoreFiles    int `json:"store_files"`
	ArtifactDirs  int `json:"artifact_dirs"`
	ExportDirs    int `json:"export_dirs"`
	FrameDirs     int `json:"frame_dirs"`
	IncomingFiles int `json:"incoming_files"`
}

// Sweeper removes the files that an interrupted emptying left behind. The
// owner runs it at start-up and doctor reports it (SDD-005 section 13.4).
type Sweeper interface {
	Sweep(ctx context.Context) (SweepResult, error)
}

// SweeperOf returns the sweeper of a Service made by New.
func SweeperOf(s Service) (Sweeper, bool) {
	impl, ok := s.(*svc)

	return impl, ok
}

// Sweep removes, under the write side of the store lock: files of store/ with
// no row and older than 24 hours, artifacts/ directories with no row, and
// exports/ and frames/ directories of projects that do not exist, and upload
// session files older than seven days. Nothing else is touched: templates/, bitmaps/ and the database are not its business,
// and a folder reached through a link is not swept (sweepRoot).
func (s *svc) Sweep(ctx context.Context) (SweepResult, error) {
	var res SweepResult

	if s.workspace == "" {
		return res, nil
	}

	s.storeMu.Lock()
	defer s.storeMu.Unlock()

	now := s.now()

	if s.media.Content != nil {
		n, err := s.media.Content.SweepIncoming(now, orphanAge)
		if err != nil {
			return res, err
		}

		res.IncomingFiles = n

		known := func(hex string) bool { return s.assetKnown(ctx, hex) }

		if res.StoreFiles, err = s.media.Content.SweepOrphans(now, orphanAge, known); err != nil {
			return res, err
		}
	}

	var err error

	if res.ArtifactDirs, err = s.sweepDirs("artifacts", func(name string) bool {
		return !enginestore.ValidHash(name) || !s.assetKnown(ctx, name)
	}); err != nil {
		return res, err
	}

	n, err := s.sweepBundleTemps()
	if err != nil {
		return res, err
	}

	res.IncomingFiles += n

	if n, err = s.sweepUploadSessions(now); err != nil {
		return res, err
	}

	res.IncomingFiles += n + s.sweepExportTemps(ctx, now)

	missing := func(name string) bool {
		_, herr := s.store.Projects().Head(ctx, name)

		return errors.Is(herr, ErrNotFound)
	}

	// exports/images/ holds the folders of the exported images, one per image: it is no project, and what is in
	// it is dropped by the image, below.
	if res.ExportDirs, err = s.sweepDirs("exports", func(name string) bool {
		return name != exportImagesDir && missing(name)
	}); err != nil {
		return res, err
	}

	// An image document is sealed with an account (ADR-0053): while the studio is locked it cannot be read to tell
	// that the image exists, and a sweep that cannot tell deletes nothing, so the folders of images wait.
	if s.gate == nil || s.gate.Check() == nil {
		images, err := s.sweepDirs(filepath.Join("exports", exportImagesDir), func(name string) bool {
			_, herr := s.store.Images().Get(ctx, name)

			return errors.Is(herr, ErrNotFound)
		})
		if err != nil {
			return res, err
		}

		res.ExportDirs += images
	}

	res.FrameDirs, err = s.sweepDirs("frames", missing)

	return res, err
}

// assetKnown is true when an assets row exists, or when the lookup fails: a
// sweep that cannot tell deletes nothing.
func (s *svc) assetKnown(ctx context.Context, hex string) bool {
	_, err := s.store.Assets().Get(ctx, "sha256:"+hex)

	return !errors.Is(err, ErrNotFound)
}

// sweepRoot is <workspace>/<rel> when every folder of rel is a plain directory. A symbolic link or a junction
// on the way would lead the sweep outside the workspace, where a folder that is not a project or an image is
// someone's own: such a root, or one that does not exist, is not swept.
func (s *svc) sweepRoot(rel string) (string, bool) {
	root := s.workspace

	for part := range strings.SplitSeq(filepath.Clean(rel), string(filepath.Separator)) {
		root = filepath.Join(root, part)

		// A junction on Windows is not a symbolic link to Go, but it is not a directory either.
		fi, err := os.Lstat(root)
		if err != nil || !fi.IsDir() || fi.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			return "", false
		}
	}

	return root, true
}

// sweepDirs removes the subdirectories of <workspace>/<dir> that drop selects.
func (s *svc) sweepDirs(dir string, drop func(name string) bool) (int, error) {
	root, ok := s.sweepRoot(dir)
	if !ok {
		return 0, nil
	}

	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}

	if err != nil {
		return 0, err
	}

	n := 0

	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") || !drop(e.Name()) {
			continue
		}

		if os.RemoveAll(filepath.Join(root, e.Name())) == nil {
			n++
		}
	}

	return n, nil
}

// uploadSessionAge is how long the address of an upload session is kept (SDD-020 section 7.4): a network
// forgets a session within days, so an older file can only start a resume that fails.
const uploadSessionAge = 7 * 24 * time.Hour

// sweepUploadSessions removes the files of publish/uploads/ (the session files and the temporary files a
// killed save left) whose modification time is more than uploadSessionAge old. They are counted with the
// incoming files, the other partial writes.
func (s *svc) sweepUploadSessions(now time.Time) (int, error) {
	root, ok := s.sweepRoot(filepath.Join("publish", "uploads"))
	if !ok {
		return 0, nil
	}

	files, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}

	if err != nil {
		return 0, err
	}

	n := 0

	for _, f := range files {
		if !f.Type().IsRegular() {
			continue
		}

		info, ierr := f.Info()
		if ierr != nil || now.Sub(info.ModTime()) <= uploadSessionAge {
			continue
		}

		if os.Remove(filepath.Join(root, f.Name())) == nil {
			n++
		}
	}

	return n, nil
}

// sweepBundleTemps removes exports/<project>/bundle-r*.vvbundle.tmp files that a killed export left
// (SDD-005 section 13.4). Only that name, one level deep, and only files that predate this service:
// an export that runs now writes a newer one. A temp file is counted with the incoming files, the
// other partial writes.
func (s *svc) sweepBundleTemps() (int, error) {
	root, ok := s.sweepRoot("exports")
	if !ok {
		return 0, nil
	}

	projects, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}

	if err != nil {
		return 0, err
	}

	n := 0

	for _, p := range projects {
		if !p.IsDir() {
			continue
		}

		files, rerr := os.ReadDir(filepath.Join(root, p.Name()))
		if rerr != nil {
			continue
		}

		for _, f := range files {
			if f.IsDir() || !strings.HasPrefix(f.Name(), "bundle-r") || !strings.HasSuffix(f.Name(), bundleExt+".tmp") {
				continue
			}

			info, ierr := f.Info()
			if ierr != nil || !info.ModTime().Before(s.startedAt) {
				continue
			}

			if os.Remove(filepath.Join(root, p.Name(), f.Name())) == nil {
				n++
			}
		}
	}

	return n, nil
}
