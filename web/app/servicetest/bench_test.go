package servicetest

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/service"
)

const (
	// nfr001P95 is the budget of a command round trip in process (NFR-001).
	nfr001P95 = 50 * time.Millisecond
	// nfr001StoreP95 is the share of it that the store may take (SDD-005 check 12).
	nfr001StoreP95 = 25 * time.Millisecond
)

// storeClock measures the store's share of one commit: what WriteProject spends
// outside its callback (lock, BEGIN IMMEDIATE, load, COMMIT) plus the repository
// calls the callback makes. The rest of the round trip is the core.
type storeClock struct {
	service.Store
	last time.Duration // the store time of the latest WriteProject, read by the benchmark after it returns
}

type clockedTx struct {
	service.ProjectTx
	spent *time.Duration
}

func (t clockedTx) Operations() service.OperationRepo {
	return clockedOps{t.ProjectTx.Operations(), t.spent}
}

func (t clockedTx) Projects() service.ProjectRepo {
	return clockedProjects{t.ProjectTx.Projects(), t.spent}
}

type clockedOps struct {
	service.OperationRepo
	spent *time.Duration
}

func (o clockedOps) Append(ctx context.Context, row service.OperationRow) error {
	defer func(t0 time.Time) { *o.spent += time.Since(t0) }(time.Now())

	return o.OperationRepo.Append(ctx, row)
}

func (o clockedOps) Since(
	ctx context.Context,
	project string,
	after int64,
	f service.OperationFilter,
) ([]service.OperationRow, error) {
	defer func(t0 time.Time) { *o.spent += time.Since(t0) }(time.Now())

	return o.OperationRepo.Since(ctx, project, after, f)
}

func (o clockedOps) CloseOpenRedo(ctx context.Context, project, actor string, by int64) error {
	defer func(t0 time.Time) { *o.spent += time.Since(t0) }(time.Now())

	return o.OperationRepo.CloseOpenRedo(ctx, project, actor, by)
}

type clockedProjects struct {
	service.ProjectRepo
	spent *time.Duration
}

func (p clockedProjects) Commit(ctx context.Context, id string, base int64, u service.ProjectUpdate) error {
	defer func(t0 time.Time) { *p.spent += time.Since(t0) }(time.Now())

	return p.ProjectRepo.Commit(ctx, id, base, u)
}

func (c *storeClock) WriteProject(
	ctx context.Context,
	id string,
	fn func(ctx context.Context, tx service.ProjectTx) error,
) error {
	var inFn, repos time.Duration

	t0 := time.Now()
	err := c.Store.WriteProject(ctx, id, func(ctx context.Context, tx service.ProjectTx) error {
		f0 := time.Now()
		defer func() { inFn = time.Since(f0) }()

		return fn(ctx, clockedTx{tx, &repos})
	})
	c.last = time.Since(t0) - inFn + repos

	return err
}

// BenchmarkCommit200Items is SDD-005 section 18 check 12: a project of about
// 200 items and 2,000 caption words, one item.move per iteration through
// Service.Edit in process. It reports the p95 and fails above the budget of
// the whole round trip (50 ms, NFR-001; the store's share is under 25 ms).
func BenchmarkCommit200Items(b *testing.B) { benchCommit200Items(b, 0) }

// BenchmarkCommit200ItemsLog100k is the same commit on a log of 100,000 rows (P4-SRV-002).
func BenchmarkCommit200ItemsLog100k(b *testing.B) { benchCommit200Items(b, longLogRows) }

