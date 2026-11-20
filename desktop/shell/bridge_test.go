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

func TestParseBridgeMessageBackground(t *testing.T) {
	get, ok := ParseBridgeMessage(`{"vukem":"getBackground","id":8,"path":"C:/x"}`)
	if !ok || get.Op != OpGetBackground || get.ID != 8 || get.Path != "" || get.Background != nil {
		t.Errorf("getBackground = %+v, %v", get, ok)
	}

	set, ok := ParseBridgeMessage(`{"vukem":"setBackground","id":9,"background":{"keep_running":true,"asked":true}}`)
	if !ok || set.Background == nil || *set.Background != (BackgroundSettings{KeepRunning: true, Asked: true}) {
		t.Errorf("setBackground = %+v, %v", set, ok)
	}

	if _, ok := ParseBridgeMessage(`{"vukem":"setBackground","id":9}`); ok {
		t.Error("setBackground without settings was accepted")
	}

	other, ok := ParseBridgeMessage(`{"vukem":"pickFiles","id":1,"background":{"keep_running":true}}`)
	if !ok || other.Background != nil {
		t.Errorf("a stray settings object reached another operation: %+v", other)
	}
}

func TestResolveValueScriptCarriesJSON(t *testing.T) {
	s := ResolveValueScript(4, BackgroundSettings{KeepRunning: true}, "")
	want := `vukemResolve(4,{"keep_running":true,"start_at_sign_in":false,"asked":false},"");`
	if !strings.Contains(s, want) {
		t.Errorf("script = %s", s)
	}

	if s := ResolveValueScript(5, nil, "no"); !strings.Contains(s, `(5,null,"no")`) {
		t.Errorf("script = %s", s)
	}

	for _, want := range []string{`getBackground:`, `setBackground:`} {
		if !strings.Contains(BridgeScript, want) {
			t.Errorf("BridgeScript lacks %s", want)
		}
	}
}
