package servicetest

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/engine/ingest"
	"github.com/AlexGalhardo/stuarty/service"
)

// A file that arrives as the bytes of a request (ImageImportFile, SDD-021 section 10.2) goes through the same
// ingest and the same rules as a path.

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) { return len(p), nil }

func storeObjects(t *testing.T, m *mediaEnv) []string {
	t.Helper()

	var out []string

	_ = filepath.WalkDir(filepath.Join(m.ws, "store"), func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			out = append(out, p)
		}

		return nil
	})

	return out
}

func TestImageImportFileMakesADocument(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{ingest: pictureIngest(64, 48)})
	data := pngBytes(t, 64, 48)

	res, err := m.svc.ImageImportFile(m.as(ui), service.ImageImportFileRequest{
		FileName: `C:\Users\ana\Pictures\Cover Shot.png`, Body: bytes.NewReader(data), Size: int64(len(data)),
	})
	if err != nil {
		t.Fatal(err)
	}

	if res.State != service.ImageImportReady || !res.Created || res.Hash != fileHash(data) || res.Width != 64 {
		t.Fatalf("%+v", res)
	}

	d, err := m.svc.ImageDescribe(m.as(ui), service.ImageDescribeRequest{ImageID: res.ImageID})
	if err != nil || d.Name != "Cover Shot" || len(d.Layers) != 1 || d.Layers[0].Asset != fileHash(data) {
		t.Fatalf("%v %+v", err, d)
	}

	// into the document just made, as one more layer
	into, err := m.svc.ImageImportFile(m.as(agent), service.ImageImportFileRequest{
		ImageID: res.ImageID, BaseRevision: 1, Name: "Second",
		FileName: "second.png", Body: bytes.NewReader(data), Size: -1,
	})
	if err != nil || into.Created || into.Revision != 2 || into.LayerID == "" {
		t.Fatalf("%v %+v", err, into)
	}
}

func TestImageImportFileRefusals(t *testing.T) {
	for _, c := range []struct {
		name string
		cfg  mediaCfg
		req  service.ImageImportFileRequest
		code string
	}{
		{"no body", mediaCfg{}, service.ImageImportFileRequest{}, "E_VALIDATION"},
		{
			"a path as well",
			mediaCfg{},
			service.ImageImportFileRequest{
				Path: "/a.png", Body: strings.NewReader("x"), Size: 1,
			},
			"E_VALIDATION",
		},
		{"an empty file", mediaCfg{ingest: pictureIngest(8, 8)}, service.ImageImportFileRequest{Body: strings.NewReader(""), Size: 0}, "E_UNSUPPORTED_MEDIA"},
		{
			"a declared size over the limit",
			mediaCfg{ingest: pictureIngest(8, 8), limits: service.Limits{MaxImportBytes: 100}},
			service.ImageImportFileRequest{Body: strings.NewReader(strings.Repeat("a", 500)), Size: 500},
			"E_LIMIT_EXCEEDED",
		},
		{
			"a streamed size over the limit",
			mediaCfg{ingest: pictureIngest(8, 8), limits: service.Limits{MaxImportBytes: 100}},
			service.ImageImportFileRequest{Body: strings.NewReader(strings.Repeat("a", 500)), Size: -1},
			"E_LIMIT_EXCEEDED",
		},
		{
			"a canvas it cannot make",
			mediaCfg{ingest: pictureIngest(5000, 3000)},
			service.ImageImportFileRequest{Body: strings.NewReader("12345678901234567890"), Size: 20},
			"E_LIMIT",
		},
		{
			"a file that is not an image (the ingest says a video)",
			mediaCfg{},
			service.ImageImportFileRequest{Body: strings.NewReader(clipBytes), Size: -1},
			"E_UNSUPPORTED_MEDIA",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := newMediaEnv(t, c.cfg)

			res, err := m.svc.ImageImportFile(m.as(ui), c.req)
			if res != nil || code(err) != c.code {
				t.Fatalf("%v %+v, want %s", err, res, c.code)
			}

			if n := imageCount(t, m); n != 0 {
				t.Fatalf("%d documents after a refusal", n)
			}
		})
	}
}

// A file the caller sends over the cap of a request is refused while it streams, with a message that names the
// cap and the way out, whatever the import limit of the server is, and nothing of it is kept.
func TestSecurity_T62_FileOverTheCapIsRefusedWhileItStreams(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{ingest: pictureIngest(8, 8)})

	res, err := m.svc.ImageImportFile(m.as(ui), service.ImageImportFileRequest{
		Body: io.LimitReader(zeroReader{}, service.MaxImageFileBytes+1000), Size: -1,
	})

	se := serviceError(t, err)
	if res != nil || se.Code != "E_LIMIT_EXCEEDED" || !strings.Contains(se.Hint, "path") ||
		se.Details["ceiling"] != service.MaxImageFileBytes {
		t.Fatalf("%+v %v", res, se)
	}

	if left := storeObjects(t, m); len(left) != 0 {
		t.Fatalf("a refused file left %v", left)
	}

	declared, err := m.svc.ImageImportFile(m.as(ui), service.ImageImportFileRequest{
		Body: strings.NewReader("x"), Size: service.MaxImageFileBytes + 1,
	})
	if declared != nil || code(err) != "E_LIMIT_EXCEEDED" {
		t.Fatalf("a declared size over the cap: %v", err)
	}
}

