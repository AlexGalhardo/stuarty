package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/AlexGalhardo/stuarty/service"
)

// The hosted agent chat commands (SDD-017 section 10): the reads only. A person drives a chat in the
// panel of the app, so there is no command that starts, steers or answers one, or that writes a
// profile; the text of a transcript is untrusted (SDD-007 section 8).

var agentProfilesCmd = &spec{
	name:    "agent:profiles",
	summary: "list the agents the chat may host, with their full command, and the known agents found on this machine",
	usage:   []string{"stuarty agent:profiles"},
	example: "stuarty agent:profiles --json",
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.AgentProfileList(ctx, service.AgentProfileListRequest{})
		if err != nil {
			return nil, err
		}

		lines := []string{fmt.Sprintf("%d agent profile(s)", len(res.Profiles))}

		for i := range res.Profiles {
			p := &res.Profiles[i]
			lines = append(
				lines,
				fmt.Sprintf("%s  %s", p.Name, strings.Join(append([]string{p.Command}, p.Args...), " ")),
			)
		}

		for i := range res.Suggestions {
			s := &res.Suggestions[i]
			found := "not found"

			if s.Found {
				found = "found"
			}

			lines = append(lines, fmt.Sprintf("known: %s (%s)", s.Name, found))
		}

		return &output{value: res, text: strings.Join(lines, "\n")}, nil
	},
}

var chatSessionsCmd = &spec{
	name:    "chat:sessions",
	summary: "list the chat sessions of a project, newest first",
	usage:   []string{"stuarty chat:sessions --project <id>"},
	example: "stuarty chat:sessions --project prj_7k2m9x4q1b",
	project: true,
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.ChatSessionList(ctx, service.ChatSessionListRequest{ProjectID: c.projectID})
		if err != nil {
			return nil, err
		}

		lines := []string{fmt.Sprintf("%d chat session(s)", len(res.Sessions))}

		for i := range res.Sessions {
			lines = append(lines, sessionLine(&res.Sessions[i]))
		}

		return &output{value: res, text: strings.Join(lines, "\n")}, nil
	},
}

var chatSessionCmd = &spec{
	name:       "chat:session",
	summary:    "show one chat session: its agent, its state and how many events it holds",
	usage:      []string{"stuarty chat:session <id>"},
	example:    "stuarty chat:session cht_7k2m9x4q1b",
	positional: true,
	run: func(ctx context.Context, c *call) (*output, error) {
		id, err := oneArg(c, "chat:session", "chat session id")
		if err != nil {
			return nil, err
		}

		res, err := c.svc.ChatSessionGet(ctx, service.ChatRef{SessionID: id})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: sessionLine(res)}, nil
	},
}

var chatTranscriptCmd = &spec{
	name:       "chat:transcript",
	summary:    "read the transcript of a chat session as events, or the whole of it as Markdown",
	usage:      []string{"stuarty chat:transcript <id> [--format json|markdown] [--after n] [--limit n]"},
	example:    "stuarty chat:transcript cht_7k2m9x4q1b --format markdown",
	positional: true,
	flags: []flagDef{
		{Name: "format", Kind: kindString, Usage: "json (the default: events after a number) or markdown"},
		{Name: "after", Kind: kindInt, Default: "0", Usage: "only the events with a number above this one"},
		{Name: "limit", Kind: kindInt, Default: "0", Usage: "the most events in the page, at most 500 (0 is 500)"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		id, err := oneArg(c, "chat:transcript", "chat session id")
		if err != nil {
			return nil, err
		}

		res, err := c.svc.ChatTranscriptGet(ctx, service.ChatTranscriptRequest{
			SessionID: id, After: int64(c.args.integer("after")), Limit: c.args.integer("limit"),
			Format: c.args.str("format"),
		})
		if err != nil {
			return nil, err
		}

		if res.Markdown != "" {
			return &output{value: res, text: res.Markdown}, nil
		}

		lines := []string{sessionLine(&res.Session)}

		for i := range res.Events {
			e := &res.Events[i]
			lines = append(lines, fmt.Sprintf("%d  %s  %s", e.Seq, e.Kind, firstLine(e.Text)))
		}

		return &output{value: res, text: strings.Join(lines, "\n")}, nil
	},
}

func sessionLine(s *service.ChatSession) string {
	return fmt.Sprintf("%s  %s  agent %s  %s  %d event(s)", s.ID, s.State, s.Profile, firstLine(s.Title), s.EventCount)
}

// firstLine is the first line of a text, cut to a length that fits a terminal; the text is
// untrusted and never interpreted.
func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")

	if len(line) > 80 {
		line = line[:80] + "..."
	}

	return line
}
