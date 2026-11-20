package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/rasterize"
)

// templateFontFiles reads the font files a template document names, for its template file (ADR-0022
// item 5). In network mode a font the caller may not use is refused as in a bundle export, so a
// template cannot copy another user's file out; the message names the family, never the hash.
func (s *svc) templateFontFiles(ctx context.Context, c Caller, doc *timeline.Document) (map[string][]byte, *Error) {
	if len(doc.Fonts) == 0 {
		return nil, nil
	}

	if c.OwnerUserID != "" {
		if family, bad := s.unusableFont(ctx, s.store, doc, ""); bad {
			return nil, newError(
				CodeForbidden,
				"the font family %q of the template is a file you did not import or bring.",
				family,
			).
				withHint("import the font file with font:import, then export again").
				withPath("fonts")
		}
	}

	files := map[string][]byte{}

	for _, family := range sortedKeys(doc.Fonts) {
		for _, f := range doc.Fonts[family].Files {
			if _, done := files[f.Hash]; done {
				continue
			}

			if s.fonts == nil || !s.hasFont(f.Hash) {
				return nil, newError(
					command.CodeNotFound,
					"the font family %q of the template is not in the workspace.",
					family,
				).
					withHint("import the font file again, then export the template").
					withPath("fonts")
			}

			data, err := s.fonts.Read(f.Hash[len("sha256:"):])
			if err != nil {
				return nil, fromFont(err)
			}

			files[f.Hash] = data
		}
	}

	return files, nil
}

// checkTemplateFontEntries pairs the fonts of the document with the font entries of the file: every
// hash of the document must be in the archive and every font entry must be used (SDD-007 section
// 7.6 item 5). Each entry was already hashed against its name.
func checkTemplateFontEntries(doc *timeline.Document, entries map[string][]byte) *Error {
	used := map[string]bool{}

	for _, family := range sortedKeys(doc.Fonts) {
		for _, f := range doc.Fonts[family].Files {
			if _, ok := entries[f.Hash]; !ok {
				return archiveRefused("a font of the document is not in the file")
			}

			used[f.Hash] = true
		}
	}

	for hash := range entries {
		if !used[hash] {
			return archiveRefused("it holds a font that the document does not use")
		}
	}

	return nil
}

// storeTemplateFonts validates every font of an imported template file and stores it by content;
// a dry run only validates. A refused font stores nothing of its own. It returns the hashes.
func (s *svc) storeTemplateFonts(entries map[string][]byte, dryRun bool) ([]string, *Error) {
	for hash, data := range entries {
		sum := sha256.Sum256(data)
		if "sha256:"+hex.EncodeToString(sum[:]) != hash {
			return nil, archiveRefused("a font does not match its name")
		}

		if err := rasterize.ValidateFont(data); err != nil {
			return nil, fromFont(err)
		}
	}

	if dryRun {
		return nil, nil
	}

	hashes := make([]string, 0, len(entries))

	for _, data := range entries {
		stored, e := s.putFont(data)
		if e != nil {
			return nil, e
		}

		hashes = append(hashes, stored)
	}

	return hashes, nil
}
