package command

import (
	"bytes"
	"cmp"
	"encoding/json"
	"flag"
	"fmt"
	"hash/crc32"
	"maps"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// randomOp returns a random operation of the test catalogue that makes sense for doc.
func randomOp(rng *rand.Rand, doc *timeline.Document) Operation {
	items := make([]string, 0, len(doc.Items))
	for id := range doc.Items {
		items = append(items, id)
	}
	slices.Sort(items)
	visual := []string{}
	var lastOrder string
	for id, tr := range doc.Tracks {
		if tr.Kind == timeline.TrackVisual {
			visual = append(visual, id)
		}
		lastOrder = max(lastOrder, tr.Order)
	}
	slices.Sort(visual)
	pickItem := func() string {
		if len(items) == 0 || rng.Intn(10) == 0 {
			return "itm_zzzzzzzzzz"
		}
		return items[rng.Intn(len(items))]
	}
	switch rng.Intn(8) {
	case 0:
		return opJSON("test.name", fmt.Sprintf(`{"name":"Project %d"}`, rng.Intn(1000)))
	case 1:
		return opJSON(
			"test.track_add",
			fmt.Sprintf(
				`{"kind":"visual","order":%q,"name":"T%d"}`,
				timeline.OrderBetween(lastOrder, ""),
				rng.Intn(100),
			),
		)
	case 2, 3:
		if len(visual) == 0 {
			return opJSON("test.noop", `{}`)
		}
		return opJSON("test.item_add", fmt.Sprintf(`{"track_id":%q,"start":"%df","duration":%d,"text":"t%d"}`,
			visual[rng.Intn(len(visual))], rng.Intn(400), 1+rng.Intn(60), rng.Intn(100)))
	case 4, 5:
		return opJSON("test.item_move", fmt.Sprintf(`{"item_id":%q,"start":"%df"}`, pickItem(), rng.Intn(400)))
	case 6:
		return opJSON("test.item_remove", fmt.Sprintf(`{"item_ids":[%q]}`, pickItem()))
	default:
		return opJSON(
			"test.settings",
			fmt.Sprintf(`{"width":%d,"height":%d}`, 16+2*rng.Intn(2000), 16+2*rng.Intn(2000)),
		)
	}
}

// SDD-008 §3 and ROADMAP P1-ENG-001: over 1,000 random batches, applying and then
// the inverse restores the canonical bytes; every result is valid; Apply does not
// mutate its input; a batch equals its operations applied one by one.
func TestPropertyApplyThenInverseIsTheIdentity(t *testing.T) {
	const seed = 20261114
	rng := rand.New(rand.NewSource(seed))
	r := testRegistry(t)
	doc := baseDoc(t)
	succeeded, failed := 0, 0
	for i := range 1000 {
		ids := &seqIDs{n: i * 7}
		ops := make([]Operation, 1+rng.Intn(4))
		for j := range ops {
			ops[j] = randomOp(rng, doc)
		}
		before := timeline.Canonical(doc)
		res, err := r.ApplyBatch(doc, ops, Env{NewIDs: ids.next})
		if !bytes.Equal(timeline.Canonical(doc), before) {
			t.Fatalf("seed %d, batch %d: ApplyBatch mutated its input", seed, i)
		}
		if err != nil {
			failed++
			e := AsError(err)
			if cerr := CheckError(e); cerr != nil || e.Code == CodeInternal && e.Details["violations"] == nil {
				t.Fatalf("seed %d, batch %d: bad error %+v: %v", seed, i, e, cerr)
			}
			continue
		}
		succeeded++
		if vs := timeline.Validate(res.Doc); len(vs) != 0 {
			t.Fatalf("seed %d, batch %d: invalid result: %v", seed, i, vs)
		}
		back, skipped := ApplyPrimitives(res.Doc, reversed(res.Inverse), nil)
		if len(skipped) != 0 || !bytes.Equal(timeline.Canonical(back.Doc), before) {
			t.Fatalf(
				"seed %d, batch %d: the inverse did not restore the document (skipped %v)\nops %v",
				seed,
				i,
				skipped,
				ops,
			)
		}
		cur := doc
		oneByOne := &seqIDs{n: i * 7}
		for _, op := range ops {
			step, err := r.Apply(cur, op, Env{NewIDs: oneByOne.next})
			if err != nil {
				t.Fatalf("seed %d, batch %d: one by one failed: %v", seed, i, err)
			}
			cur = step.Doc
		}
		if !bytes.Equal(timeline.Canonical(cur), timeline.Canonical(res.Doc)) {
			t.Fatalf("seed %d, batch %d: the batch differs from its operations one by one", seed, i)
		}
		ids2 := &seqIDs{n: i * 7}
		again, err := r.ApplyBatch(doc, ops, Env{NewIDs: ids2.next})
		if err != nil || !bytes.Equal(timeline.Canonical(again.Doc), timeline.Canonical(res.Doc)) {
			t.Fatalf("seed %d, batch %d: ApplyBatch is not deterministic", seed, i)
		}
		if len(res.Diff) == 0 != bytes.Equal(timeline.Canonical(res.Doc), before) {
			t.Fatalf("seed %d, batch %d: diff and document disagree", seed, i)
		}
		if rng.Intn(4) != 0 { // mostly keep growing the same document
			doc = res.Doc
		}
		if i%40 == 39 { // and start over now and then, to keep the documents small
			doc = baseDoc(t)
		}
	}
	if succeeded < 400 || failed < 50 {
		t.Errorf("the generator is not exercising both paths: %d succeeded, %d failed", succeeded, failed)
	}
}

// docPick lets the generators name things that exist in a document, now and then
// something that does not, so that the error paths run too.
type docPick struct {
	rng                                *rand.Rand
	items, tracks, visual, audio, asts []string
	captions, slots, clips, audios     []string
	fonts                              []string
	keys                               []keyAt
}

// keyAt is a keyframe that exists: its item, property and absolute frame.
type keyAt struct {
	item, prop string
	at         int
}

func newDocPick(rng *rand.Rand, doc *timeline.Document) *docPick {
	p := &docPick{rng: rng}

	for id := range doc.Items {
		p.items = append(p.items, id)

		if it := doc.Items[id]; it.Type == timeline.ItemCaption {
			p.captions = append(p.captions, id)
		}

		if it := doc.Items[id]; it.Type == timeline.ItemClip {
			p.clips = append(p.clips, id)
		}

		if it := doc.Items[id]; it.Type == timeline.ItemAudio {
			p.audios = append(p.audios, id)
		}

		for prop, ks := range doc.Items[id].Keyframes {
			for _, k := range ks {
				p.keys = append(p.keys, keyAt{id, prop, doc.Items[id].Start + k.T})
			}
		}

		if it := doc.Items[id]; it.Slot != nil {
			p.slots = append(p.slots, it.Slot.Name)
		}
	}

	for id, tr := range doc.Tracks {
		p.tracks = append(p.tracks, id)
		if tr.Kind == timeline.TrackVisual {
			p.visual = append(p.visual, id)
		} else {
			p.audio = append(p.audio, id)
		}
	}

	for id := range doc.Assets {
		p.asts = append(p.asts, id)
	}

	for family := range doc.Fonts {
		p.fonts = append(p.fonts, family)
	}

	for _, l := range [][]string{p.items, p.tracks, p.visual, p.audio, p.asts, p.captions, p.slots, p.clips, p.audios, p.fonts} {
		slices.Sort(l)
	}

	// The keyframes come from two maps: without an order the generator would not be deterministic.
	slices.SortFunc(p.keys, func(a, b keyAt) int {
		return cmp.Or(strings.Compare(a.item, b.item), strings.Compare(a.prop, b.prop), a.at-b.at)
	})

	return p
}

func (p *docPick) of(list []string, missing string) string {
	if len(list) == 0 || p.rng.Intn(20) == 0 {
		return missing
	}

	return list[p.rng.Intn(len(list))]
}

func (p *docPick) item() string      { return p.of(p.items, "itm_zzzzzzzzzz") }
func (p *docPick) clip() string      { return p.of(p.clips, "itm_zzzzzzzzzz") }
func (p *docPick) family() string    { return p.of(p.fonts, "Fam 0") }
func (p *docPick) audioItem() string { return p.of(p.audios, "itm_zzzzzzzzzz") }
func (p *docPick) caption() string   { return p.of(p.captions, "itm_zzzzzzzzzz") }
func (p *docPick) vtrack() string    { return p.of(p.visual, "trk_zzzzzzzzzz") }
func (p *docPick) atrack() string    { return p.of(p.audio, "trk_zzzzzzzzzz") }
func (p *docPick) asset() string     { return p.of(p.asts, "ast_zzzzzzzzzz") }
func (p *docPick) slot() string      { return p.of(p.slots, "nope") }
func (p *docPick) n(k int) int       { return p.rng.Intn(k) }
func (p *docPick) frames(k int) string {
	return fmt.Sprintf("%df", p.rng.Intn(k))
}
func (p *docPick) oneOf(opts ...string) string { return opts[p.rng.Intn(len(opts))] }

func (p *docPick) words() string {
	n := 1 + p.n(4)
	parts := make([]string, n)

	for i := range n {
		parts[i] = fmt.Sprintf(`{"text":"w%d","start":"%df","duration":"%df"}`, i, i*12, 1+p.n(11))
	}

	return "[" + strings.Join(parts, ",") + "]"
}

func (p *docPick) resolvedWords() string {
	var parts []string

	for at := p.n(80); at < 850 && len(parts) < 12; {
		end := at + 1 + p.n(30)
		parts = append(parts, fmt.Sprintf(`{"text":"r%d","start":%d,"end":%d}`, len(parts), at, end))
		at = end + p.n(40)
	}

	return `{"words":[` + strings.Join(parts, ",") + `]}`
}

func (p *docPick) silences() string {
	var parts []string

	for at := p.n(60); at < 800 && len(parts) < 6; {
		end := at + 5 + p.n(80)
		parts = append(parts, fmt.Sprintf(`{"start":%d,"end":%d}`, at, end))
		at = end + 1 + p.n(70)
	}

	return `{"silences":[` + strings.Join(parts, ",") + `]}`
}

// alphaOps are the generators of the later commands, one per command.
var alphaOps = map[string]func(p *docPick) string{
	"settings.set_fps": func(p *docPick) string {
		return fmt.Sprintf(`{"fps":%q}`, p.oneOf("24", "25", "30", "60", "30000/1001"))
	},
	"asset.remove": func(p *docPick) string { return fmt.Sprintf(`{"asset_id":%q}`, p.asset()) },
	"item.rename":  func(p *docPick) string { return fmt.Sprintf(`{"item_id":%q,"name":"n%d"}`, p.item(), p.n(9)) },
	"item.set_transform": func(p *docPick) string {
		return fmt.Sprintf(
			`{"item_id":%q,"x":%d,"scale":%.2f,"opacity":%.2f}`,
			p.item(),
			p.n(1080),
			0.5+float64(p.n(30))/10,
			float64(p.n(11))/10,
		)
	},
	"item.set_transition": func(p *docPick) string {
		return fmt.Sprintf(`{"item_id":%q,"edge":%q,"kind":%q,"duration":%q}`, p.item(), p.oneOf("in", "out"),
			p.oneOf("fade", "slide_left", "slide_right", "zoom", "whip", "dip", "wipe"), p.frames(40))
	},
	"clip.set_effects": func(p *docPick) string {
		return fmt.Sprintf(
			`{"item_id":%q,"effects":[{"kind":"vignette","amount":%.2f},{"kind":"grain","size":%d,"seed":%d},{"kind":"shake","speed":%d}]}`,
			p.clip(),
			float64(p.n(101))/100,
			1+p.n(8),
			p.n(65536),
			1+p.n(30),
		)
	},
	"item.set_audio": func(p *docPick) string {
		return fmt.Sprintf(
			`{"item_id":%q,"volume":%.1f,"fade_in":%q,"denoise":%.2f}`,
			p.item(),
			float64(p.n(30))/10,
			p.frames(30),
			float64(p.n(101))/100,
		)
	},
	"item.duplicate": func(p *docPick) string {
		return fmt.Sprintf(`{"item_id":%q,"start":%q}`, p.item(), p.frames(3000))
	},
	"item.crossfade": func(p *docPick) string {
		return fmt.Sprintf(`{"from_item_id":%q,"to_item_id":%q,"duration":%q}`, p.item(), p.item(), p.frames(12))
	},
	"timeline.close_gap": func(p *docPick) string {
		return fmt.Sprintf(`{"track_id":%q,"at":%q,"ripple":%q}`, p.vtrack(), p.frames(600), p.oneOf("track", "all"))
	},
	"audio.add": func(p *docPick) string {
		return fmt.Sprintf(`{"track_id":%q,"asset_id":%q,"start":%q}`, p.atrack(), p.asset(), p.frames(2000))
	},
	"clip.add_composition": func(p *docPick) string {
		return fmt.Sprintf(`{"track_id":%q,"timeline_id":%q,"start":%q,"duration":"%df"}`,
			p.vtrack(), tlB, p.frames(2500), 1+p.n(120))
	},
	"clip.set_framing": func(p *docPick) string {
		return fmt.Sprintf(`{"item_id":%q,"fit":%q,"crop":{"left":%.2f,"top":%.2f}}`, p.clip(),
			p.oneOf("cover", "contain", "none"), float64(p.n(40))/100, float64(p.n(40))/100)
	},
	"clip.set_speed": func(p *docPick) string {
		if p.n(3) == 0 {
			return fmt.Sprintf(
				`{"item_id":%q,"curve":[{"t":0,"speed":"%d.%dx"},{"t":0.%03d,"speed":"%d.%dx"},{"t":1,"speed":"%d.%dx"}],"ripple":%q}`,
				p.clip(),
				p.n(3),
				p.n(10),
				1+p.n(998),
				p.n(3),
				p.n(10),
				p.n(3),
				p.n(10),
				p.oneOf("none", "track"),
			)
		}
		return fmt.Sprintf(
			`{"item_id":%q,"speed":"%d.%dx","reverse":%s,"ripple":%q}`,
			p.clip(),
			p.n(3),
			p.n(10),
			p.oneOf("true", "false"),
			p.oneOf("none", "track"),
		)
	},
	"clip.set_color": func(p *docPick) string {
		return fmt.Sprintf(
			`{"item_id":%q,"brightness":%.2f,"saturation":%.2f,"temperature":%.2f,"tint":%.2f,"hue":%d}`,
			p.clip(),
			float64(p.n(200))/100-1,
			float64(p.n(200))/100,
			float64(p.n(200))/100-1,
			float64(p.n(200))/100-1,
			p.n(361)-180,
		)
	},
	"clip.set_filter": func(p *docPick) string {
		return fmt.Sprintf(
			`{"item_id":%q,"filter":%q,"amount":%.2f}`,
			p.clip(),
			p.oneOf("none", "grayscale", "sepia", "invert", "warm", "cool"),
			float64(p.n(101))/100,
		)
	},
	// The random documents hold one video, so a swap succeeds only as the same asset (a no-op); an image for a
	// video is the refusal the sequences should also meet.
	"clip.replace_media": func(p *docPick) string {
		return fmt.Sprintf(`{"item_id":%q,"asset_id":%q}`, p.clip(), p.oneOf(vidAsset, vidAsset, imgAsset))
	},
	"clip.set_blur": func(p *docPick) string {
		return fmt.Sprintf(`{"item_id":%q,"radius":%d}`, p.clip(), p.n(101))
	},
	"item.link": func(p *docPick) string {
		return fmt.Sprintf(`{"item_ids":[%q,%q]}`, p.clip(), p.audioItem())
	},
	"item.unlink": func(p *docPick) string {
		return fmt.Sprintf(`{"item_ids":[%q]}`, p.oneOf(p.clip(), p.audioItem()))
	},
	"item.extract_audio": func(p *docPick) string {
		return fmt.Sprintf(`{"item_id":%q,"track_id":%q}`, p.clip(), p.atrack())
	},
	"item.paste_attributes": func(p *docPick) string {
		return fmt.Sprintf(
			`{"from_item_id":%q,"item_ids":[%q],"attributes":[%q,%q]}`,
			p.clip(), p.audioItem(), p.oneOf("audio", "color", "blend"), p.oneOf("filter", "effects", "flip"),
		)
	},
	"item.freeze_frame": func(p *docPick) string {
		return fmt.Sprintf(`{"item_id":%q,"at":%q,"duration":%q}`, p.clip(), p.frames(400), p.frames(60))
	},
	"audio.processor_add": func(p *docPick) string {
		kind := p.oneOf("eq", "compressor", "limiter", "gate", "deesser", "noise_reduction")
		return fmt.Sprintf(`{"item_id":%q,"kind":%q,"index":%d}`, p.audioItem(), kind, p.n(3))
	},
	"audio.processor_set": func(p *docPick) string {
		return fmt.Sprintf(`{"item_id":%q,"index":%d,"enabled":%s}`, p.audioItem(), p.n(3), p.oneOf("true", "false"))
	},
	"audio.processor_move": func(p *docPick) string {
		return fmt.Sprintf(`{"item_id":%q,"index":%d,"to":%d}`, p.audioItem(), p.n(3), p.n(3))
	},
	"audio.processor_remove": func(p *docPick) string {
		return fmt.Sprintf(`{"item_id":%q,"index":%d}`, p.audioItem(), p.n(3))
	},
	"audio.apply_preset": func(p *docPick) string {
		presets := []string{"voice", "voice_with_music", "podcast", "clean_noisy_room"}
		return fmt.Sprintf(`{"item_id":%q,"preset":%q}`, p.audioItem(), p.oneOf(presets...))
	},
	"item.set_enabled": func(p *docPick) string {
		return fmt.Sprintf(`{"item_id":%q,"enabled":%s}`, p.clip(), p.oneOf("true", "false"))
	},
	"clip.set_flip": func(p *docPick) string {
		return fmt.Sprintf(
			`{"item_id":%q,"horizontal":%s,"vertical":%s}`,
			p.clip(),
			p.oneOf("true", "false"),
			p.oneOf("true", "false"),
		)
	},
	"clip.set_blend": func(p *docPick) string {
		return fmt.Sprintf(
			`{"item_id":%q,"mode":%q}`,
			p.clip(),
			p.oneOf("normal", "multiply", "screen", "overlay", "add"),
		)
	},
	"clip.set_mask": func(p *docPick) string {
		if p.n(4) == 0 {
			return fmt.Sprintf(`{"item_id":%q,"mask":null}`, p.clip())
		}
		return fmt.Sprintf(
			`{"item_id":%q,"mask":{"shape":%q,"x":%.2f,"y":%.2f,"width":%.2f,"height":%.2f,"feather":%d,"invert":%s}}`,
			p.clip(), p.oneOf("rect", "ellipse"), 0.1, 0.2, 0.5, 0.6, p.n(201), p.oneOf("true", "false"),
		)
	},
	"item.repeat": func(p *docPick) string {
		if p.n(2) == 0 {
			return fmt.Sprintf(`{"item_id":%q,"copies":%d}`, p.clip(), 1+p.n(3))
		}
		return fmt.Sprintf(`{"item_id":%q,"until":%q}`, p.clip(), p.frames(600))
	},
	"clip.set_shadow": func(p *docPick) string {
		if p.n(4) == 0 {
			return fmt.Sprintf(`{"item_id":%q,"shadow":null}`, p.clip())
		}
		return fmt.Sprintf(`{"item_id":%q,"shadow":{"opacity":%.2f,"dx":%d,"dy":%d,"blur":%d}}`,
			p.clip(), float64(p.n(101))/100, p.n(1001)-500, p.n(1001)-500, p.n(101))
	},
	"clip.set_outline": func(p *docPick) string {
		if p.n(4) == 0 {
			return fmt.Sprintf(`{"item_id":%q,"outline":null}`, p.clip())
		}
		return fmt.Sprintf(`{"item_id":%q,"outline":{"width":%d}}`, p.clip(), 1+p.n(100))
	},
	"clip.set_chroma_key": func(p *docPick) string {
		color := p.oneOf(`"#00FF00"`, `"#0000FF"`, "null")
		if color == "null" {
			return fmt.Sprintf(`{"item_id":%q,"color":null}`, p.clip())
		}
		return fmt.Sprintf(
			`{"item_id":%q,"color":%s,"similarity":%.2f,"spill":%.2f}`,
			p.clip(), color, float64(p.n(101))/100, float64(p.n(101))/100,
		)
	},
	"item.slip": func(p *docPick) string {
		return fmt.Sprintf(`{"item_id":%q,"source_in":%q}`, p.clip(), p.frames(300))
	},
	"settings.reframe": func(p *docPick) string {
		return fmt.Sprintf(`{"aspect":%q}`, p.oneOf("16:9", "9:16", "1:1", "4:5"))
	},
	"marker.add": func(p *docPick) string {
		return fmt.Sprintf(`{"at":"%df","label":"m%d"}`, p.n(20), p.n(10))
	},
	"marker.set": func(p *docPick) string {
		return fmt.Sprintf(`{"at":"%df","to":"%df","label":"s%d"}`, p.n(20), p.n(20), p.n(10))
	},
	"marker.remove": func(p *docPick) string {
		return fmt.Sprintf(`{"at":"%df"}`, p.n(20))
	},
	"audio.duck": func(p *docPick) string {
		clip := p.clip()
		var parts []string
		for at := p.n(60); at < 800 && len(parts) < 6; {
			end := at + 5 + p.n(80)
			parts = append(parts, fmt.Sprintf(`{"start":%d,"end":%d}`, at, end))
			at = end + 1 + p.n(70)
		}

		return fmt.Sprintf(
			`{"item_id":%q,"speech_item_ids":[%q],"gain_db":-%d,"resolved":{"speech":[{"item_id":%q,"ranges":[%s]}]}}`,
			p.audioItem(), clip, 1+p.n(30), clip, strings.Join(parts, ","),
		)
	},
	"font.add": func(p *docPick) string {
		return fmt.Sprintf(`{"family":"Fam %d","files":[{"weight":%d,"hash":"sha256:%s"}]}`,
			p.n(4), 100*(1+p.n(9)), strings.Repeat(p.oneOf("ab", "cd", "ef"), 32))
	},
	"font.remove": func(p *docPick) string { return fmt.Sprintf(`{"family":%q}`, p.family()) },
	"audio.mark_beats": func(p *docPick) string {
		var parts []string
		for at := p.n(20); at < 600 && len(parts) < 150; at += 3 + p.n(20) {
			parts = append(parts, strconv.Itoa(at))
		}

		return fmt.Sprintf(`{"item_id":%q,"every":%d,"resolved":{"beats":[%s]}}`,
			p.audioItem(), 1+p.n(3), strings.Join(parts, ","))
	},
	"clip.remove_silences": func(p *docPick) string {
		return fmt.Sprintf(`{"item_id":%q,"ripple":%q,"resolved":%s}`, p.clip(), p.oneOf("none", "track"), p.silences())
	},
	"text.apply_preset": func(p *docPick) string {
		return fmt.Sprintf(
			`{"item_id":%q,"preset":%q,"edge":%q,"duration":%q}`,
			p.item(),
			p.oneOf(
				"fade_up",
				"fade_down",
				"pop",
				"zoom_in",
				"slide_left",
				"slide_right",
			),
			p.oneOf("in", "out"),
			p.frames(30),
		)
	},
	"shape.add": func(p *docPick) string {
		return fmt.Sprintf(`{"track_id":%q,"start":%q,"duration":"%df","shape":%q,"width":%d,"height":%d}`,
			p.vtrack(), p.frames(2500), 1+p.n(120), p.oneOf("rect", "ellipse"), 10+p.n(500), 10+p.n(500))
	},
	"shape.set": func(p *docPick) string {
		return fmt.Sprintf(`{"item_id":%q,"width":%d,"fill":"#%06x"}`, p.item(), 10+p.n(500), p.n(0xffffff))
	},
	"caption.set_words": func(p *docPick) string {
		return fmt.Sprintf(`{"item_id":%q,"words":%s}`, p.caption(), p.words())
	},
	"caption.edit_word": func(p *docPick) string {
		return fmt.Sprintf(`{"item_id":%q,"index":%d,"text":%q}`, p.caption(), p.n(4), p.oneOf("", "edited", "x"))
	},
	"caption.set_breaks": func(p *docPick) string {
		return fmt.Sprintf(`{"item_id":%q,"breaks":[%d]}`, p.caption(), 1+p.n(3))
	},
	"caption.set_style": func(p *docPick) string {
		return fmt.Sprintf(
			`{"item_id":%q,"style":{"size":%d,"highlight":%q}}`,
			p.caption(),
			8+p.n(100),
			p.oneOf("none", "word"),
		)
	},
	"caption.split": func(p *docPick) string {
		return fmt.Sprintf(`{"item_id":%q,"before_word":%d}`, p.caption(), 1+p.n(3))
	},
	"caption.merge": func(p *docPick) string {
		return fmt.Sprintf(`{"item_ids":[%q,%q]}`, p.caption(), p.caption())
	},
	"caption.from_transcript": func(p *docPick) string {
		return fmt.Sprintf(`{"clip_item_id":%q,"track_id":%q,"max_words":%d,"replace":%t,"resolved":%s}`,
			p.clip(), p.vtrack(), 1+p.n(6), p.n(2) == 0, p.resolvedWords())
	},
	"caption.import_subtitles": func(p *docPick) string {
		return fmt.Sprintf(`{"track_id":%q,"text":%q,"start":%q}`, p.vtrack(),
			"1\n00:00:01,000 --> 00:00:02,000\nHello there\n\n2\n00:00:03,000 --> 00:00:04,000\nBye\n", p.frames(300))
	},
	"caption.retime_from_clip": func(p *docPick) string {
		return fmt.Sprintf(`{"clip_item_id":%q,"resolved":%s}`, p.clip(), p.resolvedWords())
	},
	"keyframe.set": func(p *docPick) string {
		return fmt.Sprintf(`{"item_id":%q,"property":%q,"at":%q,"value":%.2f,"easing":%q}`, p.item(),
			p.oneOf("scale", "opacity", "x", "y", "rotation", "volume"), p.frames(500), float64(p.n(100))/100+0.1,
			p.oneOf("linear", "ease_in", "ease_out", "ease_in_out", "hold"))
	},
	"animation.apply_preset": func(p *docPick) string {
		preset := p.oneOf("fade", "slide", "scale", "pop", "push_in", "pull_out")
		edge := p.oneOf("in", "out")
		extra := ""

		if preset == "slide" && p.n(2) == 0 {
			extra = fmt.Sprintf(`,"direction":%q`, p.oneOf("left", "right", "up", "down"))
		}

		return fmt.Sprintf(`{"item_id":%q,"preset":%q,"edge":%q,"duration":%q%s}`, p.item(), preset, edge,
			p.frames(120), extra)
	},
	"animation.list_presets": func(_ *docPick) string { return `{}` },
	"keyframe.set_easing": func(p *docPick) string {
		item, prop, at := p.item(), "scale", p.frames(500)
		if len(p.keys) > 0 && p.n(4) != 0 {
			k := p.keys[p.n(len(p.keys))]
			item, prop, at = k.item, k.prop, fmt.Sprintf("%df", k.at)
		}

		switch p.n(3) {
		case 0:
			return fmt.Sprintf(`{"item_id":%q,"property":%q,"at":%q,"preset":%q}`, item, prop, at,
				p.oneOf("in", "out", "in_out", "back", "overshoot"))
		case 1:
			return fmt.Sprintf(`{"item_id":%q,"property":%q,"at":%q,"bezier":[%.2f,%.2f,%.2f,%.2f]}`, item, prop, at,
				float64(p.n(100))/100, float64(p.n(500))/100-2, float64(p.n(100))/100, float64(p.n(500))/100-2)
		}

		return fmt.Sprintf(`{"item_id":%q,"property":%q,"at":%q,"easing":%q}`, item, prop, at,
			p.oneOf("linear", "ease_in", "ease_out", "ease_in_out", "hold"))
	},
	"keyframe.remove": func(p *docPick) string {
		if len(p.keys) > 0 && p.n(4) != 0 {
			k := p.keys[p.n(len(p.keys))]

			return fmt.Sprintf(`{"item_id":%q,"property":%q,"at":"%df"}`, k.item, k.prop, k.at)
		}

		return fmt.Sprintf(
			`{"item_id":%q,"property":%q,"at":%q}`,
			p.item(),
			p.oneOf("scale", "opacity", "x"),
			p.frames(500),
		)
	},
	"keyframe.clear": func(p *docPick) string {
		if p.n(2) == 0 {
			return fmt.Sprintf(`{"item_id":%q}`, p.item())
		}

		return fmt.Sprintf(`{"item_id":%q,"property":%q}`, p.item(), p.oneOf("scale", "opacity", "x"))
	},
	"slot.mark": func(p *docPick) string {
		return fmt.Sprintf(`{"item_id":%q,"name":"s%d"}`, p.item(), p.n(6))
	},
	"slot.unmark": func(p *docPick) string { return fmt.Sprintf(`{"item_id":%q}`, p.item()) },
	"slot.fill": func(p *docPick) string {
		switch p.n(3) {
		case 0:
			return fmt.Sprintf(`{"name":%q,"asset_id":%q,"source_in":%q}`, p.slot(), p.asset(), p.frames(300))
		case 1:
			return fmt.Sprintf(`{"name":%q,"text":"filled"}`, p.slot())
		}

		return fmt.Sprintf(`{"name":%q,"words":%s}`, p.slot(), p.words())
	},
	"template.set_info": func(p *docPick) string {
		return fmt.Sprintf(
			`{"description":"d%d","colors":{"c%d":"#%06x","c%d":null}}`,
			p.n(9),
			p.n(4),
			p.n(0xffffff),
			p.n(4),
		)
	},
}

// randomRegistryOp returns a random operation of the whole registry: the first
// commands of realOp and the later commands of alphaOps.
func randomRegistryOp(rng *rand.Rand, doc *timeline.Document) Operation {
	if rng.Intn(3) == 0 {
		return realOp(rng, doc)
	}

	names := slices.Sorted(maps.Keys(alphaOps))
	name := names[rng.Intn(len(names))]

	return op2(name, alphaOps[name](newDocPick(rng, doc)))
}

// randomStart is a document with every kind of item, so that the generators of all
// the commands have something to act on; half of them are templates with slots.
func randomStart(rng *rand.Rand) *timeline.Document {
	b := base().
		clip(itemA, vidTr, 0, 150, 0).
		text(itemC, vidTr, 200, 90).
		shape("itm_x000000005", vidTr, 400, 60).
		caption("itm_x000000003", 10, 4, func(it *timeline.Item) { it.LinkedItem = itemA }).
		caption("itm_x000000006", 100, 3).
		audio("itm_x000000004", 0, 300, 0).
		clip("itm_x000000007", vidTr, 150, 50, 0)

	if rng.Intn(2) == 0 {
		for _, id := range []string{itemA, "itm_x000000004"} {
			it := b.d.Items[id]
			it.Link = "lnk_x000000001"
			b.d.Items[id] = it
		}
	}

	if rng.Intn(2) == 0 {
		b.d.Markers = []timeline.Marker{{Frame: 5}, {Frame: 12, Label: "m"}}
	}

	if rng.Intn(2) == 0 {
		b.template()

		c := b.d.Items[itemA]
		c.Asset = timeline.PlaceholderAsset
		c.Slot = &timeline.Slot{Name: "hero", Kind: "media", Required: true, DurationPolicy: "stretch_timeline"}
		b.d.Items[itemA] = c

		tx := b.d.Items[itemC]
		tx.Slot = &timeline.Slot{Name: "title", Kind: "text", Required: true, DurationPolicy: "fit"}
		b.d.Items[itemC] = tx
	}

	return b.d
}

// checkStep applies one operation and checks what SDD-002 A.6 and A.8 promise: a
// failure is a well formed error and leaves the input alone, a success is a valid
// document and the inverses applied in reverse give back the starting bytes.
func checkStep(doc *timeline.Document, o Operation, ids *seqIDs) (next *timeline.Document, err error) {
	return checkStepEchoing(doc, o, ids, false)
}

// checkStepEchoing is checkStep; with echoed set, what the caller typed in args (a key
// such as "ts", which looks like a file name) is not held against the error text that
// quotes it: the rule of A.8 is about what the server leaks, not about the caller's own words.
func checkStepEchoing(
	doc *timeline.Document,
	o Operation,
	ids *seqIDs,
	echoed bool,
) (next *timeline.Document, err error) {
	before := timeline.Canonical(doc)

	res, applyErr := fullRegistry().Apply(doc, o, Env{NewIDs: ids.next, Timelines: propGraph(doc)})
	if !bytes.Equal(timeline.Canonical(doc), before) {
		return nil, fmt.Errorf("%s changed its input", o.Op)
	}

	if applyErr != nil {
		e := AsError(applyErr)
		if echoed {
			e = withoutEcho(e, o.Args)
		}

		if cerr := CheckError(e); cerr != nil {
			return nil, fmt.Errorf("%s: malformed error %s %s: %w", o.Op, e.Code, e.Message, cerr)
		}

		if e.Code == CodeInternal {
			return nil, fmt.Errorf("%s %s: an internal error: %s", o.Op, o.Args, e.Message)
		}

		return doc, nil
	}

	if vs := timeline.Validate(res.Doc); len(vs) != 0 {
		return nil, fmt.Errorf("%s %s: invalid result %v", o.Op, o.Args, vs)
	}

	back, skipped := ApplyPrimitives(res.Doc, reversed(res.Inverse), nil)
	if len(skipped) != 0 || !bytes.Equal(timeline.Canonical(back.Doc), before) {
		return nil, fmt.Errorf("%s %s: the inverse did not restore the document (skipped %v)", o.Op, o.Args, skipped)
	}

	return res.Doc, nil
}

// TestRandomBatches runs random sequences of operations of the whole registry: after
// every step the document is valid and the inverse restores the bytes before it
// (SDD-002 A.6); every command succeeds at least once.
func TestRandomBatches(t *testing.T) {
	const seed = 20261116

	rng := rand.New(rand.NewSource(seed))
	ok := map[string]int{}
	doc := randomStart(rng)
	ids := &seqIDs{}

	for i := range 9000 {
		o := randomRegistryOp(rng, doc)

		next, err := checkStep(doc, o, ids)
		if err != nil {
			t.Fatalf("seed %d, step %d: %v", seed, i, err)
		}

		if next != doc {
			ok[o.Op]++
		}

		doc = next
		if i%7 == 6 || len(doc.Items) > 80 {
			doc = randomStart(rng)
		}
	}

	// The mixed run above depends on the order of the generators, so adding a command
	// reshuffles it. A command it did not reach gets its own seeded stream, on fresh
	// starting documents, so that the check does not move with the registry.
	for _, c := range fullRegistry().List() {
		if ok[c.Name] != 0 || c.Internal {
			continue
		}

		gen, known := alphaOps[c.Name]
		if !known {
			t.Errorf("%s never succeeded in the random sequences and has no generator", c.Name)

			continue
		}

		own := rand.New(rand.NewSource(int64(crc32.ChecksumIEEE([]byte(c.Name)))))

		for try := 0; try < 2000 && ok[c.Name] == 0; try++ {
			start := randomStart(own)

			// Some commands act on state that other commands build (keyframes, links).
			for range own.Intn(12) {
				warm, err := checkStep(start, randomRegistryOp(own, start), ids)
				if err != nil {
					t.Fatalf("%s, try %d, warm-up: %v", c.Name, try, err)
				}

				start = warm
			}

			next, err := checkStep(start, op2(c.Name, gen(newDocPick(own, start))), ids)
			if err != nil {
				t.Fatalf("%s, try %d: %v", c.Name, try, err)
			}

			if next != start {
				ok[c.Name]++
			}
		}

		if ok[c.Name] == 0 {
			t.Errorf("%s never succeeded in the random sequences", c.Name)
		}
	}
}

// FuzzApplyBatch feeds the same generators with fuzzed seeds: any sequence it builds
// keeps the document valid and undoable.
func FuzzApplyBatch(f *testing.F) {
	f.Add(int64(1), uint8(5))
	f.Add(int64(20261116), uint8(9))

	f.Fuzz(func(t *testing.T, seed int64, steps uint8) {
		rng := rand.New(rand.NewSource(seed))
		doc := randomStart(rng)
		ids := &seqIDs{}

		for i := range 1 + int(steps%12) {
			next, err := checkStep(doc, randomRegistryOp(rng, doc), ids)
			if err != nil {
				t.Fatalf("seed %d, step %d: %v", seed, i, err)
			}

			doc = next
		}
	})
}

var genScale = flag.Bool("gen-scale", false, "write the 5 minute project of NFR-009 under testdata/scale")

// scaleProject is the project of NFR-009 for the benchmarks: 5
// minutes at 30 fps, a clip, a music track, 164 text items and 34 captions of 60
// words, 200 items and about 2,000 caption words.
func scaleProject() *timeline.Document {
	b := base().clip(itemA, vidTr, 0, 9000, 0)
	b.d.Assets[vidAsset] = timeline.Asset{
		ID: vidAsset, Hash: "sha256:" + strings.Repeat("9f", 32), Kind: timeline.AssetVideo, Name: "scale.mov",
		FrameRate: timeline.Rational{Num: 30, Den: 1}, FrameCount: 9000, SampleCount: 14400000,
		Width: 1080, Height: 1920, HasAudio: true,
	}
	b.d.Assets[audAsset] = timeline.Asset{
		ID: audAsset, Hash: "sha256:" + strings.Repeat("b2", 32), Kind: timeline.AssetAudio, Name: "scale.wav",
		SampleCount: 14400000,
	}
	b.audio(itemB, 0, 9000, 0)

	for i := range 164 {
		b.text(fmt.Sprintf("itm_t%09d", i), capTr, i*54, 30)
	}

	extra := timeline.Track{ID: "trk_p7q8r9s0t1", Kind: timeline.TrackVisual, Name: "Captions 2", Order: "a3"}
	b.d.Tracks[extra.ID] = extra

	for i := range 34 {
		b.caption(fmt.Sprintf("itm_c%09d", i), i*260, 60, func(it *timeline.Item) {
			it.Track = extra.ID
			for j := range it.Words {
				it.Words[j].Start, it.Words[j].Duration = j*2, 2
			}
			it.Duration = 120
		})
	}

	return b.d
}

// TestScaleProject checks the generator: the project is valid and has the size of
// NFR-009; with -gen-scale it writes testdata/scale/project.json (go generate, not
// committed).
func TestScaleProject(t *testing.T) {
	doc := scaleProject()
	if vs := timeline.Validate(doc); len(vs) != 0 {
		t.Fatalf("the scale project is invalid: %v", vs)
	}

	words := 0
	for id := range doc.Items {
		words += len(doc.Items[id].Words)
	}

	if len(doc.Items) != 200 || words != 2040 {
		t.Errorf("%d items and %d words, want 200 and 2040", len(doc.Items), words)
	}

	if !*genScale {
		return
	}

	if err := os.MkdirAll(filepath.Join("testdata", "scale"), 0o755); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join("testdata", "scale", "project.json")
	if err := os.WriteFile(out, append(timeline.Canonical(doc), '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

// FuzzApplyArgs feeds raw bytes as the arguments of any command of the registry,
// on a document with items, captions and slots: the answer is a
// well formed error or a valid, undoable document, never E_INTERNAL (SDD-002 A.3, A.8).
func FuzzApplyArgs(f *testing.F) {
	cmds := fullRegistry().List()

	hostile := `{"item_id":"itm_m1n2p3q4r5","start":"-1f","duration":"0f","words":[null],"id":"itm_m1n2p3q4r5"}`

	for i, c := range cmds {
		f.Add(uint8(i), c.Example, int64(i))
		f.Add(uint8(i), `{}`, int64(i))
		f.Add(uint8(i), hostile, int64(i))
	}

	f.Fuzz(func(t *testing.T, which uint8, args string, seed int64) {
		c := cmds[int(which)%len(cmds)]
		if c.Internal {
			return
		}

		doc := randomStart(rand.New(rand.NewSource(seed)))

		if _, err := checkStepEchoing(doc, Operation{Op: c.Name, Args: []byte(args)}, &seqIDs{}, true); err != nil {
			t.Fatalf("%s %s: %v", c.Name, args, err)
		}
	})
}

// withoutEcho returns a copy of e whose texts no longer hold the strings (keys and
// values) found in the JSON args.
func withoutEcho(e *Error, args []byte) *Error {
	var parsed any
	if json.Unmarshal(args, &parsed) != nil {
		return e
	}

	var words []string

	var walk func(v any)

	walk = func(v any) {
		switch x := v.(type) {
		case string:
			words = append(words, x)
		case []any:
			for _, e := range x {
				walk(e)
			}
		case map[string]any:
			for k, e := range x {
				words = append(words, k)
				walk(e)
			}
		}
	}
	walk(parsed)

	slices.SortFunc(words, func(a, b string) int { return len(b) - len(a) })

	cp := *e

	for _, w := range words {
		if w == "" {
			continue
		}

		cp.Message = strings.ReplaceAll(cp.Message, w, "?")
		cp.Hint = strings.ReplaceAll(cp.Hint, w, "?")
	}

	return &cp
}
