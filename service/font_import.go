package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/core/plan"
	"github.com/AlexGalhardo/vukem-video/core/timeline"
	"github.com/AlexGalhardo/vukem-video/engine/frame"
	"github.com/AlexGalhardo/vukem-video/engine/raster"
	enginestore "github.com/AlexGalhardo/vukem-video/engine/store"
)

// FontSetFor returns the font set a render of doc uses: the bundled families plus the user fonts
// of the document, read by hash from the font store of the workspace (SDD-007 section 7.7). A
// document without user fonts gets the shared bundled set. The set is never shared between
// documents, so two projects that name a family alike never see each other's files.
func FontSetFor(workspace string, doc *timeline.Document) *raster.FontSet {
	if workspace == "" || doc == nil || len(doc.Fonts) == 0 {
		return raster.Bundled()
	}

	return raster.NewFontSet(doc.Fonts, enginestore.NewFonts(workspace).Source())
}

// fontSet is FontSetFor for the workspace of the service.
func (s *svc) fontSet(doc *timeline.Document) *raster.FontSet {
	return FontSetFor(s.workspace, doc)
}

// putFont validates a font file and stores it by content, the one door through which a font
// enters the workspace (bundle import, and the font import of the brand kit). A refused font
// stores nothing. It returns the hash in the form of a document, "sha256:" and 64 hex digits.
func (s *svc) putFont(data []byte) (string, *Error) {
	if s.fonts == nil {
		return "", newError(command.CodeInternal, "fonts are not available in this build.")
	}

	hash, err := s.fonts.Put(data)
	if err != nil {
		return "", fromFont(err)
	}

	return "sha256:" + hash, nil
}

// hasFont tells whether the font store holds the font of a document hash.
func (s *svc) hasFont(hash string) bool {
	return s.fonts != nil && s.fonts.Has(strings.TrimPrefix(hash, "sha256:"))
}

func fromFont(err error) *Error {
	if ife, ok := errors.AsType[*raster.InvalidFontError](err); ok {
		return newError(ife.Code(), "the font file is refused: %s.", ife.Message).
			withHint("use a TrueType or OpenType font of at most %d MiB", raster.MaxFontBytes>>20).
			withDetail("reason", ife.Reason)
	}

	return fromEngine(err)
}

// checkFontAdd refuses a font.add that names a file the font store does not hold (E_NOT_FOUND),
// as the command assumes (SDD-002 C.53, SDD-007 section 5.4 item 3). The core checks only the
// form of the hash. A malformed operation is left to the command.
func (s *svc) checkFontAdd(op *command.Operation, index int) *Error {
	var a struct {
		Files []struct {
			Hash string `json:"hash"`
		} `json:"files"`
	}

	if json.Unmarshal(op.Args, &a) != nil {
		return nil
	}

	for i, f := range a.Files {
		if !isFontHash(f.Hash) || s.hasFont(f.Hash) {
			continue
		}

		e := newError(command.CodeNotFound, "the font file %s is not in the workspace.", f.Hash).
			withHint("import the font file first, then add the family").withPath("args.files").
			withDetail("hash", f.Hash).withDetail("file_index", i)
		e.OpIndex = &index

		return e
	}

	return nil
}

func isFontHash(h string) bool {
	return strings.HasPrefix(h, "sha256:") && isHex64(strings.TrimPrefix(h, "sha256:"))
}

// userFontKey is what the bytes of a bitmap depend on besides its spec: the files of the family
// the spec names. plan.BitmapSpec.Key covers the family name and weight, never the file, so a
// cache keyed by it alone would give a project the bitmap another project drew from a different
// file of the same name. It is empty for a bundled family.
func userFontKey(doc *timeline.Document, spec *plan.BitmapSpec) string {
	if spec.Kind == "shape" {
		return ""
	}

	fam, ok := doc.Fonts[spec.Style.FontFamily]
	if !ok {
		return ""
	}

	hashes := make([]string, 0, len(fam.Files))
	for _, f := range fam.Files {
		hashes = append(hashes, strconv.Itoa(f.Weight)+":"+f.Hash)
	}

	slices.Sort(hashes)

	sum := sha256.Sum256([]byte(strings.Join(hashes, ",")))

	return hex.EncodeToString(sum[:8])
}

