package library_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/library"
)

func TestParseFrontMatterValid(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want map[string]library.Value
	}{
		{"scalar", "name: a-b\n", map[string]library.Value{"name": {Str: "a-b"}}},
		{"quoted", "d: \"a: b, c\\n\\\"x\\\"\"\n", map[string]library.Value{"d": {Str: "a: b, c\n\"x\""}}},
		{"single", "d: 'it''s'\n", map[string]library.Value{"d": {Str: "it's"}}},
		{"plain with colon", "d: Use when: asked\n", map[string]library.Value{"d": {Str: "Use when: asked"}}},
		{"comment", "# top\nd: x # note\n", map[string]library.Value{"d": {Str: "x"}}},
		{"empty", "d:\ne: \"\"\n", map[string]library.Value{"d": {Str: ""}, "e": {Str: ""}}},
		{
			"flow list",
			"t: [a, \"b, c\", 'd']\n",
			map[string]library.Value{"t": {Kind: library.ListValue, List: []string{"a", "b, c", "d"}}},
		},
		{"empty flow list", "t: []\n", map[string]library.Value{"t": {Kind: library.ListValue}}},
		{
			"block list",
			"t:\n  - a\n  - \"b\"\n",
			map[string]library.Value{"t": {Kind: library.ListValue, List: []string{"a", "b"}}},
		},
		{"block list flush", "t:\n- a\n- b\nz: 1\n", map[string]library.Value{
			"t": {Kind: library.ListValue, List: []string{"a", "b"}}, "z": {Str: "1"},
		}},
		{"map", "metadata:\n  author: me\n  v: \"1.0\"\nz: 1\n", map[string]library.Value{
			"metadata": {Kind: library.MapValue, Map: map[string]string{"author": "me", "v": "1.0"}}, "z": {Str: "1"},
		}},
		{"literal", "d: |\n  one\n  two\n\nz: 1\n", map[string]library.Value{"d": {Str: "one\ntwo"}, "z": {Str: "1"}}},
		{"folded", "d: >-\n  one\n  two\n\n  three\n", map[string]library.Value{"d": {Str: "one two\nthree"}}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			fm, err := library.ParseFrontMatter(c.in)
			if err != nil {
				t.Fatal(err)
			}

			if got := len(fm.Keys()); got != len(c.want) {
				t.Fatalf("keys %v, want %d", fm.Keys(), len(c.want))
			}

			for k, w := range c.want {
				switch w.Kind {
				case library.ScalarValue:
					if got := fm.Scalar(k); got != w.Str {
						t.Errorf("%s = %q, want %q", k, got, w.Str)
					}
				case library.ListValue:
					if got := fm.List(k); !reflect.DeepEqual(got, w.List) && (len(got) != 0 || len(w.List) != 0) {
						t.Errorf("%s = %q, want %q", k, got, w.List)
					}
				case library.MapValue:
					if got := fm.Map(k); !reflect.DeepEqual(got, w.Map) {
						t.Errorf("%s = %v, want %v", k, got, w.Map)
					}
				}
			}
		})
	}
}

func TestParseFrontMatterRefusesHostileInput(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"anchor":          "a: &x 1\n",
		"alias":           "a: *x\n",
		"tag":             "a: !!str 1\n",
		"directive":       "a: %x\n",
		"flow map":        "a: {b: 1}\n",
		"deep map":        "m:\n  a: 1\n    b: 2\n",
		"map in list":     "l:\n  - a: 1\n",
		"duplicate key":   "a: 1\na: 2\n",
		"indent at top":   "  a: 1\n",
		"no colon":        "abc\n",
		"bad key":         "a b: 1\n",
		"no space":        "a:1\n",
		"open quote":      "a: \"x\n",
		"text after":      "a: \"x\" y\n",
		"bad escape":      "a: \"\\q\"\n",
		"open flow":       "a: [x, y\n",
		"nested flow":     "a: [[x]]\n",
		"map block value": "m:\n  a: |\n    x\n",
		"merge":           "<<: *a\n",
	}

	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := library.ParseFrontMatter(in)

			if _, ok := errors.AsType[*library.FormatError](err); !ok {
				t.Fatalf("want a FormatError, got %v", err)
			}
		})
	}
}

func TestSplitDocument(t *testing.T) {
	t.Parallel()

	front, body, line, err := library.SplitDocument("\uFEFF---\r\nname: a\r\n---\r\nbody\r\nmore\r\n")
	if err != nil || front != "name: a" || body != "body\nmore\n" || line != 4 {
		t.Fatalf("got %q %q %d %v", front, body, line, err)
	}

	for name, in := range map[string]string{
		"no fence":  "name: a\n",
		"not close": "---\nname: a\n",
		"empty":     "",
		"huge":      "---\n" + strings.Repeat("a: b\n", 3000) + "---\n",
	} {
		if _, _, _, err := library.SplitDocument(in); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestComposeThenParseIsTheIdentity(t *testing.T) {
	t.Parallel()

	fm := library.NewFrontMatter()
	fm.SetScalar("name", "a-b")
	fm.SetScalar("description", "Use when: the \"x\" is 1, then\nnewline #tag")
	fm.SetScalar("v", "true")
	fm.SetScalar("n", "12")
	fm.SetList("tags", []string{"a", "b c", "d,e"})
	fm.SetMap("metadata", map[string]string{"z": "1", "a": "x y", "v": "1.0"})

	text, err := library.Compose(fm, "body\n")
	if err != nil {
		t.Fatal(err)
	}

	back, body, err := library.ParseDocument(text)
	if err != nil {
		t.Fatalf("%v\n%s", err, text)
	}

	if body != "body\n" {
		t.Fatalf("body %q", body)
	}

	for _, k := range fm.Keys() {
		a, b := fm.Clone(), back
		if !reflect.DeepEqual(a.List(k), b.List(k)) || a.Scalar(k) != b.Scalar(k) ||
			!reflect.DeepEqual(a.Map(k), b.Map(k)) {
			t.Errorf("%s did not survive:\n%s", k, text)
		}
	}

	again, err := library.Compose(back, body)
	if err != nil || again != text {
		t.Fatalf("a second write differs:\n%s\n%s", text, again)
	}
}

func TestRenderRefusesControlCharacters(t *testing.T) {
	t.Parallel()

	fm := library.NewFrontMatter()
	fm.SetScalar("a", "x\x00y")

	if _, err := fm.Render(); err == nil {
		t.Fatal("want an error")
	}
}

func FuzzParseFrontMatter(f *testing.F) {
	f.Add("a: 1\nb: [x, y]\nm:\n  k: v\n")
	f.Add("d: |\n  x\n")
	f.Add("a: \"\\u0000\"\n")

	f.Fuzz(func(_ *testing.T, in string) {
		fm, err := library.ParseFrontMatter(in)
		if err != nil {
			return
		}

		if _, err := fm.Render(); err != nil {
			return
		}
	})
}
