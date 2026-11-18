package cli

import (
	"context"
	"os"

	"github.com/AlexGalhardo/vukem-video/service/workspace"
)

var tokenRotate = &spec{
	name:    "token:rotate",
	summary: "replace the local token; clients that hold the old one must be set up again",
	usage:   []string{"vukem-video token:rotate"},
	example: "vukem-video token:rotate",
	offline: true,
	write:   true,
	run:     runTokenRotate,
}

// tokenRotateDoc is the --json document of token:rotate. It never carries the
// token: the new one is in the token file of the workspace, readable by its owner
// only, and nowhere else (SDD-007 section 5.3).
type tokenRotateDoc struct {
	Rotated bool   `json:"rotated"`
	Text    string `json:"text"`
}

// runTokenRotate takes the workspace lock, so it refuses while a server owns the
// workspace (E_WORKSPACE_LOCKED), and replaces the token file.
func runTokenRotate(ctx context.Context, c *call) (*output, error) {
	getenv := c.rt.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}

	if dir := c.args.str("workspace"); dir != "" {
		base := getenv

		getenv = func(k string) string {
			if k == workspace.EnvHome {
				return dir
			}

			return base(k)
		}
	}

	sess, err := workspace.Open(ctx, workspace.Options{Force: workspace.InProcess, Getenv: getenv})
	if err != nil {
		return nil, err
	}

	defer func() { _ = sess.Close() }()

	if err := sess.Dir.EnsureLayout(); err != nil {
		return nil, err
	}

	if _, err := sess.Dir.RotateToken(); err != nil {
		return nil, err
	}

	text := "rotated the local token; browser sessions end and MCP clients that hold the old token must be set up again"

	return &output{value: tokenRotateDoc{Rotated: true, Text: text}, text: text}, nil
}
