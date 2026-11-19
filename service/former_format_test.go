package service

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

// A template file downloaded from Vukem Video carries the former format and is still read (SDD-000
// section 13.3, ADR-0044); any other format is refused.
func TestTemplateFileOfTheFormerNameIsRead(t *testing.T) {
	for format, ok := range map[string]bool{templateFormat: true, formerTemplateFormat: true, "other": false} {
		manifest := strings.Replace(string(goodManifest()), templateFormat, format, 1)
		data := zipOf(t,
			member{name: "manifest.json", body: []byte(manifest)},
			member{name: "template.json", body: []byte(`{"a":1}`)},
			member{name: "preview.jpg", body: jpegBytes(t)},
		)

		got, err := readTemplateArchive(data)
		if ok && (err != nil || got.Manifest.Name != "n") {
			t.Errorf("format %s: %+v, %v, want it read", format, got, err)
		}

		if !ok && err == nil {
			t.Errorf("format %s was read, want it refused", format)
		}
	}
}

// A bundle exported by Vukem Video carries the former format and is still imported (SDD-000 section 13.3,
// ADR-0044); any other format is refused.
func TestBundleManifestOfTheFormerNameIsRead(t *testing.T) {
	for format, ok := range map[string]bool{BundleFormat: true, formerBundleFormat: true, "other": false} {
		manifest := `{"format":"` + format + `","bundle_version":1,"project":{"id":"prj_0000000000"}}`
		data := zipOf(t, member{name: bundleManifestEntry, body: []byte(manifest)})

		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			t.Fatal(err)
		}

		p := &bundleParts{zr: zr, entries: map[string]*zip.File{bundleManifestEntry: zr.File[0]}}

		var s *svc

		got := s.readBundleManifest(p)
		if ok && got != nil {
			t.Errorf("format %s: %v, want it read", format, got)
		}

		if !ok && got == nil {
			t.Errorf("format %s was read, want it refused", format)
		}
	}
}
