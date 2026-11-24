package service

import (
	"context"
	"errors"
	"log/slog"

	"github.com/AlexGalhardo/stuarty/engine/export"
	"github.com/AlexGalhardo/stuarty/service/account"
)

// Lifecycle is what the Service asks of the account manager: to be told when the studio locks and when it unlocks
// (ADR-0053). account.Manager is the one implementation.
type Lifecycle interface {
	OnLock(fn func())
	OnUnlock(fn func())
}

// imageCache is the cache of the pictures of the lower layers of an image document.
func (s *svc) imageCache() *export.ImageCache {
	s.framesMu.RLock()
	defer s.framesMu.RUnlock()

	return s.imageFrames
}

// dropContent runs when the studio locks: every cache that holds content made readable by the key is forgotten, so
// nothing outlives the data key (ADR-0053 item 7, SDD-007 T-134). A render in progress keeps the cache it started
// with and lets it go when it ends.
func (s *svc) dropContent() {
	s.framesMu.Lock()
	s.imageFrames = export.NewImageCache(0)
	s.framesMu.Unlock()

	s.bitmaps.reset()
	s.snaps.reset()
	s.previews.reset()

	if s.lib != nil {
		if err := s.lib.Drop(context.Background()); err != nil {
			s.log.Error("library: the index could not be dropped at lock", "err", err)
		}
	}
}

// openContent runs when a person has unlocked the studio, after the keys are in place and while the gate is still
// closed: the library is sealed where it still holds clear files (SDD-005 section 5.17.4) and its index is built.
func (s *svc) openContent() {
	if s.lib == nil {
		return
	}

	ctx := context.Background()

	if _, err := s.lib.SealAll(ctx); err != nil && !errors.Is(err, account.ErrLocked) {
		s.logger().Error("library: the files could not all be sealed; the scan retries at the next unlock", "err", err)
	}

	if _, err := s.lib.Reindex(ctx); err != nil && !errors.Is(err, account.ErrLocked) {
		s.logger().Error("library: the index could not be built at unlock", "err", err)
	}
}

func (s *svc) logger() *slog.Logger {
	if s.log != nil {
		return s.log
	}

	return slog.Default()
}

// reset forgets the specs of the bitmaps a plan handed out (they hold the text of a caption) and the caches that
// drew them.
func (b *bitmaps) reset() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.specs, b.order, b.cache, b.caches = nil, nil, nil, nil
}

func (c *snapCache) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.m = nil
}

func (p *audioPreviews) reset() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.known, p.order = nil, nil
}

// sealCheck is the doctor line for the encryption of the workspace (ADR-0053, SDD-005 section 5.17.4): whether an
// account exists and whether everything it covers is sealed. It reads no content.
func (s *svc) sealCheck(ctx context.Context) DoctorCheck {
	check := DoctorCheck{Name: "workspace.seal", OK: true}

	// Without a gate there is no account manager in this process (network mode, a test): nothing is sealed.
	if s.gate == nil || s.store == nil {
		check.Detail = "no account in this process: nothing is sealed"

		return check
	}

	rec, err := s.store.Accounts().Get(ctx)

	switch {
	case errors.Is(err, account.ErrNoAccount):
		check.Detail = "no account: nothing is sealed"
	case err != nil:
		check.OK, check.Detail = false, "the account record cannot be read"
	case rec.SealVersion >= account.SealedAll:
		check.Detail = "everything the account covers is sealed (version 1)"
	default:
		check.Detail = "the documents, the edit log and the asset notes are not sealed yet (version 0)"
		check.Hint = "they are sealed at the next unlock"
	}

	return check
}
