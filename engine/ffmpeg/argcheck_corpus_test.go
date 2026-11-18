package ffmpeg

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg/ffargs"
)

var corpusPolicy = ffargs.Policy{StoreDir: "/ws/store", RunDir: "/ws/tmp/job_1", MaxThreads: 2}

func readArgv(tb testing.TB, path string) []string {
	tb.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		tb.Fatal(err)
	}

	var argv []string
	if err := json.Unmarshal(raw, &argv); err != nil {
		tb.Fatalf("%s: %v", path, err)
	}

	return argv
}

// SDD-007 15 check 13: the checker accepts the valid argv and rejects every fixture of testdata/argv.
func TestArgvFixturesAreRejected(t *testing.T) {
	t.Parallel()

	files, err := filepath.Glob(filepath.Join("testdata", "argv", "*.json"))
	if err != nil || len(files) < 56 {
		t.Fatalf("%d fixtures, %v", len(files), err)
	}

	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".json")
		err := ffargs.Check(ffargs.FFmpeg, readArgv(t, f), corpusPolicy)

		// The valid-* fixtures are argv the BETA features build; the rejected fixtures around
		// them (alpha-proxy-*, cpuflags-*, filter-afftdn-*) each change one token of one.
		if strings.HasPrefix(name, "valid-") {
			if err != nil {
				t.Errorf("the valid argv %s was refused: %v", name, err)
			}

			continue
		}

		if err == nil {
			t.Errorf("%s: the checker accepted it", name)
		}
	}
}

// SDD-007 15 check 14: text formats are refused by the sniff alone, FFmpeg is never started.
func TestSecurity_T14_SniffRejectsText(t *testing.T) {
	t.Parallel()

	files, err := filepath.Glob(filepath.Join("testdata", "hostile", "*"))
	if err != nil || len(files) < 29 {
		t.Fatalf("%d hostile files, %v", len(files), err)
	}

	for _, f := range files {
		err := SniffFile(f)

		var e *Error
		if !errors.As(err, &e) || e.Code != CodeUnsupportedMedia {
			t.Errorf("%s: error = %v, want E_UNSUPPORTED_MEDIA", filepath.Base(f), err)
		}
	}
}

func chunk(kind string, data []byte) []byte {
	var b bytes.Buffer

	_ = binary.Write(&b, binary.BigEndian, uint32(len(data)))
	b.WriteString(kind)
	b.Write(data)
	_ = binary.Write(&b, binary.BigEndian, crc32.ChecksumIEEE(append([]byte(kind), data...)))

	return b.Bytes()
}

// bigPNG is a whole PNG of w x h whose pixels are all zero: a few kilobytes that declare a large
// picture, the shape of a decompression bomb.
func bigPNG(tb testing.TB, w, h uint32) []byte {
	tb.Helper()

	var z bytes.Buffer

	zw, err := zlib.NewWriterLevel(&z, zlib.BestCompression)
	if err != nil {
		tb.Fatal(err)
	}

	row := make([]byte, 1+int(w)*4)
	for range h {
		if _, err := zw.Write(row); err != nil {
			tb.Fatal(err)
		}
	}

	if err := zw.Close(); err != nil {
		tb.Fatal(err)
	}

	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], w)
	binary.BigEndian.PutUint32(ihdr[4:], h)
	ihdr[8], ihdr[9] = 8, 6

	out := append([]byte("\x89PNG\r\n\x1a\n"), chunk("IHDR", ihdr)...)
	out = append(out, chunk("IDAT", z.Bytes())...)

	return append(out, chunk("IEND", nil)...)
}

// hugePNG declares w x h and carries no pixels: a decompression bomb header.
func hugePNG(w, h uint32) []byte {
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], w)
	binary.BigEndian.PutUint32(ihdr[4:], h)
	ihdr[8], ihdr[9] = 8, 6

	return append(append([]byte("\x89PNG\r\n\x1a\n"), chunk("IHDR", ihdr)...), chunk("IEND", nil)...)
}

func box(kind string, parts ...[]byte) []byte {
	body := bytes.Join(parts, nil)
	out := binary.BigEndian.AppendUint32(nil, uint32(8+len(body)))

	return append(append(out, kind...), body...)
}

