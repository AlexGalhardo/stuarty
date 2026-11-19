package asset

import (
	"strings"
	"testing"
)

// The members of a render by another tool (SDD-011 section 15.2): they belong to rendered_externally, each is a
// token, and a refusal names the member and the reason.
func TestOriginRenderMembers(t *testing.T) {
	t.Parallel()

	good := Origin{
		Kind: RenderedExternally, Source: "intro.html", Engine: "HyperFrames", EngineVersion: "0.4.2",
		RenderKey: "lower-third/intro:v2",
	}

	if v := good.Normalize().Validate(); v != nil {
		t.Fatalf("a good render origin: %v", v)
	}

	for name, c := range map[string]struct {
		edit   func(o *Origin)
		path   string
		reason string
	}{
		"engine on an imported file": {
			func(o *Origin) { o.Kind = Imported }, "origin.engine", "origin_member_not_allowed",
		},
		"key on a fetched file": {
			func(o *Origin) { o.Kind, o.RightsNote = FetchedByAgent, "mine" }, "origin.engine", "origin_member_not_allowed",
		},
		"engine with a space":   {func(o *Origin) { o.Engine = "Hyper Frames" }, "origin.engine", "origin_member_invalid"},
		"version with a quote":  {func(o *Origin) { o.EngineVersion = `1"` }, "origin.engine_version", "origin_member_invalid"},
		"key with a newline":    {func(o *Origin) { o.RenderKey = "a\nb" }, "origin.render_key", "origin_member_invalid"},
		"key starting with dot": {func(o *Origin) { o.RenderKey = ".hidden" }, "origin.render_key", "origin_member_invalid"},
		"key of 121 characters": {func(o *Origin) { o.RenderKey = strings.Repeat("a", 121) }, "origin.render_key", "origin_member_invalid"},
		"engine of 65":          {func(o *Origin) { o.Engine = strings.Repeat("a", 65) }, "origin.engine", "origin_member_invalid"},
	} {
		o := good
		c.edit(&o)

		v := o.Normalize().Validate()
		if v == nil || v.Path != c.path || v.Reason != c.reason {
			t.Errorf("%s: %v, want %s %s", name, v, c.path, c.reason)
		}
	}

	longest := good
	longest.RenderKey, longest.Engine = strings.Repeat("a", 120), strings.Repeat("b", 64)

	if v := longest.Validate(); v != nil {
		t.Errorf("the longest key and engine: %v", v)
	}

	// Normalize trims the three members like the other text.
	if o := (Origin{Kind: RenderedExternally, Source: "s", RenderKey: "  k  "}).Normalize(); o.RenderKey != "k" {
		t.Errorf("render key %q", o.RenderKey)
	}
}
