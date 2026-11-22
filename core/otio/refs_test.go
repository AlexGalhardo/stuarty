package otio

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// linkedDoc is the sample with its two clips and the music linked, a caption that follows the first clip
// and a template with a description and colours.
func linkedDoc() *timeline.Document {
	d := sampleDoc()

	for _, id := range []string{"itm_c000000001", "itm_c000000002", "itm_c000000004"} {
		it := d.Items[id]
		it.Link = "lnk_1111111111"
		d.Items[id] = it
	}

	sub := timeline.NewItem("itm_c000000007", timeline.ItemCaption, trkTop, 100, 40)
	sub.Words = []timeline.Word{{Text: "one", Start: 0, Duration: 10}, {Text: "two", Start: 10, Duration: 10}}
	sub.LinkedItem = "itm_c000000001"
	d.Items[sub.ID] = sub

	d.Template = &timeline.TemplateInfo{
		Description: "Product teaser",
		Colors:      map[string]timeline.Color{"brand": "#FF3366"},
	}

	return d
}

func itemNamed(d *timeline.Document, name string) timeline.Item {
	for id := range d.Items {
		if it := d.Items[id]; it.Name == name {
			return it
		}
	}

	return timeline.Item{}
}

// VV-025: the link of items, the link of a caption to its clip and the template information come back from a
// round trip, under new ids, and the same document writes the same file.
func TestRoundTripKeepsLinksCaptionLinksAndTemplate(t *testing.T) {
	d := linkedDoc()

	first, lost, err := Export(d)
	if err != nil {
		t.Fatal(err)
	}

	for _, l := range lost {
		if strings.Contains(l.Feature, "link") || strings.Contains(l.Feature, "template") {
			t.Errorf("a link that the file carries is reported as lost: %+v", l)
		}
	}

	for _, id := range []string{"itm_c000000001", "lnk_1111111111"} {
		if bytes.Contains(first, []byte(id)) {
			t.Errorf("the file carries the id %s of the document", id)
		}
	}

	im, err := Parse(first)
	if err != nil {
		t.Fatal(err)
	}

	doc := build(t, im, newDoc(im.FPS))

	if doc.Template == nil || doc.Template.Description != "Product teaser" ||
		doc.Template.Colors["brand"] != "#FF3366" {
		t.Errorf("template = %+v", doc.Template)
	}

	var link string

	linked := 0

	for id := range doc.Items {
		it := doc.Items[id]
		if it.Link == "" {
			continue
		}

		linked++

		if link == "" {
			link = it.Link
		}

		if it.Link != link {
			t.Errorf("the linked items are in two groups: %s and %s", link, it.Link)
		}
	}

	if linked != 3 {
		t.Errorf("%d items are linked, want 3", linked)
	}

	intro := itemNamed(doc, "intro")

	var caption timeline.Item

	for id := range doc.Items {
		if it := doc.Items[id]; it.Type == timeline.ItemCaption {
			caption = it
		}
	}

	if intro.ID == "itm_c000000001" || caption.LinkedItem != intro.ID {
		t.Errorf("the caption follows %q, want the new id of the first clip %q", caption.LinkedItem, intro.ID)
	}

	second, _, err := Export(doc)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(withoutMarkers(t, second), withoutMarkers(t, first)) {
		t.Errorf("round trip differs:\n%s\n---\n%s", second, first)
	}
}

