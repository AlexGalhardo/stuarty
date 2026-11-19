package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/frame"
	"github.com/AlexGalhardo/stuarty/engine/rasterize"
	enginestore "github.com/AlexGalhardo/stuarty/engine/store"
)

// FontSetFor returns the font set a render of doc uses: the bundled families plus the user fonts
// of the document, read by hash from the font store of the workspace (SDD-007 section 7.7). A
// document without user fonts gets the shared bundled set. The set is never shared between
// documents, so two projects that name a family alike never see each other's files.
func FontSetFor(workspace string, doc *timeline.Document) *rasterize.FontSet {
	if workspace == "" || doc == nil || len(doc.Fonts) == 0 {
		return rasterize.Bundled()
	}

	return rasterize.NewFontSet(doc.Fonts, enginestore.NewFonts(workspace).Source())
}

// fontSet is FontSetFor for the workspace of the service.
func (s *svc) fontSet(doc *timeline.Document) *rasterize.FontSet {
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
	if ife, ok := errors.AsType[*rasterize.InvalidFontError](err); ok {
		return newError(ife.Code(), "the font file is refused: %s.", ife.Message).
			withHint("use a TrueType or OpenType font of at most %d MiB", rasterize.MaxFontBytes>>20).
			withDetail("reason", ife.Reason)
	}

	return fromEngine(err)
}

// maxFontFiles is the most files a font.add may name (SDD-002 C.53); a longer list is left to the
// command to refuse, so the ownership check never works on an unvalidated list.
const maxFontFiles = 9

// checkFontAdd refuses a font.add that names a file the caller may not use (E_NOT_FOUND), as the
// command assumes (SDD-002 C.53, SDD-007 section 5.4 item 3). The core checks only the form of the
// hash. A malformed operation is left to the command. A file the store lacks and a file that
// belongs to another user are refused with the same error, which names no hash, so the refusal
// does not reveal that the file exists (SDD-007 T-21).
func (s *svc) checkFontAdd(ctx context.Context, tx Tx, scope *fontScope, op *command.Operation, index int) *Error {
	var a struct {
		Files []struct {
			Hash string `json:"hash"`
		} `json:"files"`
	}

	if json.Unmarshal(op.Args, &a) != nil || len(a.Files) == 0 || len(a.Files) > maxFontFiles {
		return nil //nolint:nilerr // the command reports malformed arguments itself
	}

	for i, f := range a.Files {
		if !isFontHash(f.Hash) || scope.allowed(ctx, tx, s, f.Hash) {
			continue
		}

		e := newError(command.CodeNotFound, "a font file of the operation is not in the workspace.").
			withHint("import the font file first, then add the family").withPath("args.files").
			withDetail("file_index", i)
		e.OpIndex = &index

		return e
	}

	return nil
}

// maxFontOwnerScan bounds the projects read to learn which fonts a user already uses.
const maxFontOwnerScan = 2000

// fontScope answers, for one request, which fonts the caller may name. In local mode the store is
// the caller's own, so a stored file is enough. In network mode the store is shared: the caller
// must be a recorded owner of the file (the user who imported it or brought it) or already use it
// in a project (SDD-007 section 7.7 item 6). The caller's fonts are read once for the whole batch,
// bounded, the recorded owners by one indexed lookup of the caller's own pair, and the store is
// asked only about a hash the caller may use, so a refusal costs the same whether the file exists
// or not and does not touch the store. except is a project left out of the scan, for an export
// that must not count the document it is about to name.
type fontScope struct {
	built  bool
	owned  map[string]bool
	except string
}

func (f *fontScope) allowed(ctx context.Context, repos Repos, s *svc, hash string) bool {
	c, ok := CallerFrom(ctx)
	if !ok {
		return false
	}

	if c.OwnerUserID == "" {
		return s.hasFont(hash)
	}

	recorded, err := repos.Fonts().Owns(ctx, c.OwnerUserID, hash)
	if err != nil {
		recorded = false
	}

	if !recorded {
		if !f.built {
			f.owned, f.built = userFontHashes(ctx, repos, c.OwnerUserID, f.except), true
		}

		recorded = f.owned[hash]
	}

	return recorded && s.hasFont(hash)
}

// recordFontOwner makes the caller an owner of the fonts of hashes, in network mode: the caller
// imported the file or brought its bytes, which proves they hold it. Local mode records nothing.
func recordFontOwner(ctx context.Context, tx Tx, c Caller, now int64, hashes ...string) error {
	if c.OwnerUserID == "" {
		return nil
	}

	for _, h := range hashes {
		if err := tx.Fonts().Record(ctx, c.OwnerUserID, h, now); err != nil {
			return err
		}
	}

	return nil
}