// externalDrefMP4 is an MP4 whose data reference points at another file: not self-contained.
func externalDrefMP4() []byte {
	url := box("url ", []byte{0, 0, 0, 0}, []byte("file:///etc/passwd\x00"))
	dref := box("dref", []byte{0, 0, 0, 0, 0, 0, 0, 1}, url)

	return append(
		box("ftyp", []byte("isom"), []byte{0, 0, 2, 0}, []byte("isomiso2")),
		box("moov", box("trak", box("mdia", box("minf", box("dinf", dref)))))...,
	)
}

// SDD-007 15 check 14 and 9.2 item 1: hostile binary files are refused with the right code and
// before any decode. Runs on the pinned build.
func TestHostileBinaryMediaIsRefused(t *testing.T) {
	r, dir := pinnedRunner(t)
	in := r.In

	var many []string

	for range 100 {
		many = append(many, "-map", "0:a")
	}

	args := append([]string{"-f", "lavfi", "-i", "sine=f=440:d=0.1"}, many...)
	manyStreams := gen(t, in, dir, "many.mkv",
		append(args, "-c:a", "pcm_s16le", "-f", "matroska")...)

	body, err := os.ReadFile(manyStreams)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		body []byte
		code string
	}{
		{"external data reference", externalDrefMP4(), CodeUnsupportedMedia},
		{"60000 pixels wide", bigPNG(t, 60000, 2), CodeLimitExceeded},
		{"36 million pixels", bigPNG(t, 6000, 6000), CodeLimitExceeded},
		{"a PNG header with no data", hugePNG(60000, 60000), CodeUnsupportedMedia},
		{"100 streams", body, CodeUnsupportedMedia},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			store := putStoreBytes(t, r, c.body)

			_, err := r.Probe(context.Background(), dir, store, Ceilings{})

			var e *Error
			if !errors.As(err, &e) || e.Code != c.code {
				t.Fatalf("error = %v, want %s", err, c.code)
			}

			if strings.Contains(e.Message, "passwd") {
				t.Errorf("the message quotes the file: %q", e.Message)
			}
		})
	}
}

func FuzzSniffMedia(f *testing.F) {
	entries, _ := filepath.Glob(filepath.Join("testdata", "hostile", "*"))
	for _, p := range entries {
		if b, err := os.ReadFile(p); err == nil {
			f.Add(b)
		}
	}

	f.Add(hugePNG(10, 10))
	f.Add(externalDrefMP4())
	f.Add(x264Style())
	f.Add([]byte("\x00\x00\x00\x18ftypisom\x00\x00\x02\x00isomiso2"))

	f.Fuzz(func(t *testing.T, head []byte) {
		err := Sniff(head)
		if err == nil {
			trimmed := bytes.TrimPrefix(head, []byte("\xef\xbb\xbf"))
			for _, p := range refusedPrefixes {
				if bytes.HasPrefix(trimmed, []byte(p)) {
					t.Fatalf("a text format was accepted: %q", p)
				}
			}

			return
		}

		var e *Error
		if !errors.As(err, &e) || e.Code != CodeUnsupportedMedia {
			t.Fatalf("a refusal that is not E_UNSUPPORTED_MEDIA: %v", err)
		}
	})
}

func FuzzFFArgsCheck(f *testing.F) {
	files, _ := filepath.Glob(filepath.Join("testdata", "argv", "*.json"))
	for _, p := range files {
		raw, err := os.ReadFile(p)
		if err != nil {
			continue
		}

		var argv []string
		if json.Unmarshal(raw, &argv) == nil {
			f.Add(strings.Join(argv, "\x00"))
		}
	}

	f.Fuzz(func(t *testing.T, joined string) {
		argv := strings.Split(joined, "\x00")
		if ffargs.Check(ffargs.FFmpeg, argv, corpusPolicy) != nil {
			return
		}

		// Whatever the checker accepts must satisfy the rules it exists for.
		all := strings.Join(argv, " ")
		for _, bad := range []string{"http:", "https:", "tcp:", "movie=", "subtitles=", "drawtext", "fontfile", "-f concat", "-f tee"} {
			if strings.Contains(all, bad) {
				t.Fatalf("accepted an argv with %q: %q", bad, argv)
			}
		}

		for i, a := range argv {
			if a != "-i" || i+1 >= len(argv) {
				continue
			}

			in := argv[i+1]
			if !strings.HasPrefix(in, "file:") && !strings.HasPrefix(in, "pipe:") && !strings.Contains(in, "=") {
				t.Fatalf("an input without file: or pipe: was accepted: %q", in)
			}
		}
	})
}
