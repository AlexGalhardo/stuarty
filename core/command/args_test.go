package command

import (
	"reflect"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/timeval"
)

type nested struct {
	A int    `json:"a"`
	B string `json:"b,omitempty"`
}

type allArgs struct {
	Name   string            `json:"name"            min:"2" max:"5"  desc:"the name"`
	Kind   string            `json:"kind,omitempty"                                   enum:"x,y"`
	Count  int               `json:"count,omitempty" min:"1" max:"10"`
	Ratio  float64           `json:"ratio,omitempty" min:"0" max:"1"`
	Flag   *bool             `json:"flag,omitempty"`
	List   []int             `json:"list,omitempty"  min:"1" max:"3"`
	Inner  *nested           `json:"inner,omitempty"`
	Table  map[string]nested `json:"table,omitempty"`
	Any    any               `json:"any,omitempty"`
	Opt    *int              `json:"opt,omitempty"   min:"5"`
	Hidden string            `json:"-"`
	Note   string            `json:"note,omitempty"          max:"3"`
}

var allArgsType = reflect.TypeFor[allArgs]()

func TestDecodeArgs(t *testing.T) {
	ok := `{"name":"ab","kind":"x","count":3,"ratio":0.5,"flag":false,"list":[1,2],"inner":{"a":1},"table":{"k":{"a":2,"b":"z"}},"any":{"q":[1]},"opt":7}`
	v, err := decodeArgs([]byte(ok), allArgsType)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := reflect.TypeAssert[allArgs](v)
	if got.Name != "ab" || got.Count != 3 || got.Ratio != 0.5 || got.Flag == nil || *got.Flag || len(got.List) != 2 ||
		got.Inner.A != 1 || got.Table["k"].B != "z" || got.Any == nil || *got.Opt != 7 {
		t.Errorf("%+v", got)
	}

	bad := []struct {
		name, args, path, message string
	}{
		{"not json", `{`, "args", "not valid JSON"},
		{"trailing", `{"name":"ab"} {}`, "args", "after the JSON object"},
		{"array", `[]`, "args", "must be an object"},
		{"unknown", `{"name":"ab","zzz":1}`, "args.zzz", "not an argument"},
		{"missing", `{}`, "args.name", "required argument"},
		{"null required", `{"name":null}`, "args.name", "required argument"},
		{"string for int", `{"name":"ab","count":"3"}`, "args.count", "must be a whole number, but got a string"},
		{"fraction", `{"name":"ab","count":1.5}`, "args.count", "whole number, but got 1.5"},
		{"int for string", `{"name":5}`, "args.name", "must be a string, but got a number"},
		{"bool", `{"name":"ab","flag":"no"}`, "args.flag", "must be a boolean"},
		{"float", `{"name":"ab","ratio":"x"}`, "args.ratio", "must be a number"},
		{"float range", `{"name":"ab","ratio":1e999}`, "args.ratio", "out of range"},
		{"array type", `{"name":"ab","list":3}`, "args.list", "must be an array, but got a number"},
		{"array item", `{"name":"ab","list":[1,"x"]}`, "args.list[1]", "must be a whole number"},
		{"object type", `{"name":"ab","inner":[]}`, "args.inner", "must be an object, but got an array"},
		{"nested missing", `{"name":"ab","inner":{}}`, "args.inner.a", "required argument"},
		{"nested unknown", `{"name":"ab","inner":{"a":1,"c":2}}`, "args.inner.c", "not an argument"},
		{"map value", `{"name":"ab","table":{"k":5}}`, "args.table.k", "must be an object"},
		{"map type", `{"name":"ab","table":[]}`, "args.table", "must be an object"},
		{"enum", `{"name":"ab","kind":"z"}`, "args.kind", "not one of the accepted values"},
		{"string too short", `{"name":"a"}`, "args.name", "length of args.name is 1"},
		{"string too long", `{"name":"abcdef"}`, "args.name", "length of args.name is 6"},
		{"max only", `{"name":"ab","note":"abcd"}`, "args.note", "length of args.note is 4"},
		{"int low", `{"name":"ab","count":0}`, "args.count", "value of args.count is 0"},
		{"int high", `{"name":"ab","count":11}`, "args.count", "value of args.count is 11"},
		{"float high", `{"name":"ab","ratio":1.5}`, "args.ratio", "value of args.ratio is 1.5"},
		{"list short", `{"name":"ab","list":[]}`, "args.list", "length of args.list is 0"},
		{"pointer low", `{"name":"ab","opt":1}`, "args.opt", "value of args.opt is 1"},
	}
	for _, c := range bad {
		_, e := decodeArgs([]byte(c.args), allArgsType)
		if e == nil {
			t.Errorf("%s: accepted", c.name)
			continue
		}
		if e.Code != CodeValidation || e.Path != c.path || !strings.Contains(e.Message, c.message) {
			t.Errorf("%s: %+v", c.name, e)
		}
		if err := CheckError(e); err != nil {
			t.Errorf("%s: %v", c.name, err)
		}
	}

	// Missing, null and empty argument lists are the empty object.
	if _, e := decodeArgs(nil, reflect.TypeFor[noArgs]()); e != nil {
		t.Error(e)
	}
	if _, e := decodeArgs([]byte(" null "), reflect.TypeFor[noArgs]()); e != nil {
		t.Error(e)
	}
	if _, e := decodeArgs([]byte(`{"x":1}`), reflect.TypeFor[noArgs]()); e == nil || e.Details["allowed"] == nil {
		t.Errorf("%v", e)
	}
}

