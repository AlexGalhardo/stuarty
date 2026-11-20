package templates_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	coreimage "github.com/AlexGalhardo/stuarty/core/image"
	"github.com/AlexGalhardo/stuarty/core/lint"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/export"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
	"github.com/AlexGalhardo/stuarty/engine/ingest"
	"github.com/AlexGalhardo/stuarty/engine/store"
)

// The golden files of the templates (SDD-018 section 17): a change in the core, in the renderer or in a template
// that changes a pixel or a document is caught here. `go test ./web/resources/templates -update` rewrites them,
// and every rewrite is a reviewed change. TEMPLATES_DUMP=<folder> also writes the frames as PNG files to look at.
var update = flag.Bool("update", false, "rewrite the golden files of testdata/golden")

const (
	envFFmpegDir = "STUARTY_TEST_FFMPEG_DIR"
	fixtureSecs  = 7
	fixtureFPS   = 30
)

// copyOf is the text each text slot gets in the golden instances: real sentences, so a line that does not fit its
// box shows in the pixels.
var copyOf = map[string]string{
	"hook": "Tired of soggy fries?", "problem": "Every batch comes out limp", "product_line": "Meet the AirCrisp basket",
	"proof": "4.8 stars from 2,300 cooks", "offer": "20% off this week", "call_to_action": "Order today",
	"title": "Bake a first sourdough", "subtitle": "For home bakers", "speaker": "Ana Souza, head baker",
}

// mediaOrder is the fixture a media slot gets, by its position: five clearly different colours.
var colours = []string{"0x2A9D8F", "0xE9C46A", "0xF4A261", "0xE76F51", "0x264653"}

func sortedSlots(doc *timeline.Document) []*timeline.Item {
	var items []*timeline.Item

	for id := range doc.Items {
		it := doc.Items[id]
		if it.Slot != nil {
			items = append(items, &it)
		}
	}

	slices.SortFunc(items, func(a, b *timeline.Item) int { return strings.Compare(a.Slot.Name, b.Slot.Name) })

	return items
}

// instance makes a project from a template the way TemplateInstantiate does: the template, marked as an instance,
// then one slot.fill for each slot. assets maps the colour index of a media slot to the asset it takes.
func instance(t *testing.T, tpl template, assetFor func(i int) timeline.Asset, withMusic bool) *timeline.Document {
	t.Helper()

	doc := tpl.doc(t)
	doc.ID = id("prj", 2)
	doc.Name = "Golden " + strings.TrimSuffix(filepath.Base(tpl.file), ".template.json")
	doc.Template.Instance = true

	var ops []op

	media := 0

	for _, it := range sortedSlots(doc) {
		switch it.Slot.Kind {
		case "text":
			text, ok := copyOf[it.Slot.Name]
			if !ok {
				t.Fatalf("%s: no copy for the text slot %s", tpl.file, it.Slot.Name)
			}

			ops = append(ops, o("slot.fill", map[string]any{"name": it.Slot.Name, "text": text}))
		case "media":
			if it.Slot.Name == "music" {
				if !withMusic {
					ops = append(ops, o("item.delete", map[string]any{"item_ids": []string{it.ID}}))
				} else {
					a := assetFor(-1)
					doc.Assets[a.ID] = a
					ops = append(ops, o("slot.fill", map[string]any{"name": "music", "asset_id": a.ID}))
				}

				continue
			}

			a := assetFor(media)
			doc.Assets[a.ID] = a
			ops = append(ops, o("slot.fill", map[string]any{"name": it.Slot.Name, "asset_id": a.ID}))
			media++
		}
	}

	// The slots are filled in name order, but the music is optional and goes first or not at all.
	res, err := command.Default().ApplyBatch(doc, operations(t, ops), command.Env{
		NewIDs: func(string) string { t.Helper(); t.Fatal("an identifier was not given"); return "" },
	})
	if err != nil {
		t.Fatalf("%s: %v", tpl.file, err)
	}

	if vs := timeline.Validate(res.Doc); len(vs) > 0 {
		t.Fatalf("%s: the instance does not validate: %v", tpl.file, vs[0])
	}

	return res.Doc
}

// fakeAssets are asset records with fixed hashes, for the golden instances that need no media.
func fakeAsset(i int) timeline.Asset {
	if i < 0 {
		return timeline.Asset{
			ID: id("ast", 99), Hash: "sha256:" + strings.Repeat("a", 63) + "9", Kind: timeline.AssetAudio, Name: "music.wav",
			SampleCount: 44100 * 40, HasAudio: true,
		}
	}

	return timeline.Asset{
		ID: id("ast", i+1), Hash: "sha256:" + strings.Repeat("b", 63) + strconv.Itoa(i), Kind: timeline.AssetVideo,
		Name: "shot-" + strconv.Itoa(i+1) + ".mp4", FrameRate: timeline.Rational{Num: fixtureFPS, Den: 1},
		FrameCount: fixtureFPS * fixtureSecs, Width: 360, Height: 640, HasAudio: true,
	}
}

