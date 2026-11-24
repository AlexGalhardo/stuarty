package cli

import (
	"context"

	"github.com/AlexGalhardo/stuarty/service"
)

// The commands of editing by text (SDD-023). They carry no rule: core/transcript maps the words to the timeline and
// finds the fillers, and the cut is the command transcript.delete_range of the edit command.

var transcriptShowCmd = &spec{
	name:     "transcript:show",
	timeline: true,
	summary:  "the words of an asset with the timeline range of each, to cut by word range with transcript.delete_range",
	usage:    []string{"stuarty transcript:show --project <id> --asset <ast_id>"},
	example:  "stuarty transcript:show --project prj_7k2m9x4q1b --asset ast_3c5v8n2w6d",
	project:  true,
	flags: []flagDef{
		{Name: "asset", Kind: kindString, Usage: "the asset whose transcript to read, transcribed first (required)"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.TranscriptView(ctx, service.TranscriptViewRequest{
			ProjectID: c.projectID, TimelineID: c.timelineID, AssetID: c.args.str("asset"),
		})
		if err != nil {
			return nil, err
		}

		shown := res.ForAgent()

		return &output{value: shown, text: shown.Text}, nil
	},
}

var transcriptFillersCmd = &spec{
	name:     "transcript:fillers",
	timeline: true,
	summary:  "the filler words and repetitions of an asset to review; it changes nothing",
	usage:    []string{"stuarty transcript:fillers --project <id> --asset <ast_id> [--repeat-window ms]"},
	example:  "stuarty transcript:fillers --project prj_7k2m9x4q1b --asset ast_3c5v8n2w6d",
	project:  true,
	flags: []flagDef{
		{Name: "asset", Kind: kindString, Usage: "the asset whose transcript to check, transcribed first (required)"},
		{
			Name: "repeat-window", Kind: kindInt,
			Usage: "the longest gap between the two copies of a repetition, in milliseconds (default 700)",
		},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.TranscriptDetectFillers(ctx, service.TranscriptDetectRequest{
			ProjectID: c.projectID, TimelineID: c.timelineID, AssetID: c.args.str("asset"),
			RepeatWindowMS: int64(c.args.integer("repeat-window")),
		})
		if err != nil {
			return nil, err
		}

		shown := res.ForAgent()

		return &output{value: shown, text: shown.Text}, nil
	},
}
