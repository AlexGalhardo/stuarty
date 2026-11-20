package service

import (
	"bytes"
	"errors"
	"io"
	"slices"
	"strings"

	"github.com/AlexGalhardo/stuarty/engine/lut"
)

// cubeSniffWindow is how much of the start of a file is read to tell a .cube table from other media.
const cubeSniffWindow = 4096

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

	for line := range strings.SplitSeq(string(head), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		word, _, _ := strings.Cut(line, " ")
		word, _, _ = strings.Cut(word, "\t")

		return slices.Contains(cubeKeywords, word)
	}

	return false
}

// checkCube reads a whole .cube file with engine/lut.Parse, the only parser of a table: the export reads the
// same bytes with the same limits (16 MiB, lines of 512 bytes, a domain of 0 to 1, 2 to 65 points on a side), so
// a file that passes here is one the export can read (VV-287). The error is the reason, with no content of the
// file.
func checkCube(r io.Reader) (size int, err error) {
	t, err := lut.Parse(r)
	if err != nil {
		return 0, errors.New(strings.TrimPrefix(err.Error(), lut.ErrInvalid.Error()+": "))
	}

	return t.Size, nil
}
