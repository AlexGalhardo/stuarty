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

// ADR-0026 item 4, T-10: a default start constructs no OpenTelemetry exporter;
// the endpoint turns it on, and it is shut down with the owner.
func TestTelemetryIsBuiltOnlyWhenTheEndpointIsSet(t *testing.T) {
	var built, stopped int

	hook := func(context.Context, func(string) string) (func(context.Context) error, error) {
		built++

		return func(context.Context) error { stopped++; return nil }, nil
	}

	for name, endpoint := range map[string]string{"default": "", "endpoint": "http://collector.example:4318"} {
		built, stopped = 0, 0
		env := map[string]string{workspace.EnvHome: filepath.Join(t.TempDir(), "ws")}

		if endpoint != "" {
			env[EnvOTLPEndpoint] = endpoint
		}

		o, err := Open(context.Background(), Config{Getenv: envOf(env), Inline: true, Telemetry: hook})
		if err != nil {
			t.Fatal(err)
		}

		if want := map[string]int{"default": 0, "endpoint": 1}[name]; built != want {
			t.Errorf("%s: %d exporter constructions, want %d", name, built, want)
		}

		if err := o.Close(); err != nil {
			t.Fatal(err)
		}

		if stopped != built {
			t.Errorf("%s: %d shutdowns for %d exporters", name, stopped, built)
		}
	}
}
