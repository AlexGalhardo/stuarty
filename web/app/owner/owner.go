// Package owner is the process that owns a workspace (SDD-005 section 6.3): it
// takes the lock, opens the store, builds the Service with its media, exporter
// and queue, and hands the HTTP adapter its dependencies. cmd/app runs it for
// `vukem-video serve`; it holds no rule about timelines or jobs, which are
// service/.
package owner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/AlexGalhardo/vukem/queue"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/core/timeline"
	"github.com/AlexGalhardo/vukem-video/engine/cache"
	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg"
	"github.com/AlexGalhardo/vukem-video/engine/frame"
	enginestore "github.com/AlexGalhardo/vukem-video/engine/store"
	"github.com/AlexGalhardo/vukem-video/service"
	"github.com/AlexGalhardo/vukem-video/service/exporter"
	"github.com/AlexGalhardo/vukem-video/service/pathguard"
	"github.com/AlexGalhardo/vukem-video/service/workspace"
	"github.com/AlexGalhardo/vukem-video/web/app/auth"
	httpapi "github.com/AlexGalhardo/vukem-video/web/app/http"
	"github.com/AlexGalhardo/vukem-video/web/app/jobs"
	"github.com/AlexGalhardo/vukem-video/web/app/mcp"
	"github.com/AlexGalhardo/vukem-video/web/app/store"
)

// Config is how the owner is started.
type Config struct {
	// Getenv defaults to os.Getenv.
	Getenv func(string) string
	// Version is written to runtime.json.
	Version string
	// Log receives start-up warnings; slog.Default when nil.
	Log *slog.Logger
	// Now defaults to time.Now.
	Now func() time.Time
	// Session is a workspace that workspace.Open already locked for this
	// process (a command that became the owner); the owner then uses its
	// directory and lock instead of taking them, and Close releases the lock.
	Session *workspace.Session
	// Inline runs the jobs inside the call that starts them (queue.Sync) and
	// starts no start-up recovery: a command is not a server, and the next
	// serve recovers what a dead owner left running.
	Inline bool
}

// ShutdownTimeout is how long the workers wait for running jobs before they
// are interrupted and requeued (SDD-005 section 6.3 step 6).
const ShutdownTimeout = 15 * time.Second

// Owner holds a workspace.
type Owner struct {
	Dir     workspace.Dir
	Store   *store.Store
	Service service.Service
	Runner  service.JobRunner
	Queue   *queue.Queue
	Token   string

	deps     *httpapi.Deps
	lock     *workspace.Lock
	limits   service.Limits
	version  string
	now      func() time.Time
	announce bool
}

// lateRunner lets the queue handlers exist before the Service that they call.
type lateRunner struct{ service.JobRunner }

// Open takes the workspace (steps 1 to 4 of SDD-005 section 6.3) and builds the
// Service. It fails with E_WORKSPACE_LOCKED when another process owns it.
func Open(ctx context.Context, cfg Config) (*Owner, error) {
	if cfg.Getenv == nil {
		cfg.Getenv = os.Getenv
	}

	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}

	if cfg.Now == nil {
		cfg.Now = time.Now
	}

	o := &Owner{version: cfg.Version, now: cfg.Now}

	if err := o.take(cfg); err != nil {
		return nil, err
	}

	if err := o.build(ctx, cfg); err != nil {
		_ = o.Close()

		return nil, err
	}

	return o, nil
}

// take is steps 1 and 2 of SDD-005 section 6.3, or the session that a command
// already holds.
func (o *Owner) take(cfg Config) error {
	if s := cfg.Session; s != nil {
		o.Dir, o.lock = s.Dir, s.Lock

		return o.Dir.EnsureLayout()
	}

	dir, err := workspace.Resolve(cfg.Getenv)
	if err != nil {
		return err
	}

	if err := dir.EnsureLayout(); err != nil {
		return err
	}

	lock, err := dir.TryLock()
	if errors.Is(err, workspace.ErrLocked) {
		return dir.LockedError()
	}

	if err != nil {
		return err
	}

	o.Dir, o.lock = dir, lock

	return nil
}