// grayPNG is a PNG of w by h pixels, 8 bits of grey, made row by row so the test never holds the picture: with
// every row zero it is a few dozen kilobytes and decodes to w*h bytes. whole false cuts the data after the first
// row, so the header declares a picture the file does not hold.
func grayPNG(w, h int, whole bool) []byte {
	chunk := func(kind string, data []byte) []byte {
		var b bytes.Buffer

		_ = binary.Write(&b, binary.BigEndian, uint32(len(data)))
		b.WriteString(kind)
		b.Write(data)

		crc := crc32.NewIEEE()
		crc.Write([]byte(kind))
		crc.Write(data)
		_ = binary.Write(&b, binary.BigEndian, crc.Sum32())

		return b.Bytes()
	}

	ihdr := new(bytes.Buffer)
	_ = binary.Write(ihdr, binary.BigEndian, uint32(w))
	_ = binary.Write(ihdr, binary.BigEndian, uint32(h))
	ihdr.Write([]byte{8, 0, 0, 0, 0}) // 8 bits, grey

	var z bytes.Buffer

	zw := zlib.NewWriter(&z)
	row := make([]byte, w+1) // the filter byte and the pixels, all zero

	rows := h
	if !whole {
		rows = 1
	}

	for range rows {
		_, _ = zw.Write(row)
	}

	_ = zw.Close()

	var png bytes.Buffer

	png.Write([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
	png.Write(chunk("IHDR", ihdr.Bytes()))
	png.Write(chunk("IDAT", z.Bytes()))
	png.Write(chunk("IEND", nil))

	return png.Bytes()
}

// TestSecurity_T63_UploadedPixelBombIsRefusedBeforeItIsDecoded: a picture over the pixel ceiling, whole or cut
// short after a header that declares 60,000 by 60,000 pixels, is refused with the dimensions the probe read, and
// no document exists. The probe and the conversion run in a child process under the limits of SDD-007 section 6.5.
func TestSecurity_T63_UploadedPixelBombIsRefusedBeforeItIsDecoded(t *testing.T) {
	// skip-env: pinned-ffmpeg
	m := newMediaEnv(t, mediaCfg{realExport: true, ingest: ingest.Run})

	whole := grayPNG(7000, 7000, true) // 49 million pixels, over the 33 million of the default ceiling
	if len(whole) > 1<<20 {
		t.Fatalf("the test picture is %d bytes", len(whole))
	}

	res, err := m.svc.ImageImportFile(m.as(ui), service.ImageImportFileRequest{
		FileName: "photo.png", Body: bytes.NewReader(whole), Size: int64(len(whole)),
	})

	se := serviceError(t, err)
	if res != nil || se.Code != "E_LIMIT_EXCEEDED" || se.Details["limit"] != "import_pixels" {
		t.Fatalf("a whole picture over the ceiling: %+v %+v", res, se)
	}

	cut := grayPNG(60000, 60000, false)

	res, err = m.svc.ImageImportFile(m.as(ui), service.ImageImportFileRequest{
		FileName: "bomb.png", Body: bytes.NewReader(cut), Size: int64(len(cut)),
	})
	if res != nil || err == nil {
		t.Fatalf("a header that declares 60,000 by 60,000 pixels was accepted: %+v", res)
	}

	if c := code(err); c != "E_LIMIT_EXCEEDED" && c != "E_UNSUPPORTED_MEDIA" {
		t.Fatalf("a header of 60,000 by 60,000 pixels: %v", err)
	}

	if n := imageCount(t, m); n != 0 {
		t.Fatalf("%d documents after a pixel bomb", n)
	}
}

// TestSecurity_T66_UploadedMarkupIsNeverAnImage: a file that is HTML, SVG or a script, whatever its name and its
// declared type, is refused by the ingest as not media; the kind of a file is decided by the ingest and never by
// the name, and only the normalized artifacts of an image are ever served.
func TestSecurity_T66_UploadedMarkupIsNeverAnImage(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{realIngest: true})

	for name, content := range map[string]string{
		"page.png":   "<!doctype html><html><body><script>alert(document.cookie)</script></body></html>",
		"vector.png": `<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"><script>alert(1)</script></svg>`,
		"script.jpg": "#!/bin/sh\nrm -rf / --no-preserve-root\n" + strings.Repeat("x", 100),
		"fake.png":   "\x89PNG\r\n\x1a\nthis is not a real png, only a signature and then text, long enough",
	} {
		res, err := m.svc.ImageImportFile(m.as(ui), service.ImageImportFileRequest{
			FileName: name, Body: strings.NewReader(content), Size: int64(len(content)),
		})

		if res != nil || code(err) != "E_UNSUPPORTED_MEDIA" {
			t.Errorf("%s: %v %+v", name, err, res)
		}
	}

	if n := imageCount(t, m); n != 0 {
		t.Fatalf("%d documents from markup", n)
	}
}
