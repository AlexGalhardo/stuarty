package main

import (
	"bytes"
	"strings"
	"testing"
)

func leb(n int) []byte {
	var out []byte
	for {
		c := byte(n & 0x7f)
		n >>= 7
		if n != 0 {
			out = append(out, c|0x80)
			continue
		}
		return append(out, c)
	}
}

func sec(id byte, payload []byte) []byte {
	return append(append([]byte{id}, leb(len(payload))...), payload...)
}

// module builds a module with one function body, active data segments and one custom section.
func module(body []byte, segments ...string) []byte {
	m := []byte{0, 'a', 's', 'm', 1, 0, 0, 0}
	code := append(leb(1), append(leb(len(body)), body...)...)
	m = append(m, sec(10, code)...)
	data := leb(len(segments))
	for i, s := range segments {
		data = append(data, 0, 0x41)
		data = append(data, leb(i*64)...)
		data = append(data, 0x0b)
		data = append(data, leb(len(s))...)
		data = append(data, s...)
	}
	m = append(m, sec(11, data)...)
	name := "name"
	custom := append(leb(len(name)), name...)
	return append(m, sec(0, append(custom, 1, 2, 3))...)
}

func report(t *testing.T, a, b []byte) (bool, string) {
	t.Helper()
	var out bytes.Buffer
	equal, err := compare(a, b, &out)
	if err != nil {
		t.Fatal(err)
	}
	return equal, out.String()
}

func TestIdenticalModules(t *testing.T) {
	m := module([]byte{0, 0x0b}, "hello", "world")
	equal, out := report(t, m, m)
	if !equal || !strings.Contains(out, "identical") {
		t.Fatalf("equal=%v\n%s", equal, out)
	}
}

func TestDifferingDataSegmentIsNamed(t *testing.T) {
	a := module([]byte{0, 0x0b}, "src/ot/map.rs", "same")
	b := module([]byte{0, 0x0b}, "src/ot\\map.rs", "same")
	equal, out := report(t, a, b)
	if equal {
		t.Fatal("modules differ but were reported equal")
	}
	for _, want := range []string{"in A section 1 data", "1 data segments differ", "first: data segment 0", "around it", `"src/ot/map.rs"`, `"src/ot\\map.rs"`} {
		if !strings.Contains(out, want) {
			t.Errorf("report lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "function body") {
		t.Errorf("the code section is equal but was described:\n%s", out)
	}
}

func TestDifferingFunctionBodyIsNamed(t *testing.T) {
	a := module([]byte{0, 1, 0x0b}, "x")
	b := module([]byte{0, 2, 0x0b}, "x")
	_, out := report(t, a, b)
	if !strings.Contains(out, "in A section 0 code") || !strings.Contains(out, "first: function body 0, byte 1") {
		t.Fatalf("report does not name the function:\n%s", out)
	}
}

func TestReorderedSegmentsAreReported(t *testing.T) {
	a := module([]byte{0, 0x0b}, "one", "two")
	b := module([]byte{0, 0x0b}, "two", "one")
	_, out := report(t, a, b)
	if !strings.Contains(out, "same set in a different order") {
		t.Fatalf("reorder not reported:\n%s", out)
	}
}

func TestNotAModule(t *testing.T) {
	var out bytes.Buffer
	if _, err := compare([]byte("nope"), []byte("nope"), &out); err == nil {
		t.Fatal("want an error")
	}
}
