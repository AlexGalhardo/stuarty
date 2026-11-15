package owner

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/AlexGalhardo/vukem-video/service"
	"github.com/AlexGalhardo/vukem-video/service/workspace"
)

// ADR-0022 item 8: a new workspace lists exactly the 3 starter templates, and
// the next start of the same workspace lists 3 again.
func TestANewWorkspaceHoldsTheStarterTemplates(t *testing.T) {
	getenv := envOf(map[string]string{workspace.EnvHome: filepath.Join(t.TempDir(), "ws")})
	ctx := service.WithCaller(context.Background(), service.Caller{Actor: "local:cli", Scopes: service.AllScopes})

	for start := 1; start <= 2; start++ {
		o, err := Open(ctx, Config{Getenv: getenv, Inline: true})
		if err != nil {
			t.Fatalf("start %d: %v", start, err)
		}

		res, lerr := o.Service.TemplateList(ctx, service.TemplateListRequest{Limit: 100})
		if lerr != nil || len(res.Templates) != 3 {
			t.Errorf("start %d lists %+v, %v; want 3 templates", start, res, lerr)
		}

		if err := o.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
