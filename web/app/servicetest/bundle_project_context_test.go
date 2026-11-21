package servicetest

import (
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/testproject"
)

// A bundle is how a project moves to another machine: what the creator wrote about it goes with it, and the name
// of the project stays its own when the first timeline has a title of its own.
func TestBundleKeepsTheContextOfTheProject(t *testing.T) {
	src := newMediaEnv(t, mediaCfg{})
	ctx := src.as(ui)

	made, err := testproject.Create(ctx, src.svc, "Launch teaser")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := src.prod().ProjectItemAdd(ctx, service.ProjectItemAddRequest{
		ProjectID: made.ProjectID, Kind: "timeline", Title: "Main cut", Preset: "16:9",
	}); err != nil {
		t.Fatal(err)
	}

	path := src.exportBundleRequest(service.ProjectBundleExportRequest{ProjectID: made.ProjectID})

	dst := newMediaEnv(t, mediaCfg{})

	res, err := dst.importBundle(path, true)
	if err != nil {
		t.Fatal(err)
	}

	got, err := dst.prod().ProjectDescribe(dst.as(ui), service.ProjectDescribeRequest{ProjectID: res.ProjectID})
	if err != nil {
		t.Fatal(err)
	}

	if got.Name != "Launch teaser" || got.Description != testproject.Description || got.Goal != testproject.Goal ||
		!got.Complete {
		t.Fatalf("the context did not travel: %+v", got)
	}
}

// A new project has no document for a bundle to hold: the export says so, instead of denying the project exists.
func TestBundleExportOfAProjectWithNoTimelineSaysWhy(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})

	made, err := testproject.Create(m.as(ui), m.svc, "No timeline")
	if err != nil {
		t.Fatal(err)
	}

	_, err = m.svc.(bundleExporter).ProjectBundleExport(
		m.as(ui), service.ProjectBundleExportRequest{ProjectID: made.ProjectID},
	)
	if code(err) != command.CodeInvalidState || !strings.Contains(serviceError(t, err).Hint, "timeline") {
		t.Fatalf("err = %v, want E_INVALID_STATE with the hint to add a timeline", err)
	}
}
