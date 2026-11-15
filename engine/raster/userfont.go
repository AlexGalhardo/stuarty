package raster

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/go-text/typesetting/font"
)

// Limits of a user font file (SDD-007 section 7.7).
const (
	// MaxFontBytes is the largest font file accepted (a CJK family can be large).
	MaxFontBytes = 16 << 20
	// MaxFontTables is the largest numTables of a font's table directory.
	MaxFontTables = 64
	// maxCollectionFonts bounds the fonts of a .ttc collection.
	maxCollectionFonts = 64
	// FontParseDeadline is the total time the validation may spend parsing.
	FontParseDeadline = 5 * time.Second
)

// ErrFontNotStored is what a FontSource returns, wrapped, for a hash it does
// not have. Lookup turns it into E_FONT_UNKNOWN.
var ErrFontNotStored = errors.New("font file is not stored")

// CodeFontInvalid is the error code of a font file that fails validation.
const CodeFontInvalid = "E_VALIDATION"

// Reasons of an InvalidFontError, for details.reason.
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
	FontParseTimeout    = "font_parse_timeout"
)

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

func invalid(reason, format string, args ...any) *InvalidFontError {
	return &InvalidFontError{Reason: reason, Message: fmt.Sprintf(format, args...)}
}

// parseFont and parseDeadline are variables so the tests can simulate a font
// that is slow to parse without a 5 second wait.
var (
	parseFont     = parseFace
	parseDeadline = FontParseDeadline
)

// ValidateFont checks the bytes of a user font before it is stored or used
// (SDD-007 section 7.7, T-21): size, magic, table directory, then a parse with
// recover and a deadline. It returns *InvalidFontError.
func ValidateFont(data []byte) error {
	_, err := validateFont(data)

	return err
}

func validateFont(data []byte) (*font.Font, error) {
	if err := validateStructure(data); err != nil {
		return nil, err
	}

	return guardedParse(data)
}

var requiredTables = [...]string{"cmap", "head", "hhea", "hmtx", "maxp"}

type tableRecord struct {
	tag         string
	off, length uint64
}

// validateStructure reads only the header and the table directories. It never
// allocates in proportion to a declared count: numTables is checked before the
// directory is read.
func validateStructure(data []byte) error {
	if len(data) > MaxFontBytes {
		return invalid(FontTooLarge, "%d bytes, at most %d", len(data), MaxFontBytes)
	}

	if len(data) < 12 {
		return invalid(FontTruncated, "%d bytes is not a font", len(data))
	}

	switch string(data[:4]) {
	case "\x00\x01\x00\x00", "true", "OTTO":
		return validateDirectory(data, 0)
	case "ttcf":
		return validateCollection(data)
	default:
		return invalid(FontBadMagic, "unknown font signature % x", data[:4])
	}
}

func validateCollection(data []byte) error {
	n := uint64(binary.BigEndian.Uint32(data[8:12]))
	if n == 0 || n > maxCollectionFonts {
		return invalid(FontTooManyTables, "a collection of %d fonts, at most %d", n, maxCollectionFonts)
	}

	if uint64(len(data)) < 12+4*n {
		return invalid(FontTruncated, "the collection header is cut")
	}

	for i := range n {
		off := uint64(binary.BigEndian.Uint32(data[12+4*i:]))
		if off+12 > uint64(len(data)) {
			return invalid(FontTableOutOfRange, "font %d starts at %d, past the end of the file", i, off)
		}

		switch string(data[off : off+4]) {
		case "\x00\x01\x00\x00", "true", "OTTO":
		default:
			return invalid(FontBadMagic, "font %d of the collection has an unknown signature", i)
		}

		if err := validateDirectory(data, off); err != nil {
			return err
		}
	}

	return nil
}

func validateDirectory(data []byte, base uint64) error {
	size := uint64(len(data))
	num := uint64(binary.BigEndian.Uint16(data[base+4:]))

	if num > MaxFontTables {
		return invalid(FontTooManyTables, "numTables is %d, at most %d", num, MaxFontTables)
	}

	dirEnd := base + 12 + 16*num
	if dirEnd > size {
		return invalid(FontTruncated, "the table directory ends at %d, the file has %d bytes", dirEnd, size)
	}

	recs := make([]tableRecord, 0, num)

	for i := range num {
		rec := data[base+12+16*i:]
		r := tableRecord{
			tag:    string(rec[:4]),
			off:    uint64(binary.BigEndian.Uint32(rec[8:])),
			length: uint64(binary.BigEndian.Uint32(rec[12:])),
		}

		if r.off < dirEnd || r.off+r.length > size {
			return invalid(
				FontTableOutOfRange,
				"table %q at %d, length %d, is outside the file",
				r.tag,
				r.off,
				r.length,
			)
		}

		if slices.ContainsFunc(recs, func(o tableRecord) bool { return o.tag == r.tag }) {
			return invalid(FontDuplicateTable, "table %q appears twice", r.tag)
		}

		recs = append(recs, r)
	}

	for _, name := range requiredTables {
		if !hasTable(recs, name) {
			return invalid(FontMissingTable, "required table %q is missing", name)
		}
	}

	if !hasTable(recs, "glyf") && !hasTable(recs, "CFF ") {
		return invalid(FontMissingTable, "neither glyf nor CFF is present")
	}

	slices.SortFunc(recs, func(a, b tableRecord) int {
		switch {
		case a.off < b.off:
			return -1
		case a.off > b.off:
			return 1
		default:
			return 0
		}
	})

	for i := 1; i < len(recs); i++ {
		if recs[i].off < recs[i-1].off+recs[i-1].length {
			return invalid(FontTablesOverlap, "tables %q and %q overlap", recs[i-1].tag, recs[i].tag)
		}
	}

	return nil
}

