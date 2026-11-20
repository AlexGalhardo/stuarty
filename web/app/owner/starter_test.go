package owner

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/AlexGalhardo/stuarty/internal/outbound"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/workspace"

	vukem "github.com/AlexGalhardo/vukem"
)

// ADR-0022 item 8 and SDD-018 section 17: a new workspace lists the 3 starter templates and the 3 templates of
// the recipes, and the next start of the same workspace lists 6 again.
func TestANewWorkspaceHoldsTheStarterTemplates(t *testing.T) {
	getenv := envOf(map[string]string{workspace.EnvHome: filepath.Join(t.TempDir(), "ws")})
	ctx := service.WithCaller(context.Background(), service.Caller{Actor: "local:cli", Scopes: service.AllScopes})

	for start := 1; start <= 2; start++ {
		o, err := Open(ctx, Config{Getenv: getenv, Inline: true})
		if err != nil {
			t.Fatalf("start %d: %v", start, err)
		}

		res, lerr := o.Service.TemplateList(ctx, service.TemplateListRequest{Limit: 100})
		if lerr != nil || len(res.Templates) != 6 {
			t.Errorf("start %d lists %+v, %v; want 6 templates", start, res, lerr)
		}

		if err := o.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

// ADR-0026 item 4, T-10: a default start constructs no OpenTelemetry exporter;
// the endpoint turns it on, and the application gets the provider to run.
func TestTelemetryIsBuiltOnlyWhenTheEndpointIsSet(t *testing.T) {
	var built int

	hook := func(string) vukem.Provider {
		built++

		return OTelProvider("test")
	}

	for name, endpoint := range map[string]string{"default": "", "endpoint": "http://collector.example:4318"} {
		built = 0
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

		if got := o.TelemetryProvider() != nil; got != (built == 1) {
			t.Errorf("%s: provider present = %v with %d constructions", name, got, built)
		}

		if err := o.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

// T-10: a default start with the real constructor builds no provider and opens
// no connection of any purpose.
func TestDefaultStartWithTheOTelConstructorDialsNothing(t *testing.T) {
	var dials int

	restore := outbound.SetDialRecorder(func(outbound.Purpose, string) error { dials++; return nil })
	t.Cleanup(restore)

	env := map[string]string{workspace.EnvHome: filepath.Join(t.TempDir(), "ws")}

	o, err := Open(context.Background(), Config{Getenv: envOf(env), Inline: true, Telemetry: OTelProvider})
	if err != nil {
		t.Fatal(err)
	}

	if o.TelemetryProvider() != nil || dials != 0 {
		t.Errorf("provider = %v, dials = %d; want none", o.TelemetryProvider(), dials)
	}

	if err := o.Close(); err != nil {
		t.Fatal(err)
	}
}
