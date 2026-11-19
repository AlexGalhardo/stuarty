//go:build soak

package servicetest

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"runtime/metrics"
	"runtime/pprof"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/internal/outbound"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/remote"
	"github.com/AlexGalhardo/stuarty/web/app/auth"
	httpapi "github.com/AlexGalhardo/stuarty/web/app/http"
	"github.com/AlexGalhardo/stuarty/web/bootstrap"

	vukem "github.com/AlexGalhardo/vukem"
	"github.com/AlexGalhardo/vukem/vukemtest"
)

const (
	// soakDurationEnv sets the length of the run (a Go duration); the nightly job leaves it unset: one hour.
	soakDurationEnv = "STUARTY_SOAK_DURATION"
	// soakHeapDirEnv names a directory for the heap profiles of the baseline and of the end.
	soakHeapDirEnv = "STUARTY_SOAK_HEAPDIR"
	soakWarmup     = 30 * time.Second
	soakWorkers    = 4
	// The HTTP workers share the one local credential, which the server limits to 600 requests a minute
	// plus a burst (SDD-007 section 4.4): they pace themselves under it, as a client that is told to. The
	// in-process workers have no such limit and only leave the machine some room.
	soakHTTPPause    = 700 * time.Millisecond
	soakProcessPause = 10 * time.Millisecond
	// soakMargin is the room that the soak gives goroutines and memory to return to the baseline (P4-SRV-002).
	soakMargin = 0.10
	// soakSettledEvery is how many minutes apart the settled figures are logged while the load runs.
	soakSettledEvery = 5
)

// soakStats counts what the workers did and what went wrong.
type soakStats struct {
	ops, edits, undos, reads, exports, bundles atomic.Int64
	serverErrors, refused                      atomic.Int64
	firstError                                 atomic.Value
	codes                                      sync.Map // "path code" -> *atomic.Int64
}

// count tallies an error by the path it took and its code, for the report.
func (s *soakStats) count(http bool, err error) {
	key := "in process "
	if http {
		key = "over HTTP "
	}

	if se, ok := errors.AsType[*service.Error](err); ok {
		key += se.Code
	} else {
		key += "other: " + err.Error()
	}

	n, _ := s.codes.LoadOrStore(key, new(atomic.Int64))
	n.(*atomic.Int64).Add(1)
}

// soakResident is the memory that the Go runtime holds from the system: everything it mapped less what it
// gave back. It is the portable stand-in for the resident set of the process.
func soakResident() uint64 {
	samples := []metrics.Sample{{Name: "/memory/classes/total:bytes"}, {Name: "/memory/classes/heap/released:bytes"}}
	metrics.Read(samples)

	return samples[0].Value.Uint64() - samples[1].Value.Uint64()
}

// soakHeapProfile writes the heap profile of the settled process to <STUARTY_SOAK_HEAPDIR>/<name>.pprof
// when that variable names a directory, so that go tool pprof -base baseline.pprof end.pprof shows what grew.
func soakHeapProfile(t *testing.T, name string) {
	t.Helper()

	dir := os.Getenv(soakHeapDirEnv)
	if dir == "" {
		return
	}

	f, err := os.Create(filepath.Join(dir, name+".pprof"))
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = f.Close() }()

	if err := pprof.Lookup("heap").WriteTo(f, 0); err != nil {
		t.Fatal(err)
	}
}

// soakSettle collects garbage until the numbers stop moving and returns them. The memory that the runtime
// still holds but no longer uses is handed back first (FreeOSMemory): without it the figure follows the
// pace of the background scavenger, and the in-use heap, which stays under 0.5 MiB between the baseline
// and the end of a run, is hidden behind a few MiB of free pages that move from run to run.
func soakSettle() (goroutines int, resident uint64) {
	for range 5 {
		runtime.GC()
		debug.FreeOSMemory()
		time.Sleep(200 * time.Millisecond)
	}

	return runtime.NumGoroutine(), soakResident()
}

type soakWorker struct {
	svc     service.Service
	actor   string
	project string
	item    string
}

