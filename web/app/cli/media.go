package cli

// The media and job commands of SDD-003 section 5.4. Each handler parses, calls
// Service methods and formats; waiting is a loop over the wait_ms of the
// Service, which is capped at 25 seconds per call.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/textview"
)

// maxServiceWait is the longest wait_ms the Service accepts.
const maxServiceWait = 25 * time.Second

// waitSlice is the wait_ms of the next call: what is left of the deadline,
// at most what the Service accepts.
func waitSlice(deadline time.Time) int {
	left := min(time.Until(deadline), maxServiceWait)
	if left <= 0 {
		return 0
	}

	return int(left.Milliseconds())
}

var mediaImport = &spec{
	name:     "media:import",
	timeline: true,
	summary:  "import media files into a project",
	usage: []string{
		"stuarty media:import <path>... --project <id> [--asset-id ast_...] [--wait] [--timeout d]",
		"stuarty media:import --hash sha256:... --project <id>   (a file the library already holds: no copy)",
		"origin and reuse: [--origin-kind k] [--origin-source s] [--provider p] [--model m] [--prompt t] [--rights-note t] [--engine e] [--engine-version v] [--render-key k] [--tag t] [--reusable] [--description t]",
		"formats: MP4, MOV, MKV, WebM, AVI, MP3, WAV, FLAC, OGG, PNG, JPEG, WebP, GIF and .cube; not SVG",
	},
	example:    "stuarty media:import clip.mov --project prj_7k2m9x4q1b --json",
	project:    true,
	positional: true,
	globals:    []string{"wait", "timeout"},
	write:      true,
	flags: append([]flagDef{
		{Name: "asset-id", Kind: kindString, Repeat: true, Usage: "id to give the asset, once per path, in order"},
	}, libraryFlags()...),
	run: runMediaImport,
}

func runMediaImport(ctx context.Context, c *call) (*output, error) {
	paths := c.args.Positional
	hashes := c.args.all("hash")

	if len(paths)+len(hashes) == 0 {
		return nil, validation("", "media:import needs at least one path or --hash.")
	}

	ids := c.args.all("asset-id")
	if len(ids) > 0 && len(ids) != len(paths)+len(hashes) {
		return nil, validation(
			"asset-id",
			"--asset-id must be given once per file: %d files, %d ids.",
			len(paths)+len(hashes),
			len(ids),
		)
	}

	files := make([]service.ImportFile, len(paths)+len(hashes))

	for i, p := range paths {
		abs, err := c.rt.abs(p)
		if err != nil {
			return nil, err
		}

		files[i].Path = abs

		if len(ids) > 0 {
			files[i].AssetID = ids[i]
		}
	}

	for i, h := range hashes {
		files[len(paths)+i].Hash = h

		if len(ids) > 0 {
			files[len(paths)+i].AssetID = ids[len(paths)+i]
		}
	}

	if err := c.noWaitInProcess(); err != nil {
		return nil, err
	}

	deadline := time.Now().Add(c.args.timeout())
	wait := c.args.boolean("wait")
	p := c.progress()

	req := service.MediaImportRequest{ProjectID: c.projectID, TimelineID: c.timelineID, Files: files}
	req.Origin, req.Info = importBlocks(c.args)

	var first *service.MediaImportResult

	if c.inProcess && wait {
		// The Service ingests inside the call: bound it by --timeout.
		res, expired, err := inline(ctx, deadline, c.watchAssets(p, len(files)), nil,
			func(ctx context.Context) (*service.MediaImportResult, error) { return c.svc.MediaImport(ctx, req) })
		if expired {
			return c.importExpired(ctx)
		}

		if err != nil {
			return nil, err
		}

		first = res
	} else {
		if wait {
			req.WaitMS = p.slice(deadline)
		}

		var err error
		if first, err = c.svc.MediaImport(ctx, req); err != nil {
			return nil, err
		}
	}

	res := first
	p.assets(importDone(res), len(res.Assets), 0)

	for wait && !c.inProcess && importPending(res) && time.Until(deadline) > 0 {
		req.WaitMS = p.slice(deadline)

		var err error

		// The same files again return the same assets (import is idempotent).
		if res, err = c.svc.MediaImport(ctx, req); err != nil {
			return nil, err
		}

		p.assets(importDone(res), len(res.Assets), 0)
	}

	if res != first {
		settleImport(res, first)
	}

	out := &output{value: res, text: res.Text}

	switch {
	case wait && importPending(res):
		out.exit = ExitTransient
		out.text += "\nstill running: run stuarty media:list --project " + c.projectID
	case res.Failed > 0:
		out.exit = ExitFailure
	}

	return out, nil
}

func importPending(r *service.MediaImportResult) bool {
	for i := range r.Assets {
		a := &r.Assets[i]
		if a.Error == nil && (a.State == "pending" || a.State == "ingesting") {
			return true
		}
	}

	return false
}

