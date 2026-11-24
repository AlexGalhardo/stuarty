package command

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/facetrack"
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// speakerTrack is a face of 1500 x 3300 ten-thousandths crossing a 1920x1080 source in ms milliseconds,
// sampled at 5 per second.
func speakerTrack(ms int) facetrack.Track {
	tr := facetrack.Track{
		Version: facetrack.Version, Model: "yunet-2023mar", Provider: "cpu",
		Source: facetrack.Size{Width: 1920, Height: 1080}, FPS: 5, EndMS: ms,
	}

	for at := 0; at < ms; at += 200 {
		cx := 1500 + 7000*at/ms
		tr.Samples = append(tr.Samples, facetrack.Sample{
			T: at, Faces: []facetrack.Face{{X: cx - 750, Y: 2500, W: 1500, H: 3300, Score: 900}},
		})
	}

	return tr
}

func faceArgs(t *testing.T, item string, tr facetrack.Track, extra string) string {
	t.Helper()

	raw, err := json.Marshal(tr)
	if err != nil {
		t.Fatal(err)
	}

	return fmt.Sprintf(`{"item_id": %q%s, "resolved": {"track": %s}}`, item, extra, raw)
}

func verticalDoc(duration int) *timeline.Document {
	doc := base().clip(itemA, vidTr, 0, duration, 0).d
	doc.Settings.Width, doc.Settings.Height = 1080, 1920

	return doc
}

func TestApplyFaceFramingKeyframesTheCrop(t *testing.T) {
	doc := verticalDoc(300)
	res, err := Default().Apply(doc, op2("clip.face_frame", faceArgs(t, itemA, speakerTrack(10_000), "")), Env{})
	if err != nil {
		t.Fatal(err)
	}

	it := res.Doc.Items[itemA]
	left, right := it.Keyframes["crop_left"], it.Keyframes["crop_right"]

	if it.Fit != "cover" || len(left) < 10 || len(left) != len(right) || len(left) > maxKeyframes {
		t.Fatalf("fit %q, %d and %d keyframes", it.Fit, len(left), len(right))
	}

	if len(it.Keyframes["crop_top"]) != 0 {
		t.Error("a 16:9 source in a vertical frame has nothing to pan vertically")
	}

	if left[0].T != 0 || left[len(left)-1].T > it.Duration {
		t.Errorf("keyframes run from %d to %d of a clip of %d frames", left[0].T, left[len(left)-1].T, it.Duration)
	}

	// The window is 0.316 of the width: left plus right is what is left of the picture, and it follows
	// the face from the left of the frame to the right.
	for i := range left {
		if sum := left[i].Value + right[i].Value; sum < 0.680 || sum > 0.687 {
			t.Fatalf(
				"keyframe %d: left %v plus right %v is %v, want about 0.684",
				i,
				left[i].Value,
				right[i].Value,
				sum,
			)
		}

		if i > 0 && (left[i].T <= left[i-1].T || left[i].Value < left[i-1].Value) {
			t.Fatalf("keyframe %d does not move forward in time and to the right", i)
		}
	}

	if left[len(left)-1].Value < 0.5 {
		t.Errorf("the window ended at %v of the width", left[len(left)-1].Value)
	}

	if it.Crop == nil || it.Crop.Left != left[0].Value {
		t.Errorf("the static crop %+v does not start at the first keyframe", it.Crop)
	}
}

func TestApplyFaceFramingIsDeterministic(t *testing.T) {
	args := faceArgs(t, itemA, speakerTrack(10_000), `, "smoothing_ms": 300`)

	var out [2][]byte

	for i := range out {
		res, err := Default().Apply(verticalDoc(300), op2("clip.face_frame", args), Env{})
		if err != nil {
			t.Fatal(err)
		}

		if out[i], err = json.Marshal(res.Doc.Items[itemA]); err != nil {
			t.Fatal(err)
		}
	}

	if !bytes.Equal(out[0], out[1]) {
		t.Error("two runs gave different items")
	}
}

