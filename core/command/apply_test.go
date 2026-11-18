package command

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
	"github.com/AlexGalhardo/vukem-video/core/timeval"
)

func mustApply(t *testing.T, r *Registry, doc *timeline.Document, op Operation) Result {
	t.Helper()
	res, err := r.Apply(doc, op, Env{NewIDs: (&seqIDs{}).next})
	if err != nil {
		t.Fatalf("%s: %v", op.Op, err)
	}
	return res
}

func TestApplyChangesTheCopyNotTheInput(t *testing.T) {
	r := testRegistry(t)
	doc := baseDoc(t)
	before := timeline.Canonical(doc)
	res := mustApply(t, r, doc, opJSON("test.item_move", `{"item_id":"`+clipID+`","start":"30f"}`))
	if !bytes.Equal(timeline.Canonical(doc), before) {
		t.Fatal("Apply mutated its input document")
	}
	if res.Doc == doc || res.Doc.Items[clipID].Start != 30 {
		t.Errorf("start %d", res.Doc.Items[clipID].Start)
	}
	if len(res.Diff) != 1 || res.Diff[0].Kind != KindItemUpdated || res.Diff[0].ID != clipID {
		t.Errorf("diff %+v", res.Diff)
	}
	if got := res.Diff[0].Changes["start"]; got != (Pair{json.Number("0"), json.Number("30")}) {
		t.Errorf("changes %v", res.Diff[0].Changes)
	}
	eff, _ := res.Effective["start"].(map[string]any)
	if eff["frame"] != 30 || eff["seconds"] != 1.0 || eff["requested"] != "30f" {
		t.Errorf("effective %v", res.Effective)
	}
	// The inverse undoes it exactly.
	back, skipped := ApplyPrimitives(res.Doc, reversed(res.Inverse), nil)
	if len(skipped) != 0 || !bytes.Equal(timeline.Canonical(back.Doc), before) {
		t.Errorf("inverse did not restore the document: %v", skipped)
	}
}

func reversed(ps []Primitive) []Primitive {
	out := make([]Primitive, len(ps))
	for i, p := range ps {
		out[len(ps)-1-i] = p
	}
	return out
}