func golden(t *testing.T, name string, got []byte) {
	t.Helper()

	path := filepath.Join("testdata", "golden", name)

	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update)", err)
	}

	if !bytes.Equal(got, want) {
		t.Errorf("%s differs from its golden file (%d bytes against %d); run with -update if the change is meant",
			name, len(got), len(want))
	}
}

// The instance of each template, filled, is the document its golden file holds, and it lints with no error.
func TestTimelineInstancesEqualTheirGolden(t *testing.T) {
	for _, tpl := range timelineTemplates() {
		t.Run(tpl.file, func(t *testing.T) {
			doc := instance(t, tpl, fakeAsset, true)

			pretty, err := json.MarshalIndent(json.RawMessage(timeline.Canonical(doc)), "", "  ")
			if err != nil {
				t.Fatal(err)
			}

			golden(t, strings.TrimSuffix(filepath.Base(tpl.file), ".template.json")+".instance.json", append(pretty, '\n'))

			for _, f := range lint.Lint(doc, lint.Env{}, lint.Fast) {
				if f.Severity == lint.SeverityError {
					t.Errorf("the filled template has the lint error %s: %s", f.Code, f.Message)
				}
			}
		})
	}
}

// A template left with an open slot is not an instance that lints clean: the person is told what to fill.
func TestTimelineOpenSlotsAreLintErrors(t *testing.T) {
	for _, tpl := range timelineTemplates() {
		doc := tpl.doc(t)
		doc.Template.Instance = true

		found := false

		for _, f := range lint.Lint(doc, lint.Env{}, lint.Fast) {
			found = found || f.Severity == lint.SeverityError
		}

		if !found {
			t.Errorf("%s: an instance with every slot open has no lint error", tpl.file)
		}
	}
}

// ---- the frames: the export of a template, drawn by the renderer of the export ----------------------------

type harness struct {
	runner *ffmpeg.Runner
	dir    string
	hashes []string // the ingested fixtures, one per colour
	facts  ingest.Facts
}

func exe(n string) string {
	if runtime.GOOS == "windows" {
		return n + ".exe"
	}

	return n
}

// newHarness finds the pinned LGPL FFmpeg, makes a workspace, makes one clip per colour and ingests them. A test
// never downloads: without the folder of the FFmpeg the frames are skipped (the instances and the images are not).
func newHarness(t *testing.T) *harness {
	t.Helper()

	dir := os.Getenv(envFFmpegDir)
	if dir == "" {
		t.Skip("no pinned FFmpeg: set " + envFFmpegDir) // verify:env-skip needs the pinned FFmpeg of the machine
	}

	ctx := context.Background()

	in, err := ffmpeg.Discover(ctx, ffmpeg.Config{FFmpegPath: filepath.Join(dir, exe("ffmpeg")), CPUCount: runtime.NumCPU()})
	if err != nil || in.Info.License.Name != "lgpl" {
		t.Skip("the FFmpeg in " + envFFmpegDir + " is not the pinned LGPL build") // verify:env-skip needs the pinned FFmpeg
	}

	ws := t.TempDir()

	r, err := ffmpeg.NewRunner(in, ws)
	if err != nil {
		t.Fatal(err)
	}

	caps, err := r.ReadCapabilities(ctx)
	if err != nil {
		t.Fatal(err)
	}

	fixtures := filepath.Join(ws, "fixtures")
	if err := os.Mkdir(fixtures, 0o700); err != nil {
		t.Fatal(err)
	}

	st := store.New(ws)
	h := &harness{runner: r, dir: ws}

	for i, c := range colours {
		name := "clip" + strconv.Itoa(i) + ".mp4"
		// One filter and one encoder thread, so that the fixtures are the same bytes on every machine.
		args := []string{
			"-hide_banner", "-v", "error", "-nostdin", "-y", "-filter_threads", "1",
			"-f", "lavfi", "-i", "color=c=" + c + ":s=360x640:r=" + strconv.Itoa(fixtureFPS) + ":d=" + strconv.Itoa(fixtureSecs),
			"-f", "lavfi", "-i", "sine=f=440:r=44100:d=" + strconv.Itoa(fixtureSecs),
			"-c:v", "mpeg4", "-dct", "int", "-q:v", "4", "-c:a", "aac", "-f", "mp4", "-threads", "1", name,
		}

		if res, err := in.FFmpeg.Run(ctx, fixtures, args...); err != nil {
			t.Fatalf("fixture %s: %v\n%s", name, err, res.Stderr)
		}

		f, err := os.Open(filepath.Join(fixtures, name))
		if err != nil {
			t.Fatal(err)
		}

		put, err := st.Put(ctx, f, 1<<33)
		_ = f.Close()

		if err != nil {
			t.Fatal(err)
		}

		res, err := ingest.Run(ctx, ingest.Input{
			Runner: r, Caps: caps, Hash: put.Hash, ArtifactsDir: filepath.Join(ws, "artifacts"), JobID: "ingest_" + strconv.Itoa(i),
		})
		if err != nil {
			t.Fatalf("ingest of %s: %v", name, err)
		}

		h.hashes = append(h.hashes, put.Hash)
		h.facts = res.Facts
	}

	return h
}