func (o *Owner) build(ctx context.Context, cfg Config) error {
	if err := o.Dir.RemoveRuntime(); err != nil {
		return err
	}

	st, err := store.Open(ctx, o.Dir, store.Options{SchemaVersion: timeline.CurrentSchemaVersion})
	if err != nil {
		return err
	}

	o.Store = st

	if o.Token, err = o.Dir.EnsureToken(); err != nil {
		return err
	}

	limits, err := limitsFromEnv(cfg.Getenv)
	if err != nil {
		return err
	}

	o.limits = limits.WithDefaults()

	media, exp, err := o.media(ctx, cfg)
	if err != nil {
		return err
	}

	var driver queue.Driver = queue.Sync{}

	if !cfg.Inline {
		if driver, err = queue.NewDatabase(st.DB(), store.QueueOptions); err != nil {
			return err
		}
	}

	late := &lateRunner{}

	if o.Queue, err = jobs.NewQueue(driver, late); err != nil {
		return err
	}

	o.Service, err = service.New(service.Options{
		Store: st, Registry: command.Default(), Now: cfg.Now, Workspace: string(o.Dir), Media: media,
		Exporter: exp, Enqueue: jobs.Dispatcher{Queue: o.Queue}, Limits: limits,
	})
	if err != nil {
		return err
	}

	var ok bool
	if o.Runner, ok = service.RunnerOf(o.Service); !ok {
		return errors.New("owner: the service has no job runner")
	}

	late.JobRunner = o.Runner

	o.deps = &httpapi.Deps{
		Service: o.Service, Registry: command.Default(), Jobs: st.Jobs(), Workspace: o.Dir, Token: o.Token,
		Sessions: auth.NewSessions(cfg.Now), Launch: auth.NewLaunch(cfg.Now), Now: cfg.Now,
	}

	if o.deps.MCP, err = mcp.Handler(o.Service, mcp.Options{Version: o.version}); err != nil {
		return err
	}

	if cfg.Inline {
		return nil
	}

	// Jobs found running belong to an owner that is gone (SDD-005 section 14.5).
	return o.Runner.Recover(ctx)
}

// media builds the media side of the Service. A missing FFmpeg is not a start
// failure: the editor still edits, and an import or an export answers with the
// error of the missing tool.
func (o *Owner) media(ctx context.Context, cfg Config) (service.Media, service.Exporter, error) {
	roots := splitList(cfg.Getenv("VUKEM_VIDEO_IMPORT_ROOTS"))

	guard, err := pathguard.New(pathguard.Config{Roots: roots, Workspace: string(o.Dir)})
	if err != nil {
		return service.Media{}, nil, err
	}

	media := service.Media{Content: enginestore.New(string(o.Dir)), Guard: guard}

	fc := ffmpeg.Config{
		FFmpegPath: cfg.Getenv("VUKEM_VIDEO_FFMPEG"), FFprobePath: cfg.Getenv("VUKEM_VIDEO_FFPROBE"), Log: cfg.Log,
	}
	if self, err := os.Executable(); err == nil {
		fc.ExecDir = filepath.Dir(self)
	}

	in, err := ffmpeg.Discover(ctx, fc)
	if err != nil {
		if fc.FFmpegPath != "" {
			return service.Media{}, nil, fmt.Errorf("owner: %w", err)
		}

		cfg.Log.Warn("FFmpeg was not found: import and export are unavailable", "error", err)

		return media, nil, nil
	}

	runner, err := ffmpeg.NewRunner(in, string(o.Dir))
	if err != nil {
		return service.Media{}, nil, err
	}

	media.Runner = runner

	maxCache, err := cacheLimit(cfg.Getenv)
	if err != nil {
		return service.Media{}, nil, err
	}

	frames, err := cache.Open(string(o.Dir), maxCache)
	if err != nil {
		return service.Media{}, nil, err
	}

	media.Frames = &frame.Frames{Runner: runner, ArtifactsDir: filepath.Join(string(o.Dir), "artifacts"), Cache: frames}

	allowX264, _ := strconv.ParseBool(cfg.Getenv("VUKEM_VIDEO_ALLOW_USER_X264"))

	return media, exporter.New(exporter.Config{
		Runner: runner, Workspace: string(o.Dir),
		Encoder: ffmpeg.DetectOptions{Forced: cfg.Getenv("VUKEM_VIDEO_ENCODER"), AllowUserX264: allowX264},
	}), nil
}

