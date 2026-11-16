package ffmpeg

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg/ffargs"
)

func validBase(t *testing.T) []string {
	t.Helper()

	return readArgv(t, filepath.Join("testdata", "argv", "valid-base.json"))
}

// replaceAfter returns a copy of argv with the value that follows flag replaced.
func replaceAfter(t *testing.T, argv []string, flag, value string) []string {
	t.Helper()

	i := slices.Index(argv, flag)
	if i < 0 || i+1 >= len(argv) {
		t.Fatalf("no %s in the base argv", flag)
	}

	out := slices.Clone(argv)
	out[i+1] = value

	return out
}

// SDD-007 15 check 13 (FF-02): no network protocol appears in an argv, in the whitelist or in an input.
func TestSecurity_T15_NoNetworkProtocolInArgv(t *testing.T) {
	t.Parallel()

	base := validBase(t)
	if err := ffargs.Check(ffargs.FFmpeg, base, corpusPolicy); err != nil {
		t.Fatalf("the base argv was refused: %v", err)
	}

	for _, w := range []string{"file,pipe,http", "file,pipe,https,tls,tcp", "file,pipe,udp", "file,pipe,rtmp", "file,crypto", "all", "file"} {
		if err := ffargs.Check(
			ffargs.FFmpeg,
			replaceAfter(t, base, "-protocol_whitelist", w),
			corpusPolicy,
		); err == nil {
			t.Errorf("protocol_whitelist %q was accepted", w)
		}
	}

	for _, in := range []string{
		"http://example.com/a.mp4", "https://example.com/a.mp4", "tcp://127.0.0.1:1", "udp://127.0.0.1:1",
		"rtmp://example.com/live", "rtsp://example.com/x", "ftp://example.com/x", "smb://host/share/x",
		"concat:a.mp4|b.mp4", "subfile,,start,0,end,1,,:/etc/passwd", "crypto:file:/x", "data:text/plain;base64,AAAA",
		"//host/share/x.mp4",
	} {
		if err := ffargs.Check(ffargs.FFmpeg, replaceAfter(t, base, "-i", in), corpusPolicy); err == nil {
			t.Errorf("input %q was accepted", in)
		}
	}

	for _, out := range []string{"http://example.com/up", "tcp://127.0.0.1:1", "rtmp://example.com/live"} {
		argv := slices.Concat(base[:len(base)-1], []string{out})
		if err := ffargs.Check(ffargs.FFmpeg, argv, corpusPolicy); err == nil {
			t.Errorf("output %q was accepted", out)
		}
	}
}

// SDD-007 15 check 13 (FF-10): a filter outside the allowlist is refused, however it is spelled or
// wherever it sits in the graph.
func TestSecurity_T15_FilterAllowlist(t *testing.T) {
	t.Parallel()

	base := validBase(t)

	refused := []string{
		"movie", "amovie", "subtitles", "ass", "drawtext", "lut3d", "haldclut", "lut", "geq", "sendcmd", "zmq",
		"azmq", "frei0r", "ladspa", "lv2", "dnn_processing", "ocr", "sofalizer", "afir", "vidstabtransform",
		"deshake", "overlay", "loudnorm", "amix", "concat", "fade",
		// In the ceiling list of FF-10 but not named by SDD-004 or SDD-011, so disabled.
		"anull", "astats",
	}
	for _, f := range refused {
		for _, graph := range []string{f + "=x", "anull," + f + "=x", "[0:a]" + f + "=x[o]", strings.ToUpper(f) + "=x"} {
			if err := ffargs.Check(ffargs.FFmpeg, replaceAfter(t, base, "-af", graph), corpusPolicy); err == nil {
				t.Errorf("filter graph %q was accepted", graph)
			}
		}
	}

	for _, graph := range []string{
		"aresample=48000", "aformat=sample_fmts=fltp:channel_layouts=stereo", "atempo=1.5", "afftdn=nr=12.50:nf=-40.00:tn=1",
		"silencedetect=noise=-35dB:d=0.300", "ebur128=peak=true:framelog=quiet",
	} {
		if err := ffargs.Check(ffargs.FFmpeg, replaceAfter(t, base, "-af", graph), corpusPolicy); err != nil {
			t.Errorf("filter graph %q was refused: %v", graph, err)
		}
	}

	// afftdn takes only a bounded noise reduction and the tracking switch (SDD-007 FF-10).
	for _, graph := range []string{
		"afftdn=nr=98", "afftdn=nr=-1", "afftdn=nr=1e2", "afftdn=nr=12:nf=-10", "afftdn=nr=12:nf=-81",
		"afftdn=nr=12:nf=x", "afftdn=nr=12:tn=0",
		"afftdn=nr=12:bn=/etc/passwd", "afftdn=nr=12;anull", "afftdn=/etc/passwd",
	} {
		if err := ffargs.Check(ffargs.FFmpeg, replaceAfter(t, base, "-af", graph), corpusPolicy); err == nil {
			t.Errorf("filter graph %q was accepted", graph)
		}
	}

	for _, opt := range []string{"fontfile=/etc/passwd", "filename=/etc/passwd", "f=/etc/passwd", "file=/etc/passwd"} {
		if err := ffargs.Check(
			ffargs.FFmpeg,
			replaceAfter(t, base, "-af", "anull,aresample=48000:"+opt),
			corpusPolicy,
		); err == nil {
			t.Errorf("filter option %q was accepted", opt)
		}
	}
}

