package asset

import (
	"math"
	"strconv"
)

// Sizes are decimal (1 KB is 1,000 bytes) and stop at GB (SDD-011 section 14.9): the one rule that every surface
// shares, so no page computes or rounds a size.
const (
	kilobyte int64 = 1000
	megabyte       = kilobyte * 1000
	gigabyte       = megabyte * 1000
)

// FormatSize writes a size in words: a whole number of bytes below 1,000, then one decimal of KB, MB or GB with the
// trailing ".0" dropped. A value that rounds up to 1,000 of its unit is written in the next unit. A size that is
// not known (nil, or a negative number a corrupt row could hold) has no text: it is not "0 B".
func FormatSize(bytes *int64) string {
	if bytes == nil || *bytes < 0 {
		return ""
	}

	n := *bytes
	if n < kilobyte {
		return strconv.FormatInt(n, 10) + " B"
	}

	units := []struct {
		size int64
		name string
	}{{kilobyte, "KB"}, {megabyte, "MB"}, {gigabyte, "GB"}}

	for i, u := range units {
		last := i == len(units)-1
		if !last && n >= units[i+1].size {
			continue
		}

		whole, rem := n/u.size, n%u.size
		tenths := (rem*10 + u.size/2) / u.size

		if tenths == 10 {
			whole, tenths = whole+1, 0
		}

		if !last && whole == 1000 {
			continue
		}

		out := strconv.FormatInt(whole, 10)
		if tenths != 0 {
			out += "." + strconv.FormatInt(tenths, 10)
		}

		return out + " " + u.name
	}

	return ""
}

// SizeItem is one file with its size as a list holds it; Bytes is nil when the size is unknown (the file is still
// importing or is missing from the store).
type SizeItem struct {
	Hash  string
	Bytes *int64
}

// Total is what a list of files weighs.
type Total struct {
	Bytes   int64 // the sum of the known sizes, each content hash once
	Files   int   // the distinct files
	Unsized int   // the distinct files whose size is unknown; they add nothing to Bytes
}

// SizeText is the total in words. A list whose every file has an unknown size has no text: its known sum is 0,
// and "0 B" would say the files weigh nothing (SDD-011 section 14.9). An empty list weighs "0 B".
func (t Total) SizeText() string {
	if t.Files > 0 && t.Unsized == t.Files {
		return ""
	}

	return FormatSize(&t.Bytes)
}

// TotalOf adds the sizes of the items counting each content hash once, however many projects or names hold it. A
// hash that appears with a known size and with an unknown one is known. The sum stops at the largest int64 instead
// of wrapping.
func TotalOf(items []SizeItem) Total {
	known := make(map[string]int64, len(items))
	unknown := make(map[string]bool)

	for _, it := range items {
		if it.Bytes == nil || *it.Bytes < 0 {
			if _, ok := known[it.Hash]; !ok {
				unknown[it.Hash] = true
			}

			continue
		}

		if _, ok := known[it.Hash]; !ok {
			known[it.Hash] = *it.Bytes
		}

		delete(unknown, it.Hash)
	}

	t := Total{Files: len(known) + len(unknown), Unsized: len(unknown)}

	for _, n := range known {
		if t.Bytes > math.MaxInt64-n {
			t.Bytes = math.MaxInt64

			break
		}

		t.Bytes += n
	}

	return t
}
