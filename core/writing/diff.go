package writing

import (
	"regexp"
	"slices"
	"strings"
)

// Ops of a hunk of a diff.
const (
	OpEqual   = "equal"
	OpRemoved = "removed"
	OpAdded   = "added"
)

// maxDiffCells is the size of the table of the comparison: paragraph counts multiplied (SDD-019 section 3).
const maxDiffCells = 4_000_000

// Hunk is one paragraph of the difference between two bodies.
type Hunk struct {
	Op   string `json:"op"`
	Text string `json:"text" untrusted:"true"`
}

// DiffResult is the difference between two bodies at paragraph level.
type DiffResult struct {
	Hunks []Hunk `json:"hunks"`
	// Coarse is true when the bodies were too large to compare paragraph by paragraph: the result is the whole of a
	// removed and the whole of b added.
	Coarse bool `json:"coarse"`
}

var blankLines = regexp.MustCompile(`\n[ \t]*\n(?:[ \t]*\n)*`)

func paragraphs(body string) []string {
	body = strings.Trim(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	if strings.TrimSpace(body) == "" {
		return nil
	}

	parts := blankLines.Split(body, -1)
	out := make([]string, 0, len(parts))

	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}

	return out
}

// Diff compares two bodies at paragraph level, as the longest common subsequence of their paragraphs (SDD-019
// section 3): paragraphs only in a are removed, paragraphs only in b are added, the others are equal.
func Diff(a, b string) DiffResult {
	pa, pb := paragraphs(a), paragraphs(b)

	if len(pa)*len(pb) > maxDiffCells {
		return DiffResult{Hunks: wholeDiff(pa, pb), Coarse: true}
	}

	// lcs[i][j] is the length of the common subsequence of pa[i:] and pb[j:].
	lcs := make([][]int, len(pa)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(pb)+1)
	}

	for i := range slices.Backward(pa) {
		for j := range slices.Backward(pb) {
			if pa[i] == pb[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}

	hunks := []Hunk{}
	i, j := 0, 0

	for i < len(pa) && j < len(pb) {
		switch {
		case pa[i] == pb[j]:
			hunks = append(hunks, Hunk{Op: OpEqual, Text: pa[i]})
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			hunks = append(hunks, Hunk{Op: OpRemoved, Text: pa[i]})
			i++
		default:
			hunks = append(hunks, Hunk{Op: OpAdded, Text: pb[j]})
			j++
		}
	}

	for ; i < len(pa); i++ {
		hunks = append(hunks, Hunk{Op: OpRemoved, Text: pa[i]})
	}

	for ; j < len(pb); j++ {
		hunks = append(hunks, Hunk{Op: OpAdded, Text: pb[j]})
	}

	return DiffResult{Hunks: hunks}
}

func wholeDiff(pa, pb []string) []Hunk {
	hunks := make([]Hunk, 0, 2)

	if len(pa) > 0 {
		hunks = append(hunks, Hunk{Op: OpRemoved, Text: strings.Join(pa, "\n\n")})
	}

	if len(pb) > 0 {
		hunks = append(hunks, Hunk{Op: OpAdded, Text: strings.Join(pb, "\n\n")})
	}

	return hunks
}
