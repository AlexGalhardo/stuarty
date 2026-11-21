package cli

import (
	"context"
	"slices"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/asset"
	"github.com/AlexGalhardo/stuarty/service"
)

type assetFake struct {
	service.Service

	list   service.AssetListRequest
	set    service.AssetSetInfoRequest
	tag    service.AssetTagRequest
	rename service.AssetRenameRequest
	get    service.AssetRef
	uses   service.AssetUsesRequest
	upload service.MediaImportRequest
}

func (f *assetFake) AssetList(_ context.Context, r service.AssetListRequest) (*service.AssetListResult, error) {
	f.list = r

	return &service.AssetListResult{Text: "list"}, nil
}

func (f *assetFake) AssetGet(_ context.Context, r service.AssetRef) (*service.AssetResult, error) {
	f.get = r

	return &service.AssetResult{Text: "one"}, nil
}

func (f *assetFake) AssetUses(_ context.Context, r service.AssetUsesRequest) (*service.AssetUsesResult, error) {
	f.uses = r

	return &service.AssetUsesResult{Text: "uses"}, nil
}

func (f *assetFake) AssetSetInfo(_ context.Context, r service.AssetSetInfoRequest) (*service.AssetResult, error) {
	f.set = r

	return &service.AssetResult{Text: "set"}, nil
}

func (f *assetFake) AssetTag(_ context.Context, r service.AssetTagRequest) (*service.AssetResult, error) {
	f.tag = r

	return &service.AssetResult{Text: "tag"}, nil
}

func (f *assetFake) AssetRename(_ context.Context, r service.AssetRenameRequest) (*service.AssetResult, error) {
	f.rename = r

	return &service.AssetResult{Text: "renamed"}, nil
}

func (f *assetFake) MediaImport(_ context.Context, r service.MediaImportRequest) (*service.MediaImportResult, error) {
	f.upload = r

	return &service.MediaImportResult{}, nil
}

func TestAssetCommandsSendWhatTheFlagsSay(t *testing.T) {
	t.Parallel()

	f := &assetFake{}
	h := newHarness(t)
	h.res.svc = f

	hash := "sha256:" + "0000000000000000000000000000000000000000000000000000000000000000"

	if code := h.run(
		"asset:list",
		"--tag",
		"logo",
		"--reusable",
		"--section",
		"visual",
		"--project",
		"prj_a",
		"--text",
		"cat",
	); code != ExitOK ||
		f.list.Tag != "logo" || f.list.Reusable == nil || !*f.list.Reusable || f.list.Section != "visual" ||
		f.list.Project != "prj_a" ||
		f.list.Text != "cat" {
		t.Errorf("asset:list: exit %d, %+v", code, f.list)
	}

	if code := h.run("asset:list", "--no-reusable"); code != ExitOK || f.list.Reusable == nil || *f.list.Reusable {
		t.Errorf("--no-reusable: %+v", f.list)
	}

	if code := h.run("asset:show", hash); code != ExitOK || f.get.Hash != hash {
		t.Errorf("asset:show: %+v", f.get)
	}

	if code := h.run("asset:uses", hash); code != ExitOK || f.uses.Hash != hash {
		t.Errorf("asset:uses: %+v", f.uses)
	}

	if code := h.run("asset:set", hash, "--rights-note", "mine", "--no-reusable"); code != ExitOK ||
		f.set.RightsNote == nil || *f.set.RightsNote != "mine" || f.set.Reusable == nil || *f.set.Reusable {
		t.Errorf("asset:set: %+v", f.set)
	}

	if code := h.run("asset:tag", hash, "--add", "a,b", "--remove", "c"); code != ExitOK ||
		!slices.Equal(f.tag.Add, []string{"a", "b"}) || !slices.Equal(f.tag.Remove, []string{"c"}) {
		t.Errorf("asset:tag: %+v", f.tag)
	}

	if code := h.run("asset:rename", hash, "--name", "Beach day"); code != ExitOK ||
		f.rename.Hash != hash || f.rename.Name != "Beach day" {
		t.Errorf("asset:rename: %+v", f.rename)
	}

	for _, args := range [][]string{
		{"asset:show"}, {"asset:set", hash}, {"asset:tag", hash}, {"asset:show", "a", "b"}, {"asset:rename", hash},
	} {
		if code := h.run(args...); code != ExitFailure {
			t.Errorf("%v: exit %d, want 1", args, code)
		}
	}
}

func TestMediaImportSendsTheOriginAndTheHash(t *testing.T) {
	t.Parallel()

	f := &assetFake{}
	h := newHarness(t)
	h.res.svc = f

	if code := h.run(
		"media:import", "--hash", "sha256:aa", "--project", "prj_7k2m9x4q1b", "--origin-kind", "generated",
		"--provider", "acme", "--model", "m1", "--prompt", "a cat", "--tag", "Cat,pet", "--reusable",
	); code != ExitOK {
		t.Fatalf("exit %d", code)
	}

	r := f.upload
	if len(r.Files) != 1 || r.Files[0].Hash != "sha256:aa" || r.Files[0].Path != "" {
		t.Errorf("files = %+v", r.Files)
	}

	if r.Origin == nil || r.Origin.Kind != asset.Generated || r.Origin.Provider != "acme" ||
		r.Origin.Prompt != "a cat" {
		t.Errorf("origin = %+v", r.Origin)
	}

	if r.Info == nil || !slices.Equal(r.Info.Tags, []string{"Cat", "pet"}) || r.Info.Reusable == nil ||
		!*r.Info.Reusable {
		t.Errorf("info = %+v", r.Info)
	}

	if code := h.run("media:import", "--project", "prj_7k2m9x4q1b"); code != ExitFailure {
		t.Errorf("an import of nothing: exit %d", code)
	}
}
