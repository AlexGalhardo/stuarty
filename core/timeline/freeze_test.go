package timeline

import "testing"

func TestValidateFreezeFrame(t *testing.T) {
	cases := []invalidCase{
		{"freeze with a speed", func(d *Document) {
			editItem(d, clipID, func(i *Item) { i.Freeze, i.Speed = true, Rational{Num: 2, Den: 1} })
		}, VShape},
		{"freeze with reverse", func(d *Document) {
			editItem(d, clipID, func(i *Item) { i.Freeze, i.Reverse = true, true })
		}, VShape},
		{"freeze on a text item", func(d *Document) {
			editItem(d, capItemID, func(i *Item) { i.Freeze = true })
		}, VShape},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := base(t)
			c.edit(d)
			if got := codesOf(Validate(d)); len(got) == 0 || got[0] != c.want {
				t.Errorf("codes %v, want %s\n%v", got, c.want, Validate(d))
			}
		})
	}
}

func TestFreezeFrameReadsOneSourceFrame(t *testing.T) {
	it := NewItem(clipID, ItemClip, vidTrack, 0, 90)
	it.Asset, it.SourceIn, it.Freeze = assetID, 12, true
	if got := it.SourceSpan(); got != 1 {
		t.Errorf("SourceSpan = %d, want 1", got)
	}
	for _, k := range []int{0, 1, 45, 89} {
		if got := it.SourceOffset(k); got != 0 {
			t.Errorf("SourceOffset(%d) = %d, want 0", k, got)
		}
	}
	d := base(t)
	d.Items[clipID] = it
	if vs := Validate(d); len(vs) != 0 {
		t.Errorf("a freeze frame longer than its asset is valid (one frame is read), got %v", vs)
	}
}
