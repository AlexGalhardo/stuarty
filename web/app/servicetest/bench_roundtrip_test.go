package servicetest

import (
	"fmt"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	vukem "github.com/AlexGalhardo/vukem"
	"github.com/AlexGalhardo/vukem/vukemtest"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/internal/outbound"
	"github.com/AlexGalhardo/vukem-video/service"
	"github.com/AlexGalhardo/vukem-video/service/remote"
	"github.com/AlexGalhardo/vukem-video/web/app/auth"
	httpapi "github.com/AlexGalhardo/vukem-video/web/app/http"
	"github.com/AlexGalhardo/vukem-video/web/bootstrap"
)

// BenchmarkRoundTrip200Items is SDD-005 section 18 check 12 over the real wire: the same
// project of about 200 items and 2,000 caption words, one item.move per iteration through the
// HTTP client of service/remote and the server on loopback. Its p95 must stay under the 50 ms of
// NFR-001.
func BenchmarkRoundTrip200Items(b *testing.B) { benchRoundTrip200Items(b, 0) }

// BenchmarkRoundTrip200ItemsLog100k is the same round trip on a log of 100,000 rows (P4-SRV-002).
func BenchmarkRoundTrip200ItemsLog100k(b *testing.B) { benchRoundTrip200Items(b, longLogRows) }

func benchRoundTrip200Items(b *testing.B, logRows int) {
	b.Helper()

	e := newEnv(b)
	command.Default().Debug = false // a test aid, not the product's cost

	project, textTrack := e.seed200(b)
	moved := "itm_00000000aa"

	e.mustEditB(b, project, op("text.add",
		fmt.Sprintf(`{"track_id":%q,"start":"500s","duration":"1s","text":"mover","id":%q}`, textTrack, moved)))

	if logRows > 0 {
		e.padLog(b, project, logRows)
	}

	token, err := e.dir.EnsureToken()
	if err != nil {
		b.Fatal(err)
	}

	deps := &httpapi.Deps{
		Service: e.svc, Registry: command.Default(), Workspace: e.dir, Token: token,
		Sessions: auth.NewSessions(time.Now), Launch: auth.NewLaunch(time.Now), Now: time.Now,
	}

	app := vukemtest.New(b, func(opts ...vukem.Option) (*vukem.App, error) { return bootstrap.NewWith(deps, opts...) })
	srv := httptest.NewServer(app.Handler())
	b.Cleanup(srv.Close)

	client := remote.New(srv.URL+"/", token, outbound.LocalServer(time.Minute))
	ctx := e.as("local:bench")
	rev := e.headB(b, project)
	times := make([]time.Duration, 0, b.N)

	b.ResetTimer()

	for i := range b.N {
		// The item starts at 500s: the first move goes to 502s, because a move to where the
		// item already is applies nothing and the revision would not advance.
		start := fmt.Sprintf("%ds", 500+((i+1)%2)*2)

		t0 := time.Now()
		res, err := client.Edit(ctx, service.EditRequest{
			ProjectID: project, BaseRevision: rev,
			Operations: []command.Operation{op("item.move", fmt.Sprintf(`{"item_id":%q,"start":%q}`, moved, start))},
		})
		times = append(times, time.Since(t0))

		if err != nil {
			b.Fatal(err)
		}

		rev = res.Revision
	}

	b.StopTimer()

	slices.Sort(times)

	p95 := times[min(len(times)-1, len(times)*95/100)]
	b.ReportMetric(float64(p95.Microseconds())/1000, "p95-ms")
	b.ReportMetric(float64(times[len(times)/2].Microseconds())/1000, "p50-ms")
	b.Logf("n=%d min %s p50 %s p95 %s max %s", len(times), times[0], times[len(times)/2], p95, times[len(times)-1])

	if b.N >= 100 && p95 > nfr001P95 {
		b.Errorf("p95 of an HTTP round trip is %s, over the %s of NFR-001", p95, nfr001P95)
	}
}
