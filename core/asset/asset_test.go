package asset

import (
	"slices"
	"strings"
	"testing"
)

func TestOriginValidate(t *testing.T) {
	t.Parallel()

	neg := int64(-1)
	cost := int64(1500)

	cases := []struct {
		name   string
		origin Origin
		path   string // empty: valid
		reason string
	}{
		{"empty is imported", Origin{}, "", ""},
		{"imported with nothing", Origin{Kind: Imported}, "", ""},
		{"imported with a source", Origin{Kind: Imported, Source: "/home/me/clips/a.mp4"}, "", ""},
		{"unknown kind", Origin{Kind: "stolen"}, "origin.kind", "origin_kind_unknown"},
		{"generated needs a provider", Origin{Kind: Generated, Model: "m"}, "origin.provider", "provider_required"},
		{"generated needs a model", Origin{Kind: Generated, Provider: "p"}, "origin.model", "model_required"},
		{
			"generated complete",
			Origin{Kind: Generated, Provider: "p", Model: "m", Prompt: "a cat", CostMicros: &cost},
			"",
			"",
		},
		{"fetched needs a source", Origin{Kind: FetchedByAgent, RightsNote: "CC0"}, "origin.source", "source_required"},
		{
			"fetched needs a rights note",
			Origin{Kind: FetchedByAgent, Source: "https://x"},
			"origin.rights_note",
			"rights_note_required",
		},
		{"fetched complete", Origin{Kind: FetchedByAgent, Source: "https://x", RightsNote: "CC0"}, "", ""},
		{"rendered needs a source", Origin{Kind: RenderedExternally}, "origin.source", "source_required"},
		{"rendered complete", Origin{Kind: RenderedExternally, Source: "Blender"}, "", ""},
		{
			"prompt too long",
			Origin{Kind: Generated, Provider: "p", Model: "m", Prompt: strings.Repeat("é", MaxPrompt+1)},
			"origin.prompt",
			"too_long",
		},
		{
			"negative cost",
			Origin{Kind: Generated, Provider: "p", Model: "m", CostMicros: &neg},
			"origin.cost_micros",
			"cost_invalid",
		},
		{
			"blank rights note does not count",
			Origin{Kind: FetchedByAgent, Source: "u", RightsNote: "   "},
			"origin.rights_note",
			"rights_note_required",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			v := c.origin.Normalize().Validate()
			if c.path == "" {
				if v != nil {
					t.Fatalf("refused: %v", v)
				}

				return
			}

			if v == nil || v.Path != c.path || v.Reason != c.reason {
				t.Fatalf("got %+v, want %s %s", v, c.path, c.reason)
			}
		})
	}
}

