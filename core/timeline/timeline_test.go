package timeline

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden canonical files")

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func files(t *testing.T, pattern string) []string {
	t.Helper()
	m, err := filepath.Glob(pattern)
	if err != nil || len(m) == 0 {
		t.Fatalf("no files for %s: %v", pattern, err)
	}
	return m
}

func codesOf(vs []Violation) []string {
	set := map[string]bool{}
	for _, v := range vs {
		set[v.Code] = true
	}
	out := make([]string, 0, len(set))
	for c := range set {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

func mustDecode(t *testing.T, data []byte) *Document {
	t.Helper()
	doc, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	return doc
}

// SDD-001 §8 check 2: every example of §5.1 validates with no violation.
func TestSDD001_ValidExamplesHaveNoViolation(t *testing.T) {
	for _, f := range files(t, "testdata/valid/*.json") {
		if strings.HasSuffix(f, ".canonical.json") {
			continue
		}
		if vs := Check(readFile(t, f)); len(vs) != 0 {
			t.Errorf("%s: %v", f, vs)
		}
	}
}

// SDD-001 §8 check 2: every row of §5.2 produces exactly the listed code.
func TestSDD001_InvalidExamplesProduceTheListedCode(t *testing.T) {
	for _, f := range files(t, "testdata/invalid/*.json") {
		want := strings.Fields(string(readFile(t, strings.TrimSuffix(f, ".json")+".codes.txt")))
		sort.Strings(want)
		got := codesOf(Check(readFile(t, f)))
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s: got %v, want %v (%v)", f, got, want, Check(readFile(t, f)))
		}
	}
}

// SDD-001 §8 check 3: decode and re-encode yields the same canonical bytes.
func TestSDD001_CanonicalRoundTrip(t *testing.T) {
	for _, f := range files(t, "testdata/valid/*.json") {
		if strings.HasSuffix(f, ".canonical.json") {
			continue
		}
		first := Canonical(mustDecode(t, readFile(t, f)))
		second := Canonical(mustDecode(t, first))
		if !bytes.Equal(first, second) {
			t.Errorf("%s: canonical form is not stable:\n%s\n%s", f, first, second)
		}
		if !json.Valid(first) || bytes.IndexByte(first, 10) >= 0 {
			t.Errorf("%s: canonical output is not compact JSON: %s", f, first)
		}
		golden := strings.TrimSuffix(f, ".json") + ".canonical.json"
		if *update {
			if err := os.WriteFile(golden, first, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if want := readFile(t, golden); !bytes.Equal(first, want) {
			t.Errorf("%s: canonical bytes differ from %s:\n%s\n%s", f, golden, first, want)
		}
	}
}

// SDD-001 §8 check 3: unknown fields at the top level, in a track and in an item
// (and in every other object) are kept.
func TestSDD001_UnknownFieldsPreserved(t *testing.T) {
	doc := mustDecode(t, readFile(t, "testdata/valid/unknown-fields.json"))
	var out map[string]any
	if err := json.Unmarshal(Canonical(doc), &out); err != nil {
		t.Fatal(err)
	}
	get := func(path ...string) any {
		var cur any = out
		for _, p := range path {
			if a, ok := cur.([]any); ok {
				cur = a[0]
			}
			if m, ok := cur.(map[string]any); ok {
				cur = m[p]
			}
		}
		return cur
	}
	checks := map[string]any{
		"top":      get("x_top", "a"),
		"settings": get("settings", "x_settings"),
		"asset":    get("assets", "ast_3c5v8n2w6d", "x_asset"),
		"style":    get("tracks", "trk_a1b2c3d4e5", "caption_style", "x_style"),
		"item":     get("items", "itm_m1n2p3q4r5", "x_item"),
		"keyframe": get("items", "itm_m1n2p3q4r5", "keyframes", "volume", "x_key"),
		"word":     get("items", "itm_s6t7v8w9x0", "words", "x_word"),
	}
	want := map[string]any{
		"top":      "text",
		"settings": float64(1),
		"asset":    "kept",
		"style":    float64(3),
		"item":     "kept",
		"keyframe": float64(1),
		"word":     "kept",
	}
	for k, w := range want {
		if checks[k] != w {
			t.Errorf("%s: got %v, want %v", k, checks[k], w)
		}
	}
	// Numbers inside unknown fields are written in the shortest form, keys sorted.
	if !bytes.Contains(Canonical(doc), []byte(`"x_top":{"a":"text","b":[1,2.5,{"a":true,"z":null}],"n":100}`)) {
		t.Errorf("unknown value not canonical: %s", Canonical(doc))
	}
	if !bytes.Contains(Canonical(doc), []byte(`"x_track":{"deep":[true]}`)) {
		t.Errorf("track unknown field lost: %s", Canonical(doc))
	}
}

func TestCanonicalOmitsDefaults(t *testing.T) {
	explicit := `{"schema_version":1,"id":"prj_7k2m9x4q1b","name":"n",
"settings":{"width":1080,"height":1920,"fps":{"num":30,"den":1},"background":"#000000","loudness_target":-14},
"assets":{},"tracks":{"trk_a1b2c3d4e5":{"id":"trk_a1b2c3d4e5","kind":"visual","name":"","order":"a0","hidden":false,"muted":false}},
"items":{"itm_s6t7v8w9x0":{"id":"itm_s6t7v8w9x0","type":"text","track":"trk_a1b2c3d4e5","start":0,"duration":10,"text":"x",
"scale":1,"opacity":1,"rotation":0,"fit":"cover","speed":{"num":1,"den":1},"volume":1,"contrast":1,"saturation":1,"brightness":0,
"fill":"#ffffff","stroke_color":"#000000","crop":{"left":0},"keyframes":{},"slot":{"name":"title","kind":"text","required":true,"duration_policy":"fit","filled":false}}}}`
	want := `{"assets":{},"id":"prj_7k2m9x4q1b","items":{"itm_s6t7v8w9x0":{"duration":10,"id":"itm_s6t7v8w9x0","slot":{"kind":"text","name":"title"},"start":0,"text":"x","track":"trk_a1b2c3d4e5","type":"text"}},"name":"n","schema_version":1,"settings":{},"tracks":{"trk_a1b2c3d4e5":{"id":"trk_a1b2c3d4e5","kind":"visual","order":"a0"}}}`
	if got := string(Canonical(mustDecode(t, []byte(explicit)))); got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

func TestCanonicalKeepsSetStyleKeysAndPosition(t *testing.T) {
	src := `{"schema_version":1,"id":"prj_7k2m9x4q1b","name":"n","settings":{},"assets":{},
"tracks":{"trk_a1b2c3d4e5":{"id":"trk_a1b2c3d4e5","kind":"visual","order":"a0","caption_style":{"size":64,"uppercase":false,"stroke_width":0,"color":"#ffffff","highlight":"none","position":"bottom"}}},
"items":{"itm_s6t7v8w9x0":{"id":"itm_s6t7v8w9x0","type":"shape","track":"trk_a1b2c3d4e5","start":0,"duration":10,"shape":"rect","width":1,"height":1,"x":0,"y":0,"style":{}}}}`
	got := string(Canonical(mustDecode(t, []byte(src))))
	for _, want := range []string{
		`"caption_style":{"color":"#FFFFFF","highlight":"none","position":"bottom","size":64,"stroke_width":0,"uppercase":false}`,
		`"x":0,"y":0`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in %s", want, got)
		}
	}
	if strings.Contains(got, `"style"`) {
		t.Errorf("an empty style object should be dropped: %s", got)
	}
}

func TestCanonicalNumbersAndStrings(t *testing.T) {
	doc := mustDecode(t, readFile(t, "testdata/valid/all-item-types.json"))
	got := string(Canonical(doc))
	for _, want := range []string{`"y":540.5`, `"rotation":-12.5`, `"loudness_target":-16`, `"text":"Hello\nworld"`, `"fill":"#FF0000"`, `"fps":{"den":1001,"num":30000}`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in %s", want, got)
		}
	}
	it := doc.Items["itm_p000000004"]
	it.Text = "a<b>&\u2028 \"q\" é"
	doc.Items["itm_p000000004"] = it
	if g := string(Canonical(doc)); !strings.Contains(g, `"text":"a<b>&\u2028 \"q\" é"`) {
		t.Errorf("string escaping: %s", g)
	}
	x := 1e21
	it.X, it.Rotation = &x, math.Copysign(0, -1)
	doc.Items["itm_p000000004"] = it
	if g := string(Canonical(doc)); !strings.Contains(g, `"x":1000000000000000000000`) || strings.Contains(g, "-0") {
		t.Errorf("number form: %s", g)
	}
}

func TestCanonicalNonFiniteAndNil(t *testing.T) {
	doc := &Document{SchemaVersion: 1, ID: "prj_7k2m9x4q1b", Name: "n", Settings: DefaultSettings()}
	nan := math.NaN()
	doc.Settings.LoudnessTarget = nan
	got := string(Canonical(doc))
	want := `{"assets":{},"id":"prj_7k2m9x4q1b","items":{},"name":"n","schema_version":1,"settings":{"loudness_target":null},"tracks":{}}`
	if got != want {
		t.Errorf("got %s", got)
	}
	if vs := Validate(doc); len(codesOf(vs)) != 1 || codesOf(vs)[0] != VShape {
		t.Errorf("non-finite loudness must be V_SHAPE: %v", vs)
	}
	b := Canonical(
		&Document{Extra: Extra{"f": 1.5, "n": json.Number("12345678901234567890"), "big": json.Number("1e999")}},
	)
	if !bytes.Contains(b, []byte(`"big":1e999,"f":1.5`)) || !bytes.Contains(b, []byte(`"n":12345678901234567890`)) {
		t.Errorf("extras: %s", b)
	}
}

func TestDecodeErrors(t *testing.T) {
	good := string(readFile(t, "testdata/versions/v1/launch-teaser.json"))
	cases := map[string]struct {
		src  string
		code string
		in   string
	}{
		"not json":        {`{`, VShape, "not valid JSON"},
		"trailing":        {good + ` {}`, VShape, "unexpected data"},
		"array":           {`[]`, VShape, "must be a JSON object"},
		"missing version": {`{}`, VShape, "schema_version"},
		"fractional": {
			strings.Replace(good, `"start":0`, `"start":1.5`, 1),
			VShape,
			"items.itm_m1n2p3q4r5.start",
		},
		"exponent int":   {strings.Replace(good, `"start":0`, `"start":1e1`, 1), VShape, "expected an integer"},
		"string for int": {strings.Replace(good, `"start":0`, `"start":"0"`, 1), VShape, "expected an integer"},
		"int for string": {strings.Replace(good, `"Launch teaser"`, `5`, 1), VShape, "expected a string"},
		"string for bool": {
			strings.Replace(good, `"has_audio":true`, `"has_audio":"yes"`, 1),
			VShape,
			"expected a boolean",
		},
		"string for float": {
			strings.Replace(good, `"source_in":60`, `"source_in":60,"scale":"big"`, 1),
			VShape,
			"expected a number",
		},
		"float overflow": {
			strings.Replace(good, `"source_in":60`, `"source_in":60,"scale":1e999`, 1),
			VShape,
			"out of range",
		},
		"object for array": {
			strings.Replace(good, `"tracks":{`, `"fonts":{"Z":{"files":{}}},"tracks":{`, 1),
			VShape,
			"expected an array",
		},
		"array for object": {strings.Replace(good, `"assets":{`, `"assets":[],"x":{`, 1), VShape, "expected an object"},
		"missing required": {
			strings.Replace(
				good,
				`"hash":"sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",`,
				``,
				1,
			),
			VShape,
			"required field is missing",
		},
		"newer schema": {
			strings.Replace(good, `"schema_version":1`, `"schema_version":2`, 1),
			VSchemaVersion,
			"upgrade the program",
		},
		"zero schema": {
			strings.Replace(good, `"schema_version":1`, `"schema_version":0`, 1),
			VSchemaVersion,
			"positive integer",
		},
		"float schema": {
			strings.Replace(good, `"schema_version":1`, `"schema_version":1.5`, 1),
			VSchemaVersion,
			"positive integer",
		},
	}
	for name, c := range cases {
		doc, err := Decode([]byte(c.src))
		var ve *ViolationsError
		if doc != nil || !errors.As(err, &ve) {
			t.Errorf("%s: got %v, %v", name, doc, err)
			continue
		}
		if ve.Violations[0].Code != c.code || !strings.Contains(ve.Error(), c.in) {
			t.Errorf("%s: %v, want %s containing %q", name, ve, c.code, c.in)
		}
		if got := codesOf(Check([]byte(c.src))); len(got) != 1 || got[0] != c.code {
			t.Errorf("%s: Check codes %v", name, got)
		}
	}
}

func TestMigrateFixturesOfEveryVersion(t *testing.T) {
	for _, f := range files(t, "testdata/versions/v*/*.json") {
		raw, err := DecodeRaw(readFile(t, f))
		if err != nil {
			t.Fatal(f, err)
		}
		migrated, err := Migrate(raw)
		if err != nil {
			t.Fatal(f, err)
		}
		doc, err := FromRaw(migrated)
		if err != nil {
			t.Fatal(f, err)
		}
		if doc.SchemaVersion != CurrentSchemaVersion {
			t.Errorf("%s: version %d", f, doc.SchemaVersion)
		}
		if vs := Validate(doc); len(vs) != 0 {
			t.Errorf("%s: %v", f, vs)
		}
	}
}

func TestMigrateChainWithFakeOlderVersion(t *testing.T) {
	// A pretend schema 2 renamed "name" to "title": v1 documents migrate through v1to2.
	v1to2 := func(d RawDoc) (RawDoc, error) {
		d["title"] = d["name"]
		delete(d, "name")
		return d, nil
	}
	v2to3 := func(d RawDoc) (RawDoc, error) { d["step3"] = true; return d, nil }
	steps := map[int]migration{1: v1to2, 2: v2to3}
	raw, _ := DecodeRaw(readFile(t, "testdata/versions/v1/launch-teaser.json"))
	out, err := migrateChain(raw, 3, steps)
	if err != nil {
		t.Fatal(err)
	}
	if out["title"] != "Launch teaser" || out["name"] != nil || out["step3"] != true ||
		out["schema_version"] != json.Number("3") {
		t.Errorf("chain result: %v", out)
	}

	// A document already at the current version is untouched.
	same, err := migrateChain(RawDoc{"schema_version": json.Number("3")}, 3, steps)
	if err != nil || len(same) != 1 {
		t.Errorf("current: %v %v", same, err)
	}

	// A missing step, a failing step, a newer document and a bad version.
	if _, err := migrateChain(
		RawDoc{"schema_version": json.Number("1")},
		2,
		nil,
	); err == nil ||
		!strings.Contains(err.Error(), "no migration from schema_version 1") {
		t.Errorf("missing step: %v", err)
	}
	boom := errors.New("boom")
	if _, err := migrateChain(
		RawDoc{"schema_version": json.Number("1")},
		2,
		map[int]migration{1: func(RawDoc) (RawDoc, error) { return nil, boom }},
	); !errors.Is(
		err,
		boom,
	) {
		t.Errorf("failing step: %v", err)
	}
	_, err = migrateChain(RawDoc{"schema_version": json.Number("4")}, 3, steps)
	var ve *ViolationsError
	if !errors.As(err, &ve) || ve.Violations[0].Code != VSchemaVersion || !strings.Contains(ve.Error(), "4") ||
		!strings.Contains(ve.Error(), "3") {
		t.Errorf("newer: %v", err)
	}
}

func TestNewID(t *testing.T) {
	seen := map[string]bool{}
	for range 1000 {
		id := NewID(PrefixItem)
		if !ValidID(PrefixItem, id) || len(id) != 14 || seen[id] {
			t.Fatalf("bad or repeated id %q", id)
		}
		seen[id] = true
	}
}

func TestValidID(t *testing.T) {
	good := []string{"prj_7k2m9x4q1b", "trk_zzzzzzzzzz", "itm_0000000000"}
	for _, id := range good {
		if !ValidID(id[:3], id) {
			t.Errorf("%s should be valid", id)
		}
	}
	bad := []string{
		"",
		"clip-1",
		"prj_7k2m9x4q1",
		"prj_7k2m9x4q1bb",
		"prj_7k2m9x4q1B",
		"prj_7k2m9x4q1i",
		"prj_7k2m9x4q1l",
		"prj_7k2m9x4q1o",
		"prj_7k2m9x4q1u",
		"prj-7k2m9x4q1b",
		"ast_7k2m9x4q1b",
	}
	for _, id := range bad {
		if ValidID("prj", id) {
			t.Errorf("%q should be invalid", id)
		}
	}
}

// ROADMAP P1-SCH-002 and SDD-001 §3.3.1: the four examples.
func TestOrderBetweenExamples(t *testing.T) {
	for _, c := range [][3]string{{"", "", "i"}, {"i", "", "r"}, {"", "i", "9"}, {"a0", "a1", "a0i"}} {
		if got := OrderBetween(c[0], c[1]); got != c[2] {
			t.Errorf("OrderBetween(%q, %q) = %q, want %q", c[0], c[1], got, c[2])
		}
	}
}

func TestOrderBetweenEdgeCases(t *testing.T) {
	for _, c := range [][3]string{{"z", "", "zi"}, {"a", "b", "ai"}, {"a", "ab", "a5"}, {"0z", "1", "0zi"}, {"a1z", "a2", "a1zi"}, {"", "1", "0i"}, {"a0", "a0i", "a09"}} {
		if got := OrderBetween(c[0], c[1]); got != c[2] {
			t.Errorf("OrderBetween(%q, %q) = %q, want %q", c[0], c[1], got, c[2])
		}
	}
	for _, c := range [][2]string{{"b", "a"}, {"a", "a"}, {"A", ""}, {"", "!"}, {"a", "a0"}, {"", "0"}} {
		if got := OrderBetween(c[0], c[1]); got != "" {
			t.Errorf("OrderBetween(%q, %q) = %q, want an empty result", c[0], c[1], got)
		}
	}
}

// SDD-001 §8 check 4.
func TestOrderBetweenProperty(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	randOrder := func() string {
		n := 1 + rng.Intn(4)
		b := make([]byte, n)
		for i := range b {
			b[i] = orderDigits[rng.Intn(36)]
		}
		if b[n-1] == '0' {
			b[n-1] = 'a'
		}
		return string(b)
	}
	for range 10000 {
		a, b := randOrder(), randOrder()
		if a == b {
			continue
		}
		if a > b {
			a, b = b, a
		}
		c := OrderBetween(a, b)
		if (a >= c || c >= b) || !ValidOrder(c) || strings.HasSuffix(c, "0") {
			t.Fatalf("OrderBetween(%q, %q) = %q", a, b, c)
		}
		if c := OrderBetween("", b); (c >= b) || c == "" {
			t.Fatalf("OrderBetween(\"\", %q) = %q", b, c)
		}
		if c := OrderBetween(a, ""); (c <= a) || c == "" {
			t.Fatalf("OrderBetween(%q, \"\") = %q", a, c)
		}
	}
}

func TestOrderBetweenSuccessiveInsertions(t *testing.T) {
	for name, next := range map[string]func(lo, hi string) string{
		"after the last":    func(_, hi string) string { return OrderBetween(hi, "") },
		"before the first":  func(lo, _ string) string { return OrderBetween("", lo) },
		"between the same":  OrderBetween,
		"just after the lo": OrderBetween,
	} {
		lo, hi := "a0", "a1"
		seen := map[string]bool{lo: true, hi: true}
		for i := range 1000 {
			c := next(lo, hi)
			if c == "" || !ValidOrder(c) || strings.HasSuffix(c, "0") || seen[c] {
				t.Fatalf("%s: insertion %d produced %q", name, i, c)
			}
			seen[c] = true
			switch name {
			case "after the last":
				hi = c
			case "before the first":
				lo = c
			case "between the same":
				hi = c
			case "just after the lo":
				lo = c
			}
			if lo >= hi && name != "before the first" && name != "after the last" {
				t.Fatalf("%s: lo %q >= hi %q", name, lo, hi)
			}
		}
	}
}

func TestDefaultsConstructors(t *testing.T) {
	s := DefaultSettings()
	if s.Width != 1080 || s.Height != 1920 || s.FPS != (Rational{30, 1}) || s.Background != "#000000" ||
		s.LoudnessTarget != -14 {
		t.Errorf("%+v", s)
	}
	it := NewItem("itm_s6t7v8w9x0", ItemClip, "trk_a1b2c3d4e5", 3, 9)
	if it.Scale != 1 || it.Opacity != 1 || it.Speed != (Rational{1, 1}) || it.Fit != "cover" || it.Start != 3 ||
		it.Duration != 9 {
		t.Errorf("%+v", it)
	}
	if sl := NewSlot("a", "text"); !sl.Required || sl.DurationPolicy != "fit" {
		t.Errorf("%+v", sl)
	}
	if k := NewKeyframe(2, 0.5); k.Easing != "linear" || k.T != 2 || k.Value != 0.5 {
		t.Errorf("%+v", k)
	}
	if !ItemText.IsVisual() || ItemAudio.IsVisual() {
		t.Error("IsVisual")
	}
}

// Group audit G2.1: every violation code of SDD-001 §4 has at least one invalid
// example with its codes file.
func TestEveryViolationCodeHasAnInvalidExample(t *testing.T) {
	covered := map[string]bool{}
	for _, f := range files(t, "testdata/invalid/*.codes.txt") {
		for _, c := range strings.Fields(string(readFile(t, f))) {
			covered[c] = true
		}
	}
	for _, code := range []string{
		VSchemaVersion, VShape, VID, VTrackOrder, VItemTrack, VItemRange, VOverlap, VAssetRef, VSourceRange,
		VSpeed, VFades, VTransitions, VKeyframes, VWords, VFont, VSlot, VColorRef, VLimits,
	} {
		if !covered[code] {
			t.Errorf("%s has no example in testdata/invalid", code)
		}
	}
}
