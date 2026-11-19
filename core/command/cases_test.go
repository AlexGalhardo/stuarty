package command

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// Golden cases of SDD-002 Part C. A case is a document, built here, and an operation;
// `go test ./core/command -gen` writes before.json and op.json for every case under
// testdata/<command>/<case>/, and `-update` writes the expected files. The files are
// the contract: the test reads them, not this table.

var gen = flag.Bool("gen", false, "write before.json and op.json of every golden case")

const (
	prjID    = "prj_7k2m9x4q1b"
	vidTr    = "trk_f6g7h8j9k0"
	capTr    = "trk_a1b2c3d4e5"
	audTr    = "trk_m1n2p3q4r5"
	vidAsset = "ast_3c5v8n2w6d" // video, 900 frames at 30 fps, with audio
	imgAsset = "ast_9m8n7p6q5r"
	audAsset = "ast_4h6j8k2m3n"
	itemA    = "itm_m1n2p3q4r5"
	itemB    = "itm_c3d4e5f6g7"
	itemC    = "itm_s6t7v8w9x0"
)

// B builds a case document.
type B struct{ d *timeline.Document }

func newCaseDoc() *B {
	d := &timeline.Document{
		SchemaVersion: 1, ID: prjID, Name: "Launch teaser", Settings: timeline.DefaultSettings(),
		Assets: map[string]timeline.Asset{
			vidAsset: {
				ID: vidAsset, Hash: "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
				Kind: timeline.AssetVideo, Name: "IMG_0421.MOV", FrameRate: timeline.Rational{Num: 30, Den: 1},
				FrameCount: 900, SampleCount: 1440000, Width: 1080, Height: 1920, HasAudio: true,
			},
			imgAsset: {
				ID: imgAsset, Hash: "sha256:" + strings.Repeat("a1", 32), Kind: timeline.AssetImage, Name: "logo.png",
				Width: 512, Height: 512,
			},
			audAsset: {
				ID: audAsset, Hash: "sha256:" + strings.Repeat("b2", 32), Kind: timeline.AssetAudio, Name: "music.wav",
				SampleCount: 1440000,
			},
		},
		Tracks: map[string]timeline.Track{
			capTr: {ID: capTr, Kind: timeline.TrackVisual, Name: "Captions", Order: "a0"},
			vidTr: {ID: vidTr, Kind: timeline.TrackVisual, Name: "Video", Order: "a1"},
			audTr: {ID: audTr, Kind: timeline.TrackAudio, Name: "Music", Order: "a2"},
		},
		Items: map[string]timeline.Item{},
	}
	return &B{d}
}

func (b *B) with(f func(d *timeline.Document)) *B { f(b.d); return b }

// clip adds a clip of the video asset.
func (b *B) clip(id, track string, start, dur, srcIn int, mods ...func(*timeline.Item)) *B {
	it := timeline.NewItem(id, timeline.ItemClip, track, start, dur)
	it.Asset, it.SourceIn = vidAsset, srcIn
	for _, m := range mods {
		m(&it)
	}
	b.d.Items[id] = it
	return b
}

func (b *B) text(id, track string, start, dur int, mods ...func(*timeline.Item)) *B {
	it := timeline.NewItem(id, timeline.ItemText, track, start, dur)
	it.Text = "Hello"
	for _, m := range mods {
		m(&it)
	}
	b.d.Items[id] = it
	return b
}

// caption adds a caption of n words of 10 frames each, one after the other.
func (b *B) caption(id string, start, n int, mods ...func(*timeline.Item)) *B {
	it := timeline.NewItem(id, timeline.ItemCaption, capTr, start, n*10)
	for i := range n {
		it.Words = append(it.Words, timeline.Word{Text: fmt.Sprintf("w%d", i+1), Start: i * 10, Duration: 10})
	}
	for _, m := range mods {
		m(&it)
	}
	b.d.Items[id] = it
	return b
}

func (b *B) shape(id, track string, start, dur int) *B {
	it := timeline.NewItem(id, timeline.ItemShape, track, start, dur)
	it.Shape, it.Width, it.Height = "rect", 100, 100
	b.d.Items[id] = it
	return b
}

func (b *B) audio(id string, start, dur, srcIn int, mods ...func(*timeline.Item)) *B {
	it := timeline.NewItem(id, timeline.ItemAudio, audTr, start, dur)
	it.Asset, it.SourceIn = audAsset, srcIn
	for _, m := range mods {
		m(&it)
	}
	b.d.Items[id] = it
	return b
}

// manyItems adds n text items one after the other on a track, from frame 1000.
func (b *B) manyItems(track string, n int) *B {
	for i := range n {
		id := fmt.Sprintf("itm_x%09d", i)
		it := timeline.NewItem(id, timeline.ItemText, track, 1000+i*2, 1)
		it.Text = "x"
		b.d.Items[id] = it
	}
	return b
}

func (b *B) template() *B {
	b.d.Template = &timeline.TemplateInfo{Description: "t", Colors: map[string]timeline.Color{"accent": "#FF8800"}}
	return b
}

func kf(t int, v float64, easing ...string) timeline.Keyframe {
	k := timeline.NewKeyframe(t, v)
	if len(easing) > 0 {
		k.Easing = easing[0]
	}
	return k
}