func TestSchemaOfArgs(t *testing.T) {
	s := schemaOf(allArgsType)
	if s["additionalProperties"] != false || s["type"] != "object" {
		t.Errorf("%v", s)
	}
	req, _ := s["required"].([]string)
	if len(req) != 1 || req[0] != "name" {
		t.Errorf("required %v", req)
	}
	props, _ := s["properties"].(map[string]any)
	name, _ := props["name"].(map[string]any)
	if name["minLength"] != 2.0 || name["maxLength"] != 5.0 || name["description"] != "the name" {
		t.Errorf("name %v", name)
	}
	count, _ := props["count"].(map[string]any)
	list, _ := props["list"].(map[string]any)
	if count["minimum"] != 1.0 || count["type"] != "integer" || list["maxItems"] != 3.0 || list["type"] != "array" {
		t.Errorf("count %v list %v", count, list)
	}
	kind, _ := props["kind"].(map[string]any)
	if !reflect.DeepEqual(kind["enum"], []string{"x", "y"}) {
		t.Errorf("kind %v", kind)
	}
	if tb, _ := props["table"].(map[string]any); tb["type"] != "object" {
		t.Errorf("table %v", tb)
	}
	if a, _ := props["any"].(map[string]any); len(a) != 0 {
		t.Errorf("any %v", a)
	}
	if f, _ := props["flag"].(map[string]any); f["type"] != "boolean" {
		t.Errorf("flag %v", f)
	}
	if r, _ := props["ratio"].(map[string]any); r["type"] != "number" {
		t.Errorf("ratio %v", r)
	}
}

type timeArgs struct {
	At Time `json:"at"`
}

func TestTimeArgumentsRejectNumbers(t *testing.T) {
	v, e := decodeArgs([]byte(`{"at": "12.5s"}`), reflect.TypeFor[timeArgs]())
	if e != nil || v.Field(0).String() != "12.5s" {
		t.Fatalf("%v", e)
	}
	_, e = decodeArgs([]byte(`{"at": 12}`), reflect.TypeFor[timeArgs]())
	if e == nil || e.Code != CodeInvalidTime || e.Path != "args.at" || !strings.Contains(e.Hint, "375f") ||
		CheckError(e) != nil {
		t.Errorf("%+v", e)
	}
	props, _ := schemaOf(reflect.TypeFor[timeArgs]())["properties"].(map[string]any)
	if at, _ := props["at"].(map[string]any); at["type"] != "string" {
		t.Errorf("%v", props)
	}
	if got := TimeEffective(timeval.FPS{Num: 30, Den: 1}, 30, ""); got["requested"] != nil {
		t.Errorf("a default time has no requested value: %v", got)
	}
}
