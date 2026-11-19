package textview

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestDiffTextHeaderAndLines(t *testing.T) {
	t.Parallel()

	lines := []string{
		"removed item itm_a clip was 0s/0f +2s/60f on trk_v",
		"trimmed item itm_b: duration 3s/90f -> 2s/60f",
		"moved item itm_c: start 10s/300f -> 12s/360f",
	}
	text := Diff("prj_7k2m9x4q1b", 3, 7, lines)

	want := Note + "\ndiff prj_7k2m9x4q1b r3 -> r7 (3 changes)\n" + strings.Join(lines, "\n")
	if text != want {
		t.Fatalf("text:\n%s\nwant:\n%s", text, want)
	}

	if none := Diff("prj_7k2m9x4q1b", 5, 5, nil); !strings.HasSuffix(none, "r5 -> r5 (no changes)") {
		t.Errorf("no changes: %q", none)
	}
}

// TestDiffTextBudget is the contract of SDD-003 section 3.7.1 on a large change: the text stays
// within the cap, is cut at a line boundary, keeps the most significant lines (the order given),
// and says exactly how many were left out.
func TestDiffTextBudget(t *testing.T) {
	t.Parallel()

	const total = 900

	lines := make([]string, total)
	for i := range lines {
		lines[i] = fmt.Sprintf(
			"added item itm_%010d text on trk_a1b2c3d4e5 %ds/%df +1s/30f \"line number %d\"",
			i,
			i,
			i*30,
			i,
		)
	}

	text := Diff("prj_7k2m9x4q1b", 1, 2, lines)

	if n := len([]rune(text)); n > CapDiff {
		t.Fatalf("the text has %d characters, the cap is %d", n, CapDiff)
	}

	shown := FitDiff(lines)
	if shown <= 0 || shown >= total {
		t.Fatalf("FitDiff = %d of %d", shown, total)
	}

	body := strings.Split(text, "\n")
	// note, header, the shown lines, the trailer
	if len(body) != 2+shown+1 {
		t.Fatalf("the text has %d lines, want %d", len(body), 2+shown+1)
	}

	for i := range shown {
		if body[2+i] != lines[i] {
			t.Fatalf("line %d was cut or reordered: %q", i, body[2+i])
		}
	}

	wantTail := fmt.Sprintf("... and %d more changes: read the diff of a narrower range, or the history", total-shown)
	if body[len(body)-1] != wantTail {
		t.Fatalf("trailer %q, want %q", body[len(body)-1], wantTail)
	}

	if !strings.Contains(body[1], fmt.Sprintf("(%d changes)", total)) {
		t.Errorf("the header does not carry the whole count: %q", body[1])
	}

	// A very long line still shows at least one line, never an empty list.
	if FitDiff([]string{strings.Repeat("x", 7000)}) != 1 {
		t.Error("a single oversized line is not shown")
	}
}

func TestCheckpointsText(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 11, 14, 12, 30, 5, 0, time.UTC)
	lines := []CheckpointLine{
		{Revision: 9, Name: "before the \"cut\"", Note: "keep the intro", Actor: "local:ui", At: at},
		{Revision: 2, Name: "auto local:claude 2026-11-14", Actor: "local:claude", At: at, Auto: true},
	}

	text := Checkpoints("prj_7k2m9x4q1b", 12, lines, 2)
	want := Note + "\ncheckpoints prj_7k2m9x4q1b rev 12 (showing 2 of 2)\n" +
		`r9 "before the \"cut\"" local:ui 2026-11-14T12:30:05Z - "keep the intro"` + "\n" +
		`r2 "auto local:claude 2026-11-14" local:claude 2026-11-14T12:30:05Z auto`

	if text != want {
		t.Fatalf("text:\n%s\nwant:\n%s", text, want)
	}

	if left := Checkpoints("prj_7k2m9x4q1b", 12, lines[:1], 2); !strings.HasSuffix(left, "... and 1 more checkpoints") {
		t.Errorf("the count of the checkpoints left out is missing:\n%s", left)
	}
}
