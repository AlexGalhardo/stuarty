package service

import (
	"archive/zip"
	"bytes"
	"image"
	"image/jpeg"
	"io/fs"
	"strings"
	"testing"
)

type member struct {
	name string
	body []byte
	mode fs.FileMode
}

func zipOf(tb testing.TB, members ...member) []byte {
	tb.Helper()

	var buf bytes.Buffer

	zw := zip.NewWriter(&buf)

	for _, m := range members {
		h := &zip.FileHeader{Name: m.name, Method: zip.Deflate}
		if m.mode != 0 {
			h.SetMode(m.mode)
		}

		w, err := zw.CreateHeader(h)
		if err != nil {
			tb.Fatal(err)
		}

		if _, err := w.Write(m.body); err != nil {
			tb.Fatal(err)
		}
	}

	if err := zw.Close(); err != nil {
		tb.Fatal(err)
	}

	return buf.Bytes()
}

func goodManifest() []byte {
	return []byte(
		`{"format":"vukem-video-template","template_version":1,"app_version":"t","schema_version":1,"name":"n"}`,
	)
}

func jpegBytes(tb testing.TB) []byte {
	tb.Helper()

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewGray(image.Rect(0, 0, 4, 4)), nil); err != nil {
		tb.Fatal(err)
	}

	return buf.Bytes()
}

func TestTemplateFileAcceptsTheThreeMembers(t *testing.T) {
	data := zipOf(t,
		member{name: "manifest.json", body: goodManifest()},
		member{name: "template.json", body: []byte(`{"a":[1,2,{"b":true}]}`)},
		member{name: "preview.jpg", body: jpegBytes(t)},
	)

	got, err := readTemplateArchive(data)
	if err != nil || got.Manifest.Name != "n" || len(got.Document) == 0 || len(got.Preview) == 0 {
		t.Fatalf("a valid file: %+v, %v", got, err)
	}
}

// SDD-007 section 15 check 20 (T-20): every hostile archive is refused as
// E_VALIDATION, and the refusal never carries an entry name.
func TestSecurity_T20_TemplateArchiveRules(t *testing.T) {
	doc := []byte(`{}`)
	man := goodManifest()

	// An entry that declares a small size and holds more: the central directory
	// and the local header both say 5 bytes, the stream does not stop there.
	var lie bytes.Buffer

	{
		zw := zip.NewWriter(&lie)
		w, _ := zw.CreateHeader(&zip.FileHeader{Name: "template.json", Method: zip.Deflate})
		_, _ = w.Write(bytes.Repeat([]byte("x"), 5000))
		_ = zw.Close()
	}

	liar := lie.Bytes()
	// Rewrite the declared uncompressed size in the central directory to 5.
	if i := bytes.Index(liar, []byte("PK\x01\x02")); i >= 0 {
		liar[i+24], liar[i+25], liar[i+26], liar[i+27] = 5, 0, 0, 0
	}

	cases := map[string][]byte{
		"parent directory": zipOf(t, member{name: "../x", body: doc}, member{name: "manifest.json", body: man}),
		"absolute":         zipOf(t, member{name: "/etc/x", body: doc}, member{name: "manifest.json", body: man}),
		"nested": zipOf(
			t,
			member{name: "dir/template.json", body: doc},
			member{name: "manifest.json", body: man},
		),
		"case duplicate": zipOf(t, member{name: "manifest.json", body: man}, member{name: "Manifest.json", body: man},
			member{name: "template.json", body: doc}),
		"symlink": zipOf(t, member{name: "manifest.json", body: man},
			member{name: "template.json", body: []byte("template.json"), mode: fs.ModeSymlink | 0o777}),
		"unlisted name": zipOf(t, member{name: "manifest.json", body: man}, member{name: "template.json", body: doc},
			member{name: "notes.txt", body: []byte("hi")}),
		"nested archive": zipOf(t, member{name: "manifest.json", body: man}, member{name: "template.json", body: doc},
			member{name: "inner.zip", body: zipOf(t, member{name: "template.json", body: doc})}),
		"bomb ratio": zipOf(t, member{name: "manifest.json", body: man},
			member{name: "template.json", body: make([]byte, 3<<20)}),
		"template over its cap": zipOf(t, member{name: "manifest.json", body: man},
			member{name: "template.json", body: bytes.Repeat([]byte("[1,"), 2<<20)}),
		"deep nesting": zipOf(t, member{name: "manifest.json", body: man},
			member{name: "template.json", body: []byte(strings.Repeat("[", 100) + strings.Repeat("]", 100))}),
		"missing template": zipOf(t, member{name: "manifest.json", body: man}),
		"bad manifest": zipOf(t, member{name: "manifest.json", body: []byte(`{"format":"other"}`)},
			member{name: "template.json", body: doc}),
		"preview that is not a jpeg": zipOf(t, member{name: "manifest.json", body: man},
			member{name: "template.json", body: doc}, member{name: "preview.jpg", body: []byte("GIF89a")}),
		"not a zip":  []byte("this is not a zip"),
		"lying size": liar,
	}

	for name, data := range cases {
		got, err := readTemplateArchive(data)
		if err == nil {
			t.Errorf("%s: accepted %+v", name, got)

			continue
		}

		if err.Code != "E_VALIDATION" {
			t.Errorf("%s: code %s", name, err.Code)
		}

		for _, leak := range []string{"../x", "/etc/x", "notes.txt", "inner.zip", "dir/"} {
			if strings.Contains(err.Message, leak) || strings.Contains(err.Hint, leak) {
				t.Errorf("%s: the refusal names an entry: %q", name, err.Message)
			}
		}
	}
}

