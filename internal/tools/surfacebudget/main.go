// Command surfacebudget measures the agent surface against the budget of NFR-020 (revised once by epic 13,
// SDD-003 section 4.8) and fails when a measure is over its limit: the minified tools/list result and its largest
// tools, the text of the first discovery call (command_help with no argument), and the description of each domain.
// It prints every measured number next to its limit. `task lint` runs it, so a tool, a parameter or a domain
// description that costs too much fails the build before it reaches an agent.
//
// Tokens are estimated as characters divided by 3.2, the figure of SDD-003 section 3.10; the vendor's own count
// (TestToolListVendorTokens) is a release measure that needs an API key and is not run here.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"text/tabwriter"
	"unicode/utf8"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/web/app/mcp"
)

// charsPerToken is the estimate of SDD-003 section 3.10.
const charsPerToken = 3.2

// measure is one number of the surface and the limit it must stay under.
type measure struct {
	what     string
	unit     string
	measured int
	limit    int
}

func (m measure) over() bool { return m.measured > m.limit }

func main() { os.Exit(run(os.Stdout, os.Stderr)) }

func run(stdout, stderr io.Writer) int {
	rows, err := measureSurface(context.Background())
	if err != nil {
		fmt.Fprintln(stderr, "surfacebudget:", err)

		return 1
	}

	return report(stdout, stderr, rows)
}

// report prints the table and returns 1 when any row is over its limit.
func report(stdout, stderr io.Writer, rows []measure) int {
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "measure\tunit\tmeasured\tlimit\tstatus")

	failed := 0

	for _, m := range rows {
		status := "ok"
		if m.over() {
			status = "OVER"
			failed++
		}

		fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%s\n", m.what, m.unit, m.measured, m.limit, status)
	}

	_ = tw.Flush()

	if failed > 0 {
		fmt.Fprintf(stderr, "surfacebudget: %d measure(s) over the budget of NFR-020 (SDD-003 section 4.1)\n", failed)

		return 1
	}

	return 0
}

// measureSurface reads the real surface: the tools/list a client receives from the server, the discovery the
// Service answers and the descriptions of the domains.
func measureSurface(ctx context.Context) ([]measure, error) {
	tools, listBytes, err := listTools(ctx)
	if err != nil {
		return nil, err
	}

	rows := []measure{
		{"tools/list, minified", "bytes", listBytes, mcp.MaxToolListBytes},
		{"tools/list, about tokens", "tokens", tokens(listBytes), tokens(mcp.MaxToolListBytes)},
		{"tool count", "tools", len(tools), mcp.MaxTools},
	}

	largest, largestBytes := "", 0

	for _, t := range tools {
		raw, merr := json.Marshal(t)
		if merr != nil {
			return nil, merr
		}

		if len(raw) > largestBytes {
			largest, largestBytes = t.Name, len(raw)
		}
	}

	rows = append(rows, measure{"largest definition: " + largest, "bytes", largestBytes, mcp.MaxToolBytes})

	help := service.Discovery(command.Default())
	chars := utf8.RuneCountInString(help.Text)
	rows = append(rows,
		measure{"first discovery call: command_help", "characters", chars, mcp.MaxDiscoveryChars},
		measure{"first discovery call, about tokens", "tokens", tokens(chars), tokens(mcp.MaxDiscoveryChars)},
	)

	for _, d := range command.Domains() {
		rows = append(
			rows,
			measure{
				"domain description: " + d.Name,
				"characters",
				utf8.RuneCountInString(d.Summary),
				command.MaxDomainSummary,
			},
		)
	}

	return rows, nil
}

func tokens(chars int) int { return int(float64(chars) / charsPerToken) }

// listTools asks an in-memory server for tools/list, as a client does, and measures the minified result. The
// server needs no Service to list its tools.
func listTools(ctx context.Context) (tools []*sdk.Tool, bytes int, err error) {
	srv, err := mcp.New(nil, mcp.Options{Session: "budget"})
	if err != nil {
		return nil, 0, err
	}

	ct, st := sdk.NewInMemoryTransports()

	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		return nil, 0, err
	}

	defer func() { _ = ss.Close() }()

	cs, err := sdk.NewClient(&sdk.Implementation{Name: "surfacebudget", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		return nil, 0, err
	}

	defer func() { _ = cs.Close() }()

	list, err := cs.ListTools(ctx, nil)
	if err != nil {
		return nil, 0, err
	}

	raw, err := json.Marshal(list)
	if err != nil {
		return nil, 0, err
	}

	return list.Tools, len(raw), nil
}
