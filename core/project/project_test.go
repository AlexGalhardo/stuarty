package project

import (
	"slices"
	"strings"
	"testing"
)

func TestKinds(t *testing.T) {
	t.Parallel()

	for _, k := range Kinds() {
		if !k.Allowed() || k.Reserved() || !k.Known() {
			t.Errorf("%s: allowed=%v reserved=%v known=%v", k, k.Allowed(), k.Reserved(), k.Known())
		}
	}

	for _, k := range []Kind{Image, Publication} {
		if k.Allowed() || !k.Reserved() || !k.Known() {
			t.Errorf("%s must be reserved and not allowed", k)
		}
	}

	if Kind("video").Known() || Kind("").Known() {
		t.Error("an unknown kind is known")
	}
}

func TestProductionStateTable(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		items []Item
		want  Production
	}{
		{"empty", nil, Production{}},
		{"one timeline", []Item{{Kind: Timeline}}, Production{Timelines: 1}},
		{"two timelines", []Item{{Kind: Timeline}, {Kind: Timeline}}, Production{Timelines: 2}},
		{"brief with text", []Item{{Kind: Brief, Text: "a"}}, Production{HasBrief: true}},
		{"brief blank", []Item{{Kind: Brief, Text: " \n\t"}}, Production{}},
		{"script", []Item{{Kind: Text, Text: "scene one"}}, Production{HasScript: true, Texts: 1}},
		{"script blank", []Item{{Kind: Text}}, Production{Texts: 1}},
		{"two texts", []Item{{Kind: Text, Text: "a"}, {Kind: Text}}, Production{HasScript: true, Texts: 2}},
		{"outputs", []Item{{Kind: Output}, {Kind: Output}}, Production{Outputs: 2}},
		{"trashed items count for nothing", []Item{
			{Kind: Brief, Text: "a", Trashed: true},
			{Kind: Text, Text: "a", Trashed: true},
			{Kind: Timeline, Trashed: true},
			{Kind: Output, Trashed: true},
			{Kind: Publication, Status: StatusPublished, Trashed: true},
		}, Production{}},
		{"publications", []Item{
			{Kind: Publication, Status: StatusPendingApproval},
			{Kind: Publication, Status: StatusPublished},
			{Kind: Publication, Status: StatusPublished},
			{Kind: Publication, Status: "draft"},
		}, Production{PendingApprovals: 1, Published: 2}},
		{"one brief trashed, one active", []Item{
			{Kind: Brief, Text: "old", Trashed: true}, {Kind: Brief, Text: "new"},
		}, Production{HasBrief: true}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			if got := ProductionState(c.items); got != c.want {
				t.Errorf("got %+v, want %+v", got, c.want)
			}
		})
	}
}

// Every combination of the six sorts of item, present or absent, active or trashed: the state is what an
// independent count of the active items says.
func TestProductionStateEveryCombination(t *testing.T) {
	t.Parallel()

	sorts := []Item{
		{Kind: Brief, Text: "b"},
		{Kind: Text, Text: "t"},
		{Kind: Timeline},
		{Kind: Output},
		{Kind: Publication, Status: StatusPendingApproval},
		{Kind: Publication, Status: StatusPublished},
	}

	for mask := range 3 * 3 * 3 * 3 * 3 * 3 { // each sort: absent, active, trashed
		var items []Item

		var want Production

		m := mask

		for i := range sorts {
			it := sorts[i]

			switch m % 3 {
			case 0:
				m /= 3
				continue
			case 2:
				it.Trashed = true
			}

			m /= 3

			items = append(items, it)

			if it.Trashed {
				continue
			}

			switch i {
			case 0:
				want.HasBrief = true
			case 1:
				want.HasScript = true
				want.Texts++
			case 2:
				want.Timelines++
			case 3:
				want.Outputs++
			case 4:
				want.PendingApprovals++
			case 5:
				want.Published++
			}
		}

		if got := ProductionState(items); got != want {
			t.Fatalf("mask %d: got %+v, want %+v", mask, got, want)
		}
	}
}

func TestCheckTitle(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("a", MaxTitle)

	good := []string{"Script", "Roteiro do vídeo", "a", long, "日本語"}
	for _, s := range good {
		if v := CheckTitle(s); v != nil {
			t.Errorf("%q refused: %v", s, v)
		}
	}

	bad := map[string]string{
		"":                               "has 0 characters",
		long + "a":                       "has 121 characters",
		"   ":                            "no visible character",
		"a\x00b":                         "control character",
		"a" + string(rune(0x202E)) + "b": "text direction",
		string(rune(0x200B)):             "no visible character",
		"line\nbreak":                    "control character",
		strings.Repeat("é", 121):         "has 121 characters",
	}

	for s, part := range bad {
		v := CheckTitle(s)
		if v == nil {
			t.Errorf("%q accepted", s)
			continue
		}

		if v.Code != "E_VALIDATION" || v.Path != "title" || !strings.Contains(v.Message, part) || v.Hint == "" {
			t.Errorf("%q: %+v, want a validation naming %q", s, v, part)
		}
	}
}

func TestCheckText(t *testing.T) {
	t.Parallel()

	if v := CheckText(""); v != nil {
		t.Errorf("empty text refused: %v", v)
	}

	if v := CheckText(strings.Repeat("a", MaxText)); v != nil {
		t.Errorf("text at the limit refused: %v", v)
	}

	if v := CheckText(strings.Repeat("a", MaxText+1)); v == nil || v.Code != "E_LIMIT_EXCEEDED" {
		t.Errorf("text over the limit: %v", v)
	}

	if v := CheckText("a\xffb"); v == nil || v.Code != "E_VALIDATION" {
		t.Errorf("invalid UTF-8: %v", v)
	}
}

