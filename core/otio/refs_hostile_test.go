package otio

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

const captionWords = `"caption":{"words":[{"text":"hi","start":0,"duration":10}]}`

func lostFeatures(lost []Lost) string {
	out := make([]string, 0, len(lost))
	for _, l := range lost {
		out = append(out, l.Feature)
	}

	return strings.Join(out, ";")
}

// A caption follows a clip. A label on itself, on another caption (a cycle) or on a text names no clip: the link is
// reported and dropped, and the batch still applies.
func TestACaptionFollowsOnlyAClipOfTheFile(t *testing.T) {
	text := `{"OTIO_SCHEMA":"Gap.1","name":"t","metadata":{"vukem":{"text":"hello","id":"txt"}},` + hostileRange + `}`

	for name, kids := range map[string]string{
		"itself": hostileCaption(`{` + captionWords + `,"id":"a","linked_item":"a"}`),
		"cycle": hostileCaption(`{`+captionWords+`,"id":"a","linked_item":"b"}`) + `,` +
			hostileCaption(`{`+captionWords+`,"id":"b","linked_item":"a"}`),
		"text": text + `,` + hostileCaption(`{`+captionWords+`,"linked_item":"txt"}`),
	} {
		t.Run(name, func(t *testing.T) {
			target := sampleDoc()
			im, byRef := parseFor(t, hostileHead+kids+`]}]}}`, target)
			batch, lost := im.Operations(byRef, seqIDs())

			if !strings.Contains(lostFeatures(lost), "caption link") {
				t.Errorf("lost = %+v, want the caption link", lost)
			}

			res, err := command.Default().ApplyBatch(target, batch, command.Env{NewIDs: seqIDs()})
			if err != nil {
				t.Fatalf("the batch does not apply: %v", err)
			}

			for id := range res.Doc.Items {
				if it := res.Doc.Items[id]; it.Type == timeline.ItemCaption && it.LinkedItem != "" {
					t.Errorf("caption %s follows %q", id, it.LinkedItem)
				}
			}
		})
	}
}

// A link on a caption and a caption link on a clip mean nothing: they are reported, never dropped silently.
func TestALabelOnTheWrongKindOfItemIsReported(t *testing.T) {
	file := hostileHead + hostileClip(`{"linked_item":"x"}`) + `,` +
		hostileCaption(`{`+captionWords+`,"link":"g"}`) + `]}]}}`
	im, _ := parseFor(t, file, sampleDoc())

	all := lostFeatures(im.Lost)
	for _, want := range []string{"linked_item of c", "link of cap"} {
		if !strings.Contains(all, want) {
			t.Errorf("%q not reported in %s", want, all)
		}
	}
}

var documentID = regexp.MustCompile(`(itm|ast|trk|lnk|prj|tml)_[0-9a-z]{10}|sha256:`)

// No id of the document leaves in a file, a lookup table included.
func TestAnExportedFileCarriesNoIdOfTheDocument(t *testing.T) {
	d := linkedDoc()

	var asset string
	for id := range d.Assets {
		asset = id
	}

	it := d.Items["itm_c000000001"]
	it.Lut = &timeline.Lut{Asset: asset, Intensity: 0.5}
	d.Items[it.ID] = it

	file, lost, err := Export(d)
	if err != nil {
		t.Fatal(err)
	}

	if found := documentID.FindAll(file, -1); len(found) > 0 {
		t.Errorf("the file carries ids of the document: %q", found)
	}

	if !strings.Contains(lostFeatures(lost), "lookup table") {
		t.Errorf("lost = %+v, want the lookup table", lost)
	}

	im, err := Parse(file)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(lostFeatures(im.Lost), "lookup table") {
		t.Errorf("import lost = %+v, want the lookup table", im.Lost)
	}
}