func TestApplyErrors(t *testing.T) {
	r := testRegistry(t)
	doc := baseDoc(t)
	cases := []struct {
		name string
		op   Operation
		code string
		path string
	}{
		{"unknown command", opJSON("nope.nope", `{}`), CodeUnknownCommand, ""},
		{"args invalid", opJSON("test.name", `{"name":5}`), CodeValidation, "args.name"},
		{"args missing", opJSON("test.item_move", `{}`), CodeValidation, "args.item_id"},
		{
			"command error",
			opJSON("test.item_move", `{"item_id":"itm_zzzzzzzzzz","start":"1f"}`),
			CodeNotFound,
			"args.item_id",
		},
		{
			"bare time",
			opJSON("test.item_move", `{"item_id":"`+clipID+`","start":"12"}`),
			CodeInvalidTime,
			"args.start",
		},
		{"typed", opJSON("test.misbehave", `{"mode":"typed"}`), CodeNotFound, ""},
		{"plain error", opJSON("test.misbehave", `{"mode":"plain"}`), CodeInternal, ""},
		{"panic", opJSON("test.misbehave", `{"mode":"panic"}`), CodeInternal, ""},
		{"invalid result", opJSON("test.misbehave", `{"mode":"bug"}`), CodeInternal, ""},
		{"no ids", opJSON("test.track_add", `{"kind":"visual","order":"b0"}`), CodeInternal, ""},
	}
	for _, c := range cases {
		before := timeline.Canonical(doc)
		_, err := r.Apply(doc, c.op, Env{})
		e, ok := errors.AsType[*Error](err)
		if !ok || e.Code != c.code || e.Path != c.path {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if cerr := CheckError(e); cerr != nil {
			t.Errorf("%s: %v", c.name, cerr)
		}
		if !bytes.Equal(timeline.Canonical(doc), before) {
			t.Errorf("%s: the document changed on error", c.name)
		}
	}
	// An invalid result is reported with the violated invariants, only in debug builds.
	_, err := r.Apply(doc, opJSON("test.misbehave", `{"mode":"bug"}`), Env{})
	if e, ok := errors.AsType[*Error](err); !ok || e.Details["violations"] == nil {
		t.Errorf("violations missing: %v", err)
	}
	r.Debug = false
	if _, err := r.Apply(doc, opJSON("test.misbehave", `{"mode":"bug"}`), Env{}); err != nil {
		t.Errorf("without Debug the result is not validated: %v", err)
	}
}

func TestApplyBatchFoldsAndNamesTheFailingOperation(t *testing.T) {
	r := testRegistry(t)
	doc := baseDoc(t)
	ids := &seqIDs{}
	ops := []Operation{
		opJSON("test.track_add", `{"kind":"visual","order":"b0"}`),
		opJSON("test.item_add", `{"track_id":"`+capTrack+`","start":"100f","duration":10,"id":"itm_k000000001"}`),
		opJSON("test.name", `{"name":"Renamed"}`),
	}
	res, err := r.ApplyBatch(doc, ops, Env{NewIDs: ids.next})
	if err != nil {
		t.Fatal(err)
	}
	if res.Doc.Name != "Renamed" || len(res.Doc.Tracks) != 3 || len(res.Doc.Items) != 3 || len(res.Effective) != 3 {
		t.Errorf("batch result: %+v", res.Doc.Name)
	}
	if id, _ := res.Effective[0]["track_id"].(string); id == "" || res.Doc.Tracks[id].Order != "b0" {
		t.Errorf("effective %v", res.Effective[0])
	}

	// The batch equals its operations applied one by one.
	cur := doc
	ids = &seqIDs{}
	for _, op := range ops {
		step, err := r.Apply(cur, op, Env{NewIDs: ids.next})
		if err != nil {
			t.Fatal(err)
		}
		cur = step.Doc
	}
	if !bytes.Equal(timeline.Canonical(cur), timeline.Canonical(res.Doc)) {
		t.Error("batch differs from one by one")
	}

	// A failure anywhere fails the whole batch with op_index.
	bad := append(append([]Operation{}, ops...), opJSON("test.item_move", `{"item_id":"itm_zzzzzzzzzz","start":"1f"}`))
	before := timeline.Canonical(doc)
	_, err = r.ApplyBatch(doc, bad, Env{NewIDs: (&seqIDs{}).next})
	var e *Error
	if !errors.As(err, &e) || e.Code != CodeNotFound || e.OpIndex == nil || *e.OpIndex != 3 {
		t.Fatalf("got %v", err)
	}
	if !bytes.Equal(timeline.Canonical(doc), before) {
		t.Error("input changed")
	}
	if _, err := r.ApplyBatch(doc, nil, Env{}); err == nil {
		t.Error("an empty batch must fail")
	} else if e, ok := errors.AsType[*Error](err); !ok || e.Code != CodeValidation || CheckError(e) != nil {
		t.Errorf("%v", err)
	}
}

func TestBatchDiffIsTheNetChange(t *testing.T) {
	r := testRegistry(t)
	doc := baseDoc(t)
	ops := []Operation{
		opJSON("test.item_add", `{"track_id":"`+capTrack+`","start":"100f","duration":10,"id":"itm_k000000001"}`),
		opJSON("test.item_move", `{"item_id":"itm_k000000001","start":"200f"}`),
		opJSON("test.item_move", `{"item_id":"`+clipID+`","start":"10f"}`),
		opJSON("test.item_move", `{"item_id":"`+clipID+`","start":"20f"}`),
		opJSON("test.item_remove", `{"item_ids":["itm_k000000001"]}`),
	}
	res, err := r.ApplyBatch(doc, ops, Env{})
	if err != nil {
		t.Fatal(err)
	}
	// The item created and removed inside the batch does not appear; the clip start
	// appears once with its first before-value and its last after-value.
	if len(res.Diff) != 1 || res.Diff[0].ID != clipID ||
		res.Diff[0].Changes["start"] != (Pair{json.Number("0"), json.Number("20")}) {
		t.Errorf("%+v", res.Diff)
	}
	if len(res.Inverse) != 1 || res.Inverse[0].Type != PrimitivePatch ||
		res.Inverse[0].Set["start"] != json.Number("0") {
		t.Errorf("%+v", res.Inverse)
	}
}

func TestContextHelpers(t *testing.T) {
	c := &Context{Doc: baseDoc(t), effective: map[string]any{}}
	if got := c.FPS(); got != (timeval.FPS{Num: 30, Den: 1}) {
		t.Errorf("%v", got)
	}
	if id, err := c.NewID("itm", "itm_given00001"); err != nil || id != "itm_given00001" {
		t.Errorf("supplied id: %v %v", id, err)
	}
	if _, err := c.NewID("itm", ""); err == nil || err.Code != CodeInternal {
		t.Errorf("no generator: %v", err)
	}
	c.Env.NewIDs = func(p string) string { return p + "_generated0" }
	if id, _ := c.NewID("itm", ""); id != "itm_generated0" {
		t.Error(id)
	}
	if f, err := c.ParseTime("args.at", "12.5s"); err != nil || f != 375 {
		t.Errorf("ParseTime: %d %v", f, err)
	}
	_, err := c.ParseTime("args.at", "-1s")
	if err == nil || err.Code != CodeInvalidTime || err.Path != "args.at" || err.Hint == "" || CheckError(err) != nil {
		t.Errorf("%+v", err)
	}
	c.Doc.Settings.FPS = timeline.Rational{Num: 0, Den: 0}
	if _, err := c.ParseTime("args.at", "1s"); err == nil || err.Code != CodeInternal {
		t.Errorf("invalid fps: %v", err)
	}
	c.Doc.Settings.FPS = timeline.Rational{Num: 30000, Den: 1001}
	c.SetEffectiveTime("start", 374, "12.48s")
	c.SetEffective("n", 1)
	eff, _ := c.effective["start"].(map[string]any)
	if eff["frame"] != 374 || eff["seconds"] != 12.479 || c.effective["n"] != 1 {
		t.Errorf("%v", c.effective)
	}
	if got := TimeEffective(timeval.FPS{Num: 30, Den: 1}, 374, "12.48s"); got["seconds"] != 12.467 {
		t.Errorf("%v", got)
	}
}

func TestBatchThatChangesNothingIsANoOp(t *testing.T) {
	r := testRegistry(t)
	doc := baseDoc(t)
	same := []Operation{
		opJSON("test.item_move", `{"item_id":"`+clipID+`","start":"0f"}`),
		opJSON("test.name", `{"name":"`+doc.Name+`"}`),
	}
	res, err := r.ApplyBatch(doc, same, Env{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.NoOp() {
		t.Errorf("diff %+v inverse %+v", res.Diff, res.Inverse)
	}

	// A change undone inside the batch is also nothing to commit.
	back := []Operation{
		opJSON("test.item_move", `{"item_id":"`+clipID+`","start":"20f"}`),
		opJSON("test.item_move", `{"item_id":"`+clipID+`","start":"0f"}`),
	}
	if res, err = r.ApplyBatch(doc, back, Env{}); err != nil || !res.NoOp() {
		t.Errorf("round trip: %v %+v", err, res.Diff)
	}

	res, err = r.ApplyBatch(doc, []Operation{opJSON("test.name", `{"name":"Other"}`)}, Env{})
	if err != nil || res.NoOp() {
		t.Errorf("a real change reported as no-op: %v", err)
	}
}
