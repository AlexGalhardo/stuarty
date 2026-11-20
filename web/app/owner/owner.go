// Package owner is the process that owns a workspace (SDD-005 section 6.3): it
// takes the lock, opens the store, builds the Service with its media, exporter
// and queue, and hands the HTTP adapter its dependencies. cmd/app runs it for
// `stuarty serve`; it holds no rule about timelines or jobs, which are
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
	"sync/atomic"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/lint"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/cache"
	"github.com/AlexGalhardo/stuarty/engine/export"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
	"github.com/AlexGalhardo/stuarty/engine/frame"
	enginestore "github.com/AlexGalhardo/stuarty/engine/store"
	"github.com/AlexGalhardo/stuarty/internal/ffmpegpins"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/agenthost"
	"github.com/AlexGalhardo/stuarty/service/config"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/exporter"
	"github.com/AlexGalhardo/stuarty/service/pathguard"
	"github.com/AlexGalhardo/stuarty/service/vault"
	"github.com/AlexGalhardo/stuarty/service/workspace"
	"github.com/AlexGalhardo/stuarty/web/app/agentdocs"
	"github.com/AlexGalhardo/stuarty/web/app/auth"
	"github.com/AlexGalhardo/stuarty/web/app/events"
	httpapi "github.com/AlexGalhardo/stuarty/web/app/http"
	"github.com/AlexGalhardo/stuarty/web/app/http/middleware"
	"github.com/AlexGalhardo/stuarty/web/app/jobs"
	"github.com/AlexGalhardo/stuarty/web/app/mcp"
	"github.com/AlexGalhardo/stuarty/web/app/store"
	"github.com/AlexGalhardo/stuarty/web/resources/skills"

	vukem "github.com/AlexGalhardo/vukem"
	"github.com/AlexGalhardo/vukem/queue"
	"github.com/AlexGalhardo/vukem/ratelimit"
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
	// Resolved is the configuration that config.Validate accepted; its zero
	// value is the mode named by STUARTY_MODE with no network facts (the
	// commands, which never serve).
	Resolved config.Resolved
	// Telemetry builds the OpenTelemetry provider; it is called only when
	// OTEL_EXPORTER_OTLP_ENDPOINT is set. Nil builds none.
	Telemetry Telemetry
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
	// runtimeSecret is the per-run secret of network mode, the key of its handshake.
	runtimeSecret string
	Events        *events.Events
	// Agents hosts the agent a user installed, for the chat panel (SDD-017); nil in a process that only
	// runs a command.
	Agents *agenthost.Host
	// mcpURL is the address of this server's own MCP endpoint, as the hosted agent reaches it; empty until the
	// listener is bound.
	mcpURL atomic.Value
	appURL string
	// agentTokens is the registry of the session tokens of the hosted agent.
	agentTokens *agenthost.Tokens
	deps        *httpapi.Deps
	lock        *workspace.Lock
	limits      service.Limits
	child       service.ChildPolicy
	// gpuRenderer is the GPU renderer request and the stuarty-render found for it.
	gpuRenderer service.GPURenderer
	version     string
	now         func() time.Time
	announce    bool
	mode        string
	network     *httpapi.Network
	netAuth     *auth.Network

	telemetry vukem.Provider
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

		// A command prints what it was asked for and nothing else (SDD-003 section 5.1).
		if cfg.Inline {
			cfg.Log = slog.New(slog.DiscardHandler)
		}
	}

	if cfg.Now == nil {
		cfg.Now = time.Now
	}

	if cfg.Resolved.Mode == "" {
		cfg.Resolved.Mode = config.ModeOf(cfg.Getenv)
	}

	if cfg.Version == "" {
		cfg.Version = agentdocs.Version
	}

	o := &Owner{version: cfg.Version, now: cfg.Now, mode: cfg.Resolved.Mode}

	if err := o.take(cfg); err != nil {
		return nil, err
	}

	o.startTelemetry(cfg)

	if err := o.build(ctx, cfg); err != nil {
		_ = o.Close()

		return nil, StartupError(err)
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
		return StartupError(err)
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

	// Step 5 of the start-up order (SDD-005 section 7): documents below the current schema.
	migrated, err := service.MigrateStoredDocuments(ctx, st, service.MigrateOptions{NowMs: cfg.Now().UnixMilli()})
	if err != nil {
		return err
	}

	if migrated != (service.MigrateResult{}) {
		cfg.Log.InfoContext(ctx, "documents migrated", "projects", migrated.Projects, "templates", migrated.Templates)
	}

	if err := o.authSetup(ctx, cfg); err != nil {
		return err
	}

	limits, err := limitsFromEnv(cfg.Getenv, o.mode == config.ModeNetwork)
	if err != nil {
		return err
	}

	o.limits = limits.WithDefaults()

	media, exp, err := o.media(ctx, cfg)
	if err != nil {
		return err
	}

	model := cfg.Getenv("STUARTY_WHISPER_MODEL")

	explicit := model != ""
	if !explicit {
		model = "small"
	}

	tools, transcriber, err := o.tools(model, explicit)
	if err != nil {
		return err
	}

	if wp, ok := transcriber.(*whisperProvider); ok {
		wp.ffmpeg = media.Runner
	}

	if transcriber, err = pickTranscriber(cfg.Getenv, o.mode, transcriber, media.Runner); err != nil {
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

	// A command has no subscribers: nothing reads its events.
	var publish func(service.Event)

	if !cfg.Inline {
		o.Events = events.New(st, events.Options{Log: cfg.Log, RedactPaths: o.mode == config.ModeNetwork})
		publish = o.Events.Publish

		// The agent a user installed, hosted in the chat panel (SDD-017). A command has no panel and
		// hosts none.
		o.appURL = cfg.Resolved.AppURL
		o.agentTokens = agenthost.NewTokens(cfg.Now)
		o.Agents = agenthost.New(agenthost.Options{
			Store: st.Chat(), Tokens: o.agentTokens, MCPURL: o.MCPURL, Workspace: string(o.Dir), Now: cfg.Now,
			Version: o.version, Log: cfg.Log,
		})
	}

	rates, err := rateLimitsFromEnv(cfg.Getenv, o.mode == config.ModeNetwork)
	if err != nil {
		return err
	}

	out, err := o.outbound(cfg.Getenv, st, cfg.Now)
	if err != nil {
		return err
	}

	heavy := rates.Heavy
	if heavy <= 0 {
		heavy = service.HeavyPerMinute
	}

	o.Service, err = service.New(service.Options{
		Keys: o.keyStore(), Connections: out.conns, Providers: out.provs, Social: out.socials, HeavyPerMinute: heavy,
		Agents: o.Agents,
		Heavy:  middleware.ServiceLimiter{L: ratelimit.NewMemory(heavy, time.Minute)},
		Store:  st, Registry: command.Default(), Now: cfg.Now, Workspace: string(o.Dir), Media: media,
		Exporter: exp, Enqueue: jobs.Dispatcher{Queue: o.Queue}, Limits: limits, Publish: publish, Version: o.version,
		BuiltInSkills: skills.FS(), Transcriber: transcriber, Tools: tools, Log: cfg.Log,
		Lint: lint.Env{Measurer: lintMeasurer{workspace: string(o.Dir)}},
		Doctor: service.DoctorOptions{
			Mode: o.mode, StartedAt: o.now().UTC(), Child: o.child, FFmpegPins: ffmpegpins.Current(),
			GPURenderer: o.gpuRenderer,
			Integrity: func(ctx context.Context) (string, error) {
				var out string
				err := st.DB().QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&out)

				return out, err
			},
		},
	})
	if err != nil {
		return err
	}

	var ok bool
	if o.Runner, ok = service.RunnerOf(o.Service); !ok {
		return errors.New("owner: the service has no job runner")
	}

	late.JobRunner = o.Runner

	// Read once and removed from the environment, so ffmpeg and the helper never inherit it. It stays
	// in memory and on no disk or log (SDD-007 section 5.3).
	launcherSecret := cfg.Getenv(auth.EnvLauncherSecret)
	if launcherSecret != "" {
		_ = os.Unsetenv(auth.EnvLauncherSecret)
	}

	o.deps = &httpapi.Deps{
		Service: o.Service, Registry: command.Default(), Workspace: o.Dir, Token: o.Token,
		RuntimeSecret: o.runtimeSecret, LauncherSecret: launcherSecret,
		Sessions: auth.NewSessions(cfg.Now), Launch: auth.NewLaunch(cfg.Now), Now: cfg.Now,
		Events: o.Events, Version: o.version, Network: o.network, Auth: o.netAuth, RateLimits: rates,
		Chat: o.chatStreams(), AgentTokens: o.agentLookup(),
	}

	if o.deps.MCP, err = mcp.Handler(o.Service, mcp.Options{Version: o.version}); err != nil {
		return err
	}

	// The library of a new workspace holds the starter templates; later starts add nothing.
	if _, serr := service.SeedStarterTemplates(ctx, st, o.now().UnixMilli()); serr != nil {
		return serr
	}

	if cfg.Inline {
		return nil
	}

	// Jobs found running belong to an owner that is gone (SDD-005 section 14.5).
	if err := o.Runner.Recover(ctx); err != nil {
		return err
	}

	// Step 8 of the start-up order: what an interrupted emptying left behind.
	if sw, ok := service.SweeperOf(o.Service); ok {
		res, serr := sw.Sweep(ctx)
		if serr != nil {
			cfg.Log.WarnContext(ctx, "orphan sweep failed", "error", serr)
		} else if res != (service.SweepResult{}) {
			cfg.Log.InfoContext(ctx, "orphan sweep", "store", res.StoreFiles, "artifacts", res.ArtifactDirs,
				"exports", res.ExportDirs, "frames", res.FrameDirs, "incoming", res.IncomingFiles)
		}
	}

	return nil
}

