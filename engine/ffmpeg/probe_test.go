package ffmpeg

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func asError(t *testing.T, err error) *Error {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("error %v is not a *ffmpeg.Error", err)
	}
	return e
}

func TestSniffRefusesTextAndUnknownBytes(t *testing.T) {
	text := map[string]string{
		"hls playlist":   "#EXTM3U\n#EXT-X-VERSION:3\n#EXTINF:10,\nhttp://example.com/a.ts\n",
		"ffconcat":       "ffconcat version 1.0\nfile '/etc/passwd'\n",
		"ass subtitle":   "[Script Info]\nTitle: x\n[Events]\nDialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,hi\n",
		"webvtt":         "WEBVTT\n\n00:00.000 --> 00:01.000\nhello\n",
		"xml":            "<?xml version=\"1.0\"?><MPD/>",
		"markup":         "<html><body>hi</body></html>",
		"passwd":         "root:x:0:0:root:/root:/bin/bash\ndaemon:x:1:1:daemon:/usr/sbin:/usr/sbin/nologin\n",
		"srt":            "1\n00:00:01,000 --> 00:00:02,000\nhello there\n",
		"bom playlist":   "\xef\xbb\xbf#EXTM3U\n",
		"text with ftyp": "xxxxftyp is a word in this sentence about files and more text to make it long",
		"empty":          "",
		"binary noise":   "\x00\x01\x02\x03\x04\x05\x06\x07\x08\x09\x0a\x0b\x0c\x0d\x0e\x0f",
		"sdp":            "v=0\no=- 0 0 IN IP4 127.0.0.1\ns=x\nm=video 5000 RTP/AVP 96\n",
	}
	for name, body := range text {
		e := asError(t, Sniff([]byte(body)))
		if e.Code != CodeUnsupportedMedia || e.Reason != "unrecognized_format" {
			t.Errorf("%s: %+v", name, e)
		}
		if strings.Contains(e.Message+e.Hint, "/") {
			t.Errorf("%s: the message names a path: %q", name, e.Message)
		}
	}
}

func TestSniffSaysThatSVGIsNotSupported(t *testing.T) {
	for name, body := range map[string]string{
		"bare":     `<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"/>`,
		"with xml": `<?xml version="1.0"?>` + "\n<!-- logo -->\n" + `<SVG viewBox="0 0 1 1"></SVG>`,
	} {
		e := asError(t, Sniff([]byte(body)))
		if e.Code != CodeUnsupportedMedia || !strings.Contains(e.Message, "SVG") || !strings.Contains(e.Hint, "PNG") {
			t.Errorf("%s: %+v", name, e)
		}
	}

	if e := asError(t, Sniff([]byte("<html><body>hi</body></html>"))); strings.Contains(e.Message, "SVG") {
		t.Errorf("html is not svg: %+v", e)
	}
}

func TestSniffAcceptsTheSignaturesOfFF05(t *testing.T) {
	ts := make([]byte, 400)
	ts[0], ts[188], ts[376] = 0x47, 0x47, 0x47
	pad := func(s string) []byte { return append([]byte(s), make([]byte, 64)...) }
	ok := map[string][]byte{
		"mp4":      pad("\x00\x00\x00\x18ftypisom"),
		"matroska": pad("\x1a\x45\xdf\xa3"),
		"avi":      pad("RIFF\x00\x00\x00\x00AVI LIST"),
		"wav":      pad("RIFF\x00\x00\x00\x00WAVEfmt "),
		"webp":     pad("RIFF\x00\x00\x00\x00WEBPVP8 "),
		"id3":      pad("ID3\x04\x00"),
		"mp3 sync": pad("\xff\xfb\x90\x00"),
		"adts":     pad("\xff\xf1\x50\x80"),
		"ogg":      pad("OggS\x00\x02"),
		"flac":     pad("fLaC\x00\x00"),
		"mpeg ps":  pad("\x00\x00\x01\xba\x44"),
		"flv":      pad("FLV\x01\x05"),
		"png":      pad("\x89PNG\r\n\x1a\n\x00\x00"),
		"jpeg":     pad("\xff\xd8\xff\xe0\x00\x10JFIF"),
		"gif":      pad("GIF89a\x01\x00"),
		"mpeg-ts":  ts,
	}
	for name, b := range ok {
		if err := Sniff(b); err != nil {
			t.Errorf("%s refused: %v", name, err)
		}
	}
	// Three sync bytes at 0, 188 and 376 are all required.
	bad := make([]byte, 400)
	bad[0], bad[188] = 0x47, 0x47
	if Sniff(bad) == nil {
		t.Error("a transport stream with two sync bytes passed")
	}
	if Sniff(pad("\xff\xe8\x00")) == nil {
		t.Error("a reserved MPEG audio version passed")
	}
}

