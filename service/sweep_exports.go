package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// exportTempAge is how old a temporary file of an export must be before the sweep removes it, besides
// predating this service: another process on the workspace may be writing a file this very moment, and a file
// it wrote a minute ago is its own.
const exportTempAge = 10 * time.Minute

// sweepExportTemps removes what a process killed in the middle of an export left (SDD-005 section 13.4):
//   - the run directory of a job under tmp/ (the partial video), when the job is not queued or running: a
//     queued job is run again by the next worker, which clears the directory itself, and a running one belongs
//     to a live process;
//   - the file .<job id>.part of exports/<project id>/, the copy that was being renamed into place;
//   - the file .tmp-* of exports/images/<image id>/, the image that was being written before its rename.
//
// Only what predates this service is removed. They are counted with the incoming files, the other partial
// writes.
func (s *svc) sweepExportTemps(ctx context.Context, now time.Time) int {
	n := 0

	n += s.sweepRunDirs(ctx)

	exports := filepath.Join(s.workspace, "exports")

	folders, err := os.ReadDir(exports)
	if err != nil {
		return n
	}

	for _, f := range folders {
		if !f.IsDir() {
			continue
		}

		if f.Name() == exportImagesDir {
			images, _ := os.ReadDir(filepath.Join(exports, f.Name()))
			for _, img := range images {
				if img.IsDir() {
					n += s.sweepTemps(filepath.Join(exports, f.Name(), img.Name()), now, func(name string) bool {
						return strings.HasPrefix(name, ".tmp-")
					})
				}
			}

			continue
		}

		n += s.sweepTemps(filepath.Join(exports, f.Name()), now, func(name string) bool {
			return strings.HasPrefix(name, ".job_") && strings.HasSuffix(name, ".part") &&
				!s.jobActive(ctx, strings.TrimSuffix(strings.TrimPrefix(name, "."), ".part"))
		})
	}

	return n
}

// sweepTemps removes the regular files of dir that match, are older than exportTempAge and predate the service.
func (s *svc) sweepTemps(dir string, now time.Time, match func(name string) bool) int {
	files, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}

	n := 0

	for _, f := range files {
		if !f.Type().IsRegular() || !match(f.Name()) {
			continue
		}

		info, ierr := f.Info()
		if ierr != nil || !info.ModTime().Before(s.startedAt) || now.Sub(info.ModTime()) < exportTempAge {
			continue
		}

		if os.Remove(filepath.Join(dir, f.Name())) == nil {
			n++
		}
	}

	return n
}

// sweepRunDirs removes the run directories tmp/<job id>/ of the jobs that are not queued or running.
func (s *svc) sweepRunDirs(ctx context.Context) int {
	root := filepath.Join(s.workspace, "tmp")

	dirs, err := os.ReadDir(root)
	if err != nil {
		return 0
	}

	n := 0

	for _, d := range dirs {
		if !d.IsDir() || !strings.HasPrefix(d.Name(), "job_") {
			continue
		}

		info, ierr := d.Info()
		if ierr != nil || !info.ModTime().Before(s.startedAt) || s.jobActive(ctx, d.Name()) {
			continue
		}

		if os.RemoveAll(filepath.Join(root, d.Name())) == nil {
			n++
		}
	}

	return n
}

// jobActive says whether the job is queued or running; when the lookup fails it says yes, so that a sweep that
// cannot tell removes nothing.
func (s *svc) jobActive(ctx context.Context, id string) bool {
	row, err := s.store.Jobs().Get(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return false
	}

	return err != nil || row.State == JobQueued || row.State == JobRunning
}
