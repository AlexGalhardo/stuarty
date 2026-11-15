package mcp

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/AlexGalhardo/vukem-video/web/contract/testenv"
)

// NFR-023 (SDD-003 section 9 check 9): a nine-cell sheet takes under 2 s and a
// frame under 1 s, as the median of cold renders, on the reference machine of
// SDD-008. The renders go through the view tool, so the budget covers the whole
// path an agent pays for: the tool, the service, the cache and FFmpeg.
const (
	sheetBudget = 2 * time.Second
	frameBudget = time.Second
)

// coldView measures one view call per iteration. Each iteration first commits a
// text item, outside the timer, so that the revision is new and the frame cache
// of the previous iteration cannot answer.
func coldView(b *testing.B, args func(project string) map[string]any, budget time.Duration) {
	b.Helper()

	env := testenv.New(b, testenv.Options{Frames: true})
	cs := connect(b, env.Service, Options{}, "claude-code")
	project := textProject(b, cs)
	revision := 2

	touch := func() {
		ok(b, call(b, cs, "edit", map[string]any{
			"project_id": project, "base_revision": revision,
			"operations": []any{map[string]any{
				"op": "settings.set", "args": map[string]any{"background": fmt.Sprintf("#%06x", revision)},
			}},
		}))

		revision++
	}

	view := func() time.Duration {
		touch()

		start := time.Now()
		res := call(b, cs, "view", args(project))
		took := time.Since(start)

		if res.IsError {
			b.Fatalf("view: %s", textOf(b, res))
		}

		return took
	}

	view() // the first render warms FFmpeg and the disk cache

	samples := make([]time.Duration, 0, b.N)

	b.ResetTimer()

	for b.Loop() {
		b.StopTimer()
		touch()
		b.StartTimer()

		start := time.Now()
		if res := call(b, cs, "view", args(project)); res.IsError {
			b.Fatalf("view: %s", textOf(b, res))
		}

		samples = append(samples, time.Since(start))
	}

	slices.Sort(samples)

	median := samples[len(samples)/2]
	b.ReportMetric(float64(median.Milliseconds()), "median-ms")

	if median > budget {
		b.Errorf("the median is %v, at most %v (NFR-023)", median, budget)
	}
}

// BenchmarkViewSheet9 is the contact sheet of nine cells, the default sheet.
func BenchmarkViewSheet9(b *testing.B) {
	coldView(b, func(project string) map[string]any {
		return map[string]any{"project_id": project, "mode": "sheet", "from": "0s", "until": "0.9s", "count": 9}
	}, sheetBudget)
}

// BenchmarkViewFrame is one frame at the default width.
func BenchmarkViewFrame(b *testing.B) {
	coldView(b, func(project string) map[string]any {
		return map[string]any{"project_id": project, "at": "0.5s"}
	}, frameBudget)
}
