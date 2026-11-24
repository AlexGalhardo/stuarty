package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/untrusted"
)

// The screen recording commands (SDD-025). They carry no rule: the Service decides who may start a capture and
// core/recording holds the states and the limits. Starting, adding to the project and "Not now" are for a person in
// the app: the commands exist so that an agent that tries gets E_PRIVILEGED_SESSION and its hint instead of
// "unknown command"; run with the local token they always fail. Stopping is open to every caller.

func recordingLine(r *service.Recording) string { return r.Text }

var recordingSourcesCmd = &spec{
	name:    "recording:sources",
	summary: "list the displays and windows that can be captured",
	usage:   []string{"stuarty recording:sources"},
	example: "stuarty recording:sources --json",
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.RecordingSources(ctx, service.RecordingSourcesRequest{})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: sourcesText(res)}, nil
	},
}

// sourcesText lists the sources. A window title is any text another program chose, so it is marked as untrusted
// for an agent that reads the output (SDD-007 section 8).
func sourcesText(res *service.RecordingSourcesResult) string {
	lines := []string{fmt.Sprintf("%d display(s), %d window(s)", len(res.Displays), len(res.Windows))}

	name := func(s string) string { return untrusted.Inline(untrusted.SourceMetadata, nil, s, 120) }

	for _, d := range res.Displays {
		lines = append(lines, fmt.Sprintf("display %s  %dx%d  %s", d.ID, d.Width, d.Height, name(d.Name)))
	}

	for _, w := range res.Windows {
		lines = append(lines, fmt.Sprintf("window  %s  %dx%d  %s", w.ID, w.Width, w.Height, name(w.Name)))
	}

	return strings.Join(lines, "\n")
}

var recordingStartCmd = &spec{
	person:  true,
	name:    "recording:start",
	summary: "start recording a display or a window (for a person in the app; the local token is refused)",
	usage:   []string{"stuarty recording:start --project <id> --kind display|window --source <id> [--fps 24|30|60]"},
	example: "stuarty recording:start --project prj_7k2m9x4q1b --kind display --source d1",
	project: true,
	write:   true,
	flags: []flagDef{
		{Name: "kind", Kind: kindString, Usage: "display or window (required)"},
		{Name: "source", Kind: kindString, Usage: "an id from recording:sources (required)"},
		{Name: "name", Kind: kindString, Usage: "the title of the source, for the record"},
		{Name: "fps", Kind: kindInt, Usage: "24, 30 (the default) or 60"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.RecordingStart(ctx, service.RecordingStartRequest{
			ProjectID: c.projectID, FPS: c.args.integer("fps"),
			Source: service.RecordingSource{
				Kind: c.args.str("kind"), ID: c.args.str("source"), Name: c.args.str("name"),
			},
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: recordingLine(res)}, nil
	},
}

// recordingByID is the commands that take one recording id.
func recordingByID(
	name, summary, example string,
	person bool,
	do func(context.Context, *call, service.RecordingRef) (*service.Recording, error),
) *spec {
	return &spec{
		person:     person,
		name:       name,
		summary:    summary,
		usage:      []string{"stuarty " + name + " <recording id>"},
		example:    example,
		positional: true,
		write:      name != "recording:show",
		run: func(ctx context.Context, c *call) (*output, error) {
			id, err := oneArg(c, name, "recording id")
			if err != nil {
				return nil, err
			}

			res, err := do(ctx, c, service.RecordingRef{ID: id})
			if err != nil {
				return nil, err
			}

			return &output{value: res, text: recordingLine(res)}, nil
		},
	}
}

var (
	recordingShowCmd = recordingByID(
		"recording:show", "show one recording: its state, its length and whether a person added it",
		"stuarty recording:show rec_0123456789", false,
		func(ctx context.Context, c *call, r service.RecordingRef) (*service.Recording, error) {
			return c.svc.RecordingGet(ctx, r)
		})
	recordingStopCmd = recordingByID(
		"recording:stop", "stop a recording and keep what was written; anyone may stop a capture",
		"stuarty recording:stop rec_0123456789", false,
		func(ctx context.Context, c *call, r service.RecordingRef) (*service.Recording, error) {
			return c.svc.RecordingStop(ctx, r)
		})
	recordingCancelCmd = recordingByID(
		"recording:cancel", "discard a recording in progress; a person deletes a finished one",
		"stuarty recording:cancel rec_0123456789", false,
		func(ctx context.Context, c *call, r service.RecordingRef) (*service.Recording, error) {
			return c.svc.RecordingCancel(ctx, r)
		})
	recordingConfirmCmd = recordingByID(
		"recording:confirm",
		"add a finished recording to the project (for a person in the app; the local token is refused)",
		"stuarty recording:confirm rec_0123456789",
		true,
		func(ctx context.Context, c *call, r service.RecordingRef) (*service.Recording, error) {
			return c.svc.RecordingConfirm(ctx, r)
		},
	)
	recordingDismissCmd = recordingByID(
		"recording:dismiss",
		"not now: keep a recording out of the project and out of sight of agents (for a person in the app)",
		"stuarty recording:dismiss rec_0123456789",
		true,
		func(ctx context.Context, c *call, r service.RecordingRef) (*service.Recording, error) {
			return c.svc.RecordingDismiss(ctx, r)
		},
	)
)

var recordingListCmd = &spec{
	name:    "recording:list",
	summary: "list the recordings of a project that you may see, newest first",
	usage:   []string{"stuarty recording:list --project <id>"},
	example: "stuarty recording:list --project prj_7k2m9x4q1b",
	project: true,
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.RecordingList(ctx, service.RecordingListRequest{ProjectID: c.projectID})
		if err != nil {
			return nil, err
		}

		lines := []string{fmt.Sprintf("%d recording(s)", len(res.Recordings))}

		for i := range res.Recordings {
			lines = append(lines, res.Recordings[i].Text)
		}

		return &output{value: res, text: strings.Join(lines, "\n")}, nil
	},
}

var recordingPlaceCmd = &spec{
	name:       "recording:place",
	summary:    "place a recording that a person added on the timeline, as one undoable batch",
	usage:      []string{"stuarty recording:place <recording id> [--timeline <id>]"},
	example:    "stuarty recording:place rec_0123456789",
	positional: true,
	write:      true,
	flags: []flagDef{
		{Name: "timeline", Kind: kindString, Usage: "the timeline of the project, when it has several"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		id, err := oneArg(c, "recording:place", "recording id")
		if err != nil {
			return nil, err
		}

		res, err := c.svc.RecordingPlace(ctx, service.RecordingPlaceRequest{ID: id, TimelineID: c.args.str("timeline")})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: service.EditText(res)}, nil
	},
}
