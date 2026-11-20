// Command agentharness runs a live agent against the recipes of the agent documents and measures
// how often it completes them (SDD-003 section 9 check 29, PRD-002 success metrics 1 and 2). The
// agent is given only web/app/agentdocs/out/SKILL.md and the CLI, and talks to the Anthropic
// Messages API over HTTPS with the key of ANTHROPIC_API_KEY.
//
// It costs money, so it never runs by itself and never in CI: it refuses to start unless
// STUARTY_HARNESS=1 is set and --runs is given, and it prints the estimated cost first. It is a
// release-checklist tool, not part of the product: no shipped binary contains it.
//
//	STUARTY_HARNESS=1 ANTHROPIC_API_KEY=... go run ./internal/tools/agentharness \
//	    --runs 10 --bin ./stuarty --clip ./clip.mp4 --out ./transcripts
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/AlexGalhardo/stuarty/internal/outbound"
)

const (
	defaultModel    = "claude-sonnet-5-5"
	defaultEndpoint = "https://api.anthropic.com/v1/messages"
	// passRate is the bar of check 29: 9 runs of 10.
	passRate = 0.9
)

func main() {
	os.Exit(realMain())
}

func realMain() int {
	ctx, stop := signalContext()
	defer stop()

	// The same HTTPS-only, allowlisted, public-address client as the install step, here for one host.
	api := outbound.HelperDownload([]string{"api.anthropic.com"}, 5*time.Minute)

	return run(ctx, os.Args[1:], os.Getenv, api, execRunner, os.Stdout, os.Stderr)
}

type options struct {
	runs                 int
	model, endpoint      string
	scenario, skill      string
	bin, clip, out       string
	maxTurns, maxTokens  int
	priceIn, priceOut    float64
	estimateOnly         bool
	backoff              time.Duration
	runnerFor            func(home, work string) Runner
	sleep                func(time.Duration)
	scenarioList         []scenario
	apiKey, systemPrompt string
	// The first-flow scenario runs against services the operator started on loopback.
	skillsDir                                  string
	flowBase, flowHome, flowSecret, flowSocial string
	flowEnv                                    *flowEnv
}

func parseOptions(argv []string, getenv func(string) string, stderr io.Writer) (options, int) {
	var o options

	fs := flag.NewFlagSet("agentharness", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.IntVar(&o.runs, "runs", 0, "runs per scenario (required)")
	fs.StringVar(&o.model, "model", defaultModel, "the model the agent runs on")
	fs.StringVar(&o.endpoint, "endpoint", defaultEndpoint, "the Messages API URL (https; tests use a loopback server)")
	fs.StringVar(&o.scenario, "scenario", "all", "prototype, template, first-flow or all (all leaves first-flow out)")
	fs.StringVar(
		&o.skillsDir,
		"skills",
		"web/resources/skills",
		"first-flow: the folder of the built-in skills given to the agent",
	)
	fs.StringVar(&o.flowBase, "flow-base", "", "first-flow: the address of the Stuarty server on loopback")
	fs.StringVar(&o.flowHome, "flow-home", "", "first-flow: the workspace of that server (STUARTY_HOME)")
	fs.StringVar(&o.flowSecret, "flow-launcher-secret", "", "first-flow: the STUARTY_LAUNCHER_SECRET of that server")
	fs.StringVar(&o.flowSocial, "flow-social", "", "first-flow: the address of the fake social network")
	fs.StringVar(&o.skill, "skill", "web/app/agentdocs/out/SKILL.md", "the SKILL.md given to the agent")
	fs.StringVar(&o.bin, "bin", "stuarty", "the stuarty binary the agent drives")
	fs.StringVar(&o.clip, "clip", "", "a short video copied to clip.mp4 for the prototype scenario")
	fs.StringVar(&o.out, "out", "agent-transcripts", "where the transcripts and summary.json are written")
	fs.IntVar(&o.maxTurns, "max-turns", 40, "model calls allowed per run")
	fs.IntVar(&o.maxTokens, "max-tokens", 4096, "output tokens allowed per model call")
	fs.Float64Var(&o.priceIn, "price-in", 3, "USD per million input tokens, for the estimate")
	fs.Float64Var(&o.priceOut, "price-out", 15, "USD per million output tokens, for the estimate")
	fs.BoolVar(&o.estimateOnly, "estimate", false, "print the estimated cost and exit without calling the API")

	if err := fs.Parse(argv); err != nil {
		return o, 2
	}

	switch {
	case getenv("STUARTY_HARNESS") != "1":
		fmt.Fprintln(stderr, "agentharness: refusing to run: it costs money; set STUARTY_HARNESS=1 to run it")

		return o, 2
	case o.runs < 1:
		fmt.Fprintln(stderr, "agentharness: refusing to run: --runs N is required (N >= 1)")

		return o, 2
	case o.maxTurns < 1 || o.maxTokens < 1:
		fmt.Fprintln(stderr, "agentharness: --max-turns and --max-tokens must be positive")

		return o, 2
	}

	var ok bool
	if o.scenarioList, ok = findScenarios(o.scenario); !ok {
		fmt.Fprintf(stderr, "agentharness: unknown scenario %q\n", o.scenario)

		return o, 2
	}

	// The estimate is shown to the author before anything is started, so it needs none of the services.
	if o.scenario == firstFlow.Name && !o.estimateOnly &&
		(o.flowBase == "" || o.flowHome == "" || o.flowSecret == "" || o.flowSocial == "") {
		fmt.Fprintln(
			stderr,
			"agentharness: first-flow needs --flow-base, --flow-home, --flow-launcher-secret and --flow-social",
		)

		return o, 2
	}

	if o.apiKey = getenv("ANTHROPIC_API_KEY"); o.apiKey == "" && !o.estimateOnly {
		fmt.Fprintln(stderr, "agentharness: ANTHROPIC_API_KEY is not set")

		return o, 2
	}

	if err := checkEndpoint(o.endpoint); err != nil {
		fmt.Fprintf(stderr, "agentharness: %v\n", err)

		return o, 2
	}

	return o, 0
}

// checkEndpoint accepts https, and plain http only to a loopback address (the tests).
func checkEndpoint(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return fmt.Errorf("the endpoint %q is not a URL", raw)
	}

	loopback := u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost" || u.Hostname() == "::1"
	if u.Scheme != "https" && (u.Scheme != "http" || !loopback) {
		return fmt.Errorf("the endpoint must be https, got %q", u.Scheme)
	}

	return nil
}