func TestApplyFaceFramingReplacesEarlierCropKeyframesAndKeepsTheSourceRange(t *testing.T) {
	doc := base().clip(itemA, vidTr, 0, 150, 60).d
	doc.Settings.Width, doc.Settings.Height = 1080, 1920
	it := doc.Items[itemA]
	it.Keyframes = timeline.Keyframes{"crop_left": {timeline.NewKeyframe(0, 0.1), timeline.NewKeyframe(10, 0.2)}}
	doc.Items[itemA] = it

	res, err := Default().Apply(doc, op2("clip.face_frame", faceArgs(t, itemA, speakerTrack(10_000), "")), Env{})
	if err != nil {
		t.Fatal(err)
	}

	got := res.Doc.Items[itemA]
	if len(got.Keyframes["crop_left"]) < 5 || got.Keyframes["crop_left"][1].Value == 0.2 {
		t.Errorf("the old keyframes were kept: %v", got.Keyframes["crop_left"])
	}

	// The clip plays 2 s to 7 s of the asset. The face is at 2900 ten-thousandths of the width 2 s in, so the
	// window of 0.3164 centred on it starts at 0.1318 and not at the 0 of the start of the asset.
	if v := got.Keyframes["crop_left"][0].Value; v < 0.12 || v > 0.14 {
		t.Errorf("the first window starts at %v, which is not where the face is 2 s in", v)
	}
}

func TestApplyFaceFramingRefusesWhatItCannotFrame(t *testing.T) {
	tr := speakerTrack(10_000)

	for name, tc := range map[string]struct {
		doc  func() *timeline.Document
		args string
		code string
	}{
		"no track": {
			func() *timeline.Document { return verticalDoc(300) },
			fmt.Sprintf(`{"item_id": %q}`, itemA), CodeValidation,
		},
		"a bad option": {
			func() *timeline.Document { return verticalDoc(300) },
			faceArgs(t, itemA, tr, `, "max_pan": 5`), CodeValidation,
		},
		"a bad track": {
			func() *timeline.Document { return verticalDoc(300) },
			faceArgs(t, itemA, facetrack.Track{Version: 9}, ""), CodeValidation,
		},
		"not a clip": {
			func() *timeline.Document { return base().audio(itemA, 0, 300, 0).d },
			faceArgs(t, itemA, tr, ""), CodeTypeMismatch,
		},
		"no item": {
			func() *timeline.Document { return verticalDoc(300) },
			faceArgs(t, itemB, tr, ""), CodeNotFound,
		},
		"a reversed clip": {func() *timeline.Document {
			d := verticalDoc(300)
			it := d.Items[itemA]
			it.Reverse = true
			d.Items[itemA] = it

			return d
		}, faceArgs(t, itemA, tr, ""), CodeInvalidState},
	} {
		_, err := Default().Apply(tc.doc(), op2("clip.face_frame", tc.args), Env{})
		if e := AsError(err); e == nil || e.Code != tc.code {
			t.Errorf("%s: error = %v, want %s", name, err, tc.code)
		}
	}
}

func TestApplyFaceFramingKeepsALongClipUnderTheKeyframeLimit(t *testing.T) {
	res, err := Default().Apply(verticalDoc(30*60), op2("clip.face_frame", faceArgs(t, itemA, speakerTrack(60_000), "")), Env{})
	if err != nil {
		t.Fatal(err)
	}

	if n := len(res.Doc.Items[itemA].Keyframes["crop_left"]); n > maxKeyframes || n < 10 {
		t.Errorf("%d keyframes for a minute", n)
	}
}

func TestApplyFaceFramingWithTheSameAspectHasNothingToPan(t *testing.T) {
	doc := base().clip(itemA, vidTr, 0, 300, 0).d
	doc.Settings.Width, doc.Settings.Height = 1920, 1080 // the frame has the aspect of the 1920x1080 source

	res, err := Default().Apply(doc, op2("clip.face_frame", faceArgs(t, itemA, speakerTrack(10_000), "")), Env{})
	if err != nil {
		t.Fatal(err)
	}

	if it := res.Doc.Items[itemA]; len(it.Keyframes) != 0 || it.Crop != nil || it.Fit != "cover" {
		t.Errorf("item = %+v", it)
	}
}
