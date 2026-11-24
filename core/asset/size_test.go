package asset

import (
	"math"
	"testing"
)

func TestFormatSize(t *testing.T) {
	t.Parallel()

	const gib = int64(1) << 30

	cases := []struct {
		name string
		in   *int64
		want string
	}{
		{"unknown is no text, never 0 B", nil, ""},
		{"a negative size from a corrupt row is unknown", new(int64(-1)), ""},
		{"zero", new(int64(0)), "0 B"},
		{"one byte", new(int64(1)), "1 B"},
		{"just below a kilobyte", new(int64(999)), "999 B"},
		{"a kilobyte", new(int64(1000)), "1 KB"},
		{"a decimal kilobyte", new(int64(1500)), "1.5 KB"},
		{"rounds down in the tenths", new(int64(1549)), "1.5 KB"},
		{"rounds up in the tenths", new(int64(1550)), "1.6 KB"},
		{"just below a megabyte rolls over", new(int64(999_999)), "1 MB"},
		{"just below the rounding of a kilobyte", new(int64(999_949)), "999.9 KB"},
		{"a megabyte", new(int64(1_000_000)), "1 MB"},
		{"a typical clip", new(int64(12_345_678)), "12.3 MB"},
		{"just below a gigabyte rolls over", new(int64(999_999_999)), "1 GB"},
		{"4 GiB, over what 32 bits hold", new(4 * gib), "4.3 GB"},
		{"a gigabyte", new(int64(1_000_000_000)), "1 GB"},
		{"there is no unit above GB", new(int64(2_000_000_000_000)), "2000 GB"},
		{"the largest size", new(int64(math.MaxInt64)), "9223372036.9 GB"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := FormatSize(tc.in); got != tc.want {
				t.Errorf("FormatSize = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTotalOfCountsEachHashOnce(t *testing.T) {
	t.Parallel()

	got := TotalOf([]SizeItem{
		{"sha256:a", new(int64(100))},
		{"sha256:a", new(int64(100))}, // the same file held by a second project
		{"sha256:b", new(int64(50))},
	})

	if want := (Total{Bytes: 150, Files: 2}); got != want {
		t.Errorf("TotalOf = %+v, want %+v", got, want)
	}
}

func TestTotalOfAFileWithNoKnownSizeAddsNothingAndIsCounted(t *testing.T) {
	t.Parallel()

	got := TotalOf([]SizeItem{
		{"sha256:a", new(int64(100))},
		{"sha256:importing", nil},
		{"sha256:missing", nil},
		{"sha256:missing", nil},
		{"sha256:corrupt", new(int64(-5))},
	})

	if want := (Total{Bytes: 100, Files: 4, Unsized: 3}); got != want {
		t.Errorf("TotalOf = %+v, want %+v", got, want)
	}
}

func TestTotalOfAHashKnownOnceIsKnown(t *testing.T) {
	t.Parallel()

	got := TotalOf([]SizeItem{{"sha256:a", nil}, {"sha256:a", new(int64(7))}})

	if want := (Total{Bytes: 7, Files: 1}); got != want {
		t.Errorf("TotalOf = %+v, want %+v", got, want)
	}
}

func TestTotalSizeTextOfFilesWithNoKnownSizeIsNeverZero(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		items []SizeItem
		want  string
	}{
		{"an empty library weighs 0 B", nil, "0 B"},
		{"only a file still importing has no total", []SizeItem{{"sha256:a", nil}}, ""},
		{"two files with no size have no total", []SizeItem{{"sha256:a", nil}, {"sha256:b", nil}}, ""},
		{"one known file gives the total", []SizeItem{{"sha256:a", nil}, {"sha256:b", new(int64(1500))}}, "1.5 KB"},
		{"an empty file is known and weighs 0 B", []SizeItem{{"sha256:a", new(int64(0))}}, "0 B"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := TotalOf(tc.items).SizeText(); got != tc.want {
				t.Errorf("SizeText = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTotalOfAnEmptyLibraryIsZero(t *testing.T) {
	t.Parallel()

	empty := TotalOf(nil)
	if empty != (Total{}) {
		t.Errorf("TotalOf(nil) = %+v, want zero", empty)
	}

	if FormatSize(&empty.Bytes) != "0 B" {
		t.Error("an empty library weighs 0 B, not an unknown size")
	}
}

func TestTotalOfOverFourGiBAndSaturation(t *testing.T) {
	t.Parallel()

	const gib = int64(1) << 30

	if got := TotalOf([]SizeItem{{"a", new(4 * gib)}, {"b", new(4 * gib)}}); got.Bytes != 8*gib {
		t.Errorf("Bytes = %d, want %d", got.Bytes, 8*gib)
	}

	got := TotalOf([]SizeItem{{"a", new(int64(math.MaxInt64))}, {"b", new(int64(10))}})
	if got.Bytes != math.MaxInt64 {
		t.Errorf("Bytes = %d, want saturation at MaxInt64", got.Bytes)
	}
}
