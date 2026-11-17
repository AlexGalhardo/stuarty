package servicetest

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/core/timeline"
	enginestore "github.com/AlexGalhardo/vukem-video/engine/store"
	"github.com/AlexGalhardo/vukem-video/service"
	"github.com/AlexGalhardo/vukem-video/service/pathguard"
)

type fontImporter interface {
	FontImport(ctx context.Context, req service.FontImportRequest) (*service.FontImportResult, error)
	BrandKitGet(ctx context.Context, req service.BrandKitGetRequest) (*service.BrandKit, error)
	BrandKitSet(ctx context.Context, req service.BrandKitSetRequest) (*service.BrandKit, error)
}

func (m *mediaEnv) writeFont(dir, name string, data []byte) string {
	m.t.Helper()

	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, data, 0o600); err != nil {
		m.t.Fatal(err)
	}

	return p
}

func (m *mediaEnv) importFont(ctx context.Context, path string) (*service.FontImportResult, error) {
	return m.svc.(fontImporter).FontImport(ctx, service.FontImportRequest{Path: path})
}

func (m *mediaEnv) setKit(ctx context.Context, req service.BrandKitSetRequest) (*service.BrandKit, error) {
	return m.svc.(fontImporter).BrandKitSet(ctx, req)
}

func storedFonts(t *testing.T, ws string) int {
	t.Helper()

	entries, err := os.ReadDir(filepath.Join(ws, "fonts"))
	if err != nil {
		t.Fatal(err)
	}

	n := 0

	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), ".") {
			n++
		}
	}

	return n
}

// SDD-007 section 15 check 21 and RF-053: the same font twice stores one file, a crafted file is
// refused and stores nothing.
func TestBrandKitFontImportStoresOnceAndRefusesACraftedFont(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{beta: true})
	anton := bundledFontFile(t, "Anton-Regular.ttf")
	path := m.writeFont(m.files, "a.ttf", anton)

	first, err := m.importFont(m.as(ui), path)
	if err != nil || first.Deduplicated {
		t.Fatalf("first = %+v, %v", first, err)
	}

	second, err := m.importFont(m.as(ui), m.writeFont(m.files, "copy.otf", anton))
	if err != nil || !second.Deduplicated || second.Hash != first.Hash {
		t.Fatalf("second = %+v, %v", second, err)
	}

	if n := storedFonts(t, m.ws); n != 1 {
		t.Errorf("%d files in the font store, want 1", n)
	}

	crafted := append([]byte{0, 1, 0, 0, 0, 64}, []byte(strings.Repeat("x", 200))...)

	if _, err := m.importFont(
		m.as(ui),
		m.writeFont(m.files, "bad.ttf", crafted),
	); !isCode(
		err,
		command.CodeValidation,
	) {
		t.Fatalf("crafted font: %v, want E_VALIDATION", err)
	}

	if n := storedFonts(t, m.ws); n != 1 {
		t.Errorf("%d files after a refused font, want 1", n)
	}

	if _, err := m.importFont(m.as(ui), "relative.ttf"); err == nil {
		t.Error("a relative path was accepted")
	}
}