// Two imports of one file into one project never collide: each gets its own ids, its own link group and its own
// caption link.
func TestTwoImportsIntoOneProjectKeepTheirOwnLinks(t *testing.T) {
	file, _, err := Export(linkedDoc())
	if err != nil {
		t.Fatal(err)
	}

	target := newDoc(timeline.Rational{Num: 30, Den: 1})
	newID := seqIDs()

	byRef := map[string]string{}
	for id := range target.Assets {
		byRef[target.Assets[id].Name] = id
	}

	for range 2 {
		im, perr := Parse(file)
		if perr != nil {
			t.Fatal(perr)
		}

		for _, m := range target.Markers {
			im.MarkedFrames = append(im.MarkedFrames, m.Frame)
		}

		// The second import finds the markers of the first at the same frames: they are reported, not a failure.
		batch, lost := im.Operations(byRef, newID)
		for _, l := range lost {
			if !strings.Contains(l.Feature, "timeline marker") {
				t.Fatalf("unexpected loss: %v", l)
			}
		}

		res, aerr := command.Default().ApplyBatch(target, batch, command.Env{NewIDs: newID})
		if aerr != nil {
			t.Fatalf("an import into a project that holds the first: %v", aerr)
		}

		target = res.Doc
	}

	groups := map[string]int{}

	for id := range target.Items {
		if l := target.Items[id].Link; l != "" {
			groups[l]++
		}
	}

	if len(groups) != 2 {
		t.Fatalf("link groups = %v, want two of three items", groups)
	}

	for l, n := range groups {
		if n != 3 {
			t.Errorf("group %s has %d items, want 3", l, n)
		}
	}

	follows := map[string]bool{}

	for id := range target.Items {
		if it := target.Items[id]; it.Type == timeline.ItemCaption {
			follows[it.LinkedItem] = true

			if target.Items[it.LinkedItem].Name != "intro" {
				t.Errorf("a caption follows %q", target.Items[it.LinkedItem].Name)
			}
		}
	}

	if len(follows) != 2 {
		t.Errorf("the two captions follow %v, want two different clips", follows)
	}
}

const hostileHead = `{"OTIO_SCHEMA":"Timeline.1","name":"x","tracks":{"OTIO_SCHEMA":"Stack.1","children":[
{"OTIO_SCHEMA":"Track.1","kind":"Video","name":"V","children":[`

const hostileRange = `"source_range":{"OTIO_SCHEMA":"TimeRange.1","start_time":{"OTIO_SCHEMA":"RationalTime.1","rate":30,"value":0},
"duration":{"OTIO_SCHEMA":"RationalTime.1","rate":30,"value":30}}`

func hostileClip(meta string) string {
	return `{"OTIO_SCHEMA":"Clip.2","name":"c","metadata":{"vukem":` + meta + `},` + hostileRange +
		`,"media_reference":{"OTIO_SCHEMA":"ExternalReference.1","target_url":"clips/a.mp4"}}`
}

func hostileCaption(meta string) string {
	return `{"OTIO_SCHEMA":"Gap.1","name":"cap","metadata":{"vukem":` + meta + `},` + hostileRange + `}`
}

func parseFor(t *testing.T, file string, target *timeline.Document) (*Import, map[string]string) {
	t.Helper()

	im, err := Parse([]byte(file))
	if err != nil {
		t.Fatal(err)
	}

	byRef := map[string]string{}
	for id := range target.Assets {
		byRef[target.Assets[id].Name] = id
	}

	return im, byRef
}

// A file cannot point a link at an item of the project it is imported into, or of another project: a label that
// no item of the file carries names nothing, and one that an item carries names the new id of that item.
func TestHostileLabelsNeverNameAnItemOfTheProject(t *testing.T) {
	target := sampleDoc()
	other := "itm_c000000001"
	word := `"caption":{"words":[{"text":"hi","start":0,"duration":10}]}`

	// The caption follows an id that no item of the file has: dropped and reported.
	file := hostileHead + hostileCaption(`{`+word+`,"linked_item":"`+other+`"}`) + `]}]}}`
	im, byRef := parseFor(t, file, target)
	batch, lost := im.Operations(byRef, seqIDs())

	if bytes.Contains(mustJSON(batch), []byte(other)) {
		t.Errorf("the batch names the item of the project: %s", mustJSON(batch))
	}

	if len(lost) != 1 || !strings.Contains(lost[0].Feature, "caption link") {
		t.Errorf("lost = %+v, want the caption link", lost)
	}

	res, err := command.Default().ApplyBatch(target, batch, command.Env{NewIDs: seqIDs()})
	if err != nil {
		t.Fatalf("the batch does not apply: %v", err)
	}

	for id := range res.Doc.Items {
		if it := res.Doc.Items[id]; it.Type == timeline.ItemCaption && it.LinkedItem != "" {
			t.Errorf("the caption follows %q", it.LinkedItem)
		}
	}

	// A file whose own clip carries the id of an item of the project as its label: the link goes to the new id.
	own := hostileClip(`{"id":"` + other + `"}`)
	follower := hostileCaption(`{` + word + `,"linked_item":"` + other + `"}`)
	file = hostileHead + own + `,` + follower + `]}]}}`
	im, byRef = parseFor(t, file, target)
	batch, lost = im.Operations(byRef, seqIDs())

	if len(lost) != 0 {
		t.Errorf("lost = %+v", lost)
	}

	res, err = command.Default().ApplyBatch(target, batch, command.Env{NewIDs: seqIDs()})
	if err != nil {
		t.Fatalf("the batch does not apply: %v", err)
	}

	for id := range res.Doc.Items {
		it := res.Doc.Items[id]
		if it.Type == timeline.ItemCaption && (it.LinkedItem == other || res.Doc.Items[it.LinkedItem].Name != "c") {
			t.Errorf("the caption follows %q, want the clip of the file", it.LinkedItem)
		}
	}
}

