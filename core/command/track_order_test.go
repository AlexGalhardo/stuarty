package command

import (
	"slices"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

// A visual track added without position goes above every track, in one batch or across
// batches; an audio track goes below every track (SDD-002 C.6, VV-175).
func TestTrackAddDefaultPlacementKeepsDocumentedOrder(t *testing.T) {
	d := base().d
	d.Tracks = map[string]timeline.Track{}
	n := 0
	env := Env{NewIDs: func(prefix string) string {
		n++
		return prefix + "_" + strings.Repeat("a", 8) + string(rune('a'+n/26)) + string(rune('a'+n%26))
	}}

	var added []string // ids in the order they should read, top first
	kinds := []string{"visual", "visual", "audio", "visual", "audio", "visual", "visual"}
	for i, kind := range kinds {
		res, err := Default().Apply(d, op2("track.add", `{"kind": "`+kind+`"}`), env)
		if err != nil {
			t.Fatalf("add %d: %v", i, err)
		}
		d = res.Doc
		id, _ := res.Effective["track_id"].(string)
		if kind == "visual" {
			added = slices.Insert(added, 0, id)
		} else {
			added = append(added, id)
		}

		got := make([]string, 0, len(d.Tracks))
		for tid := range d.Tracks {
			got = append(got, tid)
		}
		slices.SortFunc(got, func(a, b string) int { return strings.Compare(d.Tracks[a].Order, d.Tracks[b].Order) })
		if !slices.Equal(got, added) {
			t.Fatalf("after add %d (%s): order %v, want %v", i, kind, got, added)
		}
	}
}