// fontFingerprint names the user fonts of a document by their files and weights, so one bitmap
// cache serves every document with the same fonts. It is empty for a document with none.
func fontFingerprint(doc *timeline.Document) string {
	if len(doc.Fonts) == 0 {
		return ""
	}

	var b strings.Builder

	for _, name := range slices.Sorted(maps.Keys(doc.Fonts)) {
		b.WriteString(name)

		for _, f := range doc.Fonts[name].Files {
			b.WriteString("|" + strconv.Itoa(f.Weight) + ":" + f.Hash)
		}

		b.WriteString(";")
	}

	sum := sha256.Sum256([]byte(b.String()))

	return hex.EncodeToString(sum[:])
}

// framesFor is the frame maker of the workspace with the font set of doc: the user fonts of a
// document are read by hash from the workspace, so the maker is made for each document.
func (s *svc) framesFor(doc *timeline.Document) *frame.Frames {
	f := *s.media.Frames
	if f.Fonts == nil {
		f.Fonts = s.fontSet(doc)
	}

	return &f
}

// fontSize is the size of a stored font, or the bundle failure of a missing file.
func (s *svc) fontSize(hash string) (int64, *Error) {
	if s.fonts == nil {
		return 0, bundleFailed("missing_media").withDetail("hash", hash)
	}

	st, err := os.Stat(filepath.Join(s.fonts.Dir(), strings.TrimPrefix(hash, "sha256:")))
	if err != nil || !st.Mode().IsRegular() || !s.hasFont(hash) {
		return 0, bundleFailed("missing_media").withDetail("hash", hash)
	}

	return st.Size(), nil
}

// openBundleSource opens the stored file of a bundle entry: a font from the font store (read whole
// and checked against its name, at most the font size cap), anything else from the content store.
func (s *svc) openBundleSource(f bundleFile) (io.ReadCloser, error) {
	if !f.isFont() {
		return s.media.Content.Open(strings.TrimPrefix(f.hash, "sha256:"))
	}

	if s.fonts == nil {
		return nil, errors.New("no font store")
	}

	data, err := s.fonts.Read(strings.TrimPrefix(f.hash, "sha256:"))
	if err != nil {
		return nil, err
	}

	return io.NopCloser(bytes.NewReader(data)), nil
}

// copyBundleFont validates a font of a bundle and stores it by content; a font the store holds is
// not read again. The entry was already hashed against the manifest.
func (s *svc) copyBundleFont(p *bundleParts, f bundleFile) *Error {
	if s.hasFont(f.hash) {
		return nil
	}

	rc, err := p.entries[f.entry].Open()
	if err != nil {
		return bundleRefused("a file cannot be read").withDetail("hash", f.hash)
	}

	defer func() { _ = rc.Close() }()

	data, err := io.ReadAll(io.LimitReader(rc, raster.MaxFontBytes+1))
	if err != nil {
		return bundleRefused("a file cannot be read").withDetail("hash", f.hash)
	}

	hash, e := s.putFont(data)
	if e != nil {
		return e
	}

	if hash != f.hash {
		return bundleRefused("the file changed while it was read").withDetail("hash", f.hash)
	}

	return nil
}

// bundleFileLimit is the most bytes of an entry that are read: the import cap, and for a font the
// font cap, so an oversized font is refused before it is hashed to the end.
func (s *svc) bundleFileLimit(f bundleFile) int64 {
	if f.isFont() {
		return min(s.limits.MaxImportBytes, raster.MaxFontBytes)
	}

	return s.limits.MaxImportBytes
}
