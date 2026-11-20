package textview

import (
	"fmt"
	"strings"
	"testing"
)

func TestImageDiffTextHeaderAndLines(t *testing.T) {
	t.Parallel()

	lines := []string{
		`added text "Title" lyr_a1b2c3d4e5 at 80,440 800x200`,
		`removed image "Photo" lyr_f6g7h8j9k0`,
		`changed lyr_s6t7v8w9x0: opacity 100 -> 60`,
	}

	want := Note + "\ndiff img_7k2m9x4q1b r3 -> r5 (3 changes)\n" + strings.Join(lines, "\n")
	if text := ImageDiff("img_7k2m9x4q1b", 3, 5, lines); text != want {
		t.Fatalf("text:\n%s\nwant:\n%s", text, want)
	}

	if none := ImageDiff("img_7k2m9x4q1b", 5, 5, nil); !strings.HasSuffix(none, "r5 -> r5 (no changes)") ||
		!strings.HasPrefix(none, Note) {
		t.Errorf("no changes: %q", none)
	}
}

// TestImageDiffText is the budget of an edit result on a large change: at most 40 lines and 4,000 characters in
// all, cut at a line boundary, the most significant lines kept (the order given), and the exact count of what
// was left out.
func TestImageDiffText(t *testing.T) {
	t.Parallel()

	for name, line := range map[string]func(i int) string{
		"short lines": func(i int) string { return fmt.Sprintf("changed lyr_%010d: opacity 100 -> %d", i, i%100) },
		"long lines": func(i int) string {
			return fmt.Sprintf("added text %q lyr_%010d at 80,440 800x200", strings.Repeat("hostile name ", 4), i)
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			const total = 300

			lines := make([]string, total)
			for i := range lines {
				lines[i] = line(i)
			}

			text := ImageDiff("img_7k2m9x4q1b", 1, 2, lines)

			if n := len([]rune(text)); n > CapImageDiff {
				t.Fatalf("the text has %d characters, the cap is %d", n, CapImageDiff)
			}

			shown := FitImageDiff(lines)
			if shown <= 0 || shown > MaxImageDiffLines || shown >= total {
				t.Fatalf("FitImageDiff = %d of %d", shown, total)
			}

			body := strings.Split(text, "\n")
			if len(body) != 1+1+shown+1 { // note, header, the lines, the count
				t.Fatalf("the text has %d lines for %d shown", len(body), shown)
			}

			if body[1] != fmt.Sprintf("diff img_7k2m9x4q1b r1 -> r2 (%d changes)", total) {
				t.Errorf("header = %q", body[1])
			}

			for i := range shown {
				if body[2+i] != lines[i] {
					t.Fatalf("line %d is %q, want the %dth most significant", i, body[2+i], i)
				}
			}

			last := body[len(body)-1]
			if want := fmt.Sprintf(
				"... and %d more changes: read the diff of a narrower range, or the history",
				total-shown,
			); last != want {
				t.Errorf("last line = %q, want %q", last, want)
			}
		})
	}

	// A change that fits says nothing was cut.
	if text := ImageDiff("img_7k2m9x4q1b", 1, 2, []string{"moved lyr_a"}); strings.Contains(text, "more changes") {
		t.Errorf("a short diff was cut: %q", text)
	}
}
