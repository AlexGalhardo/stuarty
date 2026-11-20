package lut

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
)

// cube writes a table of the given size whose entry is f(r, g, b) with the indices in 0..size-1.
func cube(size int, header string, f func(r, g, b int) [3]string) string {
	var sb strings.Builder

	sb.WriteString(header)
	fmt.Fprintf(&sb, "LUT_3D_SIZE %d\n", size)

	for b := range size {
		for g := range size {
			for r := range size {
				v := f(r, g, b)
				fmt.Fprintf(&sb, "%s %s %s\n", v[0], v[1], v[2])
			}
		}
	}

	return sb.String()
}

func TestParseReadsATable(t *testing.T) {
	src := cube(2, "# made by hand\nTITLE \"two\"\nDOMAIN_MIN 0 0 0\nDOMAIN_MAX 1.0 1.0 1.0\n\n",
		func(r, g, _ int) [3]string {
			return [3]string{strconv.Itoa(r), fmt.Sprintf("%.1f", float64(g)/2), "1e-3 # blue"}
		})

	tb, err := Parse(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}

	if tb.Size != 2 || len(tb.Data) != 24 {
		t.Fatalf("size %d with %d values", tb.Size, len(tb.Data))
	}

	// Entry 1 is r = 1, g = 0, b = 0: red varies fastest.
	if got := tb.Data[3:6]; got[0] != 65535 || got[1] != 0 || got[2] != 66 {
		t.Errorf("entry 1 = %v, want 65535 0 66", got)
	}

	// Entry 2 is g = 1: its green is 0.5.
	if got := tb.Data[6+1]; got != 32768 {
		t.Errorf("green of entry 2 = %d, want 32768", got)
	}

	if b := tb.Bytes(); len(b) != 48 || b[6] != 0xFF || b[7] != 0xFF {
		t.Errorf("bytes %v", b[:8])
	}
}

func TestParseReadsATableOfAnotherSize(t *testing.T) {
	tb, err := Parse(strings.NewReader(cube(3, "", func(r, g, b int) [3]string {
		return [3]string{strconv.Itoa(r), strconv.Itoa(g), strconv.Itoa(b)}
	})))
	if err != nil || tb.Size != 3 || len(tb.Data) != 81 {
		t.Fatalf("%v, %+v", err, tb)
	}

	// Entry 13 is (1, 1, 1) of 0..2: values 1 are held to 1.0.
	if got := tb.Data[39:42]; got[0] != 65535 || got[1] != 65535 || got[2] != 65535 {
		t.Errorf("entry 13 = %v", got)
	}
}

func TestParseHoldsValuesToTheRange(t *testing.T) {
	tb, err := Parse(
		strings.NewReader(cube(2, "", func(int, int, int) [3]string { return [3]string{"-0.5", "1.5", "0.5"} })),
	)
	if err != nil {
		t.Fatal(err)
	}

	if tb.Data[0] != 0 || tb.Data[1] != 65535 || tb.Data[2] != 32768 {
		t.Errorf("held values %v", tb.Data[:3])
	}
}

func TestParseRefusesWhatItDoesNotUnderstand(t *testing.T) {
	ok := func(int, int, int) [3]string { return [3]string{"0", "0", "0"} }
	tooBig := fmt.Sprintf("LUT_3D_SIZE %d\n", MaxSize+1)

	for name, src := range map[string]string{
		"empty":            "",
		"no size":          "0 0 0\n",
		"1d":               "LUT_1D_SIZE 4\n0 0 0\n",
		"input range":      "LUT_3D_INPUT_RANGE 0 1\nLUT_3D_SIZE 2\n",
		"size one":         "LUT_3D_SIZE 1\n0 0 0\n",
		"size not a count": "LUT_3D_SIZE two\n",
		"size too large":   tooBig,
		"size twice":       "LUT_3D_SIZE 2\nLUT_3D_SIZE 2\n",
		"domain":           cube(2, "DOMAIN_MAX 2 2 2\n", ok),
		"domain min":       cube(2, "DOMAIN_MIN -1 0 0\n", ok),
		"short":            "LUT_3D_SIZE 2\n0 0 0\n",
		"long":             cube(2, "", ok) + "0 0 0\n",
		"two numbers":      "LUT_3D_SIZE 2\n0 0\n",
		"four numbers":     "LUT_3D_SIZE 2\n0 0 0 0\n",
		"nan":              cube(2, "", func(int, int, int) [3]string { return [3]string{"NaN", "0", "0"} }),
		"inf":              cube(2, "", func(int, int, int) [3]string { return [3]string{"1e999", "0", "0"} }),
		"hex float":        cube(2, "", func(int, int, int) [3]string { return [3]string{"0x1p-1", "0", "0"} }),
		"word":             cube(2, "", func(int, int, int) [3]string { return [3]string{"red", "0", "0"} }),
		"long line":        "TITLE " + strings.Repeat("a", maxLine+1) + "\n",
		"binary":           "LUT_3D_SIZE 2\n\x00\x01\x02\n",
	} {
		if _, err := Parse(strings.NewReader(src)); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: error %v, want ErrInvalid", name, err)
		}
	}
}