// settleImport makes the last answer of a wait loop read as the first call: the
// assets were new then, and the text is drawn again.
func settleImport(res, first *service.MediaImportResult) {
	lines := make([]textview.ImportLine, len(res.Assets))

	for i := range res.Assets {
		a := &res.Assets[i]
		a.Deduplicated = first.Assets[i].Deduplicated

		lines[i] = textview.ImportLine{
			ID: a.AssetID, Kind: a.Kind, State: a.State, Name: a.Name, Deduplicated: a.Deduplicated, JobID: a.JobID,
		}

		if a.Error != nil {
			lines[i].ErrorCode = a.Error.Code
			lines[i].ErrorMessage = a.Error.Message
			lines[i].ErrorHint = a.Error.Hint
		}
	}

	res.Text = textview.Import(lines)
}

var mediaInspect = &spec{
	name:     "media:inspect",
	timeline: true,
	summary:  "show what is known about an asset",
	usage: []string{
		"stuarty media:inspect <asset id> --project <id> [--sections probe,transcript,silences,loudness]",
	},
	example:    "stuarty media:inspect ast_3c5v8n2w6d --project prj_7k2m9x4q1b --sections probe,silences",
	project:    true,
	positional: true,
	flags: append([]flagDef{
		{Name: "sections", Kind: kindString, Default: "probe", Usage: "probe, transcript, silences, loudness, beats"},
		{Name: "min-silence", Kind: kindString, Default: "0.4s", Usage: "shortest silence to report"},
	}, pageFlags("")...),
	run: func(ctx context.Context, c *call) (*output, error) {
		if len(c.args.Positional) != 1 {
			return nil, validation("", "media:inspect takes one asset id, got %d.", len(c.args.Positional))
		}

		res, err := c.svc.MediaInspect(ctx, service.MediaInspectRequest{
			ProjectID:  c.projectID,
			TimelineID: c.timelineID,
			AssetID:    c.args.Positional[0],
			Sections:   splitList(c.args.str("sections")),
			MinSilence: c.args.str("min-silence"),
			Page:       page(c.args),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

func splitList(s string) []string {
	var out []string

	for part := range strings.SplitSeq(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}

	return out
}

func mediaStateCommand(
	name, summary string,
	fn func(service.Service, context.Context, service.MediaTrashRequest) (*service.MediaStateResult, error),
) *spec {
	return &spec{
		name:       name,
		timeline:   true,
		summary:    summary,
		usage:      []string{"stuarty " + name + " <asset id> --project <id>"},
		example:    "stuarty " + name + " ast_3c5v8n2w6d --project prj_7k2m9x4q1b",
		project:    true,
		positional: true,
		globals:    []string{"dry-run"},
		write:      true,
		run: func(ctx context.Context, c *call) (*output, error) {
			if len(c.args.Positional) != 1 {
				return nil, validation("", "%s takes one asset id, got %d.", name, len(c.args.Positional))
			}

			res, err := fn(c.svc, ctx, service.MediaTrashRequest{
				ProjectID:  c.projectID,
				TimelineID: c.timelineID,
				AssetID:    c.args.Positional[0],
				DryRun:     c.args.boolean("dry-run"),
			})
			if err != nil {
				return nil, err
			}

			return &output{value: res, text: res.Text}, nil
		},
	}
}

var mediaTrash = mediaStateCommand("media:trash", "move an asset to the trash",
	func(s service.Service, ctx context.Context, r service.MediaTrashRequest) (*service.MediaStateResult, error) {
		return s.MediaTrash(ctx, r)
	})

var mediaRestore = mediaStateCommand("media:restore", "restore a trashed asset",
	func(s service.Service, ctx context.Context, r service.MediaTrashRequest) (*service.MediaStateResult, error) {
		return s.MediaRestore(ctx, r)
	})

var mediaList = &spec{
	name:     "media:list",
	timeline: true,
	summary:  "list the assets of a project",
	usage:    []string{"stuarty media:list --project <id> [--cursor c] [--limit n]"},
	example:  "stuarty media:list --project prj_7k2m9x4q1b --json",
	project:  true,
	flags:    pageFlags("50"),
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.MediaList(
			ctx,
			service.MediaListRequest{ProjectID: c.projectID, TimelineID: c.timelineID, Page: page(c.args)},
		)
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

var export = &spec{
	name:     "export",
	timeline: true,
	summary:  "export a project to a file",
	usage: []string{
		// The first line is the one of the agent skill, whose size is capped: --timeout is global.
		"stuarty export --project <id> [--start t] [--end t] [--format f] [--at t] [--preset p] [--codec c] " +
			"[--wait] [--out f] [--force]",
	},
	example: "stuarty export --project prj_7k2m9x4q1b --json",
	project: true,
	globals: []string{"wait", "timeout"},
	write:   true,
	flags: append([]flagDef{
		{Name: "start", Kind: kindString, Usage: "first time to export (default: the start)"},
		{Name: "end", Kind: kindString, Usage: "end of the export (default: the end of the timeline)"},
		{Name: "at", Kind: kindString, Usage: "the time of the frame, only with --format png (default: the start)"},
		{Name: "preset", Kind: kindString, Usage: "a platform preset, as export:presets lists them (mp4 only)"},
		{Name: "codec", Kind: kindString, Usage: "h264 (default), hevc or av1 (mp4 only)"},
		{
			Name:  "format",
			Kind:  kindString,
			Usage: service.ExportFormats[0] + " (default), " + service.JoinOr(service.ExportFormats[1:]),
		},
	}, outFlags()...),
	run: func(ctx context.Context, c *call) (*output, error) {
		dest, err := c.outPath()
		if err != nil {
			return nil, err
		}

		req := service.JobStartRequest{ProjectID: c.projectID, TimelineID: c.timelineID, Kind: service.JobExport}

		if c.args.has("start") || c.args.has("end") || c.args.has("format") || c.args.has("at") ||
			c.args.has("preset") || c.args.has("codec") {
			raw, err := json.Marshal(service.ExportArgs{
				Start: c.args.str("start"), End: c.args.str("end"), Format: c.args.str("format"), At: c.args.str("at"),
				Preset: c.args.str("preset"), Codec: c.args.str("codec"),
			})
			if err != nil {
				return nil, err
			}

			req.Args = raw
		}

		deadline := time.Now().Add(c.args.timeout())
		wait := c.args.boolean("wait") || dest != ""

		out, err := c.runJob(ctx, wait, deadline, func(ctx context.Context, waitMS int) (*service.Job, error) {
			req.WaitMS = waitMS

			return c.svc.JobStart(ctx, req)
		})
		if err != nil {
			return nil, err
		}

		return c.saveJobFile(ctx, out, dest)
	},
}

// jobOutput waits for the job when asked and reports it. A job that failed or
// was cancelled is exit 1; a wait that ran out is exit 2 (SDD-003 section 5.1).
func jobOutput(
	ctx context.Context, c *call, job *service.Job, wait bool, deadline time.Time, p *progress,
) (*output, error) {
	p.job(job)

	for wait && !service.JobFinal(job.State) && time.Until(deadline) > 0 {
		var err error

		job, err = c.svc.JobGet(ctx, service.JobGetRequest{JobID: job.JobID, WaitMS: p.slice(deadline)})
		if err != nil {
			return nil, err
		}

		p.job(job)
	}

	out := &output{value: job, text: job.Text}

	switch {
	case !service.JobFinal(job.State) && wait:
		out.exit = ExitTransient
		out.text = strings.TrimRight(stripStill(job.Text), "\n") +
			fmt.Sprintf("\nstill running: run stuarty job:show %s --wait", job.JobID)
	case job.State == service.JobFailed || job.State == service.JobCancelled:
		out.exit = ExitFailure
	}

	return out, nil
}

// stripStill drops the "still <state>" line the Service adds after a wait; the
// CLI has its own wording.
func stripStill(text string) string {
	head, _, found := strings.CutLast(text, "\nstill ")
	if !found {
		return text
	}

	return head
}

var jobCancel = &spec{
	name:       "job:cancel",
	summary:    "cancel a queued or running job",
	usage:      []string{"stuarty job:cancel <job id>"},
	example:    "stuarty job:cancel job_4h8k2m6p9t",
	positional: true,
	write:      true,
	run: func(ctx context.Context, c *call) (*output, error) {
		if len(c.args.Positional) != 1 {
			return nil, validation("", "job:cancel takes one job id, got %d.", len(c.args.Positional))
		}

		job, err := c.svc.JobCancel(ctx, service.JobCancelRequest{JobID: c.args.Positional[0]})
		if err != nil {
			return nil, err
		}

		return &output{value: job, text: job.Text}, nil
	},
}

var jobShow = &spec{
	name:       "job:show",
	summary:    "show a job",
	usage:      []string{"stuarty job:show <job id> [--wait] [--timeout d]"},
	example:    "stuarty job:show job_4h8k2m6p9t --wait",
	positional: true,
	globals:    []string{"timeout"},
	flags: []flagDef{
		{
			Name:    "wait",
			Kind:    kindBool,
			Default: "false",
			Usage:   "block until the job is final (--no-wait to return at once)",
		},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		if len(c.args.Positional) != 1 {
			return nil, validation("", "job:show takes one job id, got %d.", len(c.args.Positional))
		}

		deadline := time.Now().Add(c.args.timeout())
		wait := c.args.boolean("wait")

		req := service.JobGetRequest{JobID: c.args.Positional[0]}
		if wait {
			req.WaitMS = waitSlice(deadline)
		}

		job, err := c.svc.JobGet(ctx, req)
		if err != nil {
			return nil, err
		}

		return jobOutput(ctx, c, job, wait, deadline, c.progress())
	},
}

var jobList = &spec{
	name:     "job:list",
	timeline: true,
	summary:  "list the jobs of a project",
	usage:    []string{"stuarty job:list --project <id> [--state s] [--cursor c] [--limit n]"},
	example:  "stuarty job:list --project prj_7k2m9x4q1b --state active",
	project:  true,
	flags: append([]flagDef{
		{Name: "state", Kind: kindString, Usage: "queued, running, succeeded, failed, cancelled, active or all"},
	}, pageFlags("20")...),
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.JobList(ctx, service.JobListRequest{
			ProjectID: c.projectID, TimelineID: c.timelineID, State: c.args.str("state"), Page: page(c.args),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}
