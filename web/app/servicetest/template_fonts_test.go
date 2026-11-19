package servicetest

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	enginestore "github.com/AlexGalhardo/stuarty/engine/store"
	"github.com/AlexGalhardo/stuarty/service"
)

func zipEntries(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()

	var buf bytes.Buffer

	zw := zip.NewWriter(&buf)

	for name, body := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}

		if _, err := w.Write(body); err != nil {
			t.Fatal(err)
		}
	}

	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	return buf.Bytes()
}

func zipNames(t *testing.T, data []byte) map[string]bool {
	t.Helper()

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}

	out := map[string]bool{}
	for _, f := range zr.File {
		out[f.Name] = true
	}

	return out
}

func (m *mediaEnv) instantiate(name, template string) string {
	m.t.Helper()

	res, err := m.svc.TemplateInstantiate(
		m.as(ui),
		service.TemplateInstantiateRequest{TemplateID: template, Name: name},
	)
	if err != nil {
		m.t.Fatal(err)
	}

	return res.ProjectID
}

// ADR-0022 item 5: a template with a custom font exports with the font file, imports on a fresh
// workspace through the font pipeline, and draws the same bitmap.
func TestTemplateFontsRoundTripRendersTheSameBitmap(t *testing.T) {
	a := newMediaEnv(t, mediaCfg{noQueue: true, realExport: true, beta: true})

	anton, err := enginestore.NewFonts(a.ws).Put(bundledFontFile(t, "Anton-Regular.ttf"))
	if err != nil {
		t.Fatal(err)
	}

	saved := a.saveTemplate(a.brandProject(anton))
	file := a.exportTemplate(saved.Template.TemplateID)

	if !zipNames(t, file)["font."+anton] {
		t.Fatalf("the template file has no font entry: %v", zipNames(t, file))
	}

	b := newMediaEnv(t, mediaCfg{noQueue: true, realExport: true, beta: true})

	imported, err := b.svc.TemplateImport(b.as(ui), service.TemplateImportRequest{Path: b.write("t.zip", string(file))})
	if err != nil {
		t.Fatal(err)
	}

	if !enginestore.NewFonts(b.ws).Has(anton) {
		t.Fatal("the font is not in the font store of the new workspace")
	}

	_, want := a.bitmapOf(a.instantiate("A", saved.Template.TemplateID))
	_, got := b.bitmapOf(b.instantiate("B", imported.Template.TemplateID))

	if !bytes.Equal(want, got) {
		t.Error("the bitmap of the imported template differs from the original")
	}
}

// SDD-007 T-20 with the extended allowlist: a font entry must hash to its name, be in the document,
// be a valid font, and the document must not name a font the file lacks.
func TestTemplateFontsImportRefusesBadFontEntries(t *testing.T) {
	a := newMediaEnv(t, mediaCfg{beta: true})

	anton, err := enginestore.NewFonts(a.ws).Put(bundledFontFile(t, "Anton-Regular.ttf"))
	if err != nil {
		t.Fatal(err)
	}

	file := a.exportTemplate(a.saveTemplate(a.brandProject(anton)).Template.TemplateID)
	manifest, doc := readZipMember(t, file, "manifest.json"), readZipMember(t, file, "template.json")
	font := readZipMember(t, file, "font."+anton)

	crafted := append([]byte{0, 1, 0, 0, 0, 64}, []byte(strings.Repeat("x", 200))...)
	sum := sha256.Sum256(crafted)
	craftedName := "font." + hex.EncodeToString(sum[:])

	cases := map[string]map[string][]byte{
		"bytes that do not hash to the name": {
			"manifest.json": manifest, "template.json": doc, "font." + anton: append(append([]byte{}, font...), 0),
		},
		"a document font that is not in the file": {"manifest.json": manifest, "template.json": doc},
		"a font the document does not use": {
			"manifest.json": manifest, "template.json": doc, "font." + anton: font, craftedName: crafted,
		},
		"an uppercase name": {
			"manifest.json":                  manifest,
			"template.json":                  doc,
			"FONT." + strings.ToUpper(anton): font,
		},
		"a crafted file under its own hash": {"manifest.json": manifest, "template.json": doc, craftedName: crafted},
	}

	for name, entries := range cases {
		b := newMediaEnv(t, mediaCfg{beta: true})

		_, err := b.svc.TemplateImport(
			b.as(ui), service.TemplateImportRequest{Path: b.write("t.zip", string(zipEntries(t, entries)))},
		)
		if !isCode(err, command.CodeValidation) {
			t.Errorf("%s: %v, want E_VALIDATION", name, err)
		}

		if len(b.templateList(b.as(ui))) != 0 && name != "" {
			// The starters are not seeded here, so any template is the import.
			t.Errorf("%s: a template was stored", name)
		}

		if enginestore.NewFonts(b.ws).Has(anton) {
			t.Errorf("%s: a font was stored by a refused import", name)
		}
	}
}