func templateFile(description string, colours map[string]string) string {
	raw, err := json.Marshal(map[string]any{"description": description, "colors": colours})
	if err != nil {
		panic(err)
	}

	return `{"OTIO_SCHEMA":"Timeline.1","name":"x","metadata":{"vukem":{"template":` + string(raw) + `}},
"tracks":{"OTIO_SCHEMA":"Stack.1","children":[{"OTIO_SCHEMA":"Track.1","kind":"Video","name":"V","children":[` +
		hostileClip(`{}`) + `]}]}}`
}

// An import adds what the project does not have. It never changes a colour the items of the project already use by
// name, or the description of the project: that would edit work outside the file.
func TestAnImportKeepsTheTemplateOfTheProject(t *testing.T) {
	target := sampleDoc()
	target.Template = &timeline.TemplateInfo{
		Description: "Mine",
		Colors:      map[string]timeline.Color{"brand": "#111111", "same": "#ABCDEF"},
	}

	file := templateFile("Theirs", map[string]string{"brand": "#222222", "same": "#abcdef", "accent": "#333333"})
	im, byRef := parseFor(t, file, target)
	im.ProjectTemplate = target.Template

	batch, lost := im.Operations(byRef, seqIDs())

	all := lostFeatures(lost)
	for _, want := range []string{"template description", "template colour brand"} {
		if !strings.Contains(all, want) {
			t.Errorf("%q not reported in %s", want, all)
		}
	}

	if strings.Contains(all, "colour same") {
		t.Errorf("a colour of the same value is reported: %s", all)
	}

	res, err := command.Default().ApplyBatch(target, batch, command.Env{NewIDs: seqIDs()})
	if err != nil {
		t.Fatalf("the batch does not apply: %v", err)
	}

	got := res.Doc.Template
	if got.Description != "Mine" || got.Colors["brand"] != "#111111" || got.Colors["accent"] != "#333333" {
		t.Errorf("template = %+v", got)
	}
}

// A file with an empty description does not wipe the one of the project.
func TestAnEmptyDescriptionLeavesTheProjectAlone(t *testing.T) {
	target := sampleDoc()
	target.Template = &timeline.TemplateInfo{Description: "Mine", Colors: map[string]timeline.Color{}}

	im, byRef := parseFor(t, templateFile("", map[string]string{"accent": "#333333"}), target)
	im.ProjectTemplate = target.Template

	batch, lost := im.Operations(byRef, seqIDs())
	if strings.Contains(lostFeatures(lost), "template") {
		t.Errorf("lost = %+v", lost)
	}

	res, err := command.Default().ApplyBatch(target, batch, command.Env{NewIDs: seqIDs()})
	if err != nil {
		t.Fatal(err)
	}

	if res.Doc.Template.Description != "Mine" || res.Doc.Template.Colors["accent"] != "#333333" {
		t.Errorf("template = %+v", res.Doc.Template)
	}
}

// Oversized template metadata is bounded and reported once.
func TestAHugeTemplateIsBounded(t *testing.T) {
	colours := map[string]string{}
	for i := range maxTemplateColours + 500 {
		colours[fmt.Sprintf("c%d", i)] = "#000000"
	}

	im, err := Parse([]byte(templateFile("d", colours)))
	if err != nil {
		t.Fatal(err)
	}

	if n := len(im.template.colors); n != maxTemplateColours {
		t.Errorf("%d colours were kept, want %d", n, maxTemplateColours)
	}

	if len(im.Lost) != 1 || !strings.Contains(im.Lost[0].Feature, "over the limit") {
		t.Errorf("lost = %+v, want one entry", im.Lost)
	}
}

// Metadata nested past what the decoder accepts is an error of the file, never a crash.
func TestDeepMetadataIsRefused(t *testing.T) {
	deep := strings.Repeat(`{"a":`, 200_000) + `1` + strings.Repeat(`}`, 200_000)
	file := hostileHead + hostileClip(`{"properties":`+deep+`}`) + `]}]}}`

	if _, err := Parse([]byte(file)); err == nil {
		t.Error("a file nested 200000 levels was accepted")
	}
}
