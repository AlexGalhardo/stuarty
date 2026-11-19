package owner

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/workspace"
	"github.com/AlexGalhardo/stuarty/web/app/agentdocs"
)

// An owner opened without a version (the in-process CLI) reports the one
// version of the binary, not an empty string (VV-046).
func TestOwnerDefaultsToTheBinaryVersion(t *testing.T) {
	getenv := envOf(map[string]string{workspace.EnvHome: filepath.Join(t.TempDir(), "ws")})
	ctx := service.WithCaller(context.Background(), service.Caller{Actor: "local:cli", Scopes: service.AllScopes})

	o, err := Open(ctx, Config{Getenv: getenv, Inline: true})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = o.Close() })

	res, serr := o.Service.Doctor(ctx, service.DoctorRequest{})
	if serr != nil {
		t.Fatal(serr)
	}

	if res.Version != agentdocs.Version {
		t.Errorf("doctor version = %q, want %q", res.Version, agentdocs.Version)
	}
}
