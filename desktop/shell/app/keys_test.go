package main

import "testing"

func TestWindowKeyBindings(t *testing.T) {
	keys := windowKeyBindings(false)
	for _, k := range []string{"F11", "F5", "ctrl+F5", "ctrl+=", "ctrl+-", "ctrl+0"} {
		if keys[k] == nil {
			t.Errorf("missing binding %s", k)
		}
	}
	if keys["F12"] != nil {
		t.Error("F12 must be bound only with -devtools")
	}
	if windowKeyBindings(true)["F12"] == nil {
		t.Error("F12 missing with -devtools")
	}
}
