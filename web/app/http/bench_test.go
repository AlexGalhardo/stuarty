package httpapi_test

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/web/app/events"
	httpapi "github.com/AlexGalhardo/stuarty/web/app/http"
)

// BenchmarkEditRoundTrip is NFR-001 through the HTTP handler: a project of 200
// items (166 text items and 34 captions of 60 words), one item.move per
// iteration over POST /edit, measured from the request to the whole response.
// It reports p50 and p95; the budget is 50 ms at p95.
func BenchmarkEditRoundTrip(b *testing.B) {
	command.Default().Debug = false // the debug validation of every result is a test aid, not the product's cost

	b.Cleanup(func() { command.Default().Debug = true })

	h := newHarnessLimited(b, events.Options{}, nil, httpapi.RateLimits{PerCredential: 1_000_000})
	id := h.newProject("bench")
	base := "/api/v1/projects/" + id + "/edit"

	edit := func(rev int64, ops string) resp {
		return h.post(base, fmt.Sprintf(`{"base_revision":%d,"operations":%s}`, rev, ops))
	}

	first := edit(0, `[{"op":"track.add","args":{"kind":"visual","name":"T","id":"trk_f6g7h8j9k0"}},`+
		`{"op":"track.add","args":{"kind":"visual","name":"C","id":"trk_a1b2c3d4e5"}}]`)
	if first.status != http.StatusOK {
		b.Fatalf("tracks = %d %s", first.status, first.body)
	}

	words := strings.TrimSpace(strings.Repeat("word ", 60))

	var ops []string

	for i := range 166 {
		ops = append(ops, fmt.Sprintf(
			`{"op":"text.add","args":{"track_id":"trk_f6g7h8j9k0","start":"%ds","duration":"1s","text":"t%d"}}`,
			i*2,
			i,
		))
	}

	for i := range 34 {
		ops = append(ops, fmt.Sprintf(
			`{"op":"caption.add","args":{"track_id":"trk_a1b2c3d4e5","start":"%ds","duration":"3s","text":%q}}`,
			i*4,
			words,
		))
	}

	rev := int64(1)

	for _, batch := range [][]string{ops[:100], ops[100:]} {
		res := edit(rev, "["+strings.Join(batch, ",")+"]")
		if res.status != http.StatusOK {
			b.Fatalf("seed = %d %s", res.status, res.body)
		}

		rev++
	}

	const moved = "itm_00000000aa"

	res := edit(rev, fmt.Sprintf(
		`[{"op":"text.add","args":{"track_id":"trk_f6g7h8j9k0","start":"500s","duration":"1s","text":"mover","id":%q}}]`,
		moved,
	))
	if res.status != http.StatusOK {
		b.Fatalf("mover = %d %s", res.status, res.body)
	}

	rev++

	times := make([]time.Duration, 0, b.N)

	b.ResetTimer()

	for i := range b.N {
		body := fmt.Sprintf(`[{"op":"item.move","args":{"item_id":%q,"start":"%ds"}}]`, moved, 500+(i%2)*2)

		t0 := time.Now()
		res := edit(rev, body)
		times = append(times, time.Since(t0))

		if res.status != http.StatusOK {
			b.Fatalf("move = %d %s", res.status, res.body)
		}

		rev++
	}

	b.StopTimer()

	slices.Sort(times)

	p95 := times[min(len(times)-1, len(times)*95/100)]
	b.ReportMetric(float64(times[len(times)/2].Microseconds())/1000, "p50-ms")
	b.ReportMetric(float64(p95.Microseconds())/1000, "p95-ms")
	b.Logf("n=%d min %s p50 %s p95 %s max %s", len(times), times[0], times[len(times)/2], p95, times[len(times)-1])
}
