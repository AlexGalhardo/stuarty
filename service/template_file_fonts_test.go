package service

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestTemplateFontsAllowlistAcceptsOnlyAFontNamedByItsHash(t *testing.T) {
	body := []byte("font bytes")
	sum := sha256.Sum256(body)
	name := "font." + hex.EncodeToString(sum[:])
	base := []member{
		{name: "manifest.json", body: goodManifest()},
		{name: "template.json", body: []byte(`{}`)},
	}

	got, err := readTemplateArchive(zipOf(t, append(base, member{name: name, body: body})...))
	if err != nil || len(got.Fonts) != 1 || string(got.Fonts["sha256:"+hex.EncodeToString(sum[:])]) != string(body) {
		t.Fatalf("a font entry: %+v, %v", got, err)
	}

	refused := map[string]member{
		"bytes that do not hash to the name": {name: name, body: []byte("other bytes")},
		"a short name":                       {name: "font.abcd", body: body},
		"upper case hex":                     {name: "font." + strings.ToUpper(hex.EncodeToString(sum[:])), body: body},
		"a directory":                        {name: "fonts/" + hex.EncodeToString(sum[:]), body: body},
		"an extension":                       {name: name + ".ttf", body: body},
	}

	for what, m := range refused {
		if _, err := readTemplateArchive(zipOf(t, append(append([]member{}, base...), m)...)); err == nil ||
			err.Code != "E_VALIDATION" {
			t.Errorf("%s: %v, want E_VALIDATION", what, err)
		}
	}

	over := make([]byte, 16<<20+1)
	osum := sha256.Sum256(over)

	if _, err := readTemplateArchive(zipOf(
		t, append(base, member{name: "font." + hex.EncodeToString(osum[:]), body: over})...,
	)); err == nil {
		t.Error("a font over 16 MiB was accepted")
	}
}
