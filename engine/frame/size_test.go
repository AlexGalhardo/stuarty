package frame

import "testing"

// SDD-003 section 3.11: the table of sizes is a test.
func TestImageSizeTable(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name         string
		aw, ah, bud  int
		w, h, tokens int
	}{
		{"9:16 frame", 1080, 1920, 800, 582, 1036, 777},
		{"9:16 sheet", 1080, 1920, 1500, 804, 1428, 1479},
		{"1:1 frame", 1, 1, 800, 784, 784, 784},
		{"1:1 sheet", 1, 1, 1500, 1064, 1064, 1444},
		{"16:9 frame", 1920, 1080, 800, 1036, 582, 777},
		{"16:9 sheet", 1920, 1080, 1500, 1428, 804, 1479},
		{"4:5 frame", 4, 5, 800, 694, 868, 775},
		{"4:5 sheet", 4, 5, 1500, 940, 1176, 1428},
		{"9:16 at the maximum", 1080, 1920, 1568, 820, 1456, 1560},
	}
	for _, c := range cases {
		w, h, tok := ImageSize(c.aw, c.ah, c.bud)
		if w != c.w || h != c.h || tok != c.tokens {
			t.Errorf("%s: %dx%d (%d tokens), want %dx%d (%d)", c.name, w, h, tok, c.w, c.h, c.tokens)
		}
		if w%2 != 0 || h%2 != 0 || tok > c.bud {
			t.Errorf("%s: %dx%d is odd or over budget", c.name, w, h)
		}
	}
	if w, h, tok := ImageSize(0, 5, 800); w != 0 || h != 0 || tok != 0 {
		t.Error("an empty aspect gave a size")
	}
}

func TestSheetTimesIncludeBothEnds(t *testing.T) {
	t.Parallel()
	got := SheetTimes(0, 80, 9)
	want := []int{0, 10, 20, 30, 40, 50, 60, 70, 80}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%v, want %v", got, want)
		}
	}
	if got := SheetTimes(5, 100, 4); got[0] != 5 || got[3] != 100 || len(got) != 4 {
		t.Errorf("%v", got)
	}
	if got := SheetTimes(7, 7, 1); len(got) != 1 || got[0] != 7 {
		t.Errorf("%v", got)
	}
}