func (h *harness) asset(i int) timeline.Asset {
	i %= len(h.hashes)

	return timeline.Asset{
		ID: id("ast", i+1), Hash: "sha256:" + h.hashes[i], Kind: timeline.AssetVideo, Name: "shot.mp4",
		FrameRate:  timeline.Rational{Num: int(h.facts.FrameRate.Num), Den: int(h.facts.FrameRate.Den)},
		FrameCount: h.facts.FrameCount, Width: h.facts.Width, Height: h.facts.Height, HasAudio: h.facts.HasAudio,
	}
}

type goldenFrame struct {
	Frame  int    `json:"frame"`
	SHA256 string `json:"sha256"`
}

// The frames of the filled template, composed by the renderer of the export, are the same bytes as their golden
// file: the golden export of a template (SDD-018 section 17).
func TestTimelineFramesEqualTheirGolden(t *testing.T) {
	h := newHarness(t)

	for _, tpl := range timelineTemplates() {
		t.Run(tpl.file, func(t *testing.T) {
			doc := instance(t, tpl, h.asset, false)
			name := strings.TrimSuffix(filepath.Base(tpl.file), ".template.json")

			rd, err := export.NewRenderer(context.Background(), export.RenderConfig{
				Runner: h.runner, Doc: doc, JobID: "frames_" + name, ArtifactsDir: filepath.Join(h.dir, "artifacts"),
			})
			if err != nil {
				t.Fatal(err)
			}

			defer func() { _ = rd.Close() }()

			total := 0

			for _, it := range doc.Items {
				total = max(total, it.Start+it.Duration)
			}

			var got []goldenFrame

			for i := range 10 {
				f := i * (total - 1) / 9

				img, err := rd.Render(context.Background(), f, 1)
				if err != nil {
					t.Fatalf("frame %d: %v", f, err)
				}

				sum := sha256.Sum256(img.Pix)
				got = append(got, goldenFrame{Frame: f, SHA256: hex.EncodeToString(sum[:])})

				if dump := os.Getenv("TEMPLATES_DUMP"); dump != "" {
					dumpPNG(t, filepath.Join(dump, name+"-"+strconv.Itoa(f)+".png"), img)
				}
			}

			raw, err := json.MarshalIndent(got, "", "  ")
			if err != nil {
				t.Fatal(err)
			}

			golden(t, name+".frames.json", append(raw, '\n'))
		})
	}
}

func dumpPNG(t *testing.T, path string, img image.Image) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}

	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, b.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

// ---- the image templates ------------------------------------------------------------------------------------

// pictureFixture is a PNG with a diagonal gradient and a bright disc, so that a picture slot that is cropped,
// moved or covered shows in the pixels.
func pictureFixture(t *testing.T) string {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, 1000, 1000))

	for y := range 1000 {
		for x := range 1000 {
			c := color.RGBA{R: uint8(x * 255 / 999), G: uint8(y * 255 / 999), B: 160, A: 255}
			if dx, dy := x-500, y-420; dx*dx+dy*dy < 190*190 {
				c = color.RGBA{R: 250, G: 220, B: 120, A: 255}
			}

			img.SetRGBA(x, y, c)
		}
	}

	path := filepath.Join(t.TempDir(), "picture.png")
	dumpPNG(t, path, img)

	return path
}

// The cover that each image template makes, filled, is the PNG of its golden file.
func TestImageTemplatesRenderTheirGolden(t *testing.T) {
	src := pictureFixture(t)
	cfg := export.ImageConfig{AssetPath: func(string) (string, error) { return src, nil }}

	env := coreimage.Env{
		NewIDs: func(prefix string) string { t.Helper(); t.Fatalf("an identifier was not given: %s", prefix); return "" },
		Asset: func(hash string) (coreimage.AssetInfo, bool) {
			return coreimage.AssetInfo{Width: 1000, Height: 1000}, hash == fixtureHash
		},
	}

	for _, tpl := range imageTemplates() {
		t.Run(tpl.file, func(t *testing.T) {
			title, sub := "Bake a first sourdough", "For home bakers"
			values := map[string]coreimage.SlotValue{
				"picture":  {Asset: fixtureHash},
				"title":    {Text: &title},
				"subtitle": {Text: &sub},
			}

			doc, err := coreimage.FromTemplate(tpl.doc(t), values, id("img", 99), env)
			if err != nil {
				t.Fatal(err)
			}

			img, err := export.RenderImage(context.Background(), doc, cfg)
			if err != nil {
				t.Fatal(err)
			}

			var b bytes.Buffer
			if err := png.Encode(&b, img); err != nil {
				t.Fatal(err)
			}

			if dump := os.Getenv("TEMPLATES_DUMP"); dump != "" {
				dumpPNG(t, filepath.Join(dump, filepath.Base(tpl.file)+".png"), img)
			}

			golden(t, strings.TrimSuffix(filepath.Base(tpl.file), ".image.json")+".png", b.Bytes())

			// An image template with an open slot cannot be exported: the lint says so.
			if _, err := coreimage.FromTemplate(tpl.doc(t), nil, id("img", 98), env); err == nil {
				t.Error("a template with every slot open made a document")
			}
		})
	}
}