func TestCheckProbe(t *testing.T) {
	video := func(format string, w, h int, dur string) string {
		return `{"streams":[{"index":0,"codec_type":"video","codec_name":"mpeg4","width":` + itoa(
			w,
		) + `,"height":` + itoa(
			h,
		) + `},` +
			`{"index":1,"codec_type":"audio","codec_name":"aac"}],"format":{"format_name":"` + format + `","nb_streams":2,"duration":"` + dur + `","size":"1000"}}`
	}
	if _, err := CheckProbe([]byte(video("mov,mp4,m4a,3gp,3g2,mj2", 1080, 1920, "60.0")), Ceilings{}); err != nil {
		t.Fatalf("a normal phone clip: %v", err)
	}

	cases := []struct {
		name, doc, code, reason, limit string
	}{
		{"format not allowed", video("hls", 10, 10, "1"), CodeUnsupportedMedia, "unrecognized_format", ""},
		{"concat format", video("concat", 10, 10, "1"), CodeUnsupportedMedia, "unrecognized_format", ""},
		{"mixed format list", video("mov,hls", 10, 10, "1"), CodeUnsupportedMedia, "unrecognized_format", ""},
		{
			"no streams",
			`{"streams":[],"format":{"format_name":"wav"}}`,
			CodeUnsupportedMedia,
			"unrecognized_format",
			"",
		},
		{
			"only data",
			`{"streams":[{"index":0,"codec_type":"data","codec_name":"bin_data"}],"format":{"format_name":"mov,mp4,m4a,3gp,3g2,mj2"}}`,
			CodeUnsupportedMedia,
			"no_usable_stream",
			"",
		},
		{
			"unknown codec",
			`{"streams":[{"index":0,"codec_type":"video","codec_name":"unknown"}],"format":{"format_name":"avi"}}`,
			CodeUnsupportedMedia,
			"no_usable_stream",
			"",
		},
		{
			"too many streams",
			`{"streams":[` + strings.TrimSuffix(
				strings.Repeat(`{"index":0,"codec_type":"audio","codec_name":"aac"},`, 17),
				",",
			) + `],"format":{"format_name":"matroska,webm"}}`,
			CodeUnsupportedMedia,
			"too_many_streams",
			"",
		},
		{"side too long", video("avi", 9000, 100, "1"), CodeLimitExceeded, "source_edge", "source_edge"},
		{"too many pixels", video("avi", 8000, 7000, "1"), CodeLimitExceeded, "import_pixels", "import_pixels"},
		{"too long", video("avi", 100, 100, "14401"), CodeLimitExceeded, "import_seconds", "import_seconds"},
		{"not json", `not json`, CodeUnsupportedMedia, "unrecognized_format", ""},
	}
	for _, c := range cases {
		_, err := CheckProbe([]byte(c.doc), Ceilings{})
		e := asError(t, err)
		if e.Code != c.code || e.Reason != c.reason {
			t.Errorf("%s: %s %s, want %s %s", c.name, e.Code, e.Reason, c.code, c.reason)
		}
		if c.limit != "" &&
			(e.Details["limit"] != c.limit || e.Details["value"] == nil || e.Details["ceiling"] == nil) {
			t.Errorf("%s: details %v", c.name, e.Details)
		}
	}

	// A cover picture is not subject to the picture ceilings, and the ceilings are configurable.
	cover := `{"streams":[{"index":0,"codec_type":"audio","codec_name":"mp3"},{"index":1,"codec_type":"video","codec_name":"mjpeg","width":9000,"height":9000,"disposition":{"attached_pic":1}}],"format":{"format_name":"mp3","nb_streams":2,"duration":"5"}}`
	if _, err := CheckProbe([]byte(cover), Ceilings{}); err != nil {
		t.Errorf("cover art refused: %v", err)
	}
	e := asError(t, errOf(CheckProbe([]byte(video("avi", 100, 100, "61")), Ceilings{MaxSeconds: 60})))
	if e.Details["variable"] != "VUKEM_VIDEO_MAX_IMPORT_SECONDS" || e.Details["ceiling"] != int64(60) {
		t.Errorf("details = %v", e.Details)
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

func errOf(_ *ProbeResult, err error) error { return err }

func TestSecurity_T14_NonMediaInputsRefused(t *testing.T) {
	// A runner that cannot start anything: a refusal before FFmpeg must not need one.
	r := &Runner{}
	text := map[string]string{
		"playlist":  "#EXTM3U\n#EXTINF:5,\nfile:///etc/passwd\n",
		"ffconcat":  "ffconcat version 1.0\nfile 'a.mp4'\nfile '/etc/passwd'\n",
		"subtitles": "[Script Info]\nTitle: x\n\n[Events]\nDialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,hello\n",
		"text":      "root:x:0:0:root:/root:/bin/bash\nnobody:x:65534:65534:nobody:/:/usr/sbin/nologin\n",
		"pattern":   "img%03d.png\nimg%03d.png\nimg%03d.png\n",
	}
	store := t.TempDir()
	for name, body := range text {
		p := filepath.Join(store, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := r.Probe(context.Background(), nil, p, Ceilings{})
		e := asError(t, err)
		if e.Code != CodeUnsupportedMedia || e.Reason != "unrecognized_format" {
			t.Errorf("%s: %+v", name, e)
		}
	}
	if r.started.Load() != 0 {
		t.Fatalf("FFmpeg or ffprobe was started %d times for a non-media file", r.started.Load())
	}
	if _, err := r.Probe(
		context.Background(),
		nil,
		filepath.Join(store, "missing"),
		Ceilings{},
	); asError(
		t,
		err,
	).Code != "E_FILE_NOT_FOUND" {
		t.Errorf("a missing file: %v", err)
	}
}

// TestSecurity_T14_RealMediaPassesAndHostileFilesFail runs the pinned build: every supported
// sample passes the format allowlist (so the multi-name demuxers of FF-04 work as a
// whitelist) and every hostile sample is refused.
func TestSecurity_T14_RealMediaPassesAndHostileFilesFail(t *testing.T) {
	r, dir := pinnedRunner(t)
	in := r.In
	src := "testsrc2=s=64x64:r=10:d=1"
	aud := "sine=f=440:d=1"
	v := []string{"-f", "lavfi", "-i", src}
	a := []string{"-f", "lavfi", "-i", aud}
	va := append(append([]string{}, v...), a...)

	samples := map[string][]string{
		"s.mp4":  append(append([]string{}, va...), "-c:v", "mpeg4", "-c:a", "aac", "-f", "mp4"),
		"s.mov":  append(append([]string{}, va...), "-c:v", "mpeg4", "-c:a", "pcm_s16le", "-f", "mov"),
		"s.mkv":  append(append([]string{}, v...), "-c:v", "mpeg4", "-f", "matroska"),
		"s.webm": append(append([]string{}, v...), "-c:v", "libvpx-vp9", "-deadline", "realtime", "-f", "webm"),
		"s.avi":  append(append([]string{}, va...), "-c:v", "mpeg4", "-c:a", "pcm_s16le", "-f", "avi"),
		"s.ts":   append(append([]string{}, va...), "-c:v", "mpeg4", "-c:a", "aac", "-f", "mpegts"),
		"s.mpg":  append(append([]string{}, v...), "-c:v", "mpeg2video", "-f", "mpeg"),
		"s.flv":  append(append([]string{}, va...), "-c:v", "flv", "-c:a", "aac", "-f", "flv"),
		"s.wav":  append(append([]string{}, a...), "-c:a", "pcm_s16le", "-f", "wav"),
		"s.mp3":  append(append([]string{}, a...), "-c:a", "libmp3lame", "-f", "mp3"),
		"s.ogg":  append(append([]string{}, a...), "-c:a", "libvorbis", "-f", "ogg"),
		"s.flac": append(append([]string{}, a...), "-c:a", "flac", "-f", "flac"),
		"s.aac":  append(append([]string{}, a...), "-c:a", "aac", "-f", "adts"),
		"s.png":  append(append([]string{}, v...), "-frames:v", "1", "-c:v", "png", "-f", "image2"),
		"s.jpg": append(
			append([]string{}, v...),
			"-frames:v",
			"1",
			"-vf",
			"format=yuvj420p",
			"-c:v",
			"mjpeg",
			"-f",
			"image2",
		),
		"s.webp": append(append([]string{}, v...), "-frames:v", "1", "-c:v", "libwebp", "-f", "webp"),
		"s.gif":  append(append([]string{}, v...), "-frames:v", "1", "-f", "gif"),
	}
	for name, args := range samples {
		out := gen(t, in, dir, name, args...)
		store := putStore(t, r, out)
		pr, err := r.Probe(context.Background(), dir, store, Ceilings{})
		if err != nil {
			t.Errorf("%s: a supported sample was refused: %v", name, err)
			continue
		}
		if len(pr.Streams) == 0 || !bytes.Contains(pr.Raw, []byte("format_name")) {
			t.Errorf("%s: empty probe %+v", name, pr)
		}
	}

	// A file that passes the signature check but whose demuxer is not in the whitelist: an
	// AIFF behind an empty ID3 tag. FFmpeg's own detection would open it; the whitelist must not.
	aiff := gen(t, in, dir, "s.aiff", append(append([]string{}, a...), "-c:a", "pcm_s16be", "-f", "aiff")...)
	raw, err := os.ReadFile(aiff)
	if err != nil {
		t.Fatal(err)
	}
	hostile := map[string][]byte{
		"aiff behind id3": append([]byte("ID3\x04\x00\x00\x00\x00\x00\x00"), raw...),
		"ftyp and garbage": append(
			[]byte("\x00\x00\x00\x18ftypisom\x00\x00\x02\x00isomiso2"),
			bytes.Repeat([]byte{0x9a, 0x01, 0xfe}, 200)...),
		"truncated riff":     []byte("RIFF\x00\x00\x00\x00WAVEjunk" + strings.Repeat("\x01", 40)),
		"matroska signature": append([]byte("\x1a\x45\xdf\xa3"), bytes.Repeat([]byte{0xff}, 64)...),
		// The containers and the files the BETA features added: animated images and user fonts.
		"gif with an empty screen": append(
			[]byte("GIF89a\x00\x00\x00\x00\xf0\x00\x00"),
			bytes.Repeat([]byte{0x00, 0x2c}, 40)...),
		"webp animation cut short": append(
			[]byte("RIFF\x20\x00\x00\x00WEBPVP8X\x0a\x00\x00\x00\x12\x00\x00\x00\x00\x00\x00\x00\x00\x00"),
			bytes.Repeat([]byte{0xff, 0x01}, 40)...),
		"apng animation chunk without header": append(
			append([]byte("\x89PNG\r\n\x1a\n"), chunk("acTL", []byte{0, 0, 0xff, 0xff, 0, 0, 0, 0})...),
			chunk("fcTL", bytes.Repeat([]byte{0xff}, 26))...),
		"truetype font": append(
			[]byte("\x00\x01\x00\x00\x00\x0d\x00\x80\x00\x03\x00\x30OS/2"),
			bytes.Repeat([]byte{0x00, 0x81}, 40)...),
		"woff2 font": append([]byte("wOF2\x00\x01\x00\x00"), bytes.Repeat([]byte{0x00, 0x81}, 40)...),
		// Reported since the ALPHA: a movie cut in half, which has its mdat and lost its moov.
		"mp4 cut before the moov": append(
			box("ftyp", []byte("isom"), []byte{0, 0, 2, 0}, []byte("isomiso2")),
			append([]byte("\x00\x10\x00\x00mdat"), bytes.Repeat([]byte{0x65, 0x88}, 300)...)...),
		// The WebM of a VP9 alpha proxy (SDD-011 4.3.5): an EBML header for webm, an open Segment and a
		// Tracks element that is cut short.
		"webm alpha proxy cut short": append([]byte(
			"\x1a\x45\xdf\xa3\x9f\x42\x86\x81\x01\x42\xf7\x81\x01\x42\xf2\x81\x04\x42\xf3\x81\x08\x42\x82\x84webm"+
				"\x42\x87\x81\x04\x42\x85\x81\x02\x18\x53\x80\x67\x01\xff\xff\xff\xff\xff\xff\xff\x16\x54\xae\x6b\xff"),
			bytes.Repeat([]byte{0xe0, 0x81}, 30)...),
	}
	for name, body := range hostile {
		store := putStoreBytes(t, r, body)
		_, err := r.Probe(context.Background(), dir, store, Ceilings{})
		if err == nil {
			t.Errorf("%s: accepted", name)
			continue
		}
		e := asError(t, err)
		t.Logf("%s: %s (%v)", name, e.Reason, e.Details["ffprobe"])
		if e.Code != CodeUnsupportedMedia {
			t.Errorf("%s: %+v", name, e)
		} else if strings.ContainsAny(e.Message, `/\`) {
			t.Errorf("%s: the message names a path: %q", name, e.Message)
		}
	}
}
