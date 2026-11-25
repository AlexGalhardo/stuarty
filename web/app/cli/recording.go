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
	lines := []string{fmt.Sprintf(
		"%d display(s), %d window(s), %d microphone(s), %d camera(s), system audio: %t",
		len(res.Displays), len(res.Windows), len(res.Microphones), len(res.Cameras), res.SystemAudio,
	)}

	name := func(s string) string { return untrusted.Inline(untrusted.SourceMetadata, nil, s, 120) }

	for _, m := range res.Microphones {
		lines = append(lines, fmt.Sprintf("microphone %s  default: %t  %s", m.ID, m.Default, name(m.Name)))
	}

	for _, d := range res.Cameras {
		lines = append(lines, fmt.Sprintf("camera %s  %s", d.ID, name(d.Name)))
	}

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
	usage: []string{
		"stuarty recording:start --project <id> --kind display|window --source <id> [--fps 24|30|60] " +
			"[--microphone <id>|default] [--system-audio] [--camera <id>] [--events]",
		"stuarty recording:start --recording <id of a prepared recording>",
	},
	example: "stuarty recording:start --project prj_7k2m9x4q1b --kind display --source d1",
	write:   true,
	flags: []flagDef{
		{Name: "project", Kind: kindString, Usage: "the project (default $" + EnvProject + "); not with --recording"},
		{
			Name:  "recording",
			Kind:  kindString,
			Usage: "a recording made with recording:prepare: its plan is the request",
		},
		{Name: "kind", Kind: kindString, Usage: "display or window (required)"},
		{Name: "source", Kind: kindString, Usage: "an id from recording:sources (required)"},
		{Name: "name", Kind: kindString, Usage: "the title of the source, for the record"},
		{Name: "fps", Kind: kindInt, Usage: "24, 30 (the default) or 60"},
		{Name: "microphone", Kind: kindString, Usage: "a microphone id from recording:sources, or default"},
		{Name: "system-audio", Kind: kindBool, Usage: "record the sound the computer plays on its own track"},
		{Name: "camera", Kind: kindString, Usage: "a camera id from recording:sources"},
		{Name: "events", Kind: kindBool, Usage: "save the pointer position and the clicks (never a key)"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		project := c.args.str("project")
		if project == "" && c.args.str("recording") == "" {
			project = c.rt.Getenv(EnvProject)
		}

		res, err := c.svc.RecordingStart(ctx, service.RecordingStartRequest{
			RecordingID: c.args.str("recording"), ProjectID: project, FPS: c.args.integer("fps"),
			Source: service.RecordingSource{
				Kind: c.args.str("kind"), ID: c.args.str("source"), Name: c.args.str("name"),
			},
			Microphone: c.args.str("microphone"), SystemAudio: c.args.boolean("system-audio"),
			Camera: c.args.str("camera"), Events: c.args.boolean("events"),
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
	recordingPauseCmd = recordingByID(
		"recording:pause", "pause a recording: every source stops writing and the paused time is cut; anyone may pause",
		"stuarty recording:pause rec_0123456789", false,
		func(ctx context.Context, c *call, r service.RecordingRef) (*service.Recording, error) {
			return c.svc.RecordingPause(ctx, r)
		})
	recordingResumeCmd = recordingByID(
		"recording:resume",
		"resume a paused recording (for a person in the app; the local token is refused, as for a start)",
		"stuarty recording:resume rec_0123456789",
		true,
		func(ctx context.Context, c *call, r service.RecordingRef) (*service.Recording, error) {
			return c.svc.RecordingResume(ctx, r)
		},
	)
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

var recordingEventsCmd = &spec{
	name:       "recording:events",
	summary:    "read the pointer and click events of a finished recording (clicks by default)",
	usage:      []string{"stuarty recording:events <recording id> [--kind clicks|all] [--from-ms <n>] [--limit <n>]"},
	example:    "stuarty recording:events rec_0123456789 --kind clicks --json",
	positional: true,
	flags: []flagDef{
		{Name: "kind", Kind: kindString, Usage: "clicks (the default) or all"},
		{Name: "from-ms", Kind: kindInt, Usage: "only events from this time on, in milliseconds"},
		{Name: "limit", Kind: kindInt, Usage: "at most this many events, 1 to 5000 (default 1000)"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		id, err := oneArg(c, "recording:events", "recording id")
		if err != nil {
			return nil, err
		}

		res, err := c.svc.RecordingEvents(ctx, service.RecordingEventsRequest{
			ID: id, Kind: c.args.str("kind"), FromMS: int64(c.args.integer("from-ms")), Limit: c.args.integer("limit"),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
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
