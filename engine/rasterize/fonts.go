package rasterize

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// CodeFontUnknown is the error code of a family or weight that is not in the
// font set (SDD-000 section 8).
const CodeFontUnknown = "E_FONT_UNKNOWN"

// CodeFontInvalid is the error code of a font file that fails validation.
const CodeFontInvalid = "E_VALIDATION"

// Limits of a user font file (SDD-007 section 7.7).
const (
	// MaxFontBytes is the largest font file accepted (a CJK family can be large).
	MaxFontBytes = 16 << 20
	// MaxFontTables is the largest numTables of a font's table directory.
	MaxFontTables = 64
	// FontParseDeadline is the total time the validation may spend parsing: the parse is bounded by
	// construction in the module, so it is the time the caller can wait for it.
	FontParseDeadline = 5 * time.Second
)

// ErrFontNotStored is what a FontSource returns, wrapped, for a hash it does
// not have. Rendering turns it into E_FONT_UNKNOWN.
var ErrFontNotStored = errors.New("font file is not stored")

// Reasons of an InvalidFontError, for details.reason (the module's names).
const (
	FontTooLarge        = "font_too_large"
	FontBadMagic        = "font_bad_magic"
	FontTruncated       = "font_truncated"
	FontTooManyTables   = "font_too_many_tables"
	FontTableOutOfRange = "font_table_out_of_range"
	FontTablesOverlap   = "font_tables_overlap"
	FontDuplicateTable  = "font_duplicate_table"
	FontMissingTable    = "font_missing_table"
	FontParseFailed     = "font_parse_failed"
)

// FontError is returned for a family or weight the set does not have.
type FontError struct {
	Code   string
	Family string
	Weight int
	// Weights lists the weights that exist for the family; nil when the family
	// is unknown.
	Weights []int
	// Hash is set when the family exists in the document's fonts map but the
	// file of that weight is not in the workspace.
	Hash string
}

func (e *FontError) Error() string {
	if e.Hash != "" {
		return fmt.Sprintf("%s: the file of family %q weight %d (sha256 %s) is not in the workspace",
			e.Code, e.Family, e.Weight, e.Hash)
	}

	if e.Weights == nil {
		return fmt.Sprintf("%s: no font family %q", e.Code, e.Family)
	}

	return fmt.Sprintf("%s: family %q has no weight %d (available: %v)", e.Code, e.Family, e.Weight, e.Weights)
}

// InvalidFontError is returned by ValidateFont for a file that must not be
// stored or used.
type InvalidFontError struct {
	Reason  string
	Message string
}

// Code is E_VALIDATION.
func (e *InvalidFontError) Code() string { return CodeFontInvalid }

func (e *InvalidFontError) Error() string {
	return fmt.Sprintf("%s: font file refused (%s): %s", CodeFontInvalid, e.Reason, e.Message)
}

// FontSource reads the bytes of a stored font by its lower-case hex SHA-256.
// A missing file is an error that wraps ErrFontNotStored.
type FontSource func(hash string) ([]byte, error)

// userFace is one weight of a user family.
type userFace struct {
	weight int
	hash   string // 64 lower-case hex characters
}

// FontSet names the fonts a bitmap may use: the bundled ones, which the module holds, and the user
// fonts of a document, whose bytes src reads when the module asks for them. It is immutable and safe
// for concurrent use.
type FontSet struct {
	user map[string][]userFace
	src  FontSource

	mu       sync.Mutex
	supplied map[string]struct{}
}

var bundled = &FontSet{}

// Bundled returns the font set shipped inside the renderer module.
func Bundled() *FontSet { return bundled }

// NewFontSet returns the bundled set plus the families of a document's fonts
// map (SDD-001 section 3.9), whose files src reads by hash. Bundled families
// win: the document validation forbids a key that names one. src is never used
// for anything but those hashes; system fonts are never read.
func NewFontSet(docFonts map[string]timeline.ProjectFont, src FontSource) *FontSet {
	user := make(map[string][]userFace, len(docFonts))

	for name, pf := range docFonts {
		for _, f := range pf.Files {
			user[name] = append(user[name], userFace{weight: f.Weight, hash: strings.TrimPrefix(f.Hash, "sha256:")})
		}
	}

	return &FontSet{user: user, src: src, supplied: map[string]struct{}{}}
}

// hashFor is the digest of the user font a text or caption spec names, or "" when it names a bundled
// family or one the set does not have (the module reports that).
func (s *FontSet) hashFor(spec plan.BitmapSpec) string {
	if s == nil || spec.Kind == KindShape {
		return ""
	}

	for _, f := range s.user[spec.Style.FontFamily] {
		if f.weight == spec.Style.FontWeight {
			return f.hash
		}
	}

	return ""
}

// wasSupplied reports whether this set has already read the file of the digest from its source and
// handed it to the module. The module keeps parsed fonts by digest for every document, so each set
// checks its own source once: a file the workspace no longer has is E_FONT_UNKNOWN for the set that
// names it, whatever another document made the module hold.
func (s *FontSet) wasSupplied(hash string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, ok := s.supplied[hash]

	return ok
}

func (s *FontSet) markSupplied(hash string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.supplied[hash] = struct{}{}
}

// faceOf finds the family and weight a digest belongs to, for an error message.
func (s *FontSet) faceOf(hash string) (string, int) {
	for name, faces := range s.user {
		for _, f := range faces {
			if f.hash == hash {
				return name, f.weight
			}
		}
	}

	return "", 0
}
