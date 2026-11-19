package cli

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/AlexGalhardo/stuarty/service"
)

// saveTemp writes bytes to a new temporary file and returns its path.
func saveTemp(pattern string, b []byte) (string, error) {
	f, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", fmt.Errorf("save the image: %w", err)
	}

	_, werr := f.Write(b)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}

	if werr != nil {
		return "", fmt.Errorf("save the image: %w", werr)
	}

	return f.Name(), nil
}

// The commands of the hand-off from an external motion engine (SDD-011 section 15). They carry no rule: the
// Service validates and core/motion holds what a brief is. The product runs no engine.

// motionRange is the range flags the commands of the hand-off share.
var motionRange = []flagDef{
	{Name: "from", Kind: kindString, Usage: "start of the range, a time (default 0s)"},
	{Name: "until", Kind: kindString, Usage: "end of the range, exclusive (default the end of the timeline)"},
	{
		Name:  "track",
		Kind:  kindString,
		Usage: "the visual track the clip will go on: tells whether the render needs alpha",
	},
}

var motionBriefCmd = &spec{
	name:     "motion:brief",
	timeline: true,
	summary:  "what a render by an external motion engine must fit: size, rate, length, safe area, brand kit, captions",
	usage:    []string{"stuarty motion:brief --project <id> [--from t] [--until t] [--track <trk_id>]"},
	example:  "stuarty motion:brief --project prj_7k2m9x4q1b --from 2s --until 5s --track trk_f6g7h8j9k0",
	project:  true,
	flags:    motionRange,
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.MotionBrief(ctx, service.MotionBriefRequest{
			ProjectID: c.projectID, TimelineID: c.timelineID, From: c.args.str("from"), Until: c.args.str("until"),
			TrackID: c.args.str("track"),
		})
		if err != nil {
			return nil, err
		}

		return motionOutput(res), nil
	},
}

var motionCheckCmd = &spec{
	name:     "motion:check",
	timeline: true,
	summary:  "check a rendered asset against the brief: each mismatch of size, rate, length or alpha, with the value expected",
	usage: []string{
		"stuarty motion:check --project <id> --asset <ast_id> [--from t] [--until t] [--track <trk_id>]",
	},
	example: "stuarty motion:check --project prj_7k2m9x4q1b --asset ast_3c5v8n2w6d --from 2s --until 5s --track trk_f6g7h8j9k0",
	project: true,
	flags: append([]flagDef{
		{Name: "asset", Kind: kindString, Usage: "the asset to check: an asset of the project, ready (required)"},
	}, motionRange...),
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.MotionCheck(ctx, service.MotionCheckRequest{
			ProjectID: c.projectID, TimelineID: c.timelineID, AssetID: c.args.str("asset"), From: c.args.str("from"),
			Until: c.args.str("until"), TrackID: c.args.str("track"),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

var motionReplaceCmd = &spec{
	name:     "motion:replace",
	timeline: true,
	summary:  "replace, in one undoable batch, every clip that reads an earlier render of an animation with a new render",
	usage: []string{
		"stuarty motion:replace --project <id> --asset <ast_id> [--render-key <key>] [--dry-run]",
	},
	example: "stuarty motion:replace --project prj_7k2m9x4q1b --asset ast_k8m2n4p6q9",
	project: true,
	globals: []string{"dry-run"},
	write:   true,
	flags: []flagDef{
		{Name: "asset", Kind: kindString, Usage: "the new render: an asset of the timeline, ready (required)"},
		{
			Name: "render-key", Kind: kindString,
			Usage: "the key of the animation (default the render key of the origin of the new asset)",
		},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.MotionReplace(ctx, service.MotionReplaceRequest{
			ProjectID: c.projectID, TimelineID: c.timelineID, AssetID: c.args.str("asset"),
			RenderKey: c.args.str("render-key"), DryRun: c.args.boolean("dry-run"),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: motionReplaceText(res)}, nil
	},
}

// motionReplaceText is the line of the replacement and the text of the edit it made.
func motionReplaceText(res *service.MotionReplaceResult) string {
	head := fmt.Sprintf("replaced %d clip(s) of the render key %s with %s: %s", len(res.Replaced), res.RenderKey,
		res.AssetID, strings.Join(res.Replaced, ", "))
	if res.DryRun {
		head = "dry run: would have " + head
	}

	return head + "\nrevision " + strconv.FormatInt(res.Revision, 10) + "; undo restores every clip"
}

// motionOutput prints the brief without the bytes of the frames: a frame is the file in its path. A server on
// another machine answers no path, so the bytes go to a temporary file here.
func motionOutput(res *service.MotionBriefResult) *output {
	out := *res
	out.Frames = make([]service.MotionFrame, len(res.Frames))

	for i, f := range res.Frames {
		out.Frames[i] = f
		out.Frames[i].Image = nil

		if f.Path == "" && len(f.Image) > 0 {
			if path, err := saveTemp("stuarty-motion-*.jpg", f.Image); err == nil {
				out.Frames[i].Path = path
				out.Text += "\nreference frame " + f.Role + " saved " + path
			}
		}
	}

	return &output{value: &out, text: out.Text}
}