func benchCommit200Items(b *testing.B, logRows int) {
	b.Helper()

	e := newEnv(b)
	clock := &storeClock{Store: e.store}

	var err error

	e.svc, err = service.New(
		service.Options{Store: clock, Registry: command.Default(), Now: e.clock.now, NewIDs: e.newID},
	)
	if err != nil {
		b.Fatal(err)
	}

	command.Default().Debug = false // the debug validation of every result is a test aid, not the product's cost

	project, textTrack := e.seed200(b)
	moved := "itm_00000000aa"

	e.mustEditB(
		b,
		project,
		op(
			"text.add",
			fmt.Sprintf(`{"track_id":%q,"start":"500s","duration":"1s","text":"mover","id":%q}`, textTrack, moved),
		),
	)

	if logRows > 0 {
		e.padLog(b, project, logRows)
	}

	rev := e.headB(b, project)
	times := make([]time.Duration, 0, b.N)
	storeTimes := make([]time.Duration, 0, b.N)

	b.ResetTimer()

	for i := range b.N {
		// The item starts at 500s: the first move goes to 502s, because a move to where the
		// item already is applies nothing and the revision would not advance.
		start := fmt.Sprintf("%ds", 500+((i+1)%2)*2)

		t0 := time.Now()
		res, err := e.edit(ui, project, rev, op("item.move", fmt.Sprintf(`{"item_id":%q,"start":%q}`, moved, start)))
		times = append(times, time.Since(t0))
		storeTimes = append(storeTimes, clock.last)

		if err != nil {
			b.Fatal(err)
		}

		rev = res.Revision
	}

	b.StopTimer()

	slices.Sort(times)
	slices.Sort(storeTimes)

	storeP95 := storeTimes[min(len(storeTimes)-1, len(storeTimes)*95/100)]
	b.ReportMetric(float64(storeP95.Microseconds())/1000, "store-p95-ms")

	p95 := times[min(len(times)-1, len(times)*95/100)]
	b.ReportMetric(float64(p95.Microseconds())/1000, "p95-ms")
	b.ReportMetric(float64(times[len(times)/2].Microseconds())/1000, "p50-ms")
	b.ReportMetric(float64(times[0].Microseconds())/1000, "min-ms")
	b.Logf("n=%d min %s p50 %s p95 %s max %s; store share p50 %s p95 %s",
		len(times), times[0], times[len(times)/2], p95, times[len(times)-1], storeTimes[len(storeTimes)/2], storeP95)

	// b.N is 1 on the first probe run, where the p95 is a single cold sample.
	if b.N >= 100 && p95 > nfr001P95 {
		b.Errorf("p95 of a commit round trip is %s, over the %s of NFR-001", p95, nfr001P95)
	}

	if b.N >= 100 && storeP95 > nfr001StoreP95 {
		b.Errorf("p95 of the store share is %s, over the %s of check 12", storeP95, nfr001StoreP95)
	}
}

func (e *env) mustEditB(b *testing.B, project string, ops ...command.Operation) {
	b.Helper()

	if _, err := e.edit(ui, project, e.headB(b, project), ops...); err != nil {
		b.Fatal(err)
	}
}

func (e *env) headB(b *testing.B, project string) int64 {
	b.Helper()

	return e.head(project).Revision
}

// seed200 builds a project of 166 text items and 34 captions of 60 words
// (2,040 words), in batches of at most 200 operations.
func (e *env) seed200(b *testing.B) (project, textTrack string) {
	b.Helper()

	project = e.newProject("bench")
	res, err := e.edit(
		ui,
		project,
		0,
		op("track.add", `{"kind":"visual","name":"T"}`),
		op("track.add", `{"kind":"visual","name":"C"}`),
	)
	if err != nil {
		b.Fatal(err)
	}

	textTrack, capTrack := res.Created[0], res.Created[1]

	words := strings.TrimSpace(strings.Repeat("word ", 60))

	var ops []command.Operation

	for i := range 166 {
		ops = append(
			ops,
			op(
				"text.add",
				fmt.Sprintf(`{"track_id":%q,"start":"%ds","duration":"1s","text":"t%d"}`, textTrack, i*2, i),
			),
		)
	}

	for i := range 34 {
		ops = append(
			ops,
			op(
				"caption.add",
				fmt.Sprintf(`{"track_id":%q,"start":"%ds","duration":"3s","text":%q}`, capTrack, i*4, words),
			),
		)
	}

	e.mustEditB(b, project, ops[:100]...)
	e.mustEditB(b, project, ops[100:]...)

	return project, textTrack
}