// authSetup is the credential side of the mode. Network mode has no local token
// and creates no token file (SDD-007 section 4); a command that is not a server
// builds no guard.
func (o *Owner) authSetup(ctx context.Context, cfg Config) error {
	if o.mode != config.ModeNetwork {
		var err error

		o.Token, err = o.Dir.EnsureToken()

		return err
	}

	if cfg.Inline {
		return nil
	}

	var err error

	if cfg.Resolved.AppURL == "" {
		return errors.New("owner: network mode runs only through `stuarty serve`, which validates the " +
			"configuration; unset STUARTY_MODE for any other command")
	}

	// Written before runtime.json: a command that finds the owner must find the secret too.
	if o.runtimeSecret, err = o.Dir.NewRuntimeSecret(); err != nil {
		return err
	}

	if o.network, err = httpapi.NewNetwork(cfg.Resolved, cfg.Log); err != nil {
		return err
	}

	o.netAuth, err = auth.NewNetwork(o.Store.DB(), auth.NetworkOptions{
		PlainHTTP: cfg.Resolved.LoopbackHTTP, Now: cfg.Now, Log: cfg.Log,
	})
	if err != nil {
		return err
	}

	return auth.BootstrapAdmin(ctx, o.netAuth.Users, cfg.Getenv, cfg.Log)
}

