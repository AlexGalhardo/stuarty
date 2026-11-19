package command

import (
	"fmt"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// Golden cases of font.add and font.remove (SDD-002 C.53, C.54) and of the font
// error that lists the project fonts.

func fontHash(pair string) string { return "sha256:" + strings.Repeat(pair, 32) }

func withFonts(b *B) *B {
	return b.with(func(d *timeline.Document) {
		d.Fonts = map[string]timeline.ProjectFont{
			"Brand Sans": {
				Files: []timeline.FontFile{{Weight: 400, Hash: fontHash("ab")}, {Weight: 700, Hash: fontHash("cd")}},
			},
			"Spare": {Files: []timeline.FontFile{{Weight: 400, Hash: fontHash("ef")}}},
		}
	})
}

func init() {
	fa := func(name, args string, b func() *B) { gc("font.add", name, b, op("font.add", args)) }
	file := func(w int, pair string) string { return fmt.Sprintf(`{"weight": %d, "hash": %q}`, w, fontHash(pair)) }
	add := func(family string, files ...string) string {
		return fmt.Sprintf(`{"family": %q, "files": [%s]}`, family, strings.Join(files, ", "))
	}

	fa("ok_new_family_two_weights", add("Brand Sans", file(400, "ab"), file(700, "cd")), base)
	fa("ok_one_file", add("Brand Sans", file(400, "ab")), base)
	fa("ok_family_name_with_space_dot_and_hyphen", add("Acme Co. Sans-2", file(400, "ab")), base)
	fa("ok_adds_a_weight_to_a_family", add("Brand Sans", file(900, "12")), func() *B { return withFonts(base()) })
	fa("ok_replaces_the_file_of_a_weight", add("Brand Sans", file(700, "34")), func() *B { return withFonts(base()) })
	fa("ok_same_file_again_changes_nothing", add("Spare", file(400, "ef")), func() *B { return withFonts(base()) })
	fa("err_validation_bundled_family", add("Inter", file(400, "ab")), base)
	fa("err_validation_family_empty", add("", file(400, "ab")), base)
	fa("err_validation_family_bad_characters", add("Brand/Sans", file(400, "ab")), base)
	fa("err_validation_family_starts_with_space", add(" Brand", file(400, "ab")), base)
	fa("err_validation_family_over_60_characters", add(strings.Repeat("a", 61), file(400, "ab")), base)
	fa("err_validation_no_files", add("Brand Sans"), base)
	fa("err_validation_weight_not_a_multiple_of_100", add("Brand Sans", file(450, "ab")), base)
	fa("err_validation_weight_above_900", add("Brand Sans", file(1000, "ab")), base)
	fa("err_validation_weight_twice", add("Brand Sans", file(400, "ab"), file(400, "cd")), base)
	fa("err_validation_hash_malformed", add("Brand Sans", `{"weight": 400, "hash": "sha256:abc"}`), base)
	fa("err_validation_hash_upper_case", add("Brand Sans", fmt.Sprintf(`{"weight": 400, "hash": %q}`,
		"sha256:"+strings.Repeat("AB", 32))), base)

	fr := func(name, args string, b func() *B) { gc("font.remove", name, b, op("font.remove", args)) }
	rm := func(family string) string { return fmt.Sprintf(`{"family": %q}`, family) }
	fr("ok_remove_an_unused_family", rm("Spare"), func() *B { return withFonts(base()) })
	fr("ok_remove_the_last_family", rm("Spare"), func() *B {
		return base().with(func(d *timeline.Document) {
			d.Fonts = map[string]timeline.ProjectFont{
				"Spare": {Files: []timeline.FontFile{{Weight: 400, Hash: fontHash("ef")}}},
			}
		})
	})
	fr("err_not_found_family", rm("Nowhere"), func() *B { return withFonts(base()) })
	fr("err_validation_bundled_family", rm("Inter"), func() *B { return withFonts(base()) })
	fr("err_in_use_text_style", rm("Brand Sans"), func() *B {
		return withFonts(
			base(),
		).text(itemC, vidTr, 0, 60, withStyle(`{"font_family": "Brand Sans", "font_weight": 700}`))
	})
	fr("err_in_use_caption_style", rm("Brand Sans"), func() *B {
		return withFonts(base()).caption(itemA, 0, 1, withStyle(`{"font_family": "Brand Sans"}`))
	})
	fr("err_in_use_track_caption_style", rm("Brand Sans"), func() *B {
		return withFonts(base()).captionStyle(capTr, `{"font_family": "Brand Sans"}`)
	})
	fr("err_in_use_lists_every_user", rm("Brand Sans"), func() *B {
		return withFonts(base()).captionStyle(capTr, `{"font_family": "Brand Sans"}`).
			text(itemC, vidTr, 0, 60, withStyle(`{"font_family": "Brand Sans"}`))
	})

	ta := func(name, args string, b func() *B) { gc("text.add", name, b, op("text.add", args)) }
	textArgs := func(style string) string {
		return `{"track_id": "` + vidTr + `", "start": "0s", "duration": "2s", "text": "Hello", "id": "` + itemC +
			`", "style": ` + style + `}`
	}
	ta("ok_style_with_a_project_font", textArgs(`{"font_family": "Brand Sans", "font_weight": 700}`),
		func() *B { return withFonts(base()) })
	ta("err_font_unknown_family_lists_project_fonts", textArgs(`{"font_family": "Comic"}`),
		func() *B { return withFonts(base()) })
	ta("err_font_unknown_weight_of_a_project_font", textArgs(`{"font_family": "Brand Sans", "font_weight": 900}`),
		func() *B { return withFonts(base()) })
}

// A style that names a family added earlier in the same batch validates; the same
// ops in the other order fail with E_FONT_UNKNOWN.
func TestFontAddThenStyleInOneBatch(t *testing.T) {
	t.Parallel()

	addOp := Operation{
		Op:   "font.add",
		Args: []byte(`{"family": "Brand Sans", "files": [{"weight": 700, "hash": "` + fontHash("ab") + `"}]}`),
	}
	textOp := Operation{
		Op: "text.add",
		Args: []byte(`{"track_id": "` + vidTr + `", "start": "0s", "duration": "2s", "text": "Hi", "id": "` +
			itemC + `", "style": {"font_family": "Brand Sans", "font_weight": 700}}`),
	}
	env := Env{NewIDs: (&seqIDs{}).next}

	res, err := FullRegistry().ApplyBatch(base().d, []Operation{addOp, textOp}, env)
	if err != nil {
		t.Fatalf("font.add then text.add: %v", err)
	}

	if got := *res.Doc.Items[itemC].Style.FontFamily; got != "Brand Sans" {
		t.Errorf("style family %q", got)
	}

	_, err = FullRegistry().ApplyBatch(base().d, []Operation{textOp, addOp}, env)
	if e := AsError(err); e == nil || e.Code != CodeFontUnknown {
		t.Errorf("text.add then font.add: %v, want %s", err, CodeFontUnknown)
	}
}
