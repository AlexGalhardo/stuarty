package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/workspace"
	"github.com/AlexGalhardo/stuarty/web/app/store"
)

// Config is one soak run.
type Config struct {
	Dir      string        // the workspace; a fresh folder
	Duration time.Duration // how long the script runs
	Sample   time.Duration // the time between two samples
	Warmup   time.Duration // the time before the baseline sample
	Seed     int64
}

// Sample is what one reading of the process and the workspace says.
type Sample struct {
	At         time.Duration `json:"at"`
	Commits    int           `json:"commits"`
	HeapInuse  uint64        `json:"heap_inuse"`
	Goroutines int           `json:"goroutines"`
	Handles    int           `json:"handles"`
	DBBytes    int64         `json:"db_bytes"`
	Jobs       int           `json:"unfinished_jobs"`
}

// Report is the whole run.
type Report struct {
	Config   Config   `json:"-"`
	Commits  int      `json:"commits"`
	Refused  int      `json:"refused"`
	Samples  []Sample `json:"samples"`
	Baseline int      `json:"baseline_sample"`
}

// Bounds are the pass rules of SDD-008 section 13.4.
type Bounds struct {
	HeapGrowth         float64 // the heap at the end over the heap at the baseline, at most
	Goroutines         int     // goroutines over the baseline, at most
	Handles            int     // handles over the baseline, at most
	DBBytesPerCommit   int64   // growth of the database after the baseline, per commit, at most
	UnfinishedAtTheEnd int     // jobs still queued or running at the end, at most
}

// DefaultBounds are the limits of SDD-008 section 13.4.
var DefaultBounds = Bounds{HeapGrowth: 1.25, Goroutines: 10, Handles: 10, DBBytesPerCommit: 2048}

// Check judges a report against the bounds and lists what failed.
func (r Report) Check(b Bounds) []string {
	if len(r.Samples) < 2 {
		return []string{"fewer than two samples: the run was too short to judge"}
	}

	base, last := r.Samples[r.Baseline], r.Samples[len(r.Samples)-1]

	var bad []string

	if base.HeapInuse > 0 && float64(last.HeapInuse) > float64(base.HeapInuse)*b.HeapGrowth {
		bad = append(bad, fmt.Sprintf("heap in use grew from %d to %d bytes (more than %.0f%%)",
			base.HeapInuse, last.HeapInuse, (b.HeapGrowth-1)*100))
	}

	if last.Goroutines > base.Goroutines+b.Goroutines {
		bad = append(bad, fmt.Sprintf("goroutines grew from %d to %d", base.Goroutines, last.Goroutines))
	}

	if last.Handles > base.Handles+b.Handles {
		bad = append(bad, fmt.Sprintf("handles grew from %d to %d", base.Handles, last.Handles))
	}

	if commits := last.Commits - base.Commits; commits > 0 {
		if per := (last.DBBytes - base.DBBytes) / int64(commits); per > b.DBBytesPerCommit {
			bad = append(bad, fmt.Sprintf("the database grew %d bytes per commit after the baseline, more than %d",
				per, b.DBBytesPerCommit))
		}
	}

	if last.Jobs > b.UnfinishedAtTheEnd {
		bad = append(bad, fmt.Sprintf("%d jobs are still queued or running at the end", last.Jobs))
	}

	return bad
}

// Markdown is the report as a table over time, for docs/perf.
func (r Report) Markdown(bad []string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# Soak %s\n\n", time.Now().UTC().Format("2006-01-02"))
	fmt.Fprintf(&b, "%d commits, %d refused by the rules, run %s, sample every %s, baseline after %s, seed %d.\n\n",
		r.Commits, r.Refused, r.Config.Duration, r.Config.Sample, r.Config.Warmup, r.Config.Seed)
	b.WriteString("| At | Commits | Heap in use (MB) | Goroutines | Handles | Database (KB) | Unfinished jobs |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- | --- |\n")

	for _, s := range r.Samples {
		fmt.Fprintf(
			&b,
			"| %s | %d | %.1f | %d | %d | %d | %d |\n",
			s.At.Round(
				time.Second,
			),
			s.Commits,
			float64(s.HeapInuse)/(1<<20),
			s.Goroutines,
			s.Handles,
			s.DBBytes>>10,
			s.Jobs,
		)
	}

	if len(bad) == 0 {
		b.WriteString("\nResult: pass.\n")
	} else {
		b.WriteString("\nResult: fail.\n\n")

		for _, line := range bad {
			fmt.Fprintf(&b, "- %s\n", line)
		}
	}

	if raw, err := json.Marshal(r); err == nil {
		fmt.Fprintf(&b, "\n<!-- soak: %s -->\n", raw)
	}

	return b.String()
}

