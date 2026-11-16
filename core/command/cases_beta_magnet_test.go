package command

// Golden cases of the track magnet (track.update magnet, TASK-0254).

import (
	"fmt"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

func init() {
	magnet := func(d *timeline.Document) { t := d.Tracks[vidTr]; t.Magnet = true; d.Tracks[vidTr] = t }
	// Three clips with gaps between them and a clip on the other visual track.
	gapped := func(mods ...func(*timeline.Document)) func() *B {
		return func() *B {
			b := base().clip(itemA, vidTr, 10, 30, 0).clip(itemB, vidTr, 60, 20, 0).clip(itemC, vidTr, 100, 10, 0)
			for _, m := range mods {
				b.with(m)
			}
			return b
		}
	}
	update := func(name, args string, b func() *B) {
		gc("track.update", name, b, op("track.update", args))
	}
	on := fmt.Sprintf(`{"track_id": %q, "magnet": true}`, vidTr)

	update("ok_magnet_on_packs_the_track", on, gapped())
	update("ok_magnet_off_keeps_the_items", fmt.Sprintf(`{"track_id": %q, "magnet": false}`, vidTr), gapped(magnet))
	update("ok_magnet_on_a_packed_track_only_sets_the_flag", on, func() *B {
		return base().clip(itemA, vidTr, 0, 30, 0).clip(itemB, vidTr, 30, 20, 0)
	})
	update("ok_magnet_on_locked_track_keeps_the_items", on, gapped(func(d *timeline.Document) {
		t := d.Tracks[vidTr]
		t.Locked = true
		d.Tracks[vidTr] = t
	}))

	gc("item.delete", "ok_magnet_closes_the_gap_of_the_deleted_item", gapped(magnet),
		op("item.delete", fmt.Sprintf(`{"item_ids": [%q]}`, itemB)))
	gc("item.move", "ok_magnet_repacks_after_a_move_to_the_front", gapped(magnet),
		op("item.move", fmt.Sprintf(`{"item_id": %q, "start": "0f"}`, itemC)))
	gc("item.trim", "ok_magnet_follows_a_trim_of_the_end", gapped(magnet),
		op("item.trim", fmt.Sprintf(`{"item_id": %q, "edge": "end", "to": "20f"}`, itemA)))
	gc("item.move", "ok_a_track_without_magnet_is_not_packed",
		func() *B { return gapped(magnet)().audio(itemE, 20, 30, 0) },
		op("item.move", fmt.Sprintf(`{"item_id": %q, "start": "50f"}`, itemE)))
}