// Labels that are not valid, used twice, or that tie a lone item are reported and ignored; the batch still applies.
func TestBadLabelsAreReportedAndIgnored(t *testing.T) {
	target := sampleDoc()
	long := strings.Repeat("x", 65)

	file := hostileHead +
		hostileClip(`{"id":"a","link":"g"}`) + `,` +
		hostileClip(`{"id":"a","link":7}`) + `,` +
		hostileClip(`{"id":"`+long+`","link":"`+long+`"}`) + `,` +
		hostileClip(`{"link":"lonely"}`) + `]}]}}`

	im, byRef := parseFor(t, file, target)

	var features []string
	for _, l := range im.Lost {
		features = append(features, l.Feature)
	}

	all := strings.Join(features, ";")
	for _, want := range []string{"id of c is used by another", "link of c is not a valid", "id of c is not a valid"} {
		if !strings.Contains(all, want) {
			t.Errorf("%q not reported in %s", want, all)
		}
	}

	batch, lost := im.Operations(byRef, seqIDs())

	if len(lost) != 2 {
		t.Errorf("lost = %+v, want the two groups with a lone item", lost)
	}

	for _, o := range batch {
		if o.Op == "item.link" {
			t.Errorf("an item.link was made for a lone item: %s", mustJSON(o))
		}
	}

	if _, err := command.Default().ApplyBatch(target, batch, command.Env{NewIDs: seqIDs()}); err != nil {
		t.Errorf("the batch does not apply: %v", err)
	}
}

// A link group over 20 items cannot be made by item.link: it is reported, never half made.
func TestALinkGroupOverTheLimitIsReported(t *testing.T) {
	var kids []string

	for range 21 {
		kids = append(kids, hostileClip(`{"link":"big"}`))
	}

	im, byRef := parseFor(t, hostileHead+strings.Join(kids, ",")+`]}]}}`, sampleDoc())
	batch, lost := im.Operations(byRef, seqIDs())

	if len(lost) != 1 || !strings.Contains(lost[0].Feature, "more than 20") {
		t.Errorf("lost = %+v", lost)
	}

	for _, o := range batch {
		if o.Op == "item.link" {
			t.Error("an item.link was made for 21 items")
		}
	}
}

// A template that is not valid is reported and the valid part is kept.
func TestBadTemplateInformationIsReported(t *testing.T) {
	file := `{"OTIO_SCHEMA":"Timeline.1","name":"x","metadata":{"vukem":{"template":{"description":"d",
"colors":{"brand":"#FF3366","Bad Name":"#000000","red":"red"}}}},"tracks":{"OTIO_SCHEMA":"Stack.1","children":[
{"OTIO_SCHEMA":"Track.1","kind":"Video","name":"V","children":[` + hostileClip(`{}`) + `]}]}}`

	im, byRef := parseFor(t, file, sampleDoc())

	if len(im.Lost) != 2 {
		t.Errorf("lost = %+v, want the two bad colours", im.Lost)
	}

	batch, _ := im.Operations(byRef, seqIDs())

	var first command.Operation
	if len(batch) > 0 {
		first = batch[0]
	}

	var args map[string]any
	if err := json.Unmarshal(first.Args, &args); err != nil || first.Op != "template.set_info" ||
		args["description"] != "d" {
		t.Errorf("the first operation = %s %s", first.Op, first.Args)
	}

	if colours, _ := args["colors"].(map[string]any); len(colours) != 1 || colours["brand"] != "#FF3366" {
		t.Errorf("colours = %v", args["colors"])
	}
}