func TestBrandKitSetValidatesAndRoundTrips(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{beta: true})
	ctx := m.as(ui)
	kit := m.svc.(fontImporter)

	if got, err := kit.BrandKitGet(ctx, service.BrandKitGetRequest{}); err != nil || len(got.Colors) != 0 {
		t.Fatalf("empty kit = %+v, %v", got, err)
	}

	imp, err := m.importFont(ctx, m.writeFont(m.files, "a.ttf", bundledFontFile(t, "Anton-Regular.ttf")))
	if err != nil {
		t.Fatal(err)
	}

	font := map[string]service.BrandFont{"Brand": {Files: []service.BrandFontFile{{Weight: 400, Hash: imp.Hash}}}}

	bad := map[string]service.BrandKitSetRequest{
		"name":    {Colors: map[string]string{"Accent": "#ffffff"}},
		"colour":  {Colors: map[string]string{"accent": "red"}},
		"bundled": {Fonts: map[string]service.BrandFont{"Inter": font["Brand"]}},
		"weight": {
			Fonts: map[string]service.BrandFont{
				"Brand": {Files: []service.BrandFontFile{{Weight: 450, Hash: imp.Hash}}},
			},
		},
		"twice": {
			Fonts: map[string]service.BrandFont{
				"Brand": {Files: []service.BrandFontFile{{Weight: 400, Hash: imp.Hash}, {Weight: 400, Hash: imp.Hash}}},
			},
		},
		"logo form": {Logo: "nope"},
	}
	bad["no member"] = service.BrandKitSetRequest{}
	for name, req := range bad {
		if _, err := m.setKit(ctx, req); !isCode(err, command.CodeValidation) {
			t.Errorf("%s: %v, want E_VALIDATION", name, err)
		}
	}

	missing := map[string]service.BrandFont{"Brand": {Files: []service.BrandFontFile{
		{Weight: 400, Hash: "sha256:" + strings.Repeat("cd", 32)},
	}}}
	if _, err := m.setKit(ctx, service.BrandKitSetRequest{Fonts: missing}); !isCode(err, command.CodeNotFound) {
		t.Errorf("a font never stored: %v, want E_NOT_FOUND", err)
	}

	set, err := m.setKit(ctx, service.BrandKitSetRequest{Colors: map[string]string{"accent": "#ff3366"}, Fonts: font})
	if err != nil || set.Colors["accent"] != "#FF3366" {
		t.Fatalf("set = %+v, %v", set, err)
	}

	got, err := kit.BrandKitGet(ctx, service.BrandKitGetRequest{})
	if err != nil || got.Colors["accent"] != "#FF3366" || len(got.Fonts["Brand"].Files) != 1 {
		t.Fatalf("get = %+v, %v", got, err)
	}

	cleared, err := m.setKit(ctx, service.BrandKitSetRequest{Colors: map[string]string{}})
	if err != nil || len(cleared.Colors) != 0 || len(cleared.Fonts) != 0 {
		t.Fatalf("an explicit empty kit = %+v, %v", cleared, err)
	}
}

// RF-053: a template that uses $accent takes the colour of the brand kit when it asks, and a kit
// without the name is reported.
func TestBrandKitTemplateTakesTheBrandColour(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{beta: true})
	ctx := m.as(ui)

	project := m.templateProject()
	m.mustEdit(ui, project, m.head(project).Revision,
		op("template.set_info", `{"colors":{"accent":"#111111"}}`),
		op("settings.set", `{"background":"$accent"}`),
	)

	tpl := m.saveTemplate(project).Template.TemplateID
	ask := service.TemplateInstantiateRequest{TemplateID: tpl, Name: "A", BrandColors: []string{"accent"}}

	_, err := m.svc.TemplateInstantiate(ctx, ask)
	se := serviceErr(err)

	if se == nil || se.Code != command.CodeValidation || se.Details["reason"] != "brand_color_missing" {
		t.Fatalf("without the colour in the kit: %v", err)
	}

	if _, err := m.setKit(ctx, service.BrandKitSetRequest{Colors: map[string]string{"accent": "#ff3366"}}); err != nil {
		t.Fatal(err)
	}

	res, err := m.svc.TemplateInstantiate(ctx, ask)
	if err != nil {
		t.Fatal(err)
	}

	doc, derr := timeline.Decode(m.document(res.ProjectID))
	if derr != nil {
		t.Fatal(derr)
	}

	if doc.Template.Colors["accent"] != "#FF3366" || doc.Settings.Background != "$accent" {
		t.Errorf("colours = %v, background = %q", doc.Template.Colors, doc.Settings.Background)
	}

	ask.BrandColors = []string{"nothing"}
	if _, err := m.svc.TemplateInstantiate(
		ctx,
		ask,
	); serviceErr(err) == nil ||
		serviceErr(err).Details["reason"] != "brand_color_unknown" {
		t.Errorf("a colour the template lacks: %v", err)
	}

	ask.BrandColors = nil

	plain, err := m.svc.TemplateInstantiate(ctx, ask)
	if err != nil {
		t.Fatal(err)
	}

	if d, _ := timeline.Decode(m.document(plain.ProjectID)); d.Template.Colors["accent"] != "#111111" {
		t.Errorf("a template that did not ask changed: %v", d.Template.Colors)
	}
}

