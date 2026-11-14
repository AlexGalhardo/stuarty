// Command covergate fails when a package is below its coverage gate
// (SDD-009 section 13.2): 90% under core/, 80% elsewhere, with commands,
// internal tooling, spikes and desktop/ excluded. CI runs it on the ubuntu
// coverage profile:
//
//	go test -coverprofile=coverage.out ./...
//	go run ./internal/tools/covergate coverage.out
//
// A package that has statements but is absent from the profile fails too, so
// a package without tests cannot hide. -v prints every package, not only the
// failing ones.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, listPackages))
}

// listPackages asks the go tool for the packages of the module.
func listPackages() ([]Package, error) {
	out, err := exec.CommandContext(context.Background(), "go", "list", "-f", `{{.ImportPath}}|{{.Dir}}|{{join .GoFiles ","}}`, "./...").
		Output()
	if err != nil {
		return nil, fmt.Errorf("go list: %w", err)
	}

	var pkgs []Package

	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		parts := strings.Split(strings.TrimSpace(line), "|")
		if len(parts) != 3 {
			continue
		}

		p := Package{Path: parts[0], Dir: parts[1]}
		if parts[2] != "" {
			p.GoFiles = strings.Split(parts[2], ",")
		}

		pkgs = append(pkgs, p)
	}

	return pkgs, nil
}

func run(args []string, stdout, stderr io.Writer, list func() ([]Package, error)) int {
	fs := flag.NewFlagSet("covergate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	verbose := fs.Bool("v", false, "print every package")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: covergate [-v] <coverprofile>")
		return 2
	}

	f, err := os.Open(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "covergate: %v\n", err)
		return 1
	}
	defer f.Close()

	results, err := Parse(f)
	if err != nil {
		fmt.Fprintf(stderr, "covergate: %s: %v\n", fs.Arg(0), err)
		return 1
	}

	pkgs, err := list()
	if err != nil {
		fmt.Fprintf(stderr, "covergate: %v\n", err)
		return 1
	}

	missing, err := Missing(results, pkgs)
	if err != nil {
		fmt.Fprintf(stderr, "covergate: %v\n", err)
		return 1
	}

	failed := 0

	for _, r := range results {
		status := "ok  "
		if !r.Pass() {
			status = "FAIL"
			failed++
		}

		if *verbose || !r.Pass() {
			fmt.Fprintf(stdout, "%s %-24s %5.1f%% (gate %.0f%%)\n", status, display(r.Package), r.Percent(), r.Gate)
		}
	}

	for _, pkg := range missing {
		fmt.Fprintf(stdout, "FAIL %-24s missing from the profile (gate %.0f%%)\n", display(pkg), gateFor(pkg))
		failed++
	}

	if failed > 0 {
		fmt.Fprintf(stderr, "covergate: %d package(s) below the coverage gate (SDD-009 section 13.2)\n", failed)
		return 1
	}

	fmt.Fprintf(stdout, "covergate: %d packages meet their gate\n", len(results))

	return 0
}

func display(pkg string) string {
	if pkg == "" {
		return "(root)"
	}

	return pkg
}
