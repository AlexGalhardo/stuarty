package image

import (
	"bytes"
	"maps"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/lint"
)

// templateDoc is a thumbnail template: a photo slot, a title slot with a default and a subtitle slot with none.
func templateDoc() *Document {
	d := baseDoc()
	d.Template = true
	d.Layers[1].Slot = "photo"
	d.Layers[1].Asset = ""
	d.Assets = nil
	d.Layers[2].Text = "Default title" // optional: it has content

	sub := Layer{
		ID: lid('k'), Kind: KindText, Name: "Subtitle", Visible: true, Opacity: 100, X: 120, Y: 600, W: 800, H: 80,
		Slot: "subtitle",
	}
	d.Layers = append(d.Layers, sub)
	ReorderAll(d)

	return d
}

func newText(s string) SlotValue { return SlotValue{Text: &s} }

func slotErr(t *testing.T, err error) *command.Error {
	t.Helper()

	e := command.AsError(err)
	if e.Code != command.CodeSlot {
		t.Fatalf("want E_SLOT, got %+v", e)
	}

	return e
}

func hasCode(fs []lint.Finding, code string) bool {
	for _, f := range fs {
		if f.Code == code {
			return true
		}
	}

	return false
}

func TestTemplateFilled(t *testing.T) {
	tpl := templateDoc()
	frozen := Canonical(tpl)

	got, err := FromTemplate(tpl, map[string]SlotValue{
		"photo":    {Asset: hashFace},
		"subtitle": newText("Episode 12"),
	}, "img_bbbbbbbbbb", testEnv())
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(frozen, Canonical(tpl)) {
		t.Fatal("the template was changed")
	}

	if got.Template || got.ID != "img_bbbbbbbbbb" || len(Validate(got)) > 0 {
		t.Fatalf("not a valid new document: %v %v", got.ID, Validate(got))
	}

	photo, _ := Find(got, lid('b'))
	if photo.Asset != hashFace || photo.Crop == nil || photo.Crop.W*720 != photo.Crop.H*1280 {
		t.Errorf("the photo covers its box with the aspect of the box: %+v", photo.Crop)
	}

	sub, _ := Find(got, lid('k'))
	title, _ := Find(got, lid('c'))

	if sub.Text != "Episode 12" || sub.H < 1 || title.Text != "Default title" {
		t.Errorf("subtitle %q, title %q", sub.Text, title.Text)
	}

	if hasCode(Lint(got, LintEnv{Measure: testMeasure}), CodeEmptySlot) {
		t.Error("a filled template has no empty slot")
	}
}

func TestTemplateTitleCanBeReplaced(t *testing.T) {
	got, err := FromTemplate(templateDoc(), map[string]SlotValue{
		"photo": {Asset: hashFace}, "subtitle": newText("x"), "title": newText("New title"),
	}, "img_bbbbbbbbbb", testEnv())
	if err != nil {
		t.Fatal(err)
	}

	if title, _ := Find(got, lid('c')); title.Text != "New title" {
		t.Fatal("the value replaces the default")
	}
}

func TestTemplateMissingValueIsRefused(t *testing.T) {
	_, err := FromTemplate(templateDoc(), map[string]SlotValue{"photo": {Asset: hashFace}}, "img_bbbbbbbbbb", testEnv())
	e := slotErr(t, err)

	missing, _ := e.Details["missing"].([]string)
	if e.Details["reason"] != SlotMissing || len(missing) != 1 || missing[0] != "subtitle" {
		t.Fatalf("a missing slot value is refused with the slots that lack one: %+v", e)
	}

	if err := command.CheckError(e); err != nil {
		t.Fatal(err)
	}

	_, err = FromTemplate(templateDoc(), nil, "img_bbbbbbbbbb", testEnv())
	if m, _ := slotErr(t, err).Details["missing"].([]string); len(m) != 2 {
		t.Fatalf("every missing slot is listed: %v", m)
	}
}

func TestTemplateBadValues(t *testing.T) {
	ok := map[string]SlotValue{"photo": {Asset: hashFace}, "subtitle": newText("x")}

	for name, tc := range map[string]struct {
		change func(m map[string]SlotValue)
		reason string
	}{
		"unknown slot":       {func(m map[string]SlotValue) { m["nope"] = newText("x") }, SlotUnknown},
		"text in image slot": {func(m map[string]SlotValue) { m["photo"] = newText("x") }, SlotWrongKind},
		"asset in text slot": {func(m map[string]SlotValue) { m["subtitle"] = SlotValue{Asset: hashFace} }, SlotWrongKind},
		"asset not in store": {
			func(m map[string]SlotValue) { m["photo"] = SlotValue{Asset: "sha256:" + strings.Repeat("c", 64)} },
			SlotInvalidValue,
		},
		"text too long":    {func(m map[string]SlotValue) { m["subtitle"] = newText(strings.Repeat("x", MaxText+1)) }, SlotInvalidValue},
		"asset not a hash": {func(m map[string]SlotValue) { m["photo"] = SlotValue{Asset: "face.png"} }, SlotInvalidValue},
	} {
		t.Run(name, func(t *testing.T) {
			m := maps.Clone(ok)

			tc.change(m)

			_, err := FromTemplate(templateDoc(), m, "img_bbbbbbbbbb", testEnv())
			e := slotErr(t, err)

			if e.Details["reason"] != tc.reason {
				t.Fatalf("reason %v, want %s", e.Details["reason"], tc.reason)
			}

			if cerr := command.CheckError(e); cerr != nil {
				t.Fatal(cerr)
			}
		})
	}
}

func TestTemplateFromADocumentThatIsNotOne(t *testing.T) {
	d := templateDoc()
	d.Template = false

	_, err := FromTemplate(d, nil, "img_bbbbbbbbbb", testEnv())
	if command.AsError(err).Code != command.CodeInvalidState {
		t.Fatal("a document that is not a template is refused")
	}
}
