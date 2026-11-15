// Command notices generates THIRD_PARTY_NOTICES.md and the licence texts under
// licenses/ (SDD-009 section 12.2). It uses only the standard library.
//
//	go run ./internal/tools/notices          write the files
//	go run ./internal/tools/notices -check   fail with a diff when they are stale
//
// A dependency whose licence is outside the allow-list, or that matches no
// known licence, fails the run with its name (NFR-043).
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr, execRunner))
}

func execRunner(ctx context.Context, dir string, env []string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)

	var stderr bytes.Buffer

	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}

	return out, nil
}

// run is main with its inputs injected. It returns the exit code.
func run(ctx context.Context, args []string, stdout, stderr io.Writer, runner Runner) int {
	fs := flag.NewFlagSet("notices", flag.ContinueOnError)
	fs.SetOutput(stderr)

	check := fs.Bool("check", false, "fail with a diff when the committed files differ from the generated ones")
	root := fs.String("root", ".", "repository root")

	auditSPDX := fs.Bool("audit-spdx", false, "print the denied and unknown SPDX expressions read on stdin")
	auditDir := fs.String("audit-tree", "", "print the denied and unknown licence texts under DIR/licenses")

	waiverFile := fs.String("waivers", "", "with -audit-spdx: the reviewed waivers (.config/sbom-waivers.json)")
	waiverRoot := fs.String("waiver-root", "", "with -waivers: the extracted archive the waiver checksums are read from")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *auditSPDX || *auditDir != "" {
		var w *waiverSet

		if *waiverFile != "" {
			var err error
			if w, err = loadWaivers(*waiverFile, *waiverRoot); err != nil {
				fmt.Fprintln(stderr, "notices:", err)

				return 2
			}
		}

		return runAudit(*auditSPDX, *auditDir, w, os.Stdin, stdout, stderr)
	}

	want, err := build(ctx, runner, *root)
	if err != nil {
		fmt.Fprintln(stderr, "notices:", err)

		return 1
	}

	if !*check {
		if err := write(*root, want); err != nil {
			fmt.Fprintln(stderr, "notices:", err)

			return 1
		}

		fmt.Fprintf(stdout, "notices: wrote %d files\n", len(want))

		return 0
	}

	have, err := existing(*root)
	if err != nil {
		fmt.Fprintln(stderr, "notices:", err)

		return 1
	}

	if d := diffFiles(have, want); d != "" {
		fmt.Fprint(stdout, d)
		fmt.Fprintln(stderr, "notices: the committed files are stale; run: task notices")

		return 1
	}

	fmt.Fprintln(stdout, "notices: up to date")

	return 0
}
