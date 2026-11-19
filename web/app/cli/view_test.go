package cli

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
)

type viewFake struct {
	service.Service

	frame service.ViewFrameRequest
	sheet service.ViewSheetRequest
	res   *service.ViewResult
}

func (f *viewFake) ViewFrame(_ context.Context, r service.ViewFrameRequest) (*service.ViewResult, error) {
	f.frame = r
	return f.res, nil
}

func (f *viewFake) ViewSheet(_ context.Context, r service.ViewSheetRequest) (*service.ViewResult, error) {
	f.sheet = r
	return f.res, nil
}

func viewHarness(t *testing.T, res *service.ViewResult) (*harness, *viewFake) {
	t.Helper()

	f := &viewFake{res: res}
	h := newHarness(t)
	h.res.svc = f

	return h, f
}

func TestViewFrameFlagsReachTheService(t *testing.T) {
	t.Parallel()

	h, f := viewHarness(
		t,
		&service.ViewResult{Mode: "frame", Path: "/ws/frames/a.jpg", Image: []byte{1}, Text: "frame"},
	)

	if code := h.run(
		"view",
		"--project",
		"prj_7k2m9x4q1b",
		"--at",
		"1s",
		"--max-tokens",
		"300",
		"--json",
	); code != ExitOK {
		t.Fatalf("exit %d: %s%s", code, h.stdout.String(), h.stderr.String())
	}

	if f.frame.ProjectID != "prj_7k2m9x4q1b" || f.frame.At != "1s" || f.frame.MaxTokens != 300 {
		t.Errorf("request = %+v", f.frame)
	}

	if out := h.stdout.String(); strings.Contains(out, "image_base64") || !strings.Contains(out, "/ws/frames/a.jpg") {
		t.Errorf("the image bytes must not be printed, the path must: %s", out)
	}
}

func TestViewSheetUsesUntilAndTimes(t *testing.T) {
	t.Parallel()

	h, f := viewHarness(t, &service.ViewResult{Mode: "sheet", Path: "/p", Text: "sheet"})

	code := h.run(
		"view",
		"--project",
		"prj_7k2m9x4q1b",
		"--mode",
		"sheet",
		"--from",
		"1s",
		"--until",
		"3s",
		"--count",
		"5",
		"--cols",
		"2",
	)
	if code != ExitOK {
		t.Fatalf("exit %d: %s%s", code, h.stdout.String(), h.stderr.String())
	}

	if f.sheet.From != "1s" || f.sheet.Until != "3s" || f.sheet.Count != 5 || f.sheet.Cols != 2 {
		t.Errorf("request = %+v", f.sheet)
	}

	if code := h.run("view", "--project", "prj_7k2m9x4q1b", "--mode", "sheet", "--times", "0s,1s"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, h.stderr.String())
	}

	if !slices.Equal(f.sheet.Times, []string{"0s", "1s"}) || f.sheet.Count != 0 {
		t.Errorf("times request = %+v", f.sheet)
	}
}

func TestViewRejectsFlagsOfTheOtherMode(t *testing.T) {
	t.Parallel()

	h, _ := viewHarness(t, &service.ViewResult{})

	for _, args := range [][]string{
		{"--mode", "sheet", "--at", "1s"},
		{"--at", "1s", "--until", "2s"},
		{"--mode", "film"},
	} {
		if code := h.run(
			append([]string{"view", "--project", "prj_7k2m9x4q1b", "--json"}, args...)...); code != ExitFailure {
			t.Errorf("%v: exit %d, want a validation failure", args, code)
		}
	}

	if code := h.run("view", "--project", "prj_7k2m9x4q1b", "--to", "2s"); code != ExitFailure {
		t.Errorf("--to must not exist: exit %d", code)
	}
}

func TestViewFromARemoteServerSavesTheImageLocally(t *testing.T) {
	t.Parallel()

	h, _ := viewHarness(t, &service.ViewResult{Mode: "frame", Image: []byte{0xff, 0xd8, 0xff}, Text: "frame 1s"})

	if code := h.run("view", "--project", "prj_7k2m9x4q1b", "--at", "1s", "--json"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, h.stderr.String())
	}

	doc := oneJSONDocument(t, h.stdout.String())

	path, _ := doc["path"].(string)
	if path == "" {
		t.Fatalf("no path: %s", h.stdout.String())
	}

	t.Cleanup(func() { _ = os.Remove(path) })

	if b, err := os.ReadFile(path); err != nil || len(b) != 3 {
		t.Errorf("saved file: %v %v", b, err)
	}
}

// The service refuses to view an asset (VV-019), so the command does not offer
// --asset in its help; when the service can view one, the flag comes back.
func TestViewDoesNotOfferAnAsset(t *testing.T) {
	t.Parallel()

	for _, u := range view.usage {
		if strings.Contains(u, "--asset") {
			t.Errorf("usage line %q names --asset", u)
		}
	}

	for _, f := range view.flags {
		if f.Name == "asset" {
			t.Error("flag asset is still declared")
		}
	}
}