// SDD-007 T-21: ownership is recorded. The importer of a font owns it; another user does not, and
// a bundle export in network mode refuses a font the caller neither owns nor uses elsewhere.
func TestSecurity_T21_BundleExportRefusesAFontTheCallerDoesNotOwn(t *testing.T) {
	root := t.TempDir()
	m := newMediaEnv(t, mediaCfg{beta: true, guard: &pathguard.Config{Roots: []string{root}, Network: true}})
	alice, bob := memberCtx("alice"), memberCtx("bob")

	hash, err := enginestore.NewFonts(m.ws).Put(bundledFontFile(t, "Anton-Regular.ttf"))
	if err != nil {
		t.Fatal(err)
	}

	created, err := m.svc.ProjectCreate(alice, service.ProjectCreateRequest{Name: "p"})
	if err != nil {
		t.Fatal(err)
	}

	project := created.ProjectID
	m.mustEdit(ui, project, 0, op("font.add", `{"family":"Brand","files":[{"weight":400,"hash":"sha256:`+hash+`"}]}`))

	export := func(ctx context.Context) error {
		_, err := m.svc.(bundleExporter).ProjectBundleExport(
			ctx,
			service.ProjectBundleExportRequest{ProjectID: project},
		)

		return err
	}

	err = export(alice)

	se := serviceErr(err)
	if se == nil || se.Code != service.CodeForbidden || strings.Contains(se.Message+se.Hint, hash) {
		t.Fatalf("export of an unowned font: %v", err)
	}

	// Bob cannot name the file either, whatever he imports: he never brought these bytes.
	path := m.writeFont(root, "a.ttf", bundledFontFile(t, "Anton-Regular.ttf"))
	if _, err := m.importFont(alice, path); err != nil {
		t.Fatal(err)
	}

	if err := export(alice); err != nil {
		t.Fatalf("after alice imported the font: %v", err)
	}

	add := op("font.add", `{"family":"Mine","files":[{"weight":400,"hash":"sha256:`+hash+`"}]}`)

	bobProject, err := m.svc.ProjectCreate(bob, service.ProjectCreateRequest{Name: "b"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := m.svc.Edit(bob, service.EditRequest{
		ProjectID: bobProject.ProjectID, Operations: []command.Operation{add},
	}); !isCode(err, command.CodeNotFound) {
		t.Fatalf("bob names alice's font: %v", err)
	}

	// Importing the same bytes makes bob an owner too.
	if _, err := m.importFont(bob, path); err != nil {
		t.Fatal(err)
	}

	if _, err := m.svc.Edit(bob, service.EditRequest{
		ProjectID: bobProject.ProjectID, Operations: []command.Operation{add},
	}); err != nil {
		t.Fatalf("bob after importing: %v", err)
	}
}

// VV-094: an imported font is listed with the hash font.add takes.
func TestFontListShowsTheImportedFontsWithTheirHash(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{beta: true})

	before, err := m.svc.FontList(m.as(ui), service.FontListRequest{})
	if err != nil || len(before.Imported) != 0 {
		t.Fatalf("before = %+v, %v", before, err)
	}

	anton := bundledFontFile(t, "Anton-Regular.ttf")

	imported, err := m.importFont(m.as(ui), m.writeFont(m.files, "a.ttf", anton))
	if err != nil {
		t.Fatal(err)
	}

	after, err := m.svc.FontList(m.as(ui), service.FontListRequest{})
	if err != nil || len(after.Imported) != 1 || after.Imported[0].Hash != imported.Hash ||
		after.Imported[0].Bytes != int64(len(anton)) {
		t.Fatalf("after = %+v, %v; want the hash %s", after, err, imported.Hash)
	}
}
