package cli

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/AlexGalhardo/stuarty/service"
)

// The commands of the guided briefing (SDD-005 section 5.12.1). They carry no rule: the Service validates and
// core/project holds the questions.

var briefingQuestionsCmd = &spec{
	name:    "briefing:questions",
	summary: "list the questions of the guided briefing; with --project, also what its brief already answers",
	usage:   []string{"stuarty briefing:questions [--project <id>] [--kind short|commercial|long]"},
	example: "stuarty briefing:questions --project prj_7k2m9x4q1b",
	flags: []flagDef{
		{Name: "project", Kind: kindString, Usage: "a project: also the answers its brief holds"},
		{Name: "kind", Kind: kindString, Usage: "the kind of production: short (the default), commercial or long"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.BriefingQuestions(
			ctx,
			service.BriefingQuestionsRequest{ProjectID: c.args.str("project"), Kind: c.args.str("kind")},
		)
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

var briefingAnswerCmd = &spec{
	name:    "briefing:answer",
	summary: "answer the guided briefing: the answers become the brief of a project",
	usage: []string{
		"stuarty briefing:answer --project <id> [--kind short|commercial|long] --answer <question>=<text> [--answer ...] [--replace]",
		"stuarty briefing:answer --project <id> [--kind short|commercial|long] --answers <file|-> [--replace]",
	},
	example: "stuarty briefing:answer --project prj_7k2m9x4q1b --answer audience=\"indie developers\" --answer goal=\"get sign-ups\" --answer platform=youtube-shorts --answer format=vertical-9x16 --answer duration=30",
	project: true,
	globals: []string{"dry-run"},
	write:   true,
	flags: []flagDef{
		{
			Name:   "answer",
			Kind:   kindString,
			Repeat: true,
			Usage:  "one answer as <question id>=<text>; repeat for each question",
		},
		{
			Name:  "answers",
			Kind:  kindString,
			Usage: "a JSON object of question id to answer: a file, or - for standard input",
		},
		{Name: "kind", Kind: kindString, Usage: "the kind of production: short (the default), commercial or long"},
		{Name: "replace", Kind: kindBool, Default: "false", Usage: "replace a brief that was written by hand"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		answers := map[string]string{}

		if c.args.has("answers") {
			text, err := c.readText("answers", c.args.str("answers"))
			if err != nil {
				return nil, err
			}

			if err := json.Unmarshal([]byte(text), &answers); err != nil {
				return nil, validation("answers", "the answers are not a JSON object of text: %s.", jsonProblem(err))
			}
		}

		for _, a := range c.args.all("answer") {
			id, value, ok := strings.Cut(a, "=")
			if !ok || id == "" {
				return nil, validation("answer", "--answer is <question id>=<text>.")
			}

			answers[id] = value
		}

		res, err := c.svc.BriefingAnswer(ctx, service.BriefingAnswerRequest{
			ProjectID: c.projectID, Kind: c.args.str("kind"), Answers: answers, Replace: c.args.boolean("replace"),
			DryRun: c.args.boolean("dry-run"),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: briefText(res)}, nil
	},
}