// runnerMaker builds the Runner of one run from the workspace and the work directory of that run.
type runnerMaker func(bin string) func(home, work string) Runner

func run(
	ctx context.Context, argv []string, getenv func(string) string, api Doer, mk runnerMaker,
	stdout, stderr io.Writer,
) int {
	o, code := parseOptions(argv, getenv, stderr)
	if code != 0 {
		return code
	}

	skill, err := os.ReadFile(o.skill)
	if err != nil {
		fmt.Fprintf(stderr, "agentharness: %v\n", err)

		return 1
	}

	o.systemPrompt = systemPrompt(string(skill))

	if o.scenario == firstFlow.Name {
		extra, err := builtinSkills(o.skillsDir)
		if err != nil {
			fmt.Fprintf(stderr, "agentharness: %v\n", err)

			return 1
		}

		o.systemPrompt += extra
	}

	est := estimate(o, len(o.systemPrompt))
	fmt.Fprintf(stdout, "agentharness: %d runs of %d scenario(s) on %s\n", o.runs, len(o.scenarioList), o.model)
	fmt.Fprintf(stdout, "agentharness: estimated cost up to USD %.2f (%d input and %d output tokens at worst; "+
		"prices USD %.2f and %.2f per million, set with --price-in and --price-out)\n",
		est.usd, est.input, est.output, o.priceIn, o.priceOut)

	if o.estimateOnly {
		return 0
	}

	for _, sc := range o.scenarioList {
		if sc.NeedsClip && o.clip == "" {
			fmt.Fprintf(stderr, "agentharness: scenario %s needs --clip\n", sc.Name)

			return 2
		}
	}

	if err := os.MkdirAll(o.out, 0o750); err != nil {
		fmt.Fprintf(stderr, "agentharness: %v\n", err)

		return 1
	}

	o.runnerFor = mk(o.bin)

	if o.scenario == firstFlow.Name {
		o.flowEnv = &flowEnv{
			person:  newLivePerson(o.flowBase, o.flowSecret, o.flowHome),
			network: newLiveNetwork(o.flowSocial),
			wait:    3 * time.Minute, poll: 2 * time.Second,
		}
	}

	return playAll(ctx, o, api, stdout, stderr)
}

func systemPrompt(skill string) string {
	return "You are an agent that edits video with the stuarty command line. You have one tool, " +
		toolName + ", that runs a command of the CLI; the working directory holds any file the task names. " +
		"The CLI reference is `help`; the skill below is everything else you know.\n\n" + skill
}

