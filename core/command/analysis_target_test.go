package command

import (
	"bytes"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

// The service fills resolved only when it can see an asset. When the target is missing or
// has no asset it leaves the field out and the command must name the item (E_NOT_FOUND or
// E_TYPE_MISMATCH), not the internal field. A real clip with no resolved is still
// E_VALIDATION: Apply has nothing else to cut or retime from.
func TestAnalysisCommandReportsTheTargetBeforeResolved(t *testing.T) {
	t.Parallel()

	reg := Default()
	shape := func() *timeline.Document { return base().shape(itemA, vidTr, 0, 90).d }
	clip := func() *timeline.Document { return base().clip(itemA, vidTr, 0, 90, 0).d }
	image := func() *timeline.Document {
		return base().clip(itemA, vidTr, 0, 90, 0, func(it *timeline.Item) { it.Asset = imgAsset }).d
	}
	music := func() *timeline.Document { return base().audio(itemB, 0, 90, 0).d }

	cases := []struct {
		name, op, args string
		doc            func() *timeline.Document
		code, path     string
	}{
		{
			name: "remove_silences missing item",
			op:   "clip.remove_silences",
			args: `{"item_id": "itm_zzzzzzzzzz"}`,
			doc:  clip,
			code: CodeNotFound, path: "args.item_id",
		},
		{
			name: "remove_silences shape",
			op:   "clip.remove_silences",
			args: `{"item_id": "` + itemA + `"}`,
			doc:  shape,
			code: CodeTypeMismatch, path: "args.item_id",
		},
		{
			name: "remove_silences image",
			op:   "clip.remove_silences",
			args: `{"item_id": "` + itemA + `"}`,
			doc:  image,
			code: CodeTypeMismatch, path: "args.item_id",
		},
		{
			name: "retime missing clip",
			op:   "caption.retime_from_clip",
			args: `{"clip_item_id": "itm_zzzzzzzzzz"}`,
			doc:  clip,
			code: CodeNotFound, path: "args.clip_item_id",
		},
		{
			name: "retime shape",
			op:   "caption.retime_from_clip",
			args: `{"clip_item_id": "` + itemA + `"}`,
			doc:  shape,
			code: CodeTypeMismatch, path: "args.clip_item_id",
		},
		{
			name: "from_transcript missing clip",
			op:   "caption.from_transcript",
			args: `{"clip_item_id": "itm_zzzzzzzzzz", "track_id": "` + capTr + `"}`,
			doc:  clip,
			code: CodeNotFound, path: "args.clip_item_id",
		},
		{
			name: "from_transcript shape",
			op:   "caption.from_transcript",
			args: `{"clip_item_id": "` + itemA + `", "track_id": "` + capTr + `"}`,
			doc:  shape,
			code: CodeTypeMismatch, path: "args.clip_item_id",
		},
		{
			name: "from_transcript missing track",
			op:   "caption.from_transcript",
			args: `{"clip_item_id": "` + itemA + `", "track_id": "trk_zzzzzzzzzz"}`,
			doc:  clip,
			code: CodeNotFound, path: "args.track_id",
		},
		{
			name: "duck missing music",
			op:   "audio.duck",
			args: `{"item_id": "itm_zzzzzzzzzz", "speech_item_ids": ["` + itemA + `"]}`,
			doc:  clip,
			code: CodeNotFound, path: "args.item_id",
		},
		{
			name: "duck missing speech item",
			op:   "audio.duck",
			args: `{"item_id": "` + itemB + `", "speech_item_ids": ["` + itemA + `"]}`,
			doc:  music,
			code: CodeNotFound, path: "args.speech_item_ids",
		},
		{
			name: "duck music is text",
			op:   "audio.duck",
			args: `{"item_id": "` + itemA + `", "speech_item_ids": ["` + itemB + `"]}`,
			doc: func() *timeline.Document {
				return base().text(itemA, vidTr, 0, 90).audio(itemB, 0, 90, 0).d
			},
			code: CodeTypeMismatch, path: "args.item_id",
		},
		{
			name: "mark_beats missing item",
			op:   "audio.mark_beats",
			args: `{"item_id": "itm_zzzzzzzzzz"}`,
			doc:  music,
			code: CodeNotFound, path: "args.item_id",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			for range 2 {
				doc := tc.doc()
				before := bytes.Clone(timeline.Canonical(doc))
				_, err := reg.Apply(doc, op2(tc.op, tc.args), Env{})
				e := AsError(err)
				if err == nil || e.Code != tc.code || e.Path != tc.path || strings.Contains(e.Message, "resolved") ||
					strings.Contains(e.Hint, "resolved") {
					t.Fatalf("got code=%s path=%s message=%q hint=%q", e.Code, e.Path, e.Message, e.Hint)
				}

				if !bytes.Equal(timeline.Canonical(doc), before) {
					t.Fatal("the failed command changed the document")
				}
			}
		})
	}

	t.Run("remove_silences on a clip still requires resolved", func(t *testing.T) {
		t.Parallel()

		doc := clip()
		before := bytes.Clone(timeline.Canonical(doc))
		_, err := reg.Apply(doc, op2("clip.remove_silences", `{"item_id": "`+itemA+`"}`), Env{})
		e := AsError(err)
		if err == nil || e.Code != CodeValidation || !strings.Contains(e.Message, "resolved") ||
			e.Path != "args.resolved" {
			t.Fatalf("got %+v", e)
		}

		if !bytes.Equal(timeline.Canonical(doc), before) {
			t.Fatal("the failed command changed the document")
		}
	})
}