// Deps is what the HTTP adapter needs; pass it to bootstrap.NewWith.
func (o *Owner) Deps() *httpapi.Deps { return o.deps }

// Announce writes runtime.json once the listener is bound (SDD-005 section
// 6.3 step 5): from then on a command finds this process and talks to it.
func (o *Owner) Announce(addr string) error {
	o.announce = true

	return o.Dir.WriteRuntime(workspace.Runtime{
		Schema: 1, PID: os.Getpid(), Addr: addr, Mode: workspace.ModeLocal, StartedAt: o.now().UTC(),
		Version: o.version,
	})
}

// Work runs the queue workers until ctx ends. Jobs still running when the
// timeout passes are interrupted; the service requeues them (SDD-005 section
// 14.5), so none is lost to a shutdown.
func (o *Owner) Work(ctx context.Context) error {
	err := o.Queue.Work(ctx, queue.WorkOptions{
		Queues:      jobs.Queues,
		Concurrency: o.limits.MaxIngests + o.limits.MaxExports + 2,
		Limits:      map[string]int{jobs.QueueIngest: o.limits.MaxIngests, jobs.QueueExport: o.limits.MaxExports},
		Sleep:       100 * time.Millisecond,
		StopTimeout: ShutdownTimeout,
	})
	if errors.Is(err, context.Canceled) {
		return nil
	}

	return err
}

// Close is the last step of a graceful shutdown: the runtime file, the
// database (checkpointed by its close) and the lock, in that order.
func (o *Owner) Close() error {
	var errs []error

	if o.announce {
		errs = append(errs, o.Dir.RemoveRuntime())
	}

	if o.Store != nil {
		errs = append(errs, o.Store.Close())
	}

	if o.lock != nil {
		errs = append(errs, o.lock.Release())
	}

	return errors.Join(errs...)
}

func cacheLimit(getenv func(string) string) (int64, error) {
	raw := getenv("VUKEM_VIDEO_CACHE_MAX_BYTES")
	if raw == "" {
		return 0, nil
	}

	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || v < 0 {
		return 0, errors.New("owner: VUKEM_VIDEO_CACHE_MAX_BYTES must be a whole number of 0 or more")
	}

	return v, nil
}

func splitList(s string) []string {
	var out []string

	for p := range strings.SplitSeq(s, string(os.PathListSeparator)) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}

	return out
}

// limitsFromEnv reads the ceilings of SDD-007 section 9.1; a value that is
// not a positive number refuses the start rather than being ignored.
func limitsFromEnv(getenv func(string) string) (service.Limits, error) {
	var l service.Limits

	for _, f := range []struct {
		name string
		set  func(int64)
	}{
		{"VUKEM_VIDEO_MAX_IMPORT_BYTES", func(v int64) { l.MaxImportBytes = v }},
		{"VUKEM_VIDEO_MIN_FREE_BYTES", func(v int64) { l.MinFreeBytes = v }},
		{"VUKEM_VIDEO_MAX_INGESTS", func(v int64) { l.MaxIngests = int(v) }},
		{"VUKEM_VIDEO_MAX_EXPORTS", func(v int64) { l.MaxExports = int(v) }},
		{"VUKEM_VIDEO_MAX_EXPORTS_PER_KEY", func(v int64) { l.MaxExportsPerActor = int(v) }},
		{"VUKEM_VIDEO_MAX_IMPORT_PIXELS", func(v int64) { l.Ceilings.MaxPixels = v }},
	} {
		raw := getenv(f.name)
		if raw == "" {
			continue
		}

		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || v < 0 {
			return service.Limits{}, fmt.Errorf("owner: %s must be a whole number of 0 or more", f.name)
		}

		f.set(v)
	}

	if raw := getenv("VUKEM_VIDEO_MAX_IMPORT_SECONDS"); raw != "" {
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil || v < 0 {
			return service.Limits{}, errors.New("owner: VUKEM_VIDEO_MAX_IMPORT_SECONDS must be a number of 0 or more")
		}

		l.Ceilings.MaxSeconds = v
	}

	return l, nil
}
