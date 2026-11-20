package provider

import "testing"

func TestValidateEdit(t *testing.T) {
	t.Parallel()

	ok := EditRequest{Model: "img-1", Operation: OperationRemoveBackground}

	cases := []struct {
		name   string
		mutate func(r *EditRequest)
		limits EditLimits
		path   string
	}{
		{"valid", func(*EditRequest) {}, EditLimits{}, ""},
		{
			"valid with a documented size",
			func(r *EditRequest) { r.Size = "1024x1024" },
			EditLimits{Sizes: []string{"1024x1024"}},
			"",
		},
		{"bad model", func(r *EditRequest) { r.Model = "a b" }, EditLimits{}, "model"},
		{"no operation", func(r *EditRequest) { r.Operation = "" }, EditLimits{}, "operation"},
		{
			"generate is not an edit",
			func(r *EditRequest) { r.Operation = OperationGenerate },
			EditLimits{},
			"operation",
		},
		{
			"the model does another operation",
			func(*EditRequest) {},
			EditLimits{Operations: []string{"upscale"}},
			"operation",
		},
		{"size not a size", func(r *EditRequest) { r.Size = "big" }, EditLimits{}, "size"},
		{
			"size not documented",
			func(r *EditRequest) { r.Size = "512x512" },
			EditLimits{Sizes: []string{"1024x1024"}},
			"size",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			r := ok
			c.mutate(&r)

			f := ValidateEdit(NormalizeEdit(r), c.limits)

			switch {
			case c.path == "" && f != nil:
				t.Fatalf("want valid, got %+v", f)
			case c.path != "" && (f == nil || f.Path != c.path):
				t.Fatalf("want an error on %q, got %+v", c.path, f)
			}
		})
	}
}

func TestEditLimitsSourceLimit(t *testing.T) {
	t.Parallel()

	if got := (EditLimits{}).SourceLimit(); got != DefaultMaxSourceBytes {
		t.Fatalf("default: %d", got)
	}

	if got := (EditLimits{MaxSourceBytes: 1000}).SourceLimit(); got != 1000 {
		t.Fatalf("documented: %d", got)
	}
}

func TestClosestSize(t *testing.T) {
	t.Parallel()

	three := []string{"1024x1024", "1536x1024", "1024x1536"}

	cases := []struct {
		name          string
		width, height int
		sizes         []string
		want          string
	}{
		{"a wide banner is the wide size", 1920, 1080, three, "1536x1024"},
		{"a square is the square", 1080, 1080, three, "1024x1024"},
		{"a tall cover is the tall size", 1080, 1920, three, "1024x1536"},
		{"an exact size", 1536, 1024, three, "1536x1024"},
		{"a tie goes to the first", 1280, 1024, []string{"1024x1024", "1536x1024"}, "1024x1024"},
		{"no sizes documented", 1920, 1080, nil, DefaultImageSize},
		{"a bad box", 0, 100, three, DefaultImageSize},
		{"bad sizes are skipped", 1920, 1080, []string{"big", "1536x1024"}, "1536x1024"},
		{"only bad sizes", 1920, 1080, []string{"big"}, DefaultImageSize},
		{"a huge box does not overflow", 16384, 16384, three, "1024x1024"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			if got := ClosestSize(c.width, c.height, c.sizes); got != c.want {
				t.Fatalf("got %s, want %s", got, c.want)
			}
		})
	}
}
