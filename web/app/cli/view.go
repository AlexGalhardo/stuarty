package cli

// The view command of SDD-003 section 5.4: a frame or a contact sheet of the
// current revision, as a file.

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/AlexGalhardo/stuarty/service"
)

var view = &spec{
	name:     "view",
	timeline: true,
	summary:  "render a frame or a contact sheet of a project, or measure its sound",
	usage: []string{
		"stuarty view --project <id> [--mode frame|sheet|audio] [--at t] [--times t1,t2,...]",
		"    [--from t --until t --count n] [--cols n] [--max-tokens n] [--width px] [--item id]",
	},
	example: "stuarty view --project prj_7k2m9x4q1b --at 1s --json",
	project: true,
	flags: []flagDef{
		{
			Name: "mode", Kind: kindString, Default: "frame",
			Usage: "frame (one image at --at), sheet (a grid) or audio (the loudness, true peak and noise floor " +
				"with and without the audio processors)",
		},
		{Name: "item", Kind: kindString, Usage: "audio: measure this clip or audio item alone"},
		{Name: "at", Kind: kindString, Usage: "frame: the time to show"},
		{Name: "times", Kind: kindString, Usage: "sheet: 2 to 16 times, comma separated"},
		{Name: "from", Kind: kindString, Usage: "sheet: first time (default 0s); audio: start of the range"},
		{
			Name: "until", Kind: kindString,
			Usage: "sheet: last time, included (default the last frame); audio: end of the range, excluded",
		},
		{Name: "count", Kind: kindInt, Default: "9", Usage: "sheet: frames evenly spaced from --from to --until"},
		{Name: "cols", Kind: kindInt, Usage: "sheet: columns (default ceil(sqrt(count)))"},
		{Name: "max-tokens", Kind: kindInt, Usage: "image token budget (default 800, a sheet 1,500; at most 1,568)"},
		{Name: "width", Kind: kindInt, Usage: "image width in pixels, even; exclusive with --max-tokens"},
	},
	run: runView,
}

func runView(ctx context.Context, c *call) (*output, error) {
	p := c.args

	var (
		res *service.ViewResult
		err error
	)

	switch mode := p.str("mode"); mode {
	case "frame":
		for _, f := range []string{"times", "from", "until", "cols"} {
			if p.has(f) {
				return nil, validation(f, "--%s is for --mode sheet.", f)
			}
		}

		if p.has("item") {
			return nil, validation("item", "--item is for --mode audio.")
		}

		res, err = c.svc.ViewFrame(ctx, service.ViewFrameRequest{
			ProjectID: c.projectID, TimelineID: c.timelineID, At: p.str("at"),
			MaxTokens: p.integer("max-tokens"), Width: p.integer("width"),
		})
	case "sheet":
		if p.has("at") {
			return nil, validation(
				"at",
				"--at is for --mode frame; a sheet takes --times, or --from, --until and --count.",
			)
		}

		if p.has("item") {
			return nil, validation("item", "--item is for --mode audio.")
		}

		req := service.ViewSheetRequest{
			ProjectID:  c.projectID,
			TimelineID: c.timelineID,
			From:       p.str("from"),
			Until:      p.str("until"),
			Cols:       p.integer("cols"),
			MaxTokens:  p.integer("max-tokens"),
		}
		if p.has("times") {
			req.Times = strings.Split(p.str("times"), ",")
		}

		if p.has("count") || !p.has("times") {
			req.Count = p.integer("count")
		}

		res, err = c.svc.ViewSheet(ctx, req)
	case "audio":
		for _, f := range []string{"at", "times", "count", "cols", "max-tokens", "width"} {
			if p.has(f) {
				return nil, validation(f, "--%s is for --mode frame or sheet.", f)
			}
		}

		measure, merr := c.svc.AudioMeasure(ctx, service.AudioMeasureRequest{
			ProjectID: c.projectID, TimelineID: c.timelineID, ItemID: p.str("item"),
			Start: p.str("from"), End: p.str("until"),
		})
		if merr != nil {
			return nil, merr
		}

		return &output{value: measure, text: measure.Text}, nil
	default:
		return nil, validation("mode", "--mode is frame, sheet or audio, not %q.", mode)
	}

	if err != nil {
		return nil, err
	}

	return viewOutput(res)
}

// viewOutput prints the result without the image bytes: the image is the file
// in Path. A server on another machine answers no path, so the bytes go to a
// temporary file here and that file is the path.
func viewOutput(res *service.ViewResult) (*output, error) {
	out := *res
	out.Image = nil

	if out.Path == "" && len(res.Image) > 0 {
		f, err := os.CreateTemp("", "stuarty-view-*.jpg")
		if err != nil {
			return nil, fmt.Errorf("save the image: %w", err)
		}

		_, werr := f.Write(res.Image)
		if cerr := f.Close(); werr == nil {
			werr = cerr
		}

		if werr != nil {
			return nil, fmt.Errorf("save the image: %w", werr)
		}

		out.Path = f.Name()
		out.Text += ", saved " + out.Path
	}

	return &output{value: &out, text: out.Text}, nil
}
