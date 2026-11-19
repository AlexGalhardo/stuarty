// Command threatcheck keeps the threat table of SDD-007 section 2 and the
// security tests in step (SDD-007 section 12, SDD-009 job 12 "Threat coverage"):
//
//   - every threat row names at least one test and has a mitigation;
//
//   - every test a row names exists in the repository;
//
//   - every TestSecurity_T<NN>_ test of the repository is named by a row;
//
//   - every operation of service.Service appears in the scope matrix of section 5.
//
//     go run ./internal/tools/threatcheck [-sdd path] [-root dir]
//
// It exits 0 when all of that holds, 1 with the list of problems otherwise.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(argv []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("threatcheck", flag.ContinueOnError)
	fs.SetOutput(stderr)

	sdd := fs.String("sdd", "docs/sdds/007-security-model.md", "the security model")
	root := fs.String("root", ".", "the repository root")

	if err := fs.Parse(argv); err != nil {
		return 2
	}

	problems, err := check(*root, *sdd)
	if err != nil {
		fmt.Fprintf(stderr, "threatcheck: %v\n", err)

		return 1
	}

	for _, p := range problems {
		fmt.Fprintln(stderr, "threatcheck:", p)
	}

	if len(problems) > 0 {
		fmt.Fprintf(stderr, "threatcheck: %d problem(s)\n", len(problems))

		return 1
	}

	fmt.Fprintln(stdout, "threatcheck: every threat has its tests and every operation its scope")

	return 0
}
