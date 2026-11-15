package cli

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/service"
)

type templateFake struct {
	service.Service

	list  service.TemplateListRequest
	save  service.TemplateSaveRequest
	newer service.TemplateInstantiateRequest
	ref   service.TemplateRef
	imp   service.TemplateImportRequest
}

func (f *templateFake) TemplateList(
	_ context.Context, r service.TemplateListRequest,
) (*service.TemplateListResult, error) {
	f.list = r

	return &service.TemplateListResult{Text: "templates"}, nil
}

func (f *templateFake) TemplateSave(_ context.Context, r service.TemplateSaveRequest) (*service.TemplateResult, error) {
	f.save = r

	return &service.TemplateResult{Text: "saved"}, nil
}

func (f *templateFake) TemplateExport(_ context.Context, r service.TemplateRef) (*service.FileResult, error) {
	f.ref = r

	return &service.FileResult{Text: "exported"}, nil
}

func (f *templateFake) TemplateImport(
	_ context.Context, r service.TemplateImportRequest,
) (*service.TemplateResult, error) {
	f.imp = r

	return &service.TemplateResult{Text: "imported"}, nil
}

func (f *templateFake) TemplateInstantiate(
	_ context.Context, r service.TemplateInstantiateRequest,
) (*service.TemplateInstantiateResult, error) {
	f.newer = r

	return &service.TemplateInstantiateResult{ProjectID: "prj_1", Text: "created"}, nil
}

func TestTemplateCommandsPassTheirFlags(t *testing.T) {
	t.Parallel()

	f := &templateFake{}
	h := newHarness(t)
	h.res.svc = f

	if code := h.run("template:list", "--limit", "5", "--cursor", "c1"); code != ExitOK || f.list.Limit != 5 ||
		f.list.Cursor != "c1" {
		t.Errorf("template:list: exit %d, %+v", code, f.list)
	}

	code := h.run("template:save", projectFlag, "--name", "Spring", "--description", "d", "--dry-run")
	if code != ExitOK || f.save.ProjectID != "prj_7k2m9x4q1b" || f.save.Name != "Spring" || f.save.Description != "d" ||
		!f.save.DryRun {
		t.Errorf("template:save: exit %d, %+v", code, f.save)
	}

	if code := h.run("template:save", projectFlag); code != ExitFailure {
		t.Errorf("template:save without --name: exit %d", code)
	}

	if code := h.run("template:new"); code != ExitFailure {
		t.Errorf("template:new without an id: exit %d", code)
	}

	if code := h.run("template:new", "tpl_1", "--id", "prj_9"); code != ExitFailure {
		t.Errorf("template:new without --name: exit %d", code)
	}
}

func TestTemplateExportAndImport(t *testing.T) {
	t.Parallel()

	f := &templateFake{}
	h := newHarness(t)
	h.res.svc = f

	if code := h.run("template:export", "tpl_1"); code != ExitOK || f.ref.TemplateID != "tpl_1" {
		t.Errorf("template:export: exit %d, %+v", code, f.ref)
	}

	if code := h.run("template:import", "spring.zip", "--dry-run"); code != ExitOK || !f.imp.DryRun ||
		!filepath.IsAbs(f.imp.Path) || filepath.Base(f.imp.Path) != "spring.zip" {
		t.Errorf("template:import must send an absolute path: exit %d, %+v", code, f.imp)
	}

	for _, args := range [][]string{{"template:export"}, {"template:import"}, {"template:import", "a", "b"}} {
		if code := h.run(args...); code != ExitFailure {
			t.Errorf("%v: exit %d, want 1", args, code)
		}
	}
}

func TestTemplateNewReadsTheFillFromStandardInput(t *testing.T) {
	t.Parallel()

	f := &templateFake{}
	h := newHarness(t)
	h.res.svc = f
	h.rt.Stdin = strings.NewReader(`{"headline":{"text":"Spring sale"},"hero":{"asset_id":"ast_1","source_in":"2s"}}`)

	if code := h.run("template:new", "tpl_1", "--name", "Sale", "--fill", "-", "--id", "prj_9"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, h.stderr.String())
	}

	got := f.newer
	if got.TemplateID != "tpl_1" || got.Name != "Sale" || got.ID != "prj_9" ||
		got.Fill["hero"].AssetID != "ast_1" || got.Fill["hero"].SourceIn != "2s" ||
		got.Fill["headline"].Text == nil || *got.Fill["headline"].Text != "Spring sale" {
		t.Errorf("request = %+v", got)
	}
}

func TestTemplateNewRefusesABadFill(t *testing.T) {
	t.Parallel()

	for _, fill := range []string{`[1]`, `{"a":{"nope":1}}`, `not json`, `null`} {
		h := newHarness(t)
		h.res.svc = &templateFake{}
		h.rt.Stdin = strings.NewReader(fill)

		if code := h.run("template:new", "tpl_1", "--name", "x", "--fill", "-"); code != ExitFailure {
			t.Errorf("fill %q: exit %d, want 1", fill, code)
		}
	}
}