// SDD-007 6.5, 15 check 15: a helper that hangs is stopped at its deadline and by the stall
// watchdog. The deadlines of the table are scaled down for the runs; the table itself is
// checked against the constants of the specification.
func TestSecurity_T16_TimeoutsEnforced(t *testing.T) {
	r, dir, tool := helperRunner(t)

	t.Run("run deadline", func(t *testing.T) {
		start := time.Now()
		_, err := r.exec(context.Background(), execSpec{
			tool: tool, dir: dir.Dir(), argv: []string{helperFlag + "sleep"}, deadline: 400 * time.Millisecond,
			stdout: &bytes.Buffer{},
		})
		if !errors.Is(err, ErrDeadline) {
			t.Fatalf("error = %v, want ErrDeadline", err)
		}

		if d := time.Since(start); d > 10*time.Second {
			t.Errorf("the run took %s to end after a 400ms deadline", d)
		}
	})

	t.Run("stall", func(t *testing.T) {
		_, err := r.exec(context.Background(), execSpec{
			tool: tool, dir: dir.Dir(), argv: []string{helperFlag + "sleep"}, deadline: time.Minute,
			stall: 400 * time.Millisecond, stdout: &bytes.Buffer{},
		})
		if !errors.Is(err, ErrStalled) {
			t.Fatalf("error = %v, want ErrStalled", err)
		}
	})

	t.Run("the table", func(t *testing.T) {
		const h = 3 * time.Hour

		got := map[string]time.Duration{
			"probe":        ProbeTimeout,
			"frame":        FrameTimeout,
			"ingest":       IngestDeadline(h),
			"export":       ExportDeadline(h),
			"audio":        AudioExtractDeadline(h),
			"transcribe":   TranscribeDeadline(h),
			"stall":        StallTimeout,
			"ingest short": IngestDeadline(10 * time.Second),
			"export short": ExportDeadline(10 * time.Second),
			"audio short": AudioExtractDeadline(
				10 * time.Second,
			),
			"transcribe short": TranscribeDeadline(10 * time.Second),
		}
		want := map[string]time.Duration{
			"probe": 30 * time.Second, "frame": 30 * time.Second, "ingest": 4 * time.Hour, "export": 2 * time.Hour,
			"audio": time.Hour, "transcribe": 6 * time.Hour, "stall": 120 * time.Second,
			"ingest short": 90 * time.Second, "export short": 180 * time.Second,
			"audio short": 70 * time.Second, "transcribe short": 150 * time.Second,
		}

		for k, w := range want {
			if got[k] != w {
				t.Errorf("%s = %s, want %s", k, got[k], w)
			}
		}
	})
}

