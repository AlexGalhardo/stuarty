package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	enginestore "github.com/AlexGalhardo/vukem-video/engine/store"
)

// orphanAge protects a file that an import copied into store/ before its row
// existed (SDD-005 section 13.4).
const orphanAge = 24 * time.Hour

// SweepResult counts what a sweep removed.
type SweepResult struct {
	StoreFiles    int
	ArtifactDirs  int
	ExportDirs    int
	FrameDirs     int
	IncomingFiles int
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
// exports/ and frames/ directories of projects that do not exist. Nothing else
// is touched: templates/, bitmaps/ and the database are not its business.
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

	missing := func(name string) bool {
		_, herr := s.store.Projects().Head(ctx, name)

		return errors.Is(herr, ErrNotFound)
	}

	if res.ExportDirs, err = s.sweepDirs("exports", missing); err != nil {
		return res, err
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

// sweepDirs removes the subdirectories of <workspace>/<dir> that drop selects.
func (s *svc) sweepDirs(dir string, drop func(name string) bool) (int, error) {
	root := filepath.Join(s.workspace, dir)

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
