package command

// Golden cases of item.repeat (VV-123).

import (
	"fmt"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func init() {
	one := func(mods ...func(*timeline.Item)) func() *B {
		return func() *B { return base().clip(itemA, vidTr, 0, 90, 0, mods...) }
	}
	withNext := func() *B {
		return base().clip(itemA, vidTr, 0, 90, 0).clip(itemB, vidTr, 120, 30, 0)
	}
	fa := func(rest string) string { return fmt.Sprintf(`{"item_id": %q, %s}`, itemA, rest) }
	rp := func(name, args string, b func() *B) { gc("item.repeat", name, b, op("item.repeat", args)) }
	rpIDs := func(name, args string, b func() *B, ids ...string) {
		gcIDs("item.repeat", name, b, op("item.repeat", args), ids...)
	}

	rpIDs("ok_copies_one", fa(`"copies": 1`), one(), "itm_g9h8j7k6m5")
	rp(
		"ok_copies_three_with_ids",
		fa(`"copies": 3, "new_ids": ["itm_g9h8j7k6m5", "itm_n1p2q3r4s5", "itm_t6v7w8x9y0"]`),
		one(),
	)
	rp("ok_until_whole_copies", fa(`"until": "9s", "new_ids": ["itm_g9h8j7k6m5", "itm_n1p2q3r4s5"]`), one())
	rp("ok_until_partial_last_copy", fa(`"until": "200f", "new_ids": ["itm_g9h8j7k6m5", "itm_n1p2q3r4s5"]`), one())
	rp(
		"ok_ripple_track_moves_later_items",
		fa(`"copies": 2, "ripple": "track", "new_ids": ["itm_g9h8j7k6m5", "itm_n1p2q3r4s5"]`),
		withNext,
	)
	rp("ok_link_the_copies", fa(`"copies": 2, "link": true, "new_ids": ["itm_g9h8j7k6m5", "itm_n1p2q3r4s5"]`), one())
	rp("ok_keyframes_are_copied", fa(`"copies": 1, "new_ids": ["itm_g9h8j7k6m5"]`), one(func(it *timeline.Item) {
		it.Keyframes = timeline.Keyframes{"opacity": {kf(0, 0), kf(60, 1)}}
	}))
	rp("ok_text_item", fa(`"copies": 1, "new_ids": ["itm_g9h8j7k6m5"]`),
		func() *B { return base().text(itemA, vidTr, 0, 60) })
	rp("err_validation_neither_copies_nor_until", fmt.Sprintf(`{"item_id": %q}`, itemA), one())
	rp("err_validation_both_copies_and_until", fa(`"copies": 2, "until": "9s"`), one())
	rp("err_validation_copies_zero", fa(`"copies": 0`), one())
	rp(
		"err_validation_new_ids_more_than_copies",
		fa(`"copies": 1, "new_ids": ["itm_g9h8j7k6m5", "itm_n1p2q3r4s5"]`),
		one(),
	)
	rp("err_invalid_time_until_not_after_the_item", fa(`"until": "90f"`), one())
	rp("err_invalid_time_until_past_the_limit", fa(`"until": "108001f"`), one())
	rp("err_not_found", `{"item_id": "itm_zzzzzzzzzz", "copies": 1}`, one())
	rp("err_overlap_next_item", fa(`"copies": 2, "new_ids": ["itm_g9h8j7k6m5", "itm_n1p2q3r4s5"]`), withNext)
	rp("err_invalid_state_already_linked", fa(`"copies": 1, "link": true`), one(func(it *timeline.Item) {
		it.Link = "lnk_a1b2c3d4e5"
	}))
	rp("err_validation_link_over_20_items", fa(`"copies": 20, "link": true`), one())
	rp("err_limit_until_needs_over_200_copies", fa(`"until": "1500f"`),
		func() *B { return base().clip(itemA, vidTr, 0, 5, 0) })
	rp("err_limit_caption_words_over_document_limit", fmt.Sprintf(`{"item_id": %q, "copies": 3}`, itemA),
		func() *B {
			return base().caption(itemA, 0, 60).with(func(d *timeline.Document) {
				for i := range 80 {
					id := fmt.Sprintf("itm_k%09d", i)
					it := d.Items[itemA]
					it.ID, it.Start = id, 10000+i*600
					d.Items[id] = it
				}
			})
		})
	rp("err_id_exists", fa(`"copies": 1, "new_ids": ["`+itemB+`"]`),
		func() *B { return base().clip(itemA, vidTr, 0, 90, 0).clip(itemB, audTr, 0, 30, 0) })
}