type goldenCase struct {
	cmd, name string
	build     func() *B
	op        string
	generated []string // ids the generator returns, in order (newids.json)
}

var goldenCases []goldenCase

func gc(cmd, name string, build func() *B, op string) {
	goldenCases = append(goldenCases, goldenCase{cmd, name, build, op, nil})
}

// gcIDs is gc for a case whose command draws these ids from the generator.
func gcIDs(cmd, name string, build func() *B, op string, generated ...string) {
	goldenCases = append(goldenCases, goldenCase{cmd, name, build, op, generated})
}

// op builds the operation JSON of a case.
func op(name, args string) string {
	return fmt.Sprintf(`{"op": %q, "args": %s}`, name, args)
}

func TestGenerateGoldenInputs(t *testing.T) {
	if !*gen {
		t.Skip("run with -gen to write before.json and op.json of every golden case")
	}
	for _, c := range goldenCases {
		dir := filepath.Join("testdata", c.cmd, c.name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		doc := c.build().d
		write := func(name string, b []byte) {
			if err := os.WriteFile(
				filepath.Join(dir, name),
				append(bytes.TrimRight(b, "\n"), '\n'),
				0o644,
			); err != nil {
				t.Fatal(err)
			}
		}
		write("before.json", timeline.Canonical(doc))
		var pretty bytes.Buffer
		if err := json.Indent(&pretty, []byte(c.op), "", "  "); err != nil {
			t.Fatalf("%s/%s: %v", c.cmd, c.name, err)
		}
		write("op.json", pretty.Bytes())
		if len(c.generated) > 0 {
			ids, err := json.Marshal(c.generated)
			if err != nil {
				t.Fatal(err)
			}
			write("newids.json", ids)
		}
	}
}

// partCCases reads the golden case names of every command from SDD-002 Part C.
func partCCases(t *testing.T) map[string][]string {
	t.Helper()
	b, err := os.ReadFile("../../docs/sdds/002-command-catalogue.md")
	if err != nil {
		t.Fatal(err)
	}
	sections := regexp.MustCompile("(?m)^### C\\.\\d+ `([a-z_.]+)`").FindAllSubmatchIndex(b, -1)
	out := map[string][]string{}
	for i, s := range sections {
		end := len(b)
		if i+1 < len(sections) {
			end = sections[i+1][0]
		}
		body := string(b[s[1]:end])
		_, after, ok := strings.Cut(body, "**Golden cases.**")
		if !ok {
			continue
		}
		for line := range strings.SplitSeq(after, "\n") {
			if name, ok := strings.CutPrefix(strings.TrimSpace(line), "- `"); ok {
				out[string(b[s[2]:s[3]])] = append(out[string(b[s[2]:s[3]])], strings.TrimSuffix(name, "`"))
			}
		}
	}
	return out
}

// TestGoldenCasesComplete fails when a case named in Part C has no directory, or a
// directory has no name in Part C, for every command of the registry.
func TestGoldenCasesComplete(t *testing.T) {
	named := partCCases(t)
	for _, cmd := range fullRegistry().List() {
		want := named[cmd.Name]
		if len(want) == 0 {
			t.Errorf("%s: Part C lists no golden case", cmd.Name)
			continue
		}
		entries, err := os.ReadDir(filepath.Join("testdata", cmd.Name))
		if err != nil {
			t.Errorf("%s: %v", cmd.Name, err)
			continue
		}
		var have []string
		for _, e := range entries {
			if e.IsDir() {
				have = append(have, e.Name())
			}
		}
		slices.Sort(have)
		sorted := slices.Sorted(slices.Values(want))
		if !slices.Equal(have, sorted) {
			for _, n := range sorted {
				if !slices.Contains(have, n) {
					t.Errorf("%s: golden case %s has no directory", cmd.Name, n)
				}
			}
			for _, n := range have {
				if !slices.Contains(sorted, n) {
					t.Errorf("%s: directory %s is not named in Part C", cmd.Name, n)
				}
			}
		}
		// Each error code a command lists has at least one err_ case (the codes that
		// every command can return from the schema alone are checked by err_validation).
		for _, code := range cmd.Errors {
			if !slices.ContainsFunc(have, func(n string) bool { return caseHasCode(n, code) }) &&
				!coveredElsewhere(cmd.Name, code) {
				t.Errorf("%s lists %s but has no err_ case for it", cmd.Name, code)
			}
		}
	}
}

// caseHasCode says whether a case name is an error case of the code: err_<code> or
// err_<code>_<what>, with the code in lower case and without its E_ prefix.
func caseHasCode(name, code string) bool {
	base := "err_" + strings.ToLower(strings.TrimPrefix(code, "E_"))
	return name == base || strings.HasPrefix(name, base+"_")
}

// serviceOnlyCodes are codes a command lists because the service raises them before Apply runs,
// so no golden case of the core can produce them (SDD-002 C.53: a font hash the workspace store
// does not hold). The service tests cover them.
var serviceOnlyCodes = map[string][]string{
	"font.add": {CodeNotFound},
}

// coveredElsewhere reports whether code needs no err_ golden case for the command.
func coveredElsewhere(command, code string) bool {
	return code == CodeInternal || slices.Contains(serviceOnlyCodes[command], code)
}