// step runs one mixed operation and returns the revision the worker knows afterwards.
func (w *soakWorker) step(ctx context.Context, rng *rand.Rand, i int, stats *soakStats) error {
	var err error

	switch pick := rng.IntN(100); {
	case pick < 60:
		head, herr := w.svc.ProjectGet(ctx, service.ProjectGetRequest{ProjectID: w.project})
		if herr != nil {
			return herr
		}

		start := fmt.Sprintf("%ds", 500+(i%2)*2)
		_, err = w.svc.Edit(ctx, service.EditRequest{
			ProjectID: w.project, BaseRevision: head.Revision,
			Operations: []command.Operation{op("item.move", fmt.Sprintf(`{"item_id":%q,"start":%q}`, w.item, start))},
		})

		stats.edits.Add(1)
	case pick < 75:
		_, err = w.svc.Undo(ctx, service.UndoRequest{ProjectID: w.project})
		if code(err) == service.CodeNothingToUndo {
			err = nil
		}

		stats.undos.Add(1)
	case pick < 90:
		_, err = w.svc.ProjectGet(ctx, service.ProjectGetRequest{ProjectID: w.project})
		stats.reads.Add(1)
	case pick < 97:
		_, err = w.svc.JobStart(ctx, service.JobStartRequest{ProjectID: w.project, Kind: "export"})
		stats.exports.Add(1)
	default:
		_, err = w.svc.ProjectBundleExport(ctx, service.ProjectBundleExportRequest{ProjectID: w.project})
		stats.bundles.Add(1)
	}

	stats.ops.Add(1)

	return err
}

