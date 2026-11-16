package command

import (
	"fmt"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

// A copy of a linked item is not part of the group, and the audio item that extract_audio
// makes and the clip it came from edit together afterwards.
func TestLinkedItemsEditTogether(t *testing.T) {
	reg := Default()
	ids := &seqIDs{}
	doc := base().clip(itemA, vidTr, 30, 90, 10).d

	res, err := reg.Apply(
		doc,
		op2("item.extract_audio", fmt.Sprintf(`{"item_id": %q, "track_id": %q, "id": %q}`, itemA, audTr, itemB)),
		Env{NewIDs: ids.next},
	)
	if err != nil {
		t.Fatal(err)
	}
	doc = res.Doc

	dup, err := reg.Apply(
		doc,
		op2("item.duplicate", fmt.Sprintf(`{"item_id": %q, "start": "500f", "new_id": %q}`, itemA, itemC)),
		Env{NewIDs: ids.next},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := dup.Doc.Items[itemC].Link; got != "" {
		t.Errorf("a copy kept the link %q", got)
	}

	moved, err := reg.Apply(
		doc,
		op2("item.move", fmt.Sprintf(`{"item_id": %q, "start": "200f"}`, itemB)),
		Env{NewIDs: ids.next},
	)
	if err != nil {
		t.Fatal(err)
	}
	if a, b := moved.Doc.Items[itemA], moved.Doc.Items[itemB]; a.Start != 200 || b.Start != 200 {
		t.Errorf("starts %d and %d after moving the audio item, want 200 for both", a.Start, b.Start)
	}

	split, err := reg.Apply(
		doc,
		op2("item.split", fmt.Sprintf(`{"item_id": %q, "at": "60f", "new_id": %q}`, itemA, itemC)),
		Env{NewIDs: ids.next},
	)
	if err != nil {
		t.Fatal(err)
	}
	if vs := timeline.Validate(split.Doc); len(vs) != 0 {
		t.Fatal(vs)
	}
	var rights []timeline.Item
	for _, it := range split.Doc.Items {
		if it.Start == 60 {
			rights = append(rights, it)
		}
	}
	if len(rights) != 2 || rights[0].Link == "" || rights[0].Link != rights[1].Link ||
		rights[0].Link == split.Doc.Items[itemA].Link {
		t.Errorf("the right parts do not form their own group: %+v", rights)
	}
}