// media builds the media side of the Service. A missing FFmpeg or ffprobe is not a
// start failure: the editor still edits, and an import or an export answers with the
// error of the missing tool. The error is kept on Media so a job returns it instead
// of panicking on a nil runner.
func (o *Owner) media(ctx context.Context, cfg Config) (service.Media, service.Exporter, error) {
	roots := splitList(cfg.Getenv("STUARTY_IMPORT_ROOTS"))

	guard, err := pathguard.New(pathguard.Config{Roots: roots, Workspace: string(o.Dir)})
	if err != nil {
		return service.Media{}, nil, err
	}

	media := service.Media{Content: enginestore.New(string(o.Dir)), Guard: guard}

	fc := ffmpeg.Config{
		FFmpegPath: cfg.Getenv("STUARTY_FFMPEG"), FFprobePath: cfg.Getenv("STUARTY_FFPROBE"), Log: cfg.Log,
		ToolsDir: filepath.Join(string(o.Dir), "tools"),
	}
	if self, err := os.Executable(); err == nil {
		fc.ExecDir = filepath.Dir(self)
	}

	in, err := ffmpeg.Discover(ctx, fc)
	if err != nil {
		cfg.Log.WarnContext(ctx, "FFmpeg was not found: import and export are unavailable", "error", err)
		media.FFmpegMissing = err

		return media, nil, nil
	}

	child, err := newChildRunners(
		ctx, in, string(o.Dir), cfg.Getenv("STUARTY_SANDBOX"), o.limits.MaxExportBytes,
	)
	if err != nil {
		return service.Media{}, nil, err
	}

	o.child = child.Policy
	runner := child.Ingest
	media.Runner = runner

	maxCache, err := cacheLimit(cfg.Getenv, o.mode == config.ModeNetwork)
	if err != nil {
		return service.Media{}, nil, err
	}

	frames, err := cache.Open(string(o.Dir), maxCache)
	if err != nil {
		return service.Media{}, nil, err
	}

	media.Frames = &frame.Frames{
		Runner:       child.Frame,
		ArtifactsDir: filepath.Join(string(o.Dir), "artifacts"),
		Cache:        frames,
	}

	allowX264, _ := strconv.ParseBool(cfg.Getenv("STUARTY_ALLOW_USER_X264"))

	pixelRenderer, err := export.ParsePixelRenderer(cfg.Getenv("STUARTY_EXPORT_RENDERER"))
	if err != nil {
		return service.Media{}, nil, fmt.Errorf("owner: STUARTY_EXPORT_RENDERER: %w", err)
	}

	o.gpuRenderer = service.GPURenderer{Requested: pixelRenderer == export.PixelRendererGPU}
	o.gpuRenderer.Host = gpuHostForRenderer(string(pixelRenderer), fc.ExecDir)

	return media, exporter.New(exporter.Config{
		Runner: child.Export, HardwareRunner: child.ExportHardware, Workspace: string(o.Dir),
		Encoder:       ffmpeg.DetectOptions{Forced: cfg.Getenv("STUARTY_ENCODER"), AllowUserX264: allowX264},
		Lint:          lint.Env{Measurer: lintMeasurer{workspace: string(o.Dir)}},
		PixelRenderer: pixelRenderer,
		GPUHost:       o.gpuRenderer.Host,
	}), nil
}

