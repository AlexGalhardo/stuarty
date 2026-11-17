package shell

import (
	"strings"
	"testing"
)

func TestParseBridgeMessage(t *testing.T) {
	tests := []struct {
		msg  string
		want BridgeRequest
		ok   bool
	}{
		{`{"vukem":"pickFiles","id":3}`, BridgeRequest{Op: OpPickFiles, ID: 3}, true},
		{`{"vukem":"exec","id":1}`, BridgeRequest{}, false},
		{`{"vukem":"","id":1}`, BridgeRequest{}, false},
		{`{"id":1}`, BridgeRequest{}, false},
		{`wails:runtime:x`, BridgeRequest{}, false},
		{`file:drop:1:2`, BridgeRequest{}, false},
		{``, BridgeRequest{}, false},
		{`{"vukem":"pickFiles","id":"x"}`, BridgeRequest{}, false},
		{`{"vukem":"pickFiles","id":1,"pad":"` + strings.Repeat("a", 5000) + `"}`, BridgeRequest{}, false},
		{
			`{"vukem":"showInFolder","id":5,"path":"C:\\out\\a.mp4"}`,
			BridgeRequest{Op: OpShowInFolder, ID: 5, Path: `C:\out\a.mp4`},
			true,
		},
		{`{"vukem":"pickTemplate","id":4}`, BridgeRequest{Op: OpPickTemplate, ID: 4}, true},
		{`{"vukem":"showInFolder","id":5}`, BridgeRequest{}, false},
		{`{"vukem":"pickFiles","id":6,"path":"C:\\x"}`, BridgeRequest{Op: OpPickFiles, ID: 6}, true},
	}
	for _, tc := range tests {
		got, ok := ParseBridgeMessage(tc.msg)
		if ok != tc.ok || got != tc.want {
			t.Errorf("ParseBridgeMessage(%.40q) = %+v, %v; want %+v, %v", tc.msg, got, ok, tc.want, tc.ok)
		}
	}
}

func TestResolveScriptEscapesPathsAndErrors(t *testing.T) {
	s := ResolveScript(7, []string{`C:\media\a "b".mp4`, `</script>`}, "")
	for _, want := range []string{`vukemResolve(7,`, `"C:\\media\\a \"b\".mp4"`, `,"");`} {
		if !strings.Contains(s, want) {
			t.Errorf("the script lacks %s: %s", want, s)
		}
	}
	if s := ResolveScript(1, nil, "no"); !strings.Contains(s, `(1,[],"no")`) {
		t.Errorf("nil paths must be an empty list: %s", s)
	}
}

func TestBridgeScriptDefinesTheMembers(t *testing.T) {
	for _, want := range []string{`w.vukemDesktop=`, `pickFiles:`, `pickTemplate:`, `showInFolder:`, `onDropPaths:`, `onMenuAction:`, `vukemMenu`, `handlePlatformFileDrop`, `vukem-desktop-ready`} {
		if !strings.Contains(BridgeScript, want) {
			t.Errorf("BridgeScript lacks %s", want)
		}
	}
}
