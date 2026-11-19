package owner

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/lint"
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// plain hides ForDocument, so Lint measures every item on its own.
type plain struct{ m lintMeasurer }

func (p plain) Measure(doc *timeline.Document, it *timeline.Item) (lint.Rect, bool) {
	return p.m.Measure(doc, it)
}

func teaser(tb testing.TB) *timeline.Document {
	tb.Helper()

	path := filepath.Join("..", "..", "..", "core", "timeline", "testdata", "valid", "launch-teaser.json")

	data, err := os.ReadFile(path)
	if err != nil {
		tb.Fatal(err)
	}

	doc, err := timeline.Decode(data)
	if err != nil {
		tb.Fatal(err)
	}

	return doc
}

// busy is the teaser with n more captions, each on a track of its own, so the lint
// measures a document of the size the budget of NFR-001 is about.
func busy(tb testing.TB, n int) *timeline.Document {
	tb.Helper()

	doc := teaser(tb)

	var capID, track string

	for id := range doc.Items {
		if doc.Items[id].Type == timeline.ItemCaption {
			capID, track = id, doc.Items[id].Track
		}
	}

	for i := range n {
		tid := fmt.Sprintf("trk_extra%04d", i)
		t := doc.Tracks[track]
		t.Order = fmt.Sprintf("z%04d", i)
		doc.Tracks[tid] = t

		it := doc.Items[capID]
		it.ID = fmt.Sprintf("itm_extra%04d", i)
		it.Track = tid
		it.Start = i % 90
		doc.Items[it.ID] = it
	}

	return doc
}

func BenchmarkLintWithMeasurer(b *testing.B) {
	doc := busy(b, 100)
	env := lint.Env{Measurer: lintMeasurer{}}

	b.ReportAllocs()

	for b.Loop() {
		_ = lint.Lint(doc, env, lint.Fast)
	}
}

// BenchmarkLintPerItemMeasure is the cost before the change: every box is a new plan.
func BenchmarkLintPerItemMeasure(b *testing.B) {
	doc := busy(b, 100)
	env := lint.Env{Measurer: plain{lintMeasurer{}}}

	b.ReportAllocs()

	for b.Loop() {
		_ = lint.Lint(doc, env, lint.Fast)
	}
}

func TestDocumentMeasurerMatchesMeasure(t *testing.T) {
	doc := teaser(t)
	base := lintMeasurer{}

	want := lint.Lint(doc, lint.Env{Measurer: plain{base}}, lint.Fast)
	got := lint.Lint(doc, lint.Env{Measurer: base}, lint.Fast)

	if !reflect.DeepEqual(got, want) {
		t.Errorf("findings differ:\n got %v\nwant %v", got, want)
	}

	dm := base.ForDocument(doc)
	measured := 0

	for id := range doc.Items {
		it := doc.Items[id]
		wr, wok := base.Measure(doc, &it)
		gr, gok := dm.Measure(doc, &it)

		if wr != gr || wok != gok {
			t.Errorf("item %s: %v %v, want %v %v", id, gr, gok, wr, wok)
		}

		if wok {
			measured++
		}
	}

	if measured == 0 {
		t.Error("the fixture measured no item, so the comparison proves nothing")
	}
}