// Deps is what the HTTP adapter needs; pass it to bootstrap.NewWith.
func (o *Owner) Deps() *httpapi.Deps { return o.deps }

// Announce writes runtime.json once the listener is bound (SDD-005 section
// 6.3 step 5): from then on a command finds this process and talks to it.
func (o *Owner) Announce(addr string) error {
	o.announce = true
	o.mcpURL.Store(mcpURLFor(addr))

	return o.Dir.WriteRuntime(workspace.Runtime{
		Schema: 1, PID: os.Getpid(), Addr: addr, Mode: o.mode, StartedAt: o.now().UTC(),
		Version: o.version,
	})
}

// Work runs the queue workers until ctx ends. Jobs still running when the
// timeout passes are interrupted; the service requeues them (SDD-005 section
// 14.5), so none is lost to a shutdown. The publication scheduler runs beside them for as long as the server
// does (SDD-020 section 27); keeping it alive with the window closed is the desktop service of epic 21.
func (o *Owner) Work(ctx context.Context) error {
	if sched, ok := service.SchedulerOf(o.Service); ok {
		done := make(chan struct{})

		go func() {
			defer close(done)

			_ = sched.Run(ctx)
		}()

		defer func() { <-done }()
	}

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

	if o.Events != nil {
		o.Events.Close() // ends the open streams at once
	}

	// No agent process is left behind: every tree is killed before the store closes (SDD-017 section 4).
	if o.Agents != nil {
		o.Agents.Shutdown()
	}

	if o.announce || o.runtimeSecret != "" {
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

func cacheLimit(getenv func(string) string, network bool) (int64, error) {
	const name = "STUARTY_CACHE_MAX_BYTES"

	raw := getenv(name)

	use, err := config.CheckLimit(name, raw, network)
	if err != nil {
		return 0, fmt.Errorf("owner: %w", err)
	}

	if !use {
		return 0, nil
	}

	v, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("owner: invalid limit: %s=%q must be a whole number", name, raw)
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

// limitsFromEnv reads the ceilings of SDD-007 section 9.1. A value that is not
// a number refuses the start rather than being ignored. Zero or a negative
// number means the default in local mode and refuses the start in network mode
// ("invalid limit", naming the variable).
func limitsFromEnv(getenv func(string) string, network bool) (service.Limits, error) {
	var l service.Limits

	for _, f := range []struct {
		name string
		set  func(int64)
	}{
		{"STUARTY_MAX_IMPORT_BYTES", func(v int64) { l.MaxImportBytes = v }},
		{"STUARTY_MIN_FREE_BYTES", func(v int64) { l.MinFreeBytes = v }},
		{"STUARTY_MAX_INGESTS", func(v int64) { l.MaxIngests = int(v) }},
		{"STUARTY_MAX_EXPORTS", func(v int64) { l.MaxExports = int(v) }},
		{"STUARTY_MAX_EXPORTS_PER_KEY", func(v int64) { l.MaxExportsPerActor = int(v) }},
		{"STUARTY_MAX_TRANSCRIBES", func(v int64) { l.MaxTranscribes = int(v) }},
		{"STUARTY_MAX_TRANSCRIBES_PER_KEY", func(v int64) { l.MaxTranscribesPerActor = int(v) }},
		{"STUARTY_MAX_TRANSCRIBE_SECONDS", func(v int64) { l.MaxTranscribeSeconds = v }},
		{"STUARTY_MAX_EXPORT_BYTES", func(v int64) { l.MaxExportBytes = v }},
		{"STUARTY_MAX_IMPORT_PIXELS", func(v int64) { l.Ceilings.MaxPixels = v }},
	} {
		raw := getenv(f.name)

		use, err := config.CheckLimit(f.name, raw, network)
		if err != nil {
			return service.Limits{}, fmt.Errorf("owner: %w", err)
		}

		if !use {
			continue
		}

		v, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil {
			return service.Limits{}, fmt.Errorf("owner: invalid limit: %s=%q must be a whole number", f.name, raw)
		}

		f.set(v)
	}

	const seconds = "STUARTY_MAX_IMPORT_SECONDS"

	raw := getenv(seconds)

	use, err := config.CheckLimit(seconds, raw, network)
	if err != nil {
		return service.Limits{}, fmt.Errorf("owner: %w", err)
	}

	if use {
		l.Ceilings.MaxSeconds, _ = strconv.ParseFloat(strings.TrimSpace(raw), 64)
	}

	return l, nil
}

// rateLimitsFromEnv reads the per-credential limits (SDD-007 section 4.4); an
// unset value, or a non-positive one in local mode, is the default.
func rateLimitsFromEnv(getenv func(string) string, network bool) (httpapi.RateLimits, error) {
	var r httpapi.RateLimits

	for _, f := range []struct {
		name string
		set  func(int)
	}{
		{"STUARTY_RATE_LIMIT_PER_MINUTE", func(v int) { r.PerCredential = v }},
		{"STUARTY_RATE_LIMIT_SESSION_PER_MINUTE", func(v int) { r.PerSession = v }},
		{"STUARTY_RATE_LIMIT_HEAVY_PER_MINUTE", func(v int) { r.Heavy = v }},
	} {
		raw := getenv(f.name)

		use, err := config.CheckLimit(f.name, raw, network)
		if err != nil {
			return httpapi.RateLimits{}, fmt.Errorf("owner: %w", err)
		}

		if !use {
			continue
		}

		v, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil {
			return httpapi.RateLimits{}, fmt.Errorf("owner: %s: %w", f.name, err)
		}

		f.set(v)
	}

	return r, nil
}

// connections builds the registry of connections over the vault of the workspace. The master key
// comes from the environment, a file or the operating system keychain (SDD-014 section 4). It is
// read once and removed from the environment, so no child process inherits it. A key that is named
// and malformed, or named twice, stops the start; a key that is merely absent does not: the vault
// is created with the first credential.
func (o *Owner) connections(
	getenv func(string) string, st service.Store, now func() time.Time,
) (*connections.Manager, error) {
	keys, err := vault.Resolve(vault.ConfigFromEnv(getenv, string(o.Dir)))
	if err != nil {
		return nil, err
	}

	if _, err := keys.Load(); err != nil && !errors.Is(err, vault.ErrKeyNotFound) {
		return nil, fmt.Errorf("owner: the master key of the vault from %s is not usable: %w", keys.Name(), err)
	}

	for _, name := range []string{vault.EnvMasterKey, vault.EnvMasterKeyFile} {
		if getenv(name) != "" {
			_ = os.Unsetenv(name)
		}
	}

	return connections.NewManager(connections.Options{
		Store: st.Connections(), Now: now,
		Vault: vault.Open(filepath.Join(string(o.Dir), connections.VaultFile), keys),
	}), nil
}

// keyStore is the API key store of network mode; local mode has none.
func (o *Owner) keyStore() service.KeyStore {
	if o.netAuth == nil {
		return nil
	}

	return o.netAuth.Keys()
}
