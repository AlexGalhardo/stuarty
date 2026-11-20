// Command perfreport runs the benchmarks that measure the non-functional requirements, writes one
// table (docs/perf/<date>.md) and compares it with the last committed table of the same platform; a
// regression of more than 20% fails (SDD-008 section 7).
//
//	go run ./internal/tools/perfreport -write          # measure, write docs/perf/<today>.md
//	go run ./internal/tools/perfreport -compare        # measure, compare with the last table, write nothing
//	go run ./internal/tools/perfreport -absolute       # also fail when a limit is missed (the reference runner)
//	go run ./internal/tools/perfreport -gated          # also run the budgets that live in tests, on the idle machine
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// Runner runs one benchmark selection and returns the output of go test.
type Runner func(ctx context.Context, pkg, bench, benchtime string) (string, error)

func goTest(ctx context.Context, pkg, bench, benchtime string) (string, error) {
	cmd := exec.CommandContext( //nolint:gosec // fixed program; the selection comes from the table in this package

		ctx,
		"go",
		"test",
		"-run",
		"^$",
		"-bench",
		"^"+bench+"$",
		"-benchtime",
		benchtime,
		"-count=1",
		pkg,
	)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr, goTest, time.Now))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer, runner Runner, now func() time.Time) int {
	fs := flag.NewFlagSet("perfreport", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("dir", filepath.Join("docs", "perf"), "folder of the committed tables")
	write := fs.Bool("write", false, "write the table to <dir>/<date>.md")
	compare := fs.Bool(
		"compare",
		false,
		"fail on a regression of more than 20% against the last table of this platform",
	)
	absolute := fs.Bool("absolute", false, "fail when a limit is missed (on the reference runner)")
	only := fs.String("only", "", "measure only the requirement with this id, for example NFR-005")
	gated := fs.Bool(
		"gated",
		false,
		"also run the absolute budgets that live in tests (STUARTY_PERF=1); one that is skipped fails",
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	specs, known := selectSpecs(*only, *gated)
	if !known {
		fmt.Fprintf(stderr, "perfreport: no requirement %q\n", *only)
		return 2
	}

	res, env, err := measure(ctx, specs, runner, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "perfreport: %v\n", err)
		return 1
	}
	if env.GOOS == "" { // only budgets in tests ran: the platform is this one
		env = Env{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH}
	}
	rep := Report{Date: now().Format("2006-01-02"), Env: env, Rows: Rows(specs, res)}
	md, err := rep.Markdown()
	if err != nil {
		fmt.Fprintf(stderr, "perfreport: %v\n", err)
		return 1
	}

	var gateRows []GateRow
	if *gated {
		gateRows = runGates(ctx, *only)
		md += gatesMarkdown(gateRows)
	}

	fmt.Fprint(stdout, md)

	failed := false
	if *write {
		if err := os.MkdirAll(*dir, 0o750); err != nil {
			fmt.Fprintf(stderr, "perfreport: %v\n", err)
			return 1
		}
		name := filepath.Join(*dir, rep.Date+"-"+env.GOOS+".md")
		if err := os.WriteFile(name, []byte(md), 0o600); err != nil {
			fmt.Fprintf(stderr, "perfreport: %v\n", err)
			return 1
		}
		fmt.Fprintf(stderr, "perfreport: wrote %s\n", name)
	}
	if *compare {
		base, name, ok := lastTable(*dir, env, rep.Date)
		if !ok {
			fmt.Fprintln(stderr, "perfreport: no table of this platform to compare with")
		}
		for _, r := range Compare(base, rep.Rows) {
			failed = true
			fmt.Fprintf(stderr, "perfreport: %s %s is %.0f%% worse than %s (%.2f, now %.2f %s)\n",
				r.Spec.NFR, r.Spec.Bench, r.Worsened*100, name, r.Before, r.Now, r.Spec.Unit)
		}
	}
	if *absolute {
		for _, row := range rep.Rows {
			if !row.Pass() {
				failed = true
				fmt.Fprintf(
					stderr,
					"perfreport: %s %s misses its limit (%s)\n",
					row.Spec.NFR,
					row.Spec.Bench,
					row.Spec.Metric,
				)
			}
		}
	}
	for _, g := range gateRows {
		if !g.Pass() {
			failed = true
			fmt.Fprintf(stderr, "perfreport: %s %s: %s\n", g.Gate.NFR, g.Test, g.Result)
		}
	}
	if failed {
		return 1
	}
	return 0
}

// selectSpecs is the benchmarks of the requirement (every one when only is empty), and whether the requirement
// is known: it has a benchmark, or, with gated, a budget in tests.
func selectSpecs(only string, gated bool) ([]Spec, bool) {
	var specs []Spec
	for i := range Specs {
		if only == "" || Specs[i].NFR == only {
			specs = append(specs, Specs[i])
		}
	}

	return specs, len(specs) > 0 || gated && hasGate(only)
}

// measure runs each distinct benchmark once and parses everything it printed.
func measure(ctx context.Context, specs []Spec, runner Runner, stderr io.Writer) (map[string]Result, Env, error) {
	type sel struct{ pkg, bench, benchtime string }
	seen := map[sel]bool{}
	var order []sel
	for i := range specs {
		k := sel{specs[i].Package, specs[i].Bench, specs[i].Benchtime}
		if !seen[k] {
			seen[k] = true
			order = append(order, k)
		}
	}

	all := map[string]Result{}
	var env Env
	for _, k := range order {
		fmt.Fprintf(stderr, "perfreport: %s %s\n", k.pkg, k.bench)
		out, err := runner(ctx, k.pkg, k.bench, k.benchtime)
		if err != nil {
			return nil, env, fmt.Errorf("%s %s: %w\n%s", k.pkg, k.bench, err, tail(out))
		}
		res, e := ParseBench(out)
		maps.Copy(all, res)
		if env.GOOS == "" {
			env = e
		}
	}
	return all, env, nil
}

func tail(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.Join(lines[max(0, len(lines)-15):], "\n")
}

// lastTable is the newest table of the folder, other than today's, measured on the same platform.
func lastTable(dir string, env Env, today string) (Values, string, bool) {
	names, _ := filepath.Glob(filepath.Join(dir, "*.md"))
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	for _, n := range names {
		if strings.HasPrefix(filepath.Base(n), today) {
			continue
		}
		b, err := os.ReadFile(n)
		if err != nil {
			continue
		}
		v, err := ParseReport(string(b))
		if err != nil || v.Env.GOOS != env.GOOS || v.Env.GOARCH != env.GOARCH {
			continue
		}
		return v, filepath.Base(n), true
	}
	return Values{}, "", false
}