// SDD-007 9.1, 9.2 item 1: each ceiling of the probe stops its hostile input before any decode,
// with E_LIMIT_EXCEEDED (E_UNSUPPORTED_MEDIA for the streams) and the details of the limit. The
// frame rate ceiling is enforced by ingest from the packet times (TestSummarizeFollowsTheClassificationTable)
// and the file size by the service (import_bytes).
func TestSecurity_T17_ProbeCeilings(t *testing.T) {
	t.Parallel()

	streams := func(n int) string {
		parts := make([]string, n)
		for i := range parts {
			parts[i] = `{"index":` + strconv.Itoa(i) + `,"codec_type":"audio","codec_name":"aac"}`
		}

		return strings.Join(parts, ",")
	}
	video := func(w, h int) string {
		return `{"index":0,"codec_type":"video","codec_name":"h264","width":` + strconv.Itoa(
			w,
		) + `,"height":` + strconv.Itoa(
			h,
		) + `}`
	}
	doc := func(streamsJSON, dur string, n int) []byte {
		return []byte(
			`{"streams":[` + streamsJSON + `],"format":{"format_name":"mov,mp4,m4a,3gp,3g2,mj2","nb_streams":` +
				strconv.Itoa(n) + `,"duration":"` + dur + `","size":"1000"}}`,
		)
	}

	cases := []struct {
		name  string
		raw   []byte
		c     Ceilings
		code  string
		limit string
	}{
		{"side", doc(video(60000, 60000), "1", 1), Ceilings{}, CodeLimitExceeded, "source_edge"},
		{"one side", doc(video(8193, 16), "1", 1), Ceilings{}, CodeLimitExceeded, "source_edge"},
		{"pixels", doc(video(8000, 8000), "1", 1), Ceilings{MaxPixels: 33_177_600}, CodeLimitExceeded, "import_pixels"},
		{
			"pixels, lowered",
			doc(video(1920, 1080), "1", 1),
			Ceilings{MaxPixels: 1_000_000},
			CodeLimitExceeded,
			"import_pixels",
		},
		{"duration", doc(video(64, 64), "2592000", 1), Ceilings{}, CodeLimitExceeded, "import_seconds"},
		{
			"duration, lowered",
			doc(video(64, 64), "61", 1),
			Ceilings{MaxSeconds: 60},
			CodeLimitExceeded,
			"import_seconds",
		},
		{"streams", doc(streams(100), "1", 100), Ceilings{}, CodeUnsupportedMedia, ""},
	}
	for _, tt := range cases {
		_, err := CheckProbe(tt.raw, tt.c)

		var e *Error
		if !errors.As(err, &e) || e.Code != tt.code {
			t.Errorf("%s: error = %v, want %s", tt.name, err, tt.code)

			continue
		}

		if tt.limit != "" && e.Details["limit"] != tt.limit {
			t.Errorf("%s: details = %v, want limit %s", tt.name, e.Details, tt.limit)
		}
	}

	if _, err := CheckProbe(doc(video(7680, 4320), "14400", 1), Ceilings{}); err != nil {
		t.Errorf("a file exactly at the ceilings was refused: %v", err)
	}
}

// SDD-007 4.5, T-16: as uid 0 the runner starts nothing unless VUKEM_VIDEO_INSECURE_ALLOW_ROOT is true.
// The uid comes through a seam so the test runs on every system; Windows has no uid 0, so the real
// uid is never 0 there and only the seam is exercised.
func TestSecurity_T16_RefusesRoot(t *testing.T) {
	r, dir, tool := helperRunner(t)
	spec := execSpec{
		tool: tool, dir: dir.Dir(), argv: []string{helperFlag + "sleep"}, deadline: 400 * time.Millisecond,
		stdout: &bytes.Buffer{},
	}

	old := geteuid
	t.Cleanup(func() { geteuid = old })

	geteuid = func() int { return 0 }

	t.Setenv(EnvAllowRoot, "")

	before := r.started.Load()

	_, err := r.exec(context.Background(), spec)
	if !errors.Is(err, ErrRefusedRoot) {
		t.Fatalf("error = %v, want ErrRefusedRoot", err)
	}

	var fe *Error
	if !errors.As(err, &fe) || fe.Reason != "refused_root" || !strings.Contains(fe.Hint, EnvAllowRoot) {
		t.Errorf("error = %+v, want reason refused_root and a hint naming %s", fe, EnvAllowRoot)
	}

	if r.started.Load() != before {
		t.Error("a child was started as root")
	}

	t.Setenv(EnvAllowRoot, "false")

	if _, err := r.exec(context.Background(), spec); !errors.Is(err, ErrRefusedRoot) {
		t.Errorf("with the variable false: error = %v, want ErrRefusedRoot", err)
	}

	t.Setenv(EnvAllowRoot, "true")

	if _, err := r.exec(context.Background(), spec); errors.Is(err, ErrRefusedRoot) {
		t.Errorf("with the variable true the runner still refused: %v", err)
	}

	geteuid = func() int { return 1000 }

	t.Setenv(EnvAllowRoot, "")

	if _, err := r.exec(context.Background(), spec); errors.Is(err, ErrRefusedRoot) {
		t.Errorf("as a regular user the runner refused: %v", err)
	}
}