// TestSoak is the soak of P4-SRV-002: eight workers (four in process, four over HTTP on loopback) run
// mixed edits, undos, reads, exports and bundle exports for the length of the run. At the end the
// goroutines and the memory are within 10% of the values measured at rest after a warm-up, no call
// answered a 5xx other than the 503 of a locked workspace, and no job is left running.
//
//	go test -tags soak -run TestSoak -timeout 90m ./web/app/servicetest/
func TestSoak(t *testing.T) {
	total := time.Hour

	if os.Getenv(soakHeapDirEnv) != "" {
		// Finer sampling than the default 512 KiB, so that a few MiB of growth is attributed.
		runtime.MemProfileRate = 4 << 10
	}

	if v := os.Getenv(soakDurationEnv); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			t.Fatalf("%s: %v", soakDurationEnv, err)
		}

		total = d
	}

	m := newMediaEnv(t, mediaCfg{exporter: goodExporter()})
	command.Default().Debug = false

	token, err := m.dir.EnsureToken()
	if err != nil {
		t.Fatal(err)
	}

	deps := &httpapi.Deps{
		Service: m.svc, Registry: command.Default(), Workspace: m.dir, Token: token,
		Sessions: auth.NewSessions(time.Now), Launch: auth.NewLaunch(time.Now), Now: time.Now,
	}

	app := vukemtest.New(t, func(opts ...vukem.Option) (*vukem.App, error) { return bootstrap.NewWith(deps, opts...) })
	srv := httptest.NewServer(app.Handler())
	client := remote.New(srv.URL+"/", token, outbound.LocalServer(time.Minute))

	workers := make([]*soakWorker, 0, 2*soakWorkers)

	for i := range 2 * soakWorkers {
		svc := m.svc
		if i >= soakWorkers {
			svc = client
		}

		project, track := m.withTrack()
		item := fmt.Sprintf("itm_%010d", i)

		m.mustEdit(ui, project, 1, op("text.add",
			fmt.Sprintf(`{"track_id":%q,"start":"500s","duration":"1s","text":"mover","id":%q}`, track, item)))

		workers = append(
			workers,
			&soakWorker{svc: svc, actor: fmt.Sprintf("local:soak-%d", i), project: project, item: item},
		)
	}

	var stats soakStats

	run := func(d time.Duration) {
		ctx, cancel := context.WithTimeout(context.Background(), d)
		defer cancel()

		// The harness keeps every event it sees (env.events): drop them, or the soak measures the harness.
		go func() {
			tick := time.NewTicker(time.Second)
			defer tick.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-tick.C:
					m.mu.Lock()
					m.events = nil
					m.mu.Unlock()
				}
			}
		}()

		var wg sync.WaitGroup

		for n, w := range workers {
			wg.Go(func() {
				rng := rand.New(rand.NewPCG(uint64(n), 1))
				cctx := service.WithCaller(ctx, service.Caller{Actor: w.actor, Scopes: service.AllScopes})

				pause := soakProcessPause
				if n >= soakWorkers {
					pause = soakHTTPPause
				}

				for i := 0; ctx.Err() == nil; i++ {
					time.Sleep(pause)

					err := w.step(cctx, rng, i, &stats)
					if err == nil || ctx.Err() != nil {
						continue
					}

					stats.count(n >= soakWorkers, err)

					if se, ok := errors.AsType[*service.Error](err); ok {
						// The refusals of a limit are the documented answer of a full queue, not a fault.
						if se.Code == service.CodeQueueFull || se.Code == service.CodeRateLimited {
							stats.refused.Add(1)

							continue
						}

						if se.HTTPStatus() >= 500 && se.Code != service.CodeWorkspaceLocked {
							stats.serverErrors.Add(1)
						}
					}

					stats.firstError.CompareAndSwap(nil, fmt.Sprintf("%s: %v", w.actor, err))
				}
			})
		}

		wg.Wait()
	}

	run(soakWarmup)

	baseG, baseMem := soakSettle()
	soakHeapProfile(t, "baseline")
	t.Logf("baseline after %s of warm-up: %d goroutines, %.1f MiB", soakWarmup, baseG, float64(baseMem)/(1<<20))

	done := make(chan struct{})

	go func() {
		tick := time.NewTicker(time.Minute)
		defer tick.Stop()

		for minute := 1; ; minute++ {
			select {
			case <-done:
				return
			case <-tick.C:
				t.Logf("running: %d ops, %d goroutines, %.1f MiB resident", stats.ops.Load(), runtime.NumGoroutine(),
					float64(soakResident())/(1<<20))

				// The figure that the margin judges is the settled one: it shows whether the memory
				// plateaus or climbs while the load is still running.
				if minute%soakSettledEvery == 0 {
					g, mem := soakSettle()
					t.Logf("settled at minute %d: %d goroutines, %.1f MiB", minute, g, float64(mem)/(1<<20))
				}
			}
		}
	}()

	run(total)
	close(done)
	srv.Close()

	endG, endMem := soakSettle()
	soakHeapProfile(t, "end")
	t.Logf("%s: %d ops (%d edits, %d undos, %d reads, %d exports, %d bundle exports), %d refused by a limit", total,
		stats.ops.Load(), stats.edits.Load(), stats.undos.Load(), stats.reads.Load(), stats.exports.Load(),
		stats.bundles.Load(), stats.refused.Load())
	t.Logf("end: %d goroutines (baseline %d), %.1f MiB (baseline %.1f MiB)", endG, baseG,
		float64(endMem)/(1<<20), float64(baseMem)/(1<<20))

	stats.codes.Range(func(k, v any) bool {
		t.Logf("answered %d times: %s", v.(*atomic.Int64).Load(), k)

		return true
	})

	if v := stats.firstError.Load(); v != nil {
		t.Logf("first error seen: %v", v)
	}

	if n := stats.serverErrors.Load(); n != 0 {
		t.Errorf("%d calls answered a 5xx other than the 503 of a locked workspace", n)
	}

	if float64(endG) > float64(baseG)*(1+soakMargin) {
		t.Errorf("goroutines: %d at the end, %d at the baseline, over the %.0f%% margin", endG, baseG, soakMargin*100)
	}

	if float64(endMem) > float64(baseMem)*(1+soakMargin) {
		t.Errorf("memory: %.1f MiB at the end, %.1f MiB at the baseline, over the %.0f%% margin",
			float64(endMem)/(1<<20), float64(baseMem)/(1<<20), soakMargin*100)
	}

	var running int
	if err := m.store.DB().QueryRowContext(context.Background(),
		"SELECT count(*) FROM jobs WHERE state = ?", "running").Scan(&running); err != nil {
		t.Fatal(err)
	}

	if running != 0 {
		t.Errorf("%d jobs are still running after the shutdown", running)
	}
}
