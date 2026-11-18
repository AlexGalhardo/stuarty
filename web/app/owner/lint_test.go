package owner

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/service"
	"github.com/AlexGalhardo/vukem-video/service/workspace"
)

func shape(track string, size int, id string) string {
	return fmt.Sprintf(
		`{"track_id":%q,"start":"0s","duration":"2s","shape":"rect","width":%d,"height":%d,"id":%q}`,
		track, size, size, id,
	)
}

func op(name, args string) command.Operation {
	return command.Operation{Op: name, Args: json.RawMessage(args)}
}

// The owner gives the service the rasterizer's measurer, so the rules that need
// the box of a text (L_SAFE_AREA, L_OFF_FRAME) fire in the real app, and the
// document rule L_OCCLUDED still does.
func TestServiceLintUsesTheRasterizerMeasurer(t *testing.T) {
	getenv := envOf(map[string]string{workspace.EnvHome: filepath.Join(t.TempDir(), "ws")})
	ctx := service.WithCaller(context.Background(), service.Caller{Actor: "local:cli", Scopes: service.AllScopes})

	o, err := Open(ctx, Config{Getenv: getenv, Inline: true})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = o.Close() })

	created, serr := o.Service.ProjectCreate(ctx, service.ProjectCreateRequest{Name: "lint"})
	if serr != nil {
		t.Fatal(serr)
	}

	edit, serr := o.Service.Edit(ctx, service.EditRequest{
		ProjectID: created.ProjectID, BaseRevision: 0,
		Operations: []command.Operation{
			op("track.add", `{"kind":"visual","name":"Base","id":"trk_f6g7h8j9k0"}`),
			op("track.add", `{"kind":"visual","name":"Top","id":"trk_m1n2p3q4r5"}`),
			op("shape.add", shape("trk_f6g7h8j9k0", 100, "itm_a1b2c3d4e5")),
			op("shape.add", shape("trk_m1n2p3q4r5", 9999, "itm_f6g7h8j9k0")),
			op(
				"text.add",
				`{"track_id":"trk_m1n2p3q4r5","start":"2s","duration":"1s","text":"Edge of the frame",`+
					`"x":20,"y":20,"id":"itm_b2c3d4e5f6"}`,
			),
		},
	})
	if serr != nil {
		t.Fatal(serr)
	}

	res, serr := o.Service.Lint(ctx, service.LintRequest{ProjectID: created.ProjectID, Limit: 100})
	if serr != nil {
		t.Fatal(serr, edit)
	}

	for _, code := range []string{"L_OCCLUDED", "L_OFF_FRAME", "L_SAFE_AREA"} {
		if !strings.Contains(res.Text, code) {
			t.Errorf("the lint lacks %s:\n%s", code, res.Text)
		}
	}
}