func TestTemplateFileRefusesTooManyEntries(t *testing.T) {
	members := []member{{name: "manifest.json", body: goodManifest()}, {name: "template.json", body: []byte(`{}`)}}
	for i := range 70 {
		members = append(
			members,
			member{name: "f" + string(rune('a'+i%26)) + string(rune('a'+i/26)), body: []byte("x")},
		)
	}

	if _, err := readTemplateArchive(zipOf(t, members...)); err == nil {
		t.Error("an archive of 72 entries was accepted")
	}
}

func FuzzTemplateArchive(f *testing.F) {
	f.Add(
		zipOf(
			f,
			member{name: "manifest.json", body: goodManifest()},
			member{name: "template.json", body: []byte(`{}`)},
		),
	)
	f.Add(zipOf(f, member{name: "../x", body: []byte("x")}))

	// Seeds shaped like a project bundle (SDD-005 section 16.1): the template reader refuses them,
	// and the mutations of their flat entry names reach the archive rules the two readers share.
	hexName := strings.Repeat("ab", 32)
	f.Add(zipOf(
		f,
		member{name: "manifest.json", body: []byte(`{"format":"vukem-video-bundle","bundle_version":1}`)},
		member{name: "document.json", body: []byte(`{}`)},
		member{name: "media." + hexName, body: []byte("media")},
		member{name: "font." + hexName, body: []byte("\x00\x01\x00\x00")},
	))
	f.Add(zipOf(
		f,
		member{name: "manifest.json", body: goodManifest()},
		member{name: "template.json", body: []byte(`{}`)},
		member{name: "font." + hexName, body: []byte("../font"), mode: fs.ModeSymlink | 0o777},
	))
	f.Add(zipOf(
		f,
		member{name: "media." + hexName, body: make([]byte, 3<<20)},
		member{name: "Media." + hexName, body: []byte("case duplicate")},
	))

	f.Add([]byte("PK\x03\x04"))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		// Whatever the bytes, the reader returns or refuses; it never panics
		// and never returns more than the policy allows.
		got, err := readTemplateArchive(data)
		if err == nil &&
			(got == nil || len(got.Document) > templateDocumentMax || len(got.Preview) > templatePreviewMax) {
			t.Fatalf("accepted an archive beyond the policy: %+v", got)
		}
	})
}