func TestParseRefusesAFileThatIsTooLarge(t *testing.T) {
	r := endlessComments{n: MaxBytes + 100}
	if _, err := Parse(&r); !errors.Is(err, ErrInvalid) {
		t.Fatalf("error %v, want ErrInvalid", err)
	}
}

// endlessComments is an endless run of comment lines, so the size limit is the only way the parse can end.
type endlessComments struct{ n int }

func (r *endlessComments) Read(p []byte) (int, error) {
	if r.n <= 0 {
		return 0, errors.New("read past the limit")
	}

	n := copy(p, strings.Repeat("# comment\n", len(p)/10+1))
	n = min(n, len(p), r.n)
	r.n -= n

	return n, nil
}

func TestErrorsNeverEchoTheContent(t *testing.T) {
	_, err := Parse(strings.NewReader("LUT_3D_SIZE 2\nsecret-token 0 0\n"))
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("error %v echoes the file", err)
	}
}

func FuzzCubeParse(f *testing.F) {
	f.Add(cube(2, "TITLE \"x\"\n", func(int, int, int) [3]string { return [3]string{"0", "0.5", "1"} }))
	f.Add("LUT_3D_SIZE 3\n# c\n")
	f.Add("LUT_1D_SIZE 4\n")
	f.Add("DOMAIN_MIN 0 0 0\nDOMAIN_MAX 1 1 1\nLUT_3D_SIZE 2\n")
	f.Add("\x00\xff\n")

	f.Fuzz(func(t *testing.T, src string) {
		tb, err := Parse(strings.NewReader(src))
		if err != nil {
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("an error that is not ErrInvalid: %v", err)
			}

			return
		}

		if tb.Size < MinSize || tb.Size > MaxSize || len(tb.Data) != 3*tb.Size*tb.Size*tb.Size {
			t.Fatalf("a table of size %d with %d values", tb.Size, len(tb.Data))
		}
	})
}

// A line of maxLine bytes is read (with LF or CR LF) and one byte more is refused, whatever the initial buffer
// of the scanner (VV-293: it was 4096, so the documented 512 was never the limit).
func TestParseHoldsLinesToMaxLine(t *testing.T) {
	body := cube(2, "", func(int, int, int) [3]string { return [3]string{"0", "0", "0"} })

	for _, eol := range []string{"\n", "\r\n"} {
		at := "#" + strings.Repeat("a", maxLine-1) + eol
		if _, err := Parse(strings.NewReader(at + body)); err != nil {
			t.Errorf("a line of %d bytes with %q: %v", maxLine, eol, err)
		}

		over := "#" + strings.Repeat("a", maxLine) + eol
		if _, err := Parse(strings.NewReader(over + body)); !errors.Is(err, ErrInvalid) {
			t.Errorf("a line of %d bytes with %q: error %v, want ErrInvalid", maxLine+1, eol, err)
		}
	}

	if _, err := Parse(strings.NewReader(body + "#" + strings.Repeat("a", 600))); !errors.Is(err, ErrInvalid) {
		t.Errorf("a last line of 601 bytes without a newline: error %v, want ErrInvalid", err)
	}
}
