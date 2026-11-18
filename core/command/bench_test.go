package command

//go:generate go test -run TestScaleProject -gen-scale .

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/lint"
	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

// benchDoc200 builds the shape of the NFR-001 budget (SDD-005 §18 check 12): 166
// text items and 34 captions of 60 words (2,040 words) on two tracks.
func benchDoc200(b *testing.B) (*timeline.Document, string) {
	b.Helper()
	data, err := os.ReadFile("../timeline/testdata/valid/launch-teaser.json")
	if err != nil {
		b.Fatal(err)
	}
	doc, err := timeline.Decode(data)
	if err != nil {
		b.Fatal(err)
	}
	reg := Default()
	seq := &seqIDs{}
	env := Env{NewIDs: seq.next}
	res, err := reg.ApplyBatch(doc, []Operation{
		opJSON("track.add", `{"kind":"visual","name":"T"}`),
		opJSON("track.add", `{"kind":"visual","name":"C"}`),
	}, env)
	if err != nil {
		b.Fatal(err)
	}
	doc = res.Doc
	var textTrack, capTrack string
	for id, t := range doc.Tracks {
		switch t.Name {
		case "T":
			textTrack = id
		case "C":
			capTrack = id
		}
	}
	words := strings.TrimSpace(strings.Repeat("word ", 60))
	var ops []Operation
	for i := range 166 {
		ops = append(ops, opJSON("text.add", fmt.Sprintf(
			`{"track_id":%q,"start":"%ds","duration":"1s","text":"t%d"}`, textTrack, 1000+i*2, i)))
	}
	for i := range 34 {
		ops = append(ops, opJSON("caption.add", fmt.Sprintf(
			`{"track_id":%q,"start":"%ds","duration":"3s","text":%q}`, capTrack, 2000+i*4, words)))
	}
	res, err = reg.ApplyBatch(doc, ops, env)
	if err != nil {
		b.Fatal(err)
	}
	var moved string
	for id := range res.Doc.Items {
		if res.Doc.Items[id].Track == textTrack {
			moved = id
			break
		}
	}
	return res.Doc, moved
}

// BenchmarkApplyBatch200Items is the core share of the commit budget: one
// item.move on a project of about 200 items, through ApplyBatch.
func BenchmarkApplyBatch200Items(b *testing.B) {
	doc, moved := benchDoc200(b)
	reg := Default()
	reg.Debug = false
	b.Cleanup(func() { reg.Debug = true })
	env := Env{NewIDs: (&seqIDs{}).next}
	ops := [2][]Operation{
		{opJSON("item.move", fmt.Sprintf(`{"item_id":%q,"start":"3000s"}`, moved))},
		{opJSON("item.move", fmt.Sprintf(`{"item_id":%q,"start":"3002s"}`, moved))},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		res, err := reg.ApplyBatch(doc, ops[i%2], env)
		if err != nil {
			b.Fatal(err)
		}
		doc = res.Doc
	}
}

// BenchmarkApplyBatch200ItemsEdits applies a batch of 20 operations to the same
// project, the heavier shape of a UI drag or an agent batch.
func BenchmarkApplyBatch200ItemsEdits(b *testing.B) {
	doc, moved := benchDoc200(b)
	reg := Default()
	reg.Debug = false
	b.Cleanup(func() { reg.Debug = true })
	env := Env{NewIDs: (&seqIDs{}).next}
	mk := func(base int) []Operation {
		ops := make([]Operation, 20)
		for i := range ops {
			ops[i] = opJSON("item.move", fmt.Sprintf(`{"item_id":%q,"start":"%ds"}`, moved, base+i))
		}
		return ops
	}
	ops := [2][]Operation{mk(3000), mk(3100)}
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		res, err := reg.ApplyBatch(doc, ops[i%2], env)
		if err != nil {
			b.Fatal(err)
		}
		doc = res.Doc
	}
}

// BenchmarkApplyAndLintFast200Items is Apply of one item.move plus the doc and item
// rules (lint.Fast) on the project of NFR-009: the core's share of NFR-001 (at most 15 ms).
func BenchmarkApplyAndLintFast200Items(b *testing.B) {
	doc, moved := benchDoc200(b)
	reg := Default()
	reg.Debug = false
	b.Cleanup(func() { reg.Debug = true })
	env := Env{NewIDs: (&seqIDs{}).next}
	ops := [2][]Operation{
		{opJSON("item.move", fmt.Sprintf(`{"item_id":%q,"start":"3000s"}`, moved))},
		{opJSON("item.move", fmt.Sprintf(`{"item_id":%q,"start":"3002s"}`, moved))},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		res, err := reg.ApplyBatch(doc, ops[i%2], env)
		if err != nil {
			b.Fatal(err)
		}
		doc = res.Doc
		_ = lint.Lint(doc, lint.Env{}, lint.Fast)
	}
}

// BenchmarkApplyAtTheLimits is one item.move on the document at every limit of SDD-001 §4.
func BenchmarkApplyAtTheLimits(b *testing.B) {
	doc := limitsDoc()
	reg := Default()
	reg.Debug = false
	b.Cleanup(func() { reg.Debug = true })
	env := Env{NewIDs: (&seqIDs{}).next}
	ops := [2][]Operation{
		{opJSON("item.move", fmt.Sprintf(`{"item_id":%q,"start":"107000f"}`, lastItem))},
		{opJSON("item.move", fmt.Sprintf(`{"item_id":%q,"start":"107900f"}`, lastItem))},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		res, err := reg.ApplyBatch(doc, ops[i%2], env)
		if err != nil {
			b.Fatal(err)
		}
		doc = res.Doc
		_ = lint.Lint(doc, lint.Env{}, lint.Fast)
	}
}
