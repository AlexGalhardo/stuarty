package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// route:list shows the routes of the owner, not only the health probe.
func TestRouteListShowsTheOwnerAPI(t *testing.T) {
	bin := buildBinary(t)

	cmd := stuartyCmd(t.Context(), t, bin, addrEnv(t), []string{
		"STUARTY_HOME=" + filepath.Join(t.TempDir(), "ws"),
		"APP_ENV=development",
		"APP_KEY=",
	}, "route:list")

	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("route:list: %v", err)
	}

	for _, want := range []string{
		"/api/v1/projects/{id}/edit", "/api/v1/projects/{id}/media/{asset_id}/trash", "/mcp", "/openapi.json", "/up",
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("route:list lacks %s:\n%s", want, out)
		}
	}
}
