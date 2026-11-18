package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg"
)

func mustGen(t *testing.T, path string, err error) string {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func (e *env) publishedPath(hash string, a *Artifact) string {
	return filepath.Join(e.artifacts(), hash, ArtifactName(a.Kind, a.Profile, a.Variant, a.Ext))
}

func TestIngestPortraitClip(t *testing.T) {
	e := newEnv(t)
	p, err := e.gen.Portrait(context.Background())
	var progress []Progress
	hash, res, ierr := e.ingest(
		context.Background(),
		mustGen(t, p, err),
		func(p Progress) { progress = append(progress, p) },
	)
	if ierr != nil {
		t.Fatal(ierr)
	}
	if res.Kind != KindVideo {
		t.Fatalf("kind %s", res.Kind)
	}
	want := []string{
		"chunks-1.json", "loudness-1.json", "normalized-1.mov", "peaks-1.bin", "probe-1.json", "proxy-1.mp4",
		"silence-1.json",
		"thumbnails-1-w160.jpg", "thumbnails-1-w320.jpg", "thumbnails-1-w640.jpg", "thumbnails-1.jpg",
	}
	if got := res.names(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("artifacts\n got  %v\n want %v", got, want)
	}
	f := res.Facts
	if f.FrameRate != (Rational{30, 1}) || f.FrameCount != 90 || f.SampleCount != 144000 || f.Width != 360 ||
		f.Height != 640 ||
		!f.HasAudio {
		t.Errorf("facts %+v", f)
	}

	// The artifact on disk is what the facts say, and is read-only.
	d := e.probeArtifact(hash, "normalized-1.mov")
	v, a := d.first("video"), d.first("audio")
	if v == nil || a == nil || v.Width != 360 || v.Height != 640 || v.CodecName != "mpeg4" || v.PixFmt != "yuv420p" ||
		!sameRate(v.RFrameRate, Rational{30, 1}) {
		t.Errorf("normalized video %+v", v)
	}
	if a.CodecName != "pcm_s16le" || a.SampleRate != "48000" || a.Channels != 2 || a.DurationTS != 144000 {
		t.Errorf("normalized audio %+v", a)
	}
	for _, art := range res.Artifacts {
		path := e.publishedPath(hash, &art)
		st, err := os.Stat(path)
		if err != nil || st.Size() != art.Size || len(art.SHA256) != 64 ||
			art.RelPath != "artifacts/"+hash+"/"+filepath.Base(path) {
			t.Errorf("%s: %v %+v", art.Kind, err, art)
		}
		if runtime.GOOS != "windows" && st.Mode().Perm() != 0o400 {
			t.Errorf("%s mode %v, want 0400", path, st.Mode().Perm())
		}
	}
	if a := res.find("thumbnails", ""); a == nil || a.Meta.(ThumbLayout).Count < 2 {
		t.Errorf("sheet meta %+v", a)
	}
	if n := res.find("normalized", ""); n == nil || n.Meta.(Facts).FrameCount != 90 {
		t.Errorf("normalized meta %+v", n)
	}

	// probe-1.json holds the summary and no server path.
	b := readJSONFile(t, e.publishedPath(hash, res.find("probe", "")))
	if strings.Contains(string(b), strings.ReplaceAll(e.ws, `\`, `\\`)) ||
		strings.Contains(string(b), filepath.ToSlash(e.ws)) {
		t.Error("probe-1.json carries the workspace path")
	}
	var pj struct {
		Summary Summary `json:"summary"`
		FFprobe struct {
			Format struct {
				Filename string `json:"filename"`
			} `json:"format"`
		} `json:"ffprobe"`
	}
	if err := json.Unmarshal(
		b,
		&pj,
	); err != nil || pj.FFprobe.Format.Filename != "" || pj.Summary.Kind != KindVideo ||
		pj.Summary.Video.NominalFPS != (Rational{30, 1}) {
		t.Errorf("probe json: %v %+v", err, pj.Summary.Video)
	}

	// Progress never decreases and ends at the publish step.
	last := -1.0
	for _, p := range progress {
		if p.Fraction < last-1e-9 {
			t.Errorf("progress went from %v to %v", last, p.Fraction)
		}
		last = p.Fraction
	}
	if len(progress) == 0 || progress[len(progress)-1].Fraction < 0.99 {
		t.Errorf("progress ended at %v", progress)
	}

	// The work directory is gone.
	if entries, _ := os.ReadDir(filepath.Join(e.ws, "tmp")); len(entries) != 0 {
		t.Errorf("work directories left: %v", entries)
	}
}

func TestIngestLandscapeAndPeaks(t *testing.T) {
	e := newEnv(t)
	p, err := e.gen.Landscape(context.Background())
	hash, res := e.mustIngest(mustGen(t, p, err))
	if f := res.Facts; f.Width != 640 || f.Height != 360 {
		t.Errorf("facts %+v", f)
	}
	b, err := os.ReadFile(e.publishedPath(hash, res.find("peaks", "")))
	if err != nil {
		t.Fatal(err)
	}
	pk, err := ParsePeaks(b)
	if err != nil || pk.SampleCount != 144000 || len(pk.Levels[0]) != 1125 {
		t.Errorf("peaks: %v %+v", err, pk)
	}
}

// TestIngestRotatedIsUpright: the display matrix is applied, so the normalized media is the
// displayed size with no matrix left (SDD-011 3.2).
func TestIngestRotatedIsUpright(t *testing.T) {
	e := newEnv(t)
	p, err := e.gen.Rotated(context.Background())
	hash, res := e.mustIngest(mustGen(t, p, err))
	sum := res.Summary.Video
	if sum.CodedWidth != 640 || sum.CodedHeight != 360 || !sum.SwapDims || sum.DisplayWidth != 360 ||
		sum.DisplayHeight != 640 {
		t.Fatalf("summary %+v", sum)
	}
	v := e.probeArtifact(hash, "normalized-1.mov").first("video")
	if v.Width != 360 || v.Height != 640 {
		t.Errorf("normalized is %dx%d, want 360x640 upright", v.Width, v.Height)
	}
	for _, sd := range v.SideData {
		if sd.Type == "Display Matrix" {
			t.Error("the normalized media still carries a display matrix")
		}
	}
}

// TestIngestVariableRateIsConstant: the nominal capture rate, not the container average, and every
// packet on its slot.
func TestIngestVariableRateIsConstant(t *testing.T) {
	e := newEnv(t)
	p, err := e.gen.VFR(context.Background())
	hash, res := e.mustIngest(mustGen(t, p, err))
	vf := res.Summary.Video
	if vf.NominalFPS != (Rational{60, 1}) || !vf.VFR {
		t.Fatalf("summary nominal %v vfr %v (measured %v)", vf.NominalFPS, vf.VFR, vf.MeasuredFPS)
	}
	v := e.probeArtifact(hash, "normalized-1.mov").first("video")
	if !sameRate(v.RFrameRate, Rational{60, 1}) || !sameRate(v.AvgFrameRate, Rational{60, 1}) {
		t.Errorf("normalized rates %s / %s, want 60/1", v.RFrameRate, v.AvgFrameRate)
	}
	if res.Facts.HasAudio || res.find("peaks", "") != nil || res.find("loudness", "") != nil {
		t.Error("a video without sound has audio artifacts")
	}
	// 4 s at 60 fps nominal: the clip is 4 s long, whatever frames were missing.
	if res.Facts.FrameCount < 230 || res.Facts.FrameCount > 244 {
		t.Errorf("frame count %d", res.Facts.FrameCount)
	}
}

func TestIngestAudioFile(t *testing.T) {
	e := newEnv(t)
	p, err := e.gen.Audio(context.Background())
	hash, res := e.mustIngest(mustGen(t, p, err))
	if res.Kind != KindAudio {
		t.Fatal(res.Kind)
	}
	want := []string{
		"loudness-1.json",
		"normalized-1.wav",
		"peaks-1.bin",
		"probe-1.json",
		"proxy-1.mp4",
		"silence-1.json",
	}
	if got := res.names(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("artifacts %v", got)
	}
	if res.Facts.SampleCount < 3*48000-3000 || res.Facts.SampleCount > 3*48000+3000 {
		t.Errorf("samples %d", res.Facts.SampleCount)
	}
	var s Silence
	if err := json.Unmarshal(
		readJSONFile(t, e.publishedPath(hash, res.find("silence", ""))),
		&s,
	); err != nil ||
		len(s.Sets) != 4 {
		t.Fatalf("silence: %v", err)
	}
	// One second of silence in the middle, within the tolerance of SDD-011 9.1.
	r := s.Sets[1].Ranges
	if len(r) != 1 || abs64(r[0].Start-48000) > 3000 || abs64(r[0].End-96000) > 3000 {
		t.Errorf("silence ranges at -35 dB: %v", r)
	}
	var l Loudness
	if err := json.Unmarshal(
		readJSONFile(t, e.publishedPath(hash, res.find("loudness", ""))),
		&l,
	); err != nil || l.Silent ||
		l.IntegratedLUFS == nil {
		t.Errorf("loudness %+v %v", l, err)
	}
	if res.find("thumbnails", "w160") != nil {
		t.Error("an audio asset has a poster")
	}
}

func abs64(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

func TestIngestImage(t *testing.T) {
	e := newEnv(t)
	p, err := e.gen.Image(context.Background())
	_, res := e.mustIngest(mustGen(t, p, err))
	want := []string{
		"normalized-1.png",
		"probe-1.json",
		"proxy-1.jpg",
		"thumbnails-1-w160.jpg",
		"thumbnails-1-w320.jpg",
		"thumbnails-1-w640.jpg",
	}
	if got := res.names(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("artifacts %v", got)
	}
	if res.Facts.Width != 200 || res.Facts.Height != 100 {
		t.Errorf("facts %+v", res.Facts)
	}
}

func TestIngestImageWithAlphaReportsIt(t *testing.T) {
	e := newEnv(t)
	p, err := e.gen.AlphaImage(context.Background())
	_, res := e.mustIngest(mustGen(t, p, err))

	if !res.Facts.HasAlpha {
		t.Errorf("a PNG with an alpha channel has no has_alpha: %+v", res.Facts)
	}

	p, err = e.gen.Image(context.Background())
	if _, res = e.mustIngest(mustGen(t, p, err)); res.Facts.HasAlpha {
		t.Errorf("an opaque PNG reports has_alpha: %+v", res.Facts)
	}
}

func TestIngestFailuresPublishNothing(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	// A file whose data ends early: the index claims the whole clip.
	p, err := e.gen.Truncated(ctx)
	hash, _, ierr := e.ingest(ctx, mustGen(t, p, err), nil)
	var fe *ffmpeg.Error
	if !errors.As(ierr, &fe) || fe.Code == "" {
		t.Fatalf("truncated: %v", ierr)
	}
	t.Logf("truncated file: %s %s: %s", fe.Code, fe.Reason, fe.Message)
	if fe.Code != CodeIngestFailed && fe.Code != ffmpeg.CodeUnsupportedMedia {
		t.Errorf("code %s", fe.Code)
	}
	if entries, _ := os.ReadDir(filepath.Join(e.artifacts(), hash)); len(entries) != 0 {
		t.Errorf("a failed ingest published %v", entries)
	}
	if entries, _ := os.ReadDir(filepath.Join(e.ws, "tmp")); len(entries) != 0 {
		t.Errorf("work directories left: %v", entries)
	}
}

func TestIngestRefusesWhatProbeRefuses(t *testing.T) {
	e := newEnv(t)
	hash := e.put(writeText(t, "playlist.m3u8", "#EXTM3U\n#EXTINF:5,\nfile:///etc/passwd\n"))
	_, err := Run(
		context.Background(),
		Input{Runner: e.r, Caps: e.caps, Hash: hash, ArtifactsDir: e.artifacts(), JobID: "job_x"},
	)
	var fe *ffmpeg.Error
	if !errors.As(err, &fe) || fe.Code != ffmpeg.CodeUnsupportedMedia {
		t.Fatalf("playlist: %v", err)
	}
	if _, err := Run(
		context.Background(),
		Input{Runner: e.r, Caps: e.caps, Hash: "../x", ArtifactsDir: e.artifacts(), JobID: "job_y"},
	); err == nil {
		t.Error("a bad hash was accepted")
	}
}

func writeText(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestIngestCancelKillsTheRunAndPublishesNothing(t *testing.T) {
	e := newEnv(t)
	p, err := e.gen.Landscape(context.Background())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hash, _, ierr := e.ingest(ctx, mustGen(t, p, err), func(Progress) { cancel() })
	if !errors.Is(ierr, context.Canceled) {
		t.Fatalf("error = %v, want one wrapping context.Canceled", ierr)
	}
	if entries, _ := os.ReadDir(filepath.Join(e.artifacts(), hash)); len(entries) != 0 {
		t.Errorf("a cancelled ingest published %v", entries)
	}
	if entries, _ := os.ReadDir(filepath.Join(e.ws, "tmp")); len(entries) != 0 {
		t.Errorf("work directories left: %v", entries)
	}
}

// TestIngestSameBytesTwice: a second ingest of the same original finds the artifacts in place.
func TestIngestSameBytesTwice(t *testing.T) {
	e := newEnv(t)
	p, err := e.gen.Landscape(context.Background())
	path := mustGen(t, p, err)
	h1, r1 := e.mustIngest(path)
	h2, r2 := e.mustIngest(path)
	if h1 != h2 || len(r1.Artifacts) != len(r2.Artifacts) {
		t.Fatal("different results for the same bytes")
	}
	if entries, _ := os.ReadDir(e.store.Dir()); len(entries) != 2 { // .incoming and one folder
		t.Errorf("store listing %v", entries)
	}
	if n, _ := os.ReadDir(filepath.Join(e.artifacts(), h1)); len(n) != len(r1.Artifacts) {
		t.Errorf("%d files for %d artifacts", len(n), len(r1.Artifacts))
	}
}

// TestIngestLong60Seconds is the NFR-005 measurement: 60 s 1080x1920 within 30 s on the reference
// machine. It is opt-in (VUKEM_VIDEO_NFR=1) because the fixture alone takes about a minute to make.
func TestIngestLong60Seconds(t *testing.T) {
	if os.Getenv("VUKEM_VIDEO_NFR") == "" {
		t.Skip("set VUKEM_VIDEO_NFR=1 to run the 60 s 1080p ingest benchmark")
	}
	e := newEnv(t)
	p, err := e.gen.Long(context.Background())
	path := mustGen(t, p, err)
	start := time.Now()
	_, res := e.mustIngest(path)
	took := time.Since(start)
	t.Logf("60 s 1080x1920 ingest: %s; per step %v", took.Round(10*time.Millisecond), res.TimingsMS)
	if res.Facts.FrameCount != 1800 || res.Facts.SampleCount != 2880000 {
		t.Errorf("facts %+v", res.Facts)
	}
	if took > 30*time.Second {
		t.Errorf("took %s, NFR-005 allows 30 s", took)
	}
}

// The chunk index next to the video proxy describes every sample of the proxy: the arrays have one
// entry per frame, the first sample is a key frame, the codec is VP9 and the coded size is the proxy's.
func TestIngestWritesTheChunkIndex(t *testing.T) {
	e := newEnv(t)
	p, err := e.gen.Portrait(context.Background())
	hash, res := e.mustIngest(mustGen(t, p, err))

	art := res.find("chunks", "")
	if art == nil {
		t.Fatal("no chunks artifact")
	}

	var idx ChunkIndex
	if err := json.Unmarshal(readJSONFile(t, e.publishedPath(hash, art)), &idx); err != nil {
		t.Fatal(err)
	}

	n := res.Facts.FrameCount
	if idx.Version != ChunkIndexVersion || !strings.HasPrefix(idx.Codec, "vp09.") ||
		len(idx.Offsets) != n || len(idx.Sizes) != n || len(idx.TimestampsUS) != n {
		t.Fatalf("index %d samples, want %d: %+v", len(idx.Sizes), n, idx.Codec)
	}

	pw, ph := ProxySize(res.Facts.Width, res.Facts.Height)
	if idx.Width != pw || idx.Height != ph {
		t.Errorf("coded size %dx%d, want %dx%d", idx.Width, idx.Height, pw, ph)
	}

	if len(idx.Keyframes) == 0 || idx.Keyframes[0] != 0 || idx.TimestampsUS[0] != 0 {
		t.Errorf("first sample: keyframes %v, first time %d", idx.Keyframes, idx.TimestampsUS[0])
	}

	proxy := e.publishedPath(hash, res.find("proxy", ""))
	st, err := os.Stat(proxy)
	if err != nil {
		t.Fatal(err)
	}

	for i := range idx.Offsets {
		if idx.Offsets[i] <= 0 || idx.Sizes[i] <= 0 || idx.Offsets[i]+idx.Sizes[i] > st.Size() {
			t.Fatalf("sample %d is outside the proxy of %d bytes: offset %d, size %d",
				i, st.Size(), idx.Offsets[i], idx.Sizes[i])
		}
	}
}

func TestReadChunkIndexRefusesWhatIsNotAnMP4WithVideo(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	junk := filepath.Join(dir, "junk.mp4")

	if err := os.WriteFile(junk, []byte("not an mp4 file at all, really"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := ReadChunkIndex(junk); err == nil {
		t.Error("a file without boxes was indexed")
	}

	if _, err := ReadChunkIndex(filepath.Join(dir, "missing.mp4")); err == nil {
		t.Error("a missing file was indexed")
	}
}

// A source with alpha gets a WebM proxy that keeps it (VP9 with an alpha plane, Opus for the sound) and PNG
// posters and thumbnails that keep it, and no chunk index (VV-013, SDD-011 4.3.5 and 4.4).
func TestIngestAlphaClipKeepsAlphaInProxyAndThumbnails(t *testing.T) {
	e := newEnv(t)
	p, err := e.gen.AlphaClipWithSound(context.Background(), "alpha-sound.mov", 160, 90, 30, 2)
	hash, res := e.mustIngest(mustGen(t, p, err))

	want := []string{
		"loudness-1.json", "normalized-1.mov", "peaks-1.bin", "probe-1.json", "proxy-1.webm", "silence-1.json",
		"thumbnails-1-w160.png", "thumbnails-1-w320.png", "thumbnails-1-w640.png", "thumbnails-1.png",
	}
	if got := res.names(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("artifacts\n got  %v\n want %v", got, want)
	}
	if !res.Facts.HasAlpha || !res.Facts.HasAudio {
		t.Fatalf("facts %+v", res.Facts)
	}

	d := e.probeArtifact(hash, "proxy-1.webm")
	v, a := d.first("video"), d.first("audio")
	if v == nil || v.CodecName != "vp9" || !hasAlphaMode(v) {
		t.Errorf("proxy video %+v", v)
	}
	if a == nil || a.CodecName != "opus" {
		t.Errorf("proxy audio %+v", a)
	}

	// The left half of the picture is transparent and the right half opaque red, in every poster and in
	// the first tile of the sheet.
	for _, name := range []string{"thumbnails-1-w160.png", "thumbnails-1-w640.png", "thumbnails-1.png"} {
		f, err := os.Open(filepath.Join(e.artifacts(), hash, name))
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(f)
		_ = f.Close()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		b := img.Bounds()
		tileW := b.Dx()
		if name == "thumbnails-1.png" {
			l, ok := res.find("thumbnails", "").Meta.(ThumbLayout)
			if !ok {
				t.Fatalf("the sheet has no layout: %+v", res.find("thumbnails", "").Meta)
			}
			tileW = l.TileW
		}
		_, _, _, left := img.At(tileW/4, b.Dy()/2).RGBA()
		r, g, bl, right := img.At(3*tileW/4, b.Dy()/2).RGBA()
		if left != 0 || right != 0xFFFF || r>>8 < 200 || g>>8 > 60 || bl>>8 > 60 {
			t.Errorf("%s: left alpha %d, right alpha %d colour (%d,%d,%d)", name, left>>8, right>>8, r>>8, g>>8, bl>>8)
		}
	}
}

// A source with alpha and no sound has a video-only WebM proxy.
func TestIngestAlphaClipWithoutSound(t *testing.T) {
	e := newEnv(t)
	p, err := e.gen.AlphaClip(context.Background(), "alpha.mov", 160, 90, 30, 2)
	hash, res := e.mustIngest(mustGen(t, p, err))
	if res.find("proxy", "") == nil || res.find("chunks", "") != nil || res.Facts.HasAudio {
		t.Fatalf("artifacts %v facts %+v", res.names(), res.Facts)
	}
	d := e.probeArtifact(hash, "proxy-1.webm")
	if v := d.first("video"); v == nil || !hasAlphaMode(v) || d.first("audio") != nil {
		t.Errorf("proxy %+v", d.Streams)
	}
}
