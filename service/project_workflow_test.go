package service

import (
	"slices"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/project"
)

func TestWorkflowAddCostSumsByCurrencyAndSkipsWhatIsNotMoney(t *testing.T) {
	t.Parallel()

	var sums []project.Cost

	for _, c := range []struct {
		currency string
		micros   int64
	}{{"USD", 100}, {"EUR", 50}, {"USD", 25}, {"USD", 0}, {"", 99}, {"USD", -5}} {
		sums = addCost(sums, c.currency, c.micros)
	}

	want := []project.Cost{{Micros: 125, Currency: "USD"}, {Micros: 50, Currency: "EUR"}}
	if !slices.Equal(sums, want) {
		t.Fatalf("sums = %v, want %v", sums, want)
	}
}

func TestProjectKindTextSummaryPicksTheTextOfTheProject(t *testing.T) {
	t.Parallel()

	text := func(id, kind, body string, trashed bool) ItemRow {
		r := ItemRow{ID: id, Kind: "text", TextKind: kind, Body: body, State: ProjectActive}
		if trashed {
			r.State = ProjectTrashed
		}

		return r
	}

	rows := []ItemRow{
		{ID: "pit_brief", Kind: "brief", Body: "a brief with many words in it"},
		text("pit_caption", "caption", "caption words here", false),
		text("pit_script", "script", "script words here", false),
		text("pit_old", "post", "a trashed post with words", true),
		text("pit_copy", "", strings.Repeat("copy ", 401), false),
	}

	sum := textSummaryOf(rows)
	if sum == nil || sum.ItemID != "pit_copy" || sum.TextKind != "copy" || sum.Words != 401 || sum.ReadingMinutes != 3 {
		t.Fatalf("summary = %+v, want the copy: an empty kind is a copy, and a trashed post is not there", sum)
	}

	if got := textSummaryOf(rows[:2]); got != nil {
		t.Fatalf("a brief and a caption have no text summary: %+v", got)
	}

	if got := textSummaryOf(nil); got != nil {
		t.Fatalf("no items: %+v", got)
	}
}

func TestProjectKindOfABundle(t *testing.T) {
	t.Parallel()

	if got := bundleKind(&BundleManifest{}); got != "video" {
		t.Errorf("a bundle of version 1 holds one timeline: %q", got)
	}

	copyBundle := &BundleManifest{Production: &BundleProduction{Items: []BundleItem{
		{Kind: "brief"}, {Kind: "text", TextKind: "copy"},
	}}}

	if got := bundleKind(copyBundle); got != "text" {
		t.Errorf("a bundle of texts reads %q", got)
	}

	copyBundle.Production.Kind = "image"
	if got := bundleKind(copyBundle); got != "image" {
		t.Errorf("a stored kind wins: %q", got)
	}

	bare := &BundleManifest{Production: &BundleProduction{Items: []BundleItem{{Kind: "brief"}}}}
	if got := bundleKind(bare); got != "undecided" {
		t.Errorf("a brief only reads %q", got)
	}

	bad := &BundleProduction{Name: "A name", Kind: "audio"}
	if e := checkBundleContext(bad); e == nil {
		t.Error("a bundle that names a kind that does not exist was accepted")
	}

	bad.Kind = "text"
	if e := checkBundleContext(bad); e != nil {
		t.Errorf("a bundle that names a kind was refused: %v", e)
	}
}