func playAll(ctx context.Context, o options, api Doer, stdout, stderr io.Writer) int {
	a := &agent{
		api: &client{
			http: api, url: o.endpoint, key: o.apiKey, backoff: o.backoff,
			sleep: func(d time.Duration) { sleepFor(o, d) },
		},
		model: o.model, maxTokens: o.maxTokens, maxTurns: o.maxTurns, system: o.systemPrompt,
		flow: o.flowEnv,
	}

	summary := map[string]any{"model": o.model, "runs": o.runs}
	bar := int(float64(o.runs)*passRate + 0.999999)
	failed := false

	for _, sc := range o.scenarioList {
		pass := 0

		for n := 1; n <= o.runs; n++ {
			res, err := playOnce(ctx, a, o, sc, n)
			if err != nil {
				fmt.Fprintf(stderr, "agentharness: %v\n", err)

				return 1
			}

			if res.Success {
				pass++
			}

			if err := saveTranscript(o.out, res); err != nil {
				fmt.Fprintf(stderr, "agentharness: %v\n", err)

				return 1
			}

			fmt.Fprintf(stdout, "%s run %d: success=%t turns=%d %s\n", sc.Name, n, res.Success, res.Turns, res.Reason)
		}

		fmt.Fprintf(stdout, "%s: %d of %d runs succeeded (the bar is %d)\n", sc.Name, pass, o.runs, bar)

		summary[sc.Name] = map[string]int{"success": pass, "runs": o.runs, "bar": bar}
		failed = failed || pass < bar
	}

	raw, err := json.MarshalIndent(summary, "", "  ")
	if err == nil {
		err = os.WriteFile(filepath.Join(o.out, "summary.json"), raw, 0o600)
	}

	if err != nil {
		fmt.Fprintf(stderr, "agentharness: %v\n", err)

		return 1
	}

	if failed {
		return 1
	}

	return 0
}

func sleepFor(o options, d time.Duration) {
	if o.sleep != nil {
		o.sleep(d)

		return
	}

	time.Sleep(d)
}

// playOnce prepares an empty workspace for the run and plays the scenario in it.
func playOnce(ctx context.Context, a *agent, o options, sc scenario, n int) (runResult, error) {
	root, err := os.MkdirTemp("", "agentharness-")
	if err != nil {
		return runResult{}, fmt.Errorf("make the work directory: %w", err)
	}
	defer os.RemoveAll(root)

	work, home := filepath.Join(root, "work"), filepath.Join(root, "home")
	if err := os.MkdirAll(work, 0o750); err != nil {
		return runResult{}, fmt.Errorf("make the work directory: %w", err)
	}

	if sc.NeedsClip {
		data, err := os.ReadFile(o.clip)
		if err != nil {
			return runResult{}, fmt.Errorf("read the clip: %w", err)
		}

		// work is a fresh temporary directory and the name is fixed.
		if err := os.WriteFile(filepath.Join(work, "clip.mp4"), data, 0o600); err != nil { //nolint:gosec // see above
			return runResult{}, fmt.Errorf("copy the clip: %w", err)
		}
	}

	if sc.Flow {
		// The post goes out from the server of the operator, so the run uses its workspace.
		home = o.flowHome
	}

	return a.play(ctx, sc, n, o.runnerFor(home, work))
}

func saveTranscript(dir string, r runResult) error {
	raw, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("encode the transcript: %w", err)
	}

	name := fmt.Sprintf("%s-%02d.json", r.Scenario, r.Run)

	return os.WriteFile(filepath.Join(dir, name), raw, 0o600)
}

// execRunner is the real Runner: the stuarty binary, run with no shell, in the work directory,
// on a workspace of its own so no run sees another one.
func execRunner(bin string) func(home, work string) Runner {
	return func(home, work string) Runner {
		return func(ctx context.Context, args []string, stdin string) (string, int, error) {
			ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
			defer cancel()

			// The binary is chosen by whoever runs the harness; the arguments come from the agent
			// and reach the program as an argument vector, never a shell.
			cmd := exec.CommandContext(ctx, bin, args...)
			cmd.Dir = work
			cmd.Env = append(os.Environ(), "STUARTY_HOME="+home)
			cmd.Stdin = strings.NewReader(stdin)

			out, err := cmd.CombinedOutput()
			if exit, ok := asExit(err); ok {
				return string(out), exit, nil
			}

			if err != nil {
				return "", -1, err
			}

			return string(out), 0, nil
		}
	}
}