func TestNormalizeTag(t *testing.T) {
	t.Parallel()

	good := map[string]string{
		"Logo":           "logo",
		"  #B-Roll ":     "b-roll",
		"reaction  clip": "reaction-clip",
		"Café":           "café",
		// "cafe\u0301" (decomposed) is among the refusals below: the caller composes it first.
		"a_b": "a_b",
	}

	for in, want := range good {
		if got, ok := NormalizeTag(in); !ok || got != want {
			t.Errorf("NormalizeTag(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}

	// A decomposed accent is refused here: the caller composes the text first (service/assets).
	for _, in := range []string{
		"", "   ", "#", "a/b", "x<script>", "a,b", strings.Repeat("a", MaxTagLength+1), "tab\there!", "cafe" + string(rune(0x0301)),
	} {
		if got, ok := NormalizeTag(in); ok {
			t.Errorf("NormalizeTag(%q) = %q, want a refusal", in, got)
		}
	}
}

func TestNormalizeAndApplyTags(t *testing.T) {
	t.Parallel()

	got, bad := NormalizeTags([]string{"B", "a", "b", "#A"})
	if bad != nil || !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("NormalizeTags = %v, %v", got, bad)
	}

	if _, bad := NormalizeTags([]string{"ok", "no way!"}); bad == nil || bad.Reason != "tags_invalid" {
		t.Fatalf("an invalid tag was accepted: %v", bad)
	}

	out, bad := ApplyTags([]string{"a", "b"}, []string{"C", "a"}, []string{"b"})
	if bad != nil || !slices.Equal(out, []string{"a", "c"}) {
		t.Fatalf("ApplyTags = %v, %v", out, bad)
	}

	var many []string
	for i := range MaxTags + 1 {
		many = append(many, "t"+strings.Repeat("x", i))
	}

	if _, bad := ApplyTags(nil, many, nil); bad == nil {
		t.Fatal("more than the maximum was accepted")
	}

	if out, bad := ApplyTags(many[:MaxTags], nil, many[:1]); bad != nil || len(out) != MaxTags-1 {
		t.Fatalf("removal: %d, %v", len(out), bad)
	}
}

func TestFilter(t *testing.T) {
	t.Parallel()

	yes := true
	logo := &Entry{
		MediaKind: KindImage, Origin: Origin{Kind: Generated}, Reusable: true, Tags: []string{"brand", "logo"},
		Projects: []string{"prj_a", "prj_a_tl"},
	}
	clip := &Entry{MediaKind: KindVideo, Origin: Origin{Kind: Imported}, Projects: []string{"prj_b"}}
	sound := &Entry{MediaKind: KindAudio, Origin: Origin{Kind: FetchedByAgent}, Tags: []string{"logo"}}

	cases := []struct {
		name string
		f    Filter
		want []*Entry
	}{
		{"none", Filter{}, []*Entry{logo, clip, sound}},
		{"kind", Filter{Kind: KindVideo}, []*Entry{clip}},
		{"visual section", Filter{Section: SectionVisual}, []*Entry{logo}},
		{"video section holds audio", Filter{Section: SectionVideo}, []*Entry{clip, sound}},
		{"tag is normalised", Filter{Tag: "#LOGO"}, []*Entry{logo, sound}},
		{"reusable and tag", Filter{Tag: "logo", Reusable: &yes}, []*Entry{logo}},
		{"project", Filter{Project: "prj_a_tl"}, []*Entry{logo}},
		{"origin", Filter{Origin: FetchedByAgent}, []*Entry{sound}},
	}

	for _, c := range cases {
		if v := c.f.Validate(); v != nil {
			t.Fatalf("%s: %v", c.name, v)
		}

		var got []*Entry

		for _, e := range []*Entry{logo, clip, sound} {
			if c.f.Match(e) {
				got = append(got, e)
			}
		}

		if !slices.Equal(got, c.want) {
			t.Errorf("%s: matched %d entries, want %d", c.name, len(got), len(c.want))
		}
	}

	for _, f := range []Filter{{Kind: "gif"}, {Section: "audio"}, {Origin: "stolen"}, {Tag: "no way!"}} {
		if f.Validate() == nil {
			t.Errorf("%+v was accepted", f)
		}
	}
}

func TestSearchQuery(t *testing.T) {
	t.Parallel()

	got, ok := SearchQuery(`  Reaction CLIP "x" OR name:* -y `)
	if !ok || got != `"reaction"* "clip"* "x"* "or"* "name"* "y"*` {
		t.Fatalf("SearchQuery = %q, %v", got, ok)
	}

	for _, in := range []string{"", "   ", `"*()-:^`} {
		if got, ok := SearchQuery(in); ok {
			t.Errorf("SearchQuery(%q) = %q, want none", in, got)
		}
	}

	long, _ := SearchQuery(strings.Repeat("w ", 20))
	if n := strings.Count(long, "*"); n != MaxTerms {
		t.Errorf("%d terms, want %d", n, MaxTerms)
	}

	// Whatever the text, the expression is made only of quoted terms: nothing of the input can be an operator.
	for _, in := range []string{`a" OR "b`, `NEAR(a b)`, `{col}: a`, "a\x00b", `a*"`} {
		got, ok := SearchQuery(in)
		if !ok {
			continue
		}

		for term := range strings.FieldsSeq(got) {
			if !strings.HasPrefix(term, `"`) || !strings.HasSuffix(term, `"*`) || strings.Count(term, `"`) != 2 {
				t.Errorf("SearchQuery(%q) = %q, term %q is not a quoted prefix", in, got, term)
			}
		}
	}
}

func TestUses(t *testing.T) {
	t.Parallel()

	got := Uses([]Holding{
		{ProjectID: "prj_b", TimelineID: "prj_b2", AssetID: "ast_2"},
		{ProjectID: "prj_a", TimelineID: "prj_a", AssetID: "ast_1"},
		{ProjectID: "prj_b", TimelineID: "prj_b1", AssetID: "ast_3"},
		{ProjectID: "prj_b", TimelineID: "prj_b1", AssetID: "ast_3"},
	})

	if len(got) != 2 || got[0].ProjectID != "prj_a" || got[1].ProjectID != "prj_b" {
		t.Fatalf("uses = %+v", got)
	}

	if !slices.Equal(got[1].Timelines, []string{"prj_b1", "prj_b2"}) || got[1].AssetID != "ast_3" {
		t.Errorf("a production with two timelines is one use with both: %+v", got[1])
	}

	if len(Uses(nil)) != 0 {
		t.Error("no holdings, no uses")
	}
}
