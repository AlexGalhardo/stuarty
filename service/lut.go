package service

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

// The limits of a .cube file. The size cap is the 65-point table that the format's tools write at most
// (274,625 lines of three numbers) with room for long numbers; a longer line is not a table. The file is
// untrusted: nothing here allocates from a number it read, so the work is bounded by the bytes.
const (
	cubeMaxBytes    = 32 << 20
	cubeMaxLine     = 4096
	cubeMinSize     = 2
	cubeMaxSize     = 65
	cubeSniffWindow = 4096
)

var cubeKeywords = []string{
	"TITLE", "DOMAIN_MIN", "DOMAIN_MAX", "LUT_1D_SIZE", "LUT_3D_SIZE", "LUT_1D_INPUT_RANGE", "LUT_3D_INPUT_RANGE",
}

// sniffCube says whether the start of a file is the header of a .cube table: text, whose first line that is
// neither empty nor a comment starts with a keyword of the format. The kind of a file is found by its
// content, never by its name (SDD-011 section 3.1).
func sniffCube(head []byte) bool {
	if bytes.IndexByte(head, 0) >= 0 {
		return false
	}

	head = bytes.TrimPrefix(head, []byte("\xef\xbb\xbf"))

	// Only whole lines are read: the window may end in the middle of one.
	if i := bytes.LastIndexByte(head, '\n'); i >= 0 {
		head = head[:i]
	} else {
		return false
	}

	for _, line := range strings.Split(string(head), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		word, _, _ := strings.Cut(line, " ")
		word, _, _ = strings.Cut(word, "\t")

		for _, k := range cubeKeywords {
			if word == k {
				return true
			}
		}

		return false
	}

	return false
}

// checkCube reads a whole .cube file and refuses what is not a valid 3D table: a 1D table, a size outside
// 2 to 65, a domain that is empty, a line that is not three finite numbers, a count of lines that is not
// size cubed, a header after the data, or a file past cubeMaxBytes.
func checkCube(r io.Reader) (size int, err error) {
	lr := &io.LimitedReader{R: r, N: cubeMaxBytes + 1}
	sc := bufio.NewScanner(lr)
	sc.Buffer(make([]byte, 0, 1024), cubeMaxLine)

	var (
		rows          int
		sawData       bool
		domainMin     = [3]float64{0, 0, 0}
		domainMax     = [3]float64{1, 1, 1}
		bad           = func(format string, a ...any) (int, error) { return 0, fmt.Errorf(format, a...) }
		first         = true
		line          string
		fields        []string
		keyword, rest string
	)

	for sc.Scan() {
		line = sc.Text()
		if first {
			line, first = strings.TrimPrefix(line, "\xef\xbb\xbf"), false
		}

		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fields = strings.Fields(line)
		keyword, rest = fields[0], strings.TrimSpace(strings.TrimPrefix(line, fields[0]))

		if cubeIsNumber(keyword) {
			sawData = true

			if size == 0 {
				return bad("the data starts before LUT_3D_SIZE")
			}

			if len(fields) != 3 {
				return bad("a data line must be three numbers")
			}

			for _, f := range fields {
				if _, ok := cubeFinite(f); !ok {
					return bad("a data line holds a value that is not a finite number")
				}
			}

			if rows++; rows > size*size*size {
				return bad("the table has more lines than LUT_3D_SIZE cubed")
			}

			continue
		}

		if sawData {
			return bad("a header line after the data")
		}

		switch keyword {
		case "TITLE":
		case "LUT_1D_SIZE", "LUT_1D_INPUT_RANGE":
			return bad("a 1D table is not supported; use a 3D table (LUT_3D_SIZE)")
		case "LUT_3D_SIZE":
			n, perr := strconv.Atoi(rest)
			if perr != nil || size != 0 || n < cubeMinSize || n > cubeMaxSize {
				return bad("LUT_3D_SIZE must appear once, as a whole number from %d to %d", cubeMinSize, cubeMaxSize)
			}

			size = n
		case "DOMAIN_MIN", "DOMAIN_MAX":
			v, ok := cubeTriple(fields[1:])
			if !ok {
				return bad("%s must be three finite numbers", keyword)
			}

			if keyword == "DOMAIN_MIN" {
				domainMin = v
			} else {
				domainMax = v
			}
		case "LUT_3D_INPUT_RANGE":
			if _, ok := cubePair(fields[1:]); !ok {
				return bad("LUT_3D_INPUT_RANGE must be two finite numbers")
			}
		default:
			return bad("%q is not a line of a .cube file", cubeTruncate(keyword, 24))
		}
	}

	if err := sc.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return bad("a line is longer than %d bytes", cubeMaxLine)
		}

		return 0, err
	}

	switch {
	case lr.N <= 0:
		return bad("the file is larger than %d MiB", cubeMaxBytes>>20)
	case size == 0:
		return bad("LUT_3D_SIZE is missing")
	case rows != size*size*size:
		return bad("LUT_3D_SIZE %d needs %d lines of data, the file has %d", size, size*size*size, rows)
	}

	for i := range domainMin {
		if domainMin[i] >= domainMax[i] {
			return bad("DOMAIN_MIN must be below DOMAIN_MAX")
		}
	}

	return size, nil
}

func cubeIsNumber(s string) bool {
	if s == "" {
		return false
	}

	c := s[0]

	return c == '-' || c == '+' || c == '.' || c >= '0' && c <= '9'
}

func cubeFinite(s string) (float64, bool) {
	v, err := strconv.ParseFloat(s, 64)

	return v, err == nil && !math.IsNaN(v) && !math.IsInf(v, 0)
}

func cubeTriple(f []string) ([3]float64, bool) {
	var out [3]float64

	if len(f) != 3 {
		return out, false
	}

	for i, s := range f {
		v, ok := cubeFinite(s)
		if !ok {
			return out, false
		}

		out[i] = v
	}

	return out, true
}

func cubePair(f []string) ([2]float64, bool) {
	var out [2]float64

	if len(f) != 2 {
		return out, false
	}

	for i, s := range f {
		v, ok := cubeFinite(s)
		if !ok {
			return out, false
		}

		out[i] = v
	}

	return out, true
}

func cubeTruncate(s string, n int) string {
	if len(s) <= n {
		return s
	}

	return s[:n]
}
