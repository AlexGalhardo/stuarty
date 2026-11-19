package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/AlexGalhardo/stuarty/service"
)

// The OpenTimelineIO commands have no MCP tool: a timeline moves as a file, which is for the person
// or the script at the terminal (SDD-003 section 7.1, exception E-3).

// otioExportDoc is what otio:export prints: the file it wrote, not the file itself.
type otioExportDoc struct {
	ProjectID string             `json:"project_id"`
	Revision  int64              `json:"revision"`
	File      string             `json:"file"`
	Bytes     int                `json:"bytes"`
	Lost      []service.OtioLost `json:"lost"`
	Text      string             `json:"text"`
}

var otioExport = &spec{
	name:     "otio:export",
	timeline: true,
	summary:  "write the timeline of a project as an OpenTimelineIO file",
	usage:    []string{"stuarty otio:export --project <id> [--out f] [--force]"},
	example:  "stuarty otio:export --project prj_7k2m9x4q1b --out cut.otio",
	project:  true,
	flags: []flagDef{
		{
			Name:  "out",
			Kind:  kindString,
			Usage: "the file to write (default: the name of the project, in the current directory)",
		},
		{Name: "force", Kind: kindBool, Default: "false", Usage: "replace the file when it exists"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.ProjectOtioExport(
			ctx,
			service.ProjectOtioExportRequest{ProjectID: c.projectID, TimelineID: c.timelineID},
		)
		if err != nil {
			return nil, err
		}

		// A path the person typed may create folders; a name the server chose never leaves the
		// current directory (SDD-007 T-13).
		name, typed := c.args.str("out"), c.args.has("out")
		if !typed {
			if !safeOtioName(res.FileName) {
				return nil, validation("out", "the server named the file in a way the CLI does not accept; give --out.")
			}

			name = res.FileName
		}

		path, err := c.rt.abs(name)
		if err != nil {
			return nil, err
		}

		mode := os.O_WRONLY | os.O_CREATE | os.O_EXCL
		if c.args.boolean("force") {
			mode = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
		}

		if err := writeFile(path, []byte(res.Content), mode, typed); err != nil {
			if errors.Is(err, os.ErrExist) {
				return nil, validation("out", "%s exists; give another --out or --force.", path)
			}

			return nil, err
		}

		text := fmt.Sprintf("wrote %s (%d bytes, revision %d)\n%s", path, len(res.Content), res.Revision, res.Text)

		return &output{value: otioExportDoc{
			ProjectID: res.ProjectID, Revision: res.Revision, File: path, Bytes: len(res.Content),
			Lost: res.Lost, Text: text,
		}, text: text}, nil
	},
}

// safeOtioName reports whether a file name that came from the server is one plain ".otio" name: no
// separator, drive or stream colon, no control character, not "." or "..", not only the extension.
func safeOtioName(name string) bool {
	if name == "" || name == "." || name == ".." || name == ".otio" || !strings.HasSuffix(name, ".otio") ||
		name != filepath.Base(name) || strings.ContainsAny(name, `/\:`) {
		return false
	}

	return !strings.ContainsFunc(name, unicode.IsControl)
}

// writeFile writes a file the person asked for, with the flags the caller chose. Folders are created
// only for a path the person typed.
func writeFile(path string, data []byte, mode int, makeFolders bool) error {
	if makeFolders {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return fmt.Errorf("create the folder of %s: %w", path, err)
		}
	}

	f, err := os.OpenFile(path, mode, 0o600)
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	if _, err := f.Write(data); err != nil {
		_ = f.Close()

		return fmt.Errorf("write %s: %w", path, err)
	}

	if err := f.Close(); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	return nil
}

var otioImport = &spec{
	name:       "otio:import",
	timeline:   true,
	summary:    "apply an OpenTimelineIO file to a project as one undo step",
	usage:      []string{"stuarty otio:import <path> --project <id> [--base-revision n]"},
	example:    "stuarty otio:import cut.otio --project prj_7k2m9x4q1b --dry-run",
	project:    true,
	positional: true,
	globals:    []string{"dry-run"},
	write:      true,
	flags: []flagDef{
		{Name: "base-revision", Kind: kindInt, Usage: "the revision the file is applied to (default: the current one)"},
		{
			Name: "allow-partial", Kind: kindBool, Default: "false",
			Usage: "import the rest when the file refers to media the project does not have (default: refuse)",
		},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		if len(c.args.Positional) != 1 {
			return nil, validation("", "otio:import takes one path, got %d.", len(c.args.Positional))
		}

		path, err := c.rt.abs(c.args.Positional[0])
		if err != nil {
			return nil, err
		}

		req := service.ProjectOtioImportRequest{
			ProjectID: c.projectID, TimelineID: c.timelineID, Path: path, DryRun: c.args.boolean("dry-run"),
			AllowPartial: c.args.boolean("allow-partial"),
		}

		if c.args.has("base-revision") {
			req.BaseRevision = int64(c.args.integer("base-revision"))
		} else {
			cur, gerr := c.svc.ProjectGet(
				ctx,
				service.ProjectGetRequest{ProjectID: c.projectID, TimelineID: c.timelineID},
			)
			if gerr != nil {
				return nil, gerr
			}

			req.BaseRevision = cur.Revision
		}

		res, err := c.svc.ProjectOtioImport(ctx, req)
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: res.Text}, nil
	},
}
