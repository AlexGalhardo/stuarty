package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/asset"
	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service/assets"
)

// AssetRevealRequest names a stored file of the library by its content hash. It never carries a path
// (SDD-011 section 14.10).
type AssetRevealRequest struct {
	Hash string `json:"hash"`
}

// AssetRevealResult is the checked path of the stored original, for a person to open its folder. Path is the
// resolved absolute path (the file manager takes it); Display is the same path with control and bidirectional
// characters written out, which is what a page draws. It is personal data: it is never logged, put in an event
// or an audit row, or returned by any other operation.
type AssetRevealResult struct {
	Path    string `json:"path"`
	Display string `json:"display"`
}

// AssetReveal returns the path of the stored original of a file of the library, only to a person in a privileged
// browser session on the owner machine (local mode). The checks run in this order and the first that fails
// answers (SDD-011 section 14.10): the studio is locked, no caller, a caller that is not a person, network mode,
// a hash nobody holds, a file in a trash, a file still importing, a file missing from the store, a stored path
// that resolves outside the media root.
func (s *svc) AssetReveal(ctx context.Context, req AssetRevealRequest) (*AssetRevealResult, error) {
	c, err := s.person(ctx, "AssetReveal")
	if err != nil {
		return nil, err
	}

	if c.OwnerUserID != "" {
		return nil, newError(command.CodeNotFound, "this operation exists in local mode only.").
			withDetail("reason", "network_mode")
	}

	h, _, ferr := s.heldFile(ctx, c, req.Hash)
	if ferr != nil {
		return nil, ferr
	}

	switch h.State {
	case assets.StateImporting:
		return nil, newError("E_ASSET_NOT_READY", "the file is still importing.").
			withHint("wait until the import ends, then open its folder")
	case assets.StateMissing:
		return nil, fileMissing()
	}

	stored, ok := s.storedOriginal(h.Hash)
	if !ok {
		return nil, fileMissing()
	}

	resolved, rerr := resolveUnderRoot(filepath.Dir(filepath.Dir(stored)), stored)
	if rerr != nil {
		return nil, rerr
	}

	return &AssetRevealResult{Path: resolved, Display: asset.DisplayPath(resolved)}, nil
}

func fileMissing() *Error {
	return newError("E_FILE_NOT_FOUND", "the file is missing from the workspace.").
		withDetail("reason", "file_missing").
		withHint("the stored file was deleted or moved outside Stuarty; import it again")
}

// resolveUnderRoot is the stored path of a file with its symbolic links resolved, if it is a regular file under
// the media root (also resolved) and refused otherwise. Neither message nor detail carries a path.
func resolveUnderRoot(root, stored string) (string, *Error) {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fileMissing()
	}

	target, err := filepath.EvalSymlinks(stored)
	if err != nil {
		// A file that exists but whose links cannot be resolved (a Windows junction, a link in a loop) cannot be
		// proved to be under the media root: refused, not guessed.
		if _, serr := os.Stat(stored); serr == nil {
			return "", newError(
				"E_PATH_NOT_ALLOWED",
				"the stored file is reached through a link that cannot be checked.",
			).
				withDetail("reason", "link_unresolved").
				withHint("Stuarty opens only files it stored; the folder that holds them must not be a link it cannot resolve")
		}

		return "", fileMissing()
	}

	info, err := os.Stat(target)
	if err != nil || !info.Mode().IsRegular() {
		return "", fileMissing()
	}

	if !underRoot(realRoot, target) {
		return "", newError("E_PATH_NOT_ALLOWED", "the stored file is outside the media folder of the workspace.").
			withDetail("reason", "outside_media_root").
			withHint("Stuarty opens only files it stored; the link that leads out of the media folder was not followed")
	}

	return target, nil
}

// underRoot is whether target, a resolved path, lies strictly inside root, a resolved folder: by the relative
// path, never by a text prefix, so /ws/store-evil is not under /ws/store.
func underRoot(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil || filepath.IsAbs(rel) || rel == "." || rel == ".." {
		return false
	}

	return !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// trashedFile says why a hash that no active project holds may still be a file of the caller: every use of it is
// in a trash. A hash the caller never held answers as not found (SDD-007 T-20).
func (s *svc) trashedFile(ctx context.Context, c Caller, hash string) error {
	targets, err := s.renameTargets(ctx, c, &assets.Held{Hash: hash})
	if err != nil {
		return err
	}

	if len(targets) == 0 {
		return nil
	}

	return newError(command.CodeNotFound, "the file is in the trash of its project.").
		withDetail("reason", "in_trash").
		withHint("restore it from the trash first")
}