// userFontHashes is the set of font hashes that the projects of the user use, in any state, except
// the project named skip.
func userFontHashes(ctx context.Context, repos Repos, user, skip string) map[string]bool {
	out := map[string]bool{}

	var after string

	for scanned := 0; scanned < maxFontOwnerScan; {
		items, next, err := repos.Projects().List(ctx, ProjectQuery{
			IncludeTrashed: true, OwnerUserID: user, After: after, Limit: 200,
		})
		if err != nil {
			return out
		}

		for i := range items {
			scanned++

			if scanned > maxFontOwnerScan {
				return out
			}

			if items[i].ID == skip {
				continue
			}

			row, err := repos.Projects().Get(ctx, items[i].ID)
			if err != nil {
				continue
			}

			doc, err := decodeRow(&row)
			if err != nil {
				continue
			}

			for _, fam := range doc.Fonts {
				for _, f := range fam.Files {
					out[f.Hash] = true
				}
			}
		}

		if next == "" {
			return out
		}

		after = next
	}

	return out
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
func (s *svc) framesFor(doc *timeline.Document, nested map[string]*timeline.Document) *frame.Frames {
	f := *s.media.Frames
	f.Nested = nested

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

	data, err := io.ReadAll(io.LimitReader(rc, rasterize.MaxFontBytes+1))
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
		return min(s.limits.MaxImportBytes, rasterize.MaxFontBytes)
	}

	return s.limits.MaxImportBytes
}

// unusableFont is the first family of doc that has a file the caller may not use, or "" when the
// caller may use all of them. except is a project the ownership scan leaves out.
func (s *svc) unusableFont(ctx context.Context, repos Repos, doc *timeline.Document, except string) (string, bool) {
	if len(doc.Fonts) == 0 {
		return "", false
	}

	scope := fontScope{except: except}

	for _, family := range sortedKeys(doc.Fonts) {
		for _, f := range doc.Fonts[family].Files {
			if !scope.allowed(ctx, repos, s, f.Hash) {
				return family, true
			}
		}
	}

	return "", false
}

// fontNotUsable is the refusal of a font file the caller may not name: the one of a file that was
// never stored, with no hash in it (SDD-007 T-21).
func fontNotUsable(path string) *Error {
	return newError(command.CodeNotFound, "a font file is not in the workspace.").
		withHint("import the font file first, then use the family").withPath(path)
}

// FontImportRequest reads a font file from the machine of the server (SDD-007 section 7.7).
type FontImportRequest struct {
	Path string `json:"path"` // absolute
}

// FontImportResult is the stored font. Hash is in the form of a document, so a font.add can name it.
type FontImportResult struct {
	Hash         string `json:"hash"`
	Bytes        int64  `json:"bytes"`
	Deduplicated bool   `json:"deduplicated"`
	Text         string `json:"text"`
}

// FontImport validates a font file and stores it by content hash in the font store. The same bytes
// twice store one file; in network mode the caller is recorded as an owner either way.
func (s *svc) FontImport(ctx context.Context, req FontImportRequest) (*FontImportResult, error) {
	c, err := authorizeScope(ctx, ScopeMedia, "")
	if err != nil {
		return nil, err
	}

	if c.ProjectID != "" {
		return nil, newError(CodeForbidden, "the key is limited to one project and cannot import fonts.").
			withHint("use a key without a project limit")
	}

	if err := s.heavyLimit(ctx, c); err != nil {
		return nil, err
	}

	if s.media.Guard == nil || s.fonts == nil {
		return nil, internalError()
	}

	file, gerr := s.media.Guard.OpenImport(req.Path)
	if gerr != nil {
		return nil, fromEngine(gerr)
	}

	defer func() { _ = file.Close() }()

	st, serr := file.Stat()
	if serr != nil || !st.Mode().IsRegular() {
		return nil, newError("E_FILE_NOT_FOUND", "the file cannot be read.").withPath("path")
	}

	if st.Size() > rasterize.MaxFontBytes {
		return nil, newError(
			rasterize.CodeFontInvalid,
			"the font file is refused: it is over %d MiB.",
			rasterize.MaxFontBytes>>20,
		).
			withHint("use a TrueType or OpenType font of at most %d MiB", rasterize.MaxFontBytes>>20).
			withDetail("reason", "too_large")
	}

	data, rerr := io.ReadAll(io.LimitReader(file, rasterize.MaxFontBytes+1))
	if rerr != nil {
		return nil, newError("E_FILE_NOT_FOUND", "the file cannot be read.").withPath("path")
	}

	sum := sha256.Sum256(data)
	existed := s.hasFont("sha256:" + hex.EncodeToString(sum[:]))

	hash, perr := s.putFont(data)
	if perr != nil {
		return nil, forCaller(c, perr)
	}

	werr := s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		now := s.nowMs()

		if err := recordFontOwner(ctx, tx, c, now, hash); err != nil {
			return err
		}

		return appendAudit(ctx, tx, now, c.Actor, "font.import", "", hash)
	})
	if werr != nil {
		return nil, asError(werr)
	}

	res := &FontImportResult{Hash: hash, Bytes: int64(len(data)), Deduplicated: existed}
	res.Text = fmt.Sprintf("font %s, %d bytes", hash, res.Bytes)

	if existed {
		res.Text += ", already stored"
	}

	return res, nil
}
