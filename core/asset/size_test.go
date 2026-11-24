package asset

import (
	"math"
	"testing"
)

func size(n int64) *int64 { return &n }

func TestFormatSize(t *testing.T) {
	t.Parallel()

	const gib = int64(1) << 30

	cases := []struct {
		name string
		in   *int64
		want string
	}{
		{"unknown is no text, never 0 B", nil, ""},
		{"a negative size from a corrupt row is unknown", size(-1), ""},
		{"zero", size(0), "0 B"},
		{"one byte", size(1), "1 B"},
		{"just below a kilobyte", size(999), "999 B"},
		{"a kilobyte", size(1000), "1 KB"},
		{"a decimal kilobyte", size(1500), "1.5 KB"},
		{"rounds down in the tenths", size(1549), "1.5 KB"},
		{"rounds up in the tenths", size(1550), "1.6 KB"},
		{"just below a megabyte rolls over", size(999_999), "1 MB"},
		{"just below the rounding of a kilobyte", size(999_949), "999.9 KB"},
		{"a megabyte", size(1_000_000), "1 MB"},
		{"a typical clip", size(12_345_678), "12.3 MB"},
		{"just below a gigabyte rolls over", size(999_999_999), "1 GB"},
		{"4 GiB, over what 32 bits hold", size(4 * gib), "4.3 GB"},
		{"a gigabyte", size(1_000_000_000), "1 GB"},
		{"there is no unit above GB", size(2_000_000_000_000), "2000 GB"},
		{"the largest size", size(math.MaxInt64), "9223372036.9 GB"},
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
		{"sha256:a", size(100)},
		{"sha256:a", size(100)}, // the same file held by a second project
		{"sha256:b", size(50)},
	})

	if want := (Total{Bytes: 150, Files: 2}); got != want {
		t.Errorf("TotalOf = %+v, want %+v", got, want)
	}
}

func TestTotalOfAFileWithNoKnownSizeAddsNothingAndIsCounted(t *testing.T) {
	t.Parallel()

	got := TotalOf([]SizeItem{
		{"sha256:a", size(100)},
		{"sha256:importing", nil},
		{"sha256:missing", nil},
		{"sha256:missing", nil},
		{"sha256:corrupt", size(-5)},
	})

	if want := (Total{Bytes: 100, Files: 4, Unsized: 3}); got != want {
		t.Errorf("TotalOf = %+v, want %+v", got, want)
	}
}

func TestTotalOfAHashKnownOnceIsKnown(t *testing.T) {
	t.Parallel()

	got := TotalOf([]SizeItem{{"sha256:a", nil}, {"sha256:a", size(7)}})

	if want := (Total{Bytes: 7, Files: 1}); got != want {
		t.Errorf("TotalOf = %+v, want %+v", got, want)
	}
}

func TestTotalOfAnEmptyLibraryIsZero(t *testing.T) {
	t.Parallel()

	if got := TotalOf(nil); got != (Total{}) {
		t.Errorf("TotalOf(nil) = %+v, want zero", got)
	}

	if FormatSize(size(TotalOf(nil).Bytes)) != "0 B" {
		t.Error("an empty library weighs 0 B, not an unknown size")
	}
}

func TestTotalOfOverFourGiBAndSaturation(t *testing.T) {
	t.Parallel()

	const gib = int64(1) << 30

	if got := TotalOf([]SizeItem{{"a", size(4 * gib)}, {"b", size(4 * gib)}}); got.Bytes != 8*gib {
		t.Errorf("Bytes = %d, want %d", got.Bytes, 8*gib)
	}

	got := TotalOf([]SizeItem{{"a", size(math.MaxInt64)}, {"b", size(10)}})
	if got.Bytes != math.MaxInt64 {
		t.Errorf("Bytes = %d, want saturation at MaxInt64", got.Bytes)
	}
}
