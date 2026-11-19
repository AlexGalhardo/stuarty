package cli

import (
	"context"

	"github.com/AlexGalhardo/stuarty/service"
)

var revertCmd = &spec{
	name:     "revert",
	timeline: true,
	summary:  "go back to the document of an earlier revision, as a new step that undo takes back",
	usage: []string{
		"stuarty revert --project <id> (--revision n | --checkpoint name) [--base n] [--dry-run]",
	},
	example: "stuarty revert --project prj_7k2m9x4q1b --checkpoint \"before the intro\" --dry-run",
	project: true,
	globals: []string{"dry-run"},
	write:   true,
	flags: []flagDef{
		{Name: "revision", Kind: kindInt, Usage: "the revision to go back to"},
		{Name: "checkpoint", Kind: kindString, Usage: "the name of a checkpoint to go back to"},
		{
			Name:  "base",
			Kind:  kindInt,
			Usage: "the revision you read (default the current one); an older one conflicts with other actors' changes",
		},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.HistoryRevert(ctx, service.HistoryRevertRequest{
			ProjectID: c.projectID, TimelineID: c.timelineID, Revision: int64(c.args.integer("revision")),
			Checkpoint: c.args.str("checkpoint"), BaseRevision: int64(c.args.integer("base")),
			DryRun: c.args.boolean("dry-run"),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

var diffCmd = &spec{
	name:     "diff",
	timeline: true,
	summary:  "list what changed between two revisions, most significant first",
	usage:    []string{"stuarty diff --project <id> (--from n | --from-checkpoint name) [--until n]"},
	example:  "stuarty diff --project prj_7k2m9x4q1b --from 12",
	project:  true,
	flags: []flagDef{
		{Name: "from", Kind: kindInt, Usage: "the revision the diff starts from"},
		{Name: "from-checkpoint", Kind: kindString, Usage: "the name of a checkpoint the diff starts from"},
		{Name: "until", Kind: kindInt, Usage: "the revision the diff ends at (default the current one)"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.HistoryDiff(ctx, service.HistoryDiffRequest{
			ProjectID: c.projectID, TimelineID: c.timelineID, From: int64(c.args.integer("from")),
			FromCheckpoint: c.args.str("from-checkpoint"), Until: int64(c.args.integer("until")),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

var checkpointCreateCmd = &spec{
	name:     "checkpoint:create",
	timeline: true,
	summary:  "name a revision so that it can be gone back to",
	usage:    []string{"stuarty checkpoint:create --project <id> --name <name> [--note t] [--revision n]"},
	example:  "stuarty checkpoint:create --project prj_7k2m9x4q1b --name \"before the intro\"",
	project:  true,
	write:    true,
	flags: []flagDef{
		{Name: "name", Kind: kindString, Usage: "1 to 60 characters, no slash"},
		{Name: "note", Kind: kindString, Usage: "up to 300 characters"},
		{Name: "revision", Kind: kindInt, Usage: "the revision to name (default the current one)"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.CheckpointCreate(ctx, service.CheckpointCreateRequest{
			ProjectID: c.projectID, TimelineID: c.timelineID, Name: c.args.str("name"), Note: c.args.str("note"),
			Revision: int64(c.args.integer("revision")),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: service.CheckpointText("created", res)}, nil
	},
}

var checkpointListCmd = &spec{
	name:     "checkpoint:list",
	timeline: true,
	summary:  "list the checkpoints of a project, newest revision first",
	usage:    []string{"stuarty checkpoint:list --project <id>"},
	example:  "stuarty checkpoint:list --project prj_7k2m9x4q1b --json",
	project:  true,
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.CheckpointList(ctx, service.CheckpointListRequest{
			ProjectID: c.projectID, TimelineID: c.timelineID,
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}

var checkpointRenameCmd = &spec{
	name:     "checkpoint:rename",
	timeline: true,
	summary:  "rename a checkpoint, and change its note",
	usage:    []string{"stuarty checkpoint:rename --project <id> --name <name> --new-name <name> [--note t]"},
	example:  "stuarty checkpoint:rename --project prj_7k2m9x4q1b --name draft --new-name \"first cut\"",
	project:  true,
	write:    true,
	flags: []flagDef{
		{Name: "name", Kind: kindString, Usage: "the checkpoint to rename"},
		{Name: "new-name", Kind: kindString, Usage: "the new name"},
		{Name: "note", Kind: kindString, Usage: "replaces the note when given"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		req := service.CheckpointRenameRequest{
			ProjectID: c.projectID, TimelineID: c.timelineID, Name: c.args.str("name"), NewName: c.args.str("new-name"),
		}

		if c.args.has("note") {
			note := c.args.str("note")
			req.Note = &note
		}

		res, err := c.svc.CheckpointRename(ctx, req)
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: service.CheckpointText("renamed", res)}, nil
	},
}

var checkpointRemoveCmd = &spec{
	name:     "checkpoint:remove",
	timeline: true,
	summary:  "remove a checkpoint; no revision is removed",
	usage:    []string{"stuarty checkpoint:remove --project <id> --name <name>"},
	example:  "stuarty checkpoint:remove --project prj_7k2m9x4q1b --name draft",
	project:  true,
	write:    true,
	flags:    []flagDef{{Name: "name", Kind: kindString, Usage: "the checkpoint to remove"}},
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.CheckpointRemove(ctx, service.CheckpointRemoveRequest{
			ProjectID: c.projectID, TimelineID: c.timelineID, Name: c.args.str("name"),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: service.CheckpointText("removed", res)}, nil
	},
}
