package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestCheckJudgesEveryBound(t *testing.T) {
	t.Parallel()

	base := Sample{Commits: 100, HeapInuse: 100 << 20, Goroutines: 40, Handles: 200, DBBytes: 1 << 20}
	ok := Report{Samples: []Sample{base, {
		At: time.Hour, Commits: 1100, HeapInuse: 110 << 20, Goroutines: 42, Handles: 205, DBBytes: 2 << 20,
	}}}

	if bad := ok.Check(DefaultBounds); len(bad) != 0 {
		t.Fatalf("a bounded run failed: %v", bad)
	}

	for name, last := range map[string]Sample{
		"heap":       {Commits: 1100, HeapInuse: 130 << 20, Goroutines: 40, Handles: 200, DBBytes: 1 << 20},
		"goroutines": {Commits: 1100, HeapInuse: 100 << 20, Goroutines: 80, Handles: 200, DBBytes: 1 << 20},
		"handles":    {Commits: 1100, HeapInuse: 100 << 20, Goroutines: 40, Handles: 900, DBBytes: 1 << 20},
		"database":   {Commits: 1100, HeapInuse: 100 << 20, Goroutines: 40, Handles: 200, DBBytes: 50 << 20},
		"jobs":       {Commits: 1100, HeapInuse: 100 << 20, Goroutines: 40, Handles: 200, DBBytes: 1 << 20, Jobs: 2},
	} {
		bad := Report{Samples: []Sample{base, last}}.Check(DefaultBounds)
		if len(bad) != 1 || !strings.Contains(bad[0], name[:4]) {
			t.Errorf("%s: %v, want exactly that bound to fail", name, bad)
		}
	}

	if bad := (Report{Samples: []Sample{base}}).Check(DefaultBounds); len(bad) != 1 {
		t.Errorf("a run with one sample passed: %v", bad)
	}
}

// Threads the runtime starts under load add handles that are not a leak (VV-363); the same growth with no new
// thread is still one.
func TestCheckAllowsTheHandlesOfNewThreadsOnly(t *testing.T) {
	t.Parallel()

	bounds := Bounds{HeapGrowth: 1.25, Goroutines: 10, Handles: 10, HandlesPerThread: 6, DBBytesPerCommit: 2048}
	base := Sample{Commits: 100, HeapInuse: 100 << 20, Goroutines: 40, Threads: 20, Handles: 168}
	last := base
	last.Commits, last.Handles = 1100, 180

	if bad := (Report{Samples: []Sample{base, last}}).Check(bounds); len(bad) != 1 {
		t.Fatalf("12 more handles with no new thread passed or failed another bound: %v", bad)
	}

	last.Threads = 22

	if bad := (Report{Samples: []Sample{base, last}}).Check(bounds); len(bad) != 0 {
		t.Fatalf("12 more handles with 2 new threads failed: %v", bad)
	}

	last.Handles = 168 + 10 + 12 + 1

	if bad := (Report{Samples: []Sample{base, last}}).Check(bounds); len(bad) != 1 {
		t.Fatalf("a growth beyond the allowance of the new threads passed: %v", bad)
	}
}

// The smoke run: a few seconds of the real script on a real store, with the bounds of the full run.
func TestSmokeRunStaysBounded(t *testing.T) {
	var out, errb bytes.Buffer

	code := run(context.Background(),
		[]string{"-duration", "6s", "-sample", "1s", "-warmup", "2s", "-workdir", t.TempDir()}, &out, &errb)
	if code != 0 {
		t.Fatalf("the smoke run failed (%d):\n%s\n%s", code, out.String(), errb.String())
	}

	if !strings.Contains(out.String(), "Result: pass") || !strings.Contains(out.String(), "| At |") {
		t.Errorf("the report has no table or no verdict:\n%s", out.String())
	}
}