// driver is the scripted user: it edits video projects and image documents through the Service, one step
// per call, from a seeded generator.
type driver struct {
	svc     service.Service
	rng     *rand.Rand
	ids     atomic.Int64
	commits int
	refused int

	projects []*videoProject
	images   []*imageDoc
}

type videoProject struct {
	id       string
	revision int64
	track    string
	items    []string
}

type imageDoc struct {
	id       string
	revision int64
}

func (d *driver) newID(prefix string) string {
	return fmt.Sprintf("%s_%010d", prefix, d.ids.Add(1))
}

func caller() context.Context {
	return service.WithCaller(context.Background(), service.Caller{Actor: "local:soak", Scopes: service.AllScopes})
}

func op(name, args string) command.Operation {
	return command.Operation{Op: name, Args: json.RawMessage(args)}
}

// seed makes the projects and the documents the script works on.
func (d *driver) seed(ctx context.Context) error {
	for i := range 3 {
		p, err := d.svc.ProjectCreate(ctx, service.ProjectCreateRequest{Name: fmt.Sprintf("soak video %d", i)})
		if err != nil {
			return err
		}

		res, err := d.svc.Edit(ctx, service.EditRequest{
			ProjectID: p.ProjectID, BaseRevision: p.Revision,
			Operations: []command.Operation{op("track.add", `{"kind":"visual","name":"Captions"}`)},
		})
		if err != nil {
			return err
		}

		d.commits++
		d.projects = append(d.projects, &videoProject{id: p.ProjectID, revision: res.Revision, track: res.Created[0]})
	}

	for i := range 2 {
		img, err := d.svc.ImageCreate(
			ctx,
			service.ImageCreateRequest{Name: fmt.Sprintf("soak image %d", i), Preset: "video_thumbnail"},
		)
		if err != nil {
			return err
		}

		d.images = append(d.images, &imageDoc{id: img.ImageID, revision: img.Revision})
	}

	return nil
}

// step runs one action. A refusal by the rules (E_VALIDATION, E_CONFLICT and the like) is part of a hostile
// script and is counted; an internal error is a defect and ends the run.
func (d *driver) step(ctx context.Context) error {
	var err error

	switch n := d.rng.Intn(10); {
	case n < 4:
		err = d.videoAdd(ctx, d.projects[d.rng.Intn(len(d.projects))])
	case n < 6:
		err = d.videoDelete(ctx, d.projects[d.rng.Intn(len(d.projects))])
	case n < 7:
		err = d.videoUndo(ctx, d.projects[d.rng.Intn(len(d.projects))])
	case n < 9:
		err = d.imageStep(ctx, d.images[d.rng.Intn(len(d.images))])
	default:
		_, err = d.svc.ProjectGet(ctx, service.ProjectGetRequest{ProjectID: d.projects[d.rng.Intn(len(d.projects))].id})
	}

	if err == nil {
		return nil
	}

	if e, ok := errors.AsType[*service.Error](err); ok && e.Code != command.CodeInternal {
		d.refused++

		return nil
	}

	return err
}

func (d *driver) videoAdd(ctx context.Context, p *videoProject) error {
	args := fmt.Sprintf(`{"track_id":%q,"start":"%ds","duration":"1s","text":"line %d"}`,
		p.track, d.rng.Intn(120), d.ids.Add(1))

	res, err := d.svc.Edit(ctx, service.EditRequest{
		ProjectID: p.id, BaseRevision: p.revision, Operations: []command.Operation{op("text.add", args)},
	})
	if err != nil {
		return d.resync(ctx, p, err)
	}

	d.commits++
	p.revision = res.Revision
	p.items = append(p.items, res.Created...)

	return nil
}

func (d *driver) videoDelete(ctx context.Context, p *videoProject) error {
	if len(p.items) < 40 {
		return nil
	}

	id := p.items[0]
	p.items = p.items[1:]

	res, err := d.svc.Edit(ctx, service.EditRequest{
		ProjectID: p.id, BaseRevision: p.revision,
		Operations: []command.Operation{op("item.delete", fmt.Sprintf(`{"item_ids":[%q]}`, id))},
	})
	if err != nil {
		return d.resync(ctx, p, err)
	}

	d.commits++
	p.revision = res.Revision

	return nil
}

func (d *driver) videoUndo(ctx context.Context, p *videoProject) error {
	if _, err := d.svc.Undo(ctx, service.UndoRequest{ProjectID: p.id}); err != nil {
		return d.resync(ctx, p, err)
	}

	d.commits++

	if _, err := d.svc.Redo(ctx, service.UndoRequest{ProjectID: p.id}); err != nil {
		return d.resync(ctx, p, err)
	}

	d.commits++

	return d.resync(ctx, p, nil)
}

