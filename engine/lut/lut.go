// Package lut reads the 3D colour lookup tables of the Adobe/Resolve .cube format (SDD-004 section 3.6).
//
// A table comes from a file nobody vouches for (SDD-007), so the parser reads at most MaxBytes, lines of at
// most maxLine bytes and tables of at most MaxSize points on a side, accepts only the keywords it understands and
// refuses everything else. It never echoes the content of the file in an error. The values are kept as 16-bit
// integers, so the renderers (Go and Rust) interpolate in integer arithmetic and parse no text.
package lut

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

// The limits of a table.
const (
	MinSize = 2
	MaxSize = 65
	// MaxBytes is the largest .cube file read: a 65 point table written with 6 decimals is about 8 MiB.
	MaxBytes = 16 << 20
	maxLine  = 512
)

// ErrInvalid is wrapped by every error of Parse.
var ErrInvalid = errors.New("lut: invalid .cube file")

// Table is a 3D lookup table: Size points on each side, Size^3 entries of red, green and blue as 16-bit
// values (0 is 0.0 and 65535 is 1.0), the red index varying fastest, as in the file.
type Table struct {
	Size int
	Data []uint16
}

// Bytes is the table as the renderer takes it: the entries, little endian.
func (t *Table) Bytes() []byte {
	out := make([]byte, 0, 2*len(t.Data))
	for _, v := range t.Data {
		out = append(out, byte(v&0xFF), byte(v>>8))
	}

	return out
}

func invalid(line int, why string) error {
	if line == 0 {
		return fmt.Errorf("%w: %s", ErrInvalid, why)
	}

	return fmt.Errorf("%w: line %d: %s", ErrInvalid, line, why)
}

// parser is the state of one Parse: the table being filled, the entries read and the domains seen.
type parser struct {
	t      *Table
	n      int
	domain [2]bool
}

// Parse reads a .cube file. It understands TITLE, LUT_3D_SIZE and DOMAIN_MIN and DOMAIN_MAX (only the default
// 0 to 1), comments and the data lines; a 1D table, another keyword, a size outside MinSize to MaxSize, a value
// that is not a finite number, or a count of entries other than Size^3 is refused. Values outside 0 to 1 are
// held to it.
func Parse(r io.Reader) (*Table, error) {
	lr := &io.LimitedReader{R: r, N: MaxBytes + 1}
	sc := bufio.NewScanner(lr)
	// The buffer holds a line of maxLine bytes with its CR LF; a longer line is refused below. A larger initial
	// buffer would raise the limit, because the scanner never caps below its capacity (VV-293).
	sc.Buffer(make([]byte, 0, maxLine+2), maxLine+2)

	var (
		p  parser
		ln int
	)

	for sc.Scan() {
		ln++

		if len(sc.Bytes()) > maxLine {
			return nil, invalid(0, "a line is too long or the file cannot be read")
		}

		line := sc.Text()
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}

		if f := strings.Fields(line); len(f) > 0 {
			if err := p.line(f, ln); err != nil {
				return nil, err
			}
		}
	}

	switch {
	case sc.Err() != nil:
		return nil, invalid(0, "a line is too long or the file cannot be read")
	case lr.N <= 0:
		return nil, invalid(0, "the file is too large")
	case p.t == nil:
		return nil, invalid(0, "no LUT_3D_SIZE")
	case p.n != p.t.Size*p.t.Size*p.t.Size:
		return nil, invalid(0, "fewer entries than LUT_3D_SIZE says")
	}

	return p.t, nil
}

// line reads one line of fields: a keyword or a data line.
func (p *parser) line(f []string, ln int) error {
	switch strings.ToUpper(f[0]) {
	case "TITLE":
		return nil
	case "LUT_3D_SIZE":
		return p.size(f, ln)
	case "DOMAIN_MIN", "DOMAIN_MAX":
		return p.domainLine(f, ln)
	case "LUT_1D_SIZE", "LUT_1D_INPUT_RANGE", "LUT_3D_INPUT_RANGE":
		return invalid(ln, "1D tables and input ranges are not supported")
	}

	return p.data(f, ln)
}

func (p *parser) size(f []string, ln int) error {
	if p.t != nil || len(f) != 2 {
		return invalid(ln, "LUT_3D_SIZE is repeated or has no single value")
	}

	size, err := strconv.Atoi(f[1])
	if err != nil || size < MinSize || size > MaxSize {
		return invalid(ln, fmt.Sprintf("the size is not a whole number from %d to %d", MinSize, MaxSize))
	}

	p.t = &Table{Size: size, Data: make([]uint16, 0, 3*size*size*size)}

	return nil
}

func (p *parser) domainLine(f []string, ln int) error {
	isMax := strings.EqualFold(f[0], "DOMAIN_MAX")
	if len(f) != 4 || p.domain[b2i(isMax)] {
		return invalid(ln, "a domain needs three values, once")
	}

	want := 0.0
	if isMax {
		want = 1
	}

	for _, s := range f[1:] {
		if v, err := parseValue(s); err != nil || v != want {
			return invalid(ln, "only the domain 0 to 1 is supported")
		}
	}

	p.domain[b2i(isMax)] = true

	return nil
}

func (p *parser) data(f []string, ln int) error {
	switch {
	case p.t == nil:
		return invalid(ln, "data before LUT_3D_SIZE")
	case len(f) != 3:
		return invalid(ln, "a data line has three numbers")
	case p.n >= p.t.Size*p.t.Size*p.t.Size:
		return invalid(ln, "more entries than LUT_3D_SIZE says")
	}

	p.n++

	for _, s := range f {
		v, err := parseValue(s)
		if err != nil {
			return invalid(ln, "a value is not a finite number")
		}

		p.t.Data = append(p.t.Data, uint16(math.Round(min(max(v, 0), 1)*65535)))
	}

	return nil
}

func b2i(b bool) int {
	if b {
		return 1
	}

	return 0
}

// parseValue is a finite decimal number; hexadecimal floats, NaN and infinities are not numbers of a .cube file.
func parseValue(s string) (float64, error) {
	if strings.ContainsAny(s, "xXpP_nNiI") {
		return 0, ErrInvalid
	}

	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, ErrInvalid
	}

	return v, nil
}