func TestCheckAdd(t *testing.T) {
	t.Parallel()

	for _, k := range []Kind{Text, Timeline, Output} {
		if v := CheckAdd(k); v != nil {
			t.Errorf("%s refused: %v", k, v)
		}
	}

	for _, k := range []Kind{Brief, Image, Publication, "video", ""} {
		if v := CheckAdd(k); v == nil || v.Code != "E_VALIDATION" || v.Path != "kind" || v.Hint == "" {
			t.Errorf("%q: %v", k, v)
		}
	}
}

func TestCheckTrash(t *testing.T) {
	t.Parallel()

	one := []Item{{ID: "pit_a", Kind: Timeline}, {ID: "pit_b", Kind: Text}}
	if v := CheckTrash(one, "pit_a"); v == nil || v.Code != "E_INVALID_STATE" {
		t.Errorf("the last timeline was trashed: %v", v)
	}

	if v := CheckTrash(one, "pit_b"); v != nil {
		t.Errorf("a text was refused: %v", v)
	}

	two := []Item{{ID: "pit_a", Kind: Timeline}, {ID: "pit_c", Kind: Timeline}}
	if v := CheckTrash(two, "pit_a"); v != nil {
		t.Errorf("one of two timelines refused: %v", v)
	}

	oneTrashed := []Item{{ID: "pit_a", Kind: Timeline}, {ID: "pit_c", Kind: Timeline, Trashed: true}}
	if v := CheckTrash(oneTrashed, "pit_a"); v == nil {
		t.Error("the only active timeline was trashed beside a trashed one")
	}

	if v := CheckTrash(oneTrashed, "pit_c"); v != nil {
		t.Errorf("trashing an already trashed timeline was refused: %v", v)
	}

	if v := CheckTrash(one, "pit_zzzzzzzzzz"); v != nil {
		t.Errorf("an unknown id is not a rule of the core: %v", v)
	}

	withBrief := []Item{{ID: "pit_a", Kind: Timeline}, {ID: "pit_d", Kind: Brief, Text: "x"}}
	if v := CheckTrash(withBrief, "pit_d"); v == nil || v.Code != "E_INVALID_STATE" || v.Path != "item_id" {
		t.Errorf("the brief was trashed: %v", v)
	}
}

func TestCheckRoom(t *testing.T) {
	t.Parallel()

	fill := func(n int, k Kind, trashed bool) []Item {
		out := make([]Item, n)
		for i := range out {
			out[i] = Item{Kind: k, Trashed: trashed}
		}

		return out
	}

	cases := []struct {
		name  string
		items []Item
		kind  Kind
		code  string
	}{
		{"empty", nil, Text, ""},
		{"one short of the items", fill(MaxItems-1, Text, false), Text, ""},
		{"at the items", fill(MaxItems, Text, false), Text, "E_LIMIT_EXCEEDED"},
		{"trashed items do not count", append(fill(MaxItems, Text, true), fill(MaxItems-1, Text, false)...), Text, ""},
		{"one short of the timelines", fill(MaxTimelines-1, Timeline, false), Timeline, ""},
		{"at the timelines", fill(MaxTimelines, Timeline, false), Timeline, "E_LIMIT_EXCEEDED"},
		{"at the timelines, a text still fits", fill(MaxTimelines, Timeline, false), Text, ""},
		{"trashed timelines do not count", fill(MaxTimelines, Timeline, true), Timeline, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := CheckRoom(tc.items, tc.kind)

			switch {
			case tc.code == "" && v != nil:
				t.Errorf("refused: %v", v)
			case tc.code != "" && (v == nil || v.Code != tc.code):
				t.Errorf("got %v, want %s", v, tc.code)
			}
		})
	}
}

func TestPositionAndOrder(t *testing.T) {
	t.Parallel()

	if _, v := Position(3, -1); v == nil || v.Path != "position" {
		t.Errorf("negative position: %v", v)
	}

	for in, want := range map[int]int{0: 0, 2: 2, 3: 3, 9: 3} {
		if got, v := Position(3, in); v != nil || got != want {
			t.Errorf("Position(3, %d) = %d, %v; want %d", in, got, v, want)
		}
	}

	ids := []string{"a", "b", "c", "d"}

	cases := []struct {
		id   string
		pos  int
		want []string
	}{
		{"a", 0, []string{"a", "b", "c", "d"}},
		{"a", 2, []string{"b", "c", "a", "d"}},
		{"a", 3, []string{"b", "c", "d", "a"}},
		{"a", 99, []string{"b", "c", "d", "a"}},
		{"d", 0, []string{"d", "a", "b", "c"}},
		{"c", 1, []string{"a", "c", "b", "d"}},
		{"b", -5, []string{"b", "a", "c", "d"}},
	}

	for _, c := range cases {
		got, ok := Order(ids, c.id, c.pos)
		if !ok || !slices.Equal(got, c.want) {
			t.Errorf("Order(%s, %d) = %v, %v; want %v", c.id, c.pos, got, ok, c.want)
		}
	}

	if !slices.Equal(ids, []string{"a", "b", "c", "d"}) {
		t.Errorf("Order changed its argument: %v", ids)
	}

	if got, ok := Order(ids, "z", 0); ok || got != nil {
		t.Errorf("an unknown id: %v, %v", got, ok)
	}
}

func TestCheckItemID(t *testing.T) {
	t.Parallel()

	if !CheckItemID("pit_0123456789") || CheckItemID("itm_0123456789") || CheckItemID("pit_01234") || CheckItemID("") {
		t.Error("item identifier check is wrong")
	}
}
