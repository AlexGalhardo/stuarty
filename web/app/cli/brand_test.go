package cli

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/service"
)

type brandFake struct {
	service.Service

	font service.FontImportRequest
	set  service.BrandKitSetRequest
}

func (f *brandFake) FontImport(_ context.Context, r service.FontImportRequest) (*service.FontImportResult, error) {
	f.font = r

	return &service.FontImportResult{Text: "font stored"}, nil
}

func (f *brandFake) BrandKitGet(context.Context, service.BrandKitGetRequest) (*service.BrandKit, error) {
	return &service.BrandKit{Text: "brand kit"}, nil
}

func (f *brandFake) BrandKitSet(_ context.Context, r service.BrandKitSetRequest) (*service.BrandKit, error) {
	f.set = r

	return &service.BrandKit{Text: "kit saved"}, nil
}

func TestFontImportSendsAnAbsolutePath(t *testing.T) {
	t.Parallel()

	f := &brandFake{}
	h := newHarness(t)
	h.res.svc = f

	if code := h.run("font:import", "Brand.ttf"); code != ExitOK || !filepath.IsAbs(f.font.Path) ||
		filepath.Base(f.font.Path) != "Brand.ttf" {
		t.Errorf("font:import: exit %d, %+v", code, f.font)
	}

	for _, args := range [][]string{{"font:import"}, {"font:import", "a", "b"}} {
		if code := h.run(args...); code != ExitFailure {
			t.Errorf("%v: exit %d, want 1", args, code)
		}
	}
}

func TestBrandShowAndSet(t *testing.T) {
	t.Parallel()

	f := &brandFake{}
	h := newHarness(t)
	h.res.svc = f

	if code := h.run("brand:show"); code != ExitOK || !strings.Contains(h.stdout.String(), "brand kit") {
		t.Errorf("brand:show: exit %d, %s", code, h.stdout.String())
	}

	h.rt.Stdin = strings.NewReader(`{"colors":{"accent":"#FF0000"}}`)

	if code := h.run("brand:set", "--kit", "-"); code != ExitOK || f.set.Colors["accent"] != "#FF0000" {
		t.Errorf("brand:set: exit %d, %+v", code, f.set)
	}

	for _, args := range [][]string{{"brand:set"}, {"brand:set", "--kit", "missing-file.json"}} {
		if code := h.run(args...); code != ExitFailure {
			t.Errorf("%v: exit %d, want 1", args, code)
		}
	}

	h.rt.Stdin = strings.NewReader(`{"nope":1}`)

	if code := h.run("brand:set", "--kit", "-"); code != ExitFailure {
		t.Errorf("brand:set with an unknown member: exit %d, want 1", code)
	}
}

func TestTemplateNewPassesTheBrandLists(t *testing.T) {
	t.Parallel()

	f := &templateFake{}
	h := newHarness(t)
	h.res.svc = f

	code := h.run(
		"template:new", "tpl_1", "--name", "Sale", "--brand-color", "accent", "--brand-color", "bg,ink",
		"--brand-font", "Acme Sans",
	)
	if code != ExitOK || strings.Join(f.newer.BrandColors, "|") != "accent|bg|ink" ||
		strings.Join(f.newer.BrandFonts, "|") != "Acme Sans" {
		t.Errorf("template:new: exit %d, %+v", code, f.newer)
	}
}
