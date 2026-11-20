// Command soak drives the product through a long scripted session and checks that it stays bounded
// (SDD-008 section 13.4): the heap in use, the goroutines, the handles and the size of the database do not grow
// after a warm-up, and no job is left unfinished.
//
//	go run ./internal/tools/soak -duration 4h -report docs/perf/<date>-soak.md   # the run of the orchestrator
//	go run ./internal/tools/soak -duration 2m                                    # the smoke test
//
// It exits 1 when a bound is broken.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"
)

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("soak", flag.ContinueOnError)
	fs.SetOutput(stderr)

	duration := fs.Duration("duration", 4*time.Hour, "how long the script runs")
	sample := fs.Duration("sample", 30*time.Second, "the time between two samples")
	warmup := fs.Duration(
		"warmup",
		0,
		"the time before the baseline; a quarter of the duration, at most 10 minutes, when 0",
	)
	seed := fs.Int64("seed", 20261120, "seed of the script")
	report := fs.String("report", "", "write the report (a Markdown table over time) to this file")
	base := fs.String(
		"workdir",
		"",
		"the folder for the workspace of the run (a new folder inside it); the system temporary folder when empty",
	)

	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *warmup == 0 {
		*warmup = min(10*time.Minute, *duration/4)
	}

	dir, err := homeFor(*base)
	if err != nil {
		fmt.Fprintf(stderr, "soak: %v\n", err)

		return 1
	}

	defer func() { _ = os.RemoveAll(dir) }()

	rep, err := Run(ctx, Config{Dir: dir, Duration: *duration, Sample: *sample, Warmup: *warmup, Seed: *seed})
	if err != nil {
		fmt.Fprintf(stderr, "soak: %v\n", err)

		return 1
	}

	bad := rep.Check(DefaultBounds)
	md := rep.Markdown(bad)

	fmt.Fprint(stdout, md)

	if *report != "" {
		if err := os.WriteFile(*report, []byte(md), 0o600); err != nil {
			fmt.Fprintf(stderr, "soak: %v\n", err)

			return 1
		}
	}

	if len(bad) > 0 {
		for _, line := range bad {
			fmt.Fprintf(stderr, "soak: %s\n", line)
		}

		return 1
	}

	return 0
}
