package cli

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"

	"github.com/AlexGalhardo/stuarty/service"
)

// The agent flow of a recording (epic 64, SDD-025 section 19). They carry no rule: the Service decides who may do
// what. An agent prepares a recording, marks takes and proposes cuts; starting it and deciding the cuts are for a
// person in the app, and the commands that do them exist so that an agent that tries gets the stable refusal.

var recordingPrepareCmd = &spec{
	name:    "recording:prepare",
	summary: "prepare a recording for a person: source, tracks, script and notes; starts nothing",
	usage: []string{
		"stuarty recording:prepare --project <id> --kind display|window --source <id> [--fps 24|30|60] " +
			"[--microphone <id>|default] [--system-audio] [--camera <id>] [--events] [--script <item id>] " +
			"[--notes <text>|--notes-file <file|->]",
	},
	example: "stuarty recording:prepare --project prj_7k2m9x4q1b --kind display --source d1 --notes \"Say the hook first\"",
	project: true,
	write:   true,
	flags: []flagDef{
		{Name: "kind", Kind: kindString, Usage: "display or window (required)"},
		{Name: "source", Kind: kindString, Usage: "an id from recording:sources (required)"},
		{Name: "name", Kind: kindString, Usage: "the title of the source, for the record"},
		{Name: "fps", Kind: kindInt, Usage: "24, 30 (the default) or 60"},
		{Name: "microphone", Kind: kindString, Usage: "a microphone id from recording:sources, or default"},
		{Name: "system-audio", Kind: kindBool, Usage: "record the sound the computer plays on its own track"},
		{Name: "camera", Kind: kindString, Usage: "a camera id from recording:sources"},
		{Name: "events", Kind: kindBool, Usage: "save the pointer position and the clicks (never a key)"},
		{Name: "script", Kind: kindString, Usage: "the id of a script of the project the person will read"},
		{Name: "notes", Kind: kindString, Usage: "a note for the person who presses record"},
		{Name: "notes-file", Kind: kindString, Usage: "read the note from a file, or - for standard input"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		notes := c.args.str("notes")
		if c.args.has("notes-file") {
			var err error
			if notes, err = readFileOrStdin(c, c.args.str("notes-file"), "notes-file"); err != nil {
				return nil, err
			}
		}

		res, err := c.svc.RecordingPrepare(ctx, service.RecordingPrepareRequest{
			ProjectID: c.projectID, FPS: c.args.integer("fps"),
			Source: service.RecordingSource{
				Kind: c.args.str("kind"), ID: c.args.str("source"), Name: c.args.str("name"),
			},
			Microphone: c.args.str("microphone"), SystemAudio: c.args.boolean("system-audio"),
			Camera: c.args.str("camera"), Events: c.args.boolean("events"),
			ScriptID: c.args.str("script"), Notes: notes,
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: recordingLine(res)}, nil
	},
}

var recordingMarkCmd = &spec{
	name:    "recording:mark",
	summary: "mark a point or a stretch of a take as good, bad or a mistake",
	usage: []string{
		"stuarty recording:mark <recording id> --kind good|bad|mistake --at-ms <n> [--end-ms <n>] [--note <text>]",
	},
	example:    "stuarty recording:mark rec_0123456789 --kind mistake --at-ms 12500 --note \"stumbled on the name\"",
	positional: true,
	write:      true,
	flags: []flagDef{
		{Name: "kind", Kind: kindString, Usage: "good, bad or mistake (required)"},
		{Name: "at-ms", Kind: kindInt, Usage: "where in the take, in milliseconds of recording time"},
		{Name: "end-ms", Kind: kindInt, Usage: "the end of a stretch; leave out for a point"},
		{Name: "note", Kind: kindString, Usage: "a short note, plain text"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		id, err := oneArg(c, "recording:mark", "recording id")
		if err != nil {
			return nil, err
		}

		res, err := c.svc.RecordingMark(ctx, service.RecordingMarkRequest{
			ID: id, Kind: c.args.str("kind"), AtMS: int64(c.args.integer("at-ms")),
			EndMS: int64(c.args.integer("end-ms")), Note: c.args.str("note"),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: recordingLine(res)}, nil
	},
}

var recordingProposeCmd = &spec{
	name:       "recording:propose",
	summary:    "propose cuts of a ready take, each with a reason; changes no timeline until a person decides",
	usage:      []string{"stuarty recording:propose <recording id> --ranges <file|->"},
	example:    "stuarty recording:propose rec_0123456789 --ranges cuts.json",
	positional: true,
	write:      true,
	flags: []flagDef{
		{
			Name: "ranges", Kind: kindString,
			Usage: "a JSON list of {from_ms, to_ms, reason}, from a file or - for standard input",
		},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		id, err := oneArg(c, "recording:propose", "recording id")
		if err != nil {
			return nil, err
		}

		raw, err := readFileOrStdin(c, c.args.str("ranges"), "ranges")
		if err != nil {
			return nil, err
		}

		var ranges []service.RecordingCutRange
		if jerr := json.Unmarshal([]byte(raw), &ranges); jerr != nil {
			return nil, validation("ranges", "the ranges are not a JSON list of {from_ms, to_ms, reason}.")
		}

		res, err := c.svc.RecordingProposeCuts(ctx, service.RecordingProposeCutsRequest{ID: id, Ranges: ranges})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: recordingLine(res)}, nil
	},
}

var recordingDecideCmd = &spec{
	person:  true,
	name:    "recording:decide",
	summary: "decide a proposal of cuts, one yes or no per range (for a person in the app; the local token is refused)",
	usage: []string{
		"stuarty recording:decide <recording id> <proposal id> --accept <yes|no,...> [--timeline <id>]",
	},
	example:    "stuarty recording:decide rec_0123456789 prp_0123456789 --accept yes,no,yes",
	positional: true,
	write:      true,
	flags: []flagDef{
		{Name: "accept", Kind: kindString, Usage: "one yes or no for every range, in order, separated by commas"},
		{
			Name:  "timeline",
			Kind:  kindString,
			Usage: "the timeline the recording was placed on, when the project has several",
		},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		if len(c.args.Positional) != 2 {
			return nil, validation("", "recording:decide takes a recording id and a proposal id, got %d.",
				len(c.args.Positional))
		}

		var accept []bool

		for word := range strings.SplitSeq(c.args.str("accept"), ",") {
			switch strings.ToLower(strings.TrimSpace(word)) {
			case "yes", "y", "true":
				accept = append(accept, true)
			case "no", "n", "false":
				accept = append(accept, false)
			default:
				return nil, validation("accept", "send yes or no for every range, separated by commas, not %q.", word)
			}
		}

		res, err := c.svc.RecordingProposalDecide(ctx, service.RecordingProposalDecideRequest{
			ID: c.args.Positional[0], ProposalID: c.args.Positional[1], Accept: accept,
			TimelineID: c.args.str("timeline"),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

// readFileOrStdin reads the value of a flag that names a file, or - for standard input.
func readFileOrStdin(c *call, name, flag string) (string, error) {
	if name == "" {
		return "", validation(flag, "pass --%s <file|->.", flag)
	}

	var (
		raw []byte
		err error
	)

	if name == "-" {
		raw, err = io.ReadAll(io.LimitReader(c.rt.Stdin, 1<<20))
	} else {
		raw, err = os.ReadFile(name)
	}

	if err != nil {
		return "", validation(flag, "cannot read %s: %s.", flag, readReason(err))
	}

	return strings.TrimRight(string(raw), "\r\n"), nil
}
