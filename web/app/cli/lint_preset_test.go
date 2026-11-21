package cli

import (
	"context"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
)

var lintCheckReq service.ExportCheckRequest

func (f *fakeService) ExportCheck(_ context.Context, r service.ExportCheckRequest) (*service.ExportCheckResult, error) {
	lintCheckReq = r

	return &service.ExportCheckResult{Text: "fit check"}, nil
}

// VV-317: lint takes preset, start and end as the lint tool of MCP does, and a range with no preset is refused.
func TestLintTakesThePresetAndItsRange(t *testing.T) {
	h := newHarness(t)

	if code := h.run("lint", "--project", "prj_a", "--preset", "tiktok", "--start", "1", "--end", "2"); code != ExitOK {
		t.Fatalf("lint --preset exit = %d, stderr %s", code, h.stderr.String())
	}

	if lintCheckReq.Preset != "tiktok" || lintCheckReq.Start != "1" || lintCheckReq.End != "2" {
		t.Errorf("the fit check got %+v", lintCheckReq)
	}

	if code := h.run("lint", "--project", "prj_a", "--start", "1"); code == ExitOK {
		t.Error("--start without --preset was accepted")
	}
}