// resync reads the revision and the items back after a step that the rules refused or that moved the head.
func (d *driver) resync(ctx context.Context, p *videoProject, cause error) error {
	got, err := d.svc.ProjectGet(ctx, service.ProjectGetRequest{ProjectID: p.id, Detail: "document"})
	if err != nil {
		return err
	}

	p.revision = got.Revision

	p.items = p.items[:0]
	if got.Document != nil {
		for id := range got.Document.Items {
			if got.Document.Items[id].Type == timeline.ItemText {
				p.items = append(p.items, id)
			}
		}
	}

	return cause
}

func (d *driver) imageStep(ctx context.Context, im *imageDoc) error {
	res, err := d.svc.ImageEdit(ctx, service.ImageEditRequest{
		ImageID: im.id, BaseRevision: im.revision,
		Operations: []command.Operation{
			op("layer.add_shape", `{"shape":"rounded_rect","w":600,"h":200,"radius":24,"fill":"#E63946"}`),
		},
	})
	if err != nil {
		return err
	}

	d.commits++
	im.revision = res.Revision

	undone, err := d.svc.ImageUndo(ctx, service.ImageStepRequest{ImageID: im.id})
	if err != nil {
		return err
	}

	d.commits++
	im.revision = undone.Revision

	return nil
}

// unfinished counts the jobs of every project that are not final.
func (d *driver) unfinished(ctx context.Context) int {
	n := 0

	for _, p := range d.projects {
		list, err := d.svc.JobList(ctx, service.JobListRequest{ProjectID: p.id})
		if err != nil {
			continue
		}

		for i := range list.Jobs {
			if !service.JobFinal(list.Jobs[i].State) {
				n++
			}
		}
	}

	return n
}

// dbBytes is the size of the database files of the workspace: the database, its log and its shared memory.
func dbBytes(dir string) int64 {
	var total int64

	_ = filepath.WalkDir(dir, func(_ string, e fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // a file that goes away while it is counted is not an error of the count
		}

		if e.IsDir() {
			return nil
		}

		name := e.Name()
		if strings.HasSuffix(name, ".db") || strings.HasSuffix(name, ".db-wal") || strings.HasSuffix(name, ".db-shm") ||
			strings.HasSuffix(name, ".sqlite") || strings.HasSuffix(name, ".sqlite-wal") {
			if info, ierr := e.Info(); ierr == nil {
				total += info.Size()
			}
		}

		return nil
	})

	return total
}

// Run drives the script until the duration ends, taking a sample every cfg.Sample, and returns the report.
func Run(ctx context.Context, cfg Config) (Report, error) {
	if cfg.Sample <= 0 {
		cfg.Sample = 30 * time.Second
	}

	dir := workspace.Dir(cfg.Dir)

	st, err := store.Open(ctx, dir, store.Options{SchemaVersion: timeline.CurrentSchemaVersion})
	if err != nil {
		return Report{}, err
	}

	defer func() { _ = st.Close() }()

	d := &driver{rng: rand.New(rand.NewSource(cfg.Seed))} //nolint:gosec // a seeded script, not a secret

	d.svc, err = service.New(service.Options{
		Store: st, Registry: command.Default(), Now: time.Now, NewIDs: d.newID,
	})
	if err != nil {
		return Report{}, err
	}

	c := caller()
	if err := d.seed(c); err != nil {
		return Report{}, fmt.Errorf("seed: %w", err)
	}

	rep := Report{Config: cfg, Baseline: -1}
	start := time.Now()
	next := cfg.Sample

	take := func() {
		runtime.GC()

		var ms runtime.MemStats

		runtime.ReadMemStats(&ms)

		at := time.Since(start)
		rep.Samples = append(rep.Samples, Sample{
			At: at, Commits: d.commits, HeapInuse: ms.HeapInuse, Goroutines: runtime.NumGoroutine(),
			Handles: handleCount(), DBBytes: dbBytes(cfg.Dir), Jobs: d.unfinished(c),
		})

		if rep.Baseline < 0 && at >= cfg.Warmup {
			rep.Baseline = len(rep.Samples) - 1
		}
	}

	for time.Since(start) < cfg.Duration {
		if err := ctx.Err(); err != nil {
			return rep, err
		}

		if err := d.step(c); err != nil {
			return rep, fmt.Errorf("step after %d commits: %w", d.commits, err)
		}

		if time.Since(start) >= next {
			take()

			next += cfg.Sample
		}
	}

	take()

	if rep.Baseline < 0 {
		rep.Baseline = 0
	}

	rep.Commits, rep.Refused = d.commits, d.refused

	return rep, nil
}

// homeFor makes a fresh workspace folder under base.
func homeFor(base string) (string, error) {
	return os.MkdirTemp(base, "soak-")
}
