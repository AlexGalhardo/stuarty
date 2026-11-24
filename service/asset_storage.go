package service

import (
	"os"
	"path/filepath"
	"strings"

	enginestore "github.com/AlexGalhardo/stuarty/engine/store"
	"github.com/AlexGalhardo/stuarty/service/assets"
)

// storedOriginal is where the original file of a hash is kept: store/<aa>/<sha256> under the workspace. It says
// nothing of whether the file is there, and it is never sent to a caller except by AssetReveal (SDD-011
// section 14.10).
func (s *svc) storedOriginal(hash string) (string, bool) {
	if s.workspace == "" {
		return "", false
	}

	hex := strings.TrimPrefix(hash, "sha256:")
	if !enginestore.ValidHash(hex) {
		return "", false
	}

	p, err := enginestore.New(s.workspace).Path(hex)

	return p, err == nil
}

// heldSize gives a held file its state and its size from the asset row (SDD-011 section 14.9). A file whose ingest
// has not finished is importing and has no size yet; the size of every other file is the one the row recorded.
// With check set, a file that the library says is ready but that is not in the store is missing and has no size.
func (s *svc) heldSize(h *assets.Held, ar *AssetRow, check bool) {
	switch ar.IngestState {
	case IngestPending, IngestIngesting:
		h.State, h.Bytes = assets.StateImporting, nil

		return
	case IngestFailed:
		h.State = assets.StateFailed
	default:
		h.State = assets.StateReady
	}

	n := ar.SizeBytes
	h.Bytes = &n

	if check {
		s.checkStored(h)
	}
}

// checkStored marks a held file missing when its original is not a regular file in the store. A workspace-less
// service (a test with no folder) has nothing to check and trusts the row.
func (s *svc) checkStored(h *assets.Held) {
	if h.State == assets.StateImporting {
		return
	}

	p, ok := s.storedOriginal(h.Hash)
	if !ok {
		return
	}

	if info, err := os.Stat(filepath.Clean(p)); err != nil || !info.Mode().IsRegular() {
		h.State, h.Bytes = assets.StateMissing, nil
	}
}
