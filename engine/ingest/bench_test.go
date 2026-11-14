package ingest

import (
	"context"
	"testing"
	"time"
)

// BenchmarkIngest60s is NFR-005: the ingest of a 60 s 1080p phone clip (portrait, with sound) completes
// within 30 s. It reports the time of one ingest in seconds as s_ingest, which the performance report reads.
func BenchmarkIngest60s(b *testing.B) {
	e := newEnv(b)
	path, err := e.gen.Clip(context.Background(), "phone-60s.mp4", 1080, 1920, 30, 60)
	if err != nil {
		b.Fatal(err)
	}
	hash := e.put(path)
	b.ResetTimer()
	for i := range b.N {
		e.n++
		start := time.Now()
		_, err := Run(context.Background(), Input{
			Runner: e.r, Caps: e.caps, Hash: hash, ArtifactsDir: e.artifacts(), JobID: "bench_" + itoa(i),
		})
		if err != nil {
			b.Fatal(err)
		}
		b.ReportMetric(time.Since(start).Seconds(), "s_ingest")
	}
}
