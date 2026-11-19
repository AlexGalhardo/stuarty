// Command gen-agent-docs writes the material for agents (SDD-003 section 8) into
// web/app/agentdocs/out/, or with --check fails when what is there differs from
// what the registry, the lint catalogue and the templates produce.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/AlexGalhardo/stuarty/web/app/agentdocs"
	"github.com/AlexGalhardo/stuarty/web/app/cli"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("gen-agent-docs", flag.ContinueOnError)
	fs.SetOutput(stderr)

	check := fs.Bool("check", false, "regenerate in memory and fail on any difference")
	dir := fs.String("dir", filepath.FromSlash("web/app/agentdocs/out"), "the directory of the generated files")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	files, err := agentdocs.Generate(cli.AgentDocsInputs())
	if err != nil {
		fmt.Fprintln(stderr, "gen-agent-docs:", err)

		return 1
	}

	code := 0

	for _, name := range agentdocs.Files {
		path := filepath.Join(*dir, name)

		if !*check {
			if err := os.WriteFile(path, files[name], 0o600); err != nil {
				fmt.Fprintln(stderr, "gen-agent-docs:", err)

				return 1
			}

			continue
		}

		have, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(have, files[name]) {
			fmt.Fprintf(stdout, "%s is out of date: run go run ./web/cmd/gen-agent-docs\n", path)

			code = 1
		}
	}

	return code
}