func hasTable(recs []tableRecord, tag string) bool {
	return slices.ContainsFunc(recs, func(r tableRecord) bool { return r.tag == tag && r.length > 0 })
}

// parseFace parses the font and touches a few glyphs, so lazy tables that are
// broken fail here and not while a frame renders.
func parseFace(data []byte) (*font.Font, error) {
	face, err := font.ParseTTF(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	f := font.NewFace(face.Font)
	for _, r := range "Aa0 " {
		if gid, ok := f.NominalGlyph(r); ok {
			f.GlyphData(gid)
		}
	}

	return face.Font, nil
}

type parseResult struct {
	font *font.Font
	err  error
}

// guardedParse runs parseFont with recover and the deadline. On a timeout the
// parsing goroutine is abandoned; it only works on its own copy of the input,
// so nothing else waits for it.
func guardedParse(data []byte) (*font.Font, error) {
	done := make(chan parseResult, 1)
	own := bytes.Clone(data)
	parse, deadline := parseFont, parseDeadline

	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- parseResult{err: invalid(FontParseFailed, "the parser panicked: %v", r)}
			}
		}()

		f, err := parse(own)
		done <- parseResult{font: f, err: err}
	}()

	timer := time.NewTimer(deadline)
	defer timer.Stop()

	select {
	case res := <-done:
		if res.err != nil {
			if inv, ok := errors.AsType[*InvalidFontError](res.err); ok {
				return nil, inv
			}

			return nil, invalid(FontParseFailed, "%v", res.err)
		}

		return res.font, nil
	case <-timer.C:
		return nil, invalid(FontParseTimeout, "parsing took longer than %s", deadline)
	}
}

// FontSource reads the bytes of a stored font by its lower-case hex SHA-256.
// A missing file is an error that wraps ErrFontNotStored. (The package does
// not import io/fs or os: TestNoSystemFontAccess.)
type FontSource func(hash string) ([]byte, error)

// userFace is one weight of a user family.
type userFace struct {
	weight int
	hash   string // 64 lower-case hex characters
}

// lookupUser resolves a family of the document's fonts map to a face. The
// family and weight come from the document, never from the font's name table.
func (s *FontSet) lookupUser(familyName string, weight int, faces []userFace) (*font.Face, error) {
	idx := slices.IndexFunc(faces, func(f userFace) bool { return f.weight == weight })
	if idx < 0 {
		weights := make([]int, len(faces))
		for i, f := range faces {
			weights[i] = f.weight
		}

		slices.Sort(weights)

		return nil, &FontError{Code: CodeFontUnknown, Family: familyName, Weight: weight, Weights: weights}
	}

	parsed, err := s.loadUser(familyName, weight, faces[idx].hash)
	if err != nil {
		return nil, err
	}

	return font.NewFace(parsed), nil
}

func (s *FontSet) loadUser(familyName string, weight int, hash string) (*font.Font, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if parsed, ok := s.parsed[hash]; ok {
		return parsed, nil
	}

	if s.src == nil {
		return nil, &FontError{Code: CodeFontUnknown, Family: familyName, Weight: weight, Hash: hash}
	}

	data, err := s.src(hash)
	if errors.Is(err, ErrFontNotStored) {
		return nil, &FontError{Code: CodeFontUnknown, Family: familyName, Weight: weight, Hash: hash}
	}

	if err != nil {
		return nil, fmt.Errorf("raster: read font %s: %w", hash, err)
	}

	if sum := sha256.Sum256(data); hex.EncodeToString(sum[:]) != hash {
		return nil, fmt.Errorf("raster: font %s does not match its hash", hash)
	}

	parsed, err := validateFont(data)
	if err != nil {
		return nil, err
	}

	s.parsed[hash] = parsed

	return parsed, nil
}

// trimHashPrefix turns "sha256:<hex>" of a document into the bare digest.
func trimHashPrefix(h string) string { return strings.TrimPrefix(h, "sha256:") }
