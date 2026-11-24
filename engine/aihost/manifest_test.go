package aihost

import (
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/engine/transcribe"
)

func TestSecurity_T27_AIManifestPinsEveryHelper(t *testing.T) {
	m, err := DefaultManifest()
	if err != nil {
		t.Fatal(err)
	}

	entries, ok := m.Set("windows", "amd64")
	if !ok || len(entries) != 3 {
		t.Fatalf("the catalogue has no set for windows/amd64: %v, %d entries", ok, len(entries))
	}

	wantNames := []string{ToolHelper, ToolRuntime, ToolModel}
	for i, e := range entries {
		if e.Name != wantNames[i] {
			t.Errorf("entry %d is %s, want %s", i, e.Name, wantNames[i])
		}

		if !strings.HasPrefix(e.URL, "https://") {
			t.Errorf("%s: %s is not https", e.Name, e.URL)
		}

		if e.Published && (e.Bytes <= 0 || !isHex64(e.SHA256)) {
			t.Errorf("%s is published without its size and digest", e.Name)
		}
	}

	// The runtime and the model are pinned; the helper waits for a CI run to build it.
	if !entries[1].Published || !entries[2].Published || entries[0].Published {
		t.Errorf(
			"published: helper %v, runtime %v, model %v",
			entries[0].Published,
			entries[1].Published,
			entries[2].Published,
		)
	}

	if m.Dataset() == "" {
		t.Error("the model entry does not name its dataset")
	}

	if _, ok := m.Set("plan9", "mips"); ok {
		t.Error("a platform with no entry got a set")
	}
}

func TestSecurity_T27_AIManifestRefusesWhatTheGateForbids(t *testing.T) {
	base, err := DefaultManifest()
	if err != nil {
		t.Fatal(err)
	}

	model := func() Entry {
		for _, e := range base.Entries {
			if e.Name == ToolModel {
				return e
			}
		}

		t.Fatal("no model entry")

		return Entry{}
	}()

	cases := map[string]func(*Entry){
		"a licence outside the allowlist": func(e *Entry) { e.License = "CC-BY-NC-4.0" },
		"GPL":                             func(e *Entry) { e.License = "GPL-3.0-only" },
		"no licence":                      func(e *Entry) { e.License = "" },
		"no notice":                       func(e *Entry) { e.Notice = " " },
		"http":                            func(e *Entry) { e.URL = strings.Replace(e.URL, "https:", "http:", 1) },
		"a host that is not listed":       func(e *Entry) { e.URL = "https://example.com/model.onnx" },
		"userinfo":                        func(e *Entry) { e.URL = "https://user@github.com/a.onnx" },
		"a path in the file name":         func(e *Entry) { e.File = "../model.onnx" },
		"no digest":                       func(e *Entry) { e.SHA256 = "" },
		"an upper-case digest":            func(e *Entry) { e.SHA256 = strings.ToUpper(e.SHA256) },
		"no size":                         func(e *Entry) { e.Bytes = 0 },
		"another model":                   func(e *Entry) { e.Model = "tiny" },
		"no dataset":                      func(e *Entry) { e.Dataset = "" },
		"a tool outside the set":          func(e *Entry) { e.Name = "depth" },
	}

	for name, mutate := range cases {
		e := model
		mutate(&e)

		if err := e.validate(base.Hosts); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}

	if err := model.validate(base.Hosts); err != nil {
		t.Errorf("the real model entry is refused: %v", err)
	}
}

func TestManifestMembersAreChecked(t *testing.T) {
	m, err := DefaultManifest()
	if err != nil {
		t.Fatal(err)
	}

	var rt Entry

	for _, e := range m.Entries {
		if e.Name == ToolRuntime {
			rt = e
		}
	}

	bad := rt
	bad.Format = ""

	if bad.validate(m.Hosts) == nil {
		t.Error("members with no archive format were accepted")
	}

	bad = rt
	bad.Members = []transcribe.Member{{From: "x/y.dll", To: "../y.dll"}}

	if bad.validate(m.Hosts) == nil {
		t.Error("a member with a path in its name was accepted")
	}

	bad = rt
	bad.Members = []transcribe.Member{{From: "x/y.dll", To: "y.dll"}}

	if bad.validate(m.Hosts) == nil {
		t.Error("a manifest whose members do not include the program was accepted")
	}

	if _, err := ParseManifest([]byte(`{"hosts":[],"entries":[{}]}`)); err == nil {
		t.Error("an empty entry was accepted")
	}

	if _, err := ParseManifest([]byte(`not json`)); err == nil {
		t.Error("text was accepted")
	}
}
