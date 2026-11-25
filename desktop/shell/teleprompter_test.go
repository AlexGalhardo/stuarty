package shell

import (
	"strings"
	"testing"
)

const projectOK = "prj_7k2m9x4q1b"

func TestParseBridgeMessageTeleprompter(t *testing.T) {
	open, ok := ParseBridgeMessage(`{"vukem":"openTeleprompter","id":11,"project":"` + projectOK + `","path":"C:/x"}`)
	if !ok || open.Op != OpOpenTeleprompter || open.ID != 11 || open.Project != projectOK || open.Path != "" {
		t.Errorf("openTeleprompter = %+v, %v", open, ok)
	}

	closed, ok := ParseBridgeMessage(`{"vukem":"closeTeleprompter","id":12,"project":"` + projectOK + `"}`)
	if !ok || closed.Op != OpCloseTeleprompter || closed.Project != "" {
		t.Errorf("closeTeleprompter = %+v, %v", closed, ok)
	}

	other, ok := ParseBridgeMessage(`{"vukem":"pickFiles","id":1,"project":"` + projectOK + `"}`)
	if !ok || other.Project != "" {
		t.Errorf("a stray project reached another operation: %+v", other)
	}
}

func TestTeleprompterURLBuildsOnlyThePageOfAProject(t *testing.T) {
	got, ok := TeleprompterURL(8123, projectOK)
	if !ok || got != "http://127.0.0.1:8123/projects/"+projectOK+"/teleprompter" {
		t.Errorf("TeleprompterURL = %q, %v", got, ok)
	}

	if !Allowed(got, 8123) {
		t.Errorf("the shell would refuse to navigate to its own teleprompter address %q", got)
	}

	if _, ok := TeleprompterURL(0, projectOK); ok {
		t.Error("an address was built with no port")
	}
}

// T-170: the page names a project and nothing else, so no value it sends can make the window open another address.
func TestSecurity_T170_TheTeleprompterBridgeOpensOnlyThePageOfAProject(t *testing.T) {
	hostile := []string{
		"",
		"prj_x",
		"prj_7k2m9x4q1",
		"prj_7k2m9x4q1bb",
		"PRJ_7K2M9X4Q1B",
		"prj_7k2m9x4q1u",
		"../prj_7k2m9x4q1b",
		"prj_7k2m9x4q1b/../../auth",
		"prj_7k2m9x4q1b?next=https://example.com",
		"prj_7k2m9x4q1b#frag",
		"prj_7k2m9x4q1b\n",
		"https://example.com/projects/prj_7k2m9x4q1b/teleprompter",
		"//example.com",
		`prj_7k2m9x4q1b"`,
		"prj_7k2m9x4q1b%2f..",
	}

	for _, project := range hostile {
		if _, ok := TeleprompterURL(8123, project); ok {
			t.Errorf("TeleprompterURL accepted %q", project)
		}

		msg := `{"vukem":"openTeleprompter","id":1,"project":` + quote(project) + `}`
		if req, ok := ParseBridgeMessage(msg); ok {
			t.Errorf("the bridge accepted the project %q: %+v", project, req)
		}
	}

	// The only address that can come out of a valid id is on the origin of the server, whatever the id says.
	got, _ := TeleprompterURL(8123, projectOK)
	if !strings.HasPrefix(got, "http://127.0.0.1:8123/projects/") || strings.Count(got, "/") != 5 {
		t.Errorf("the address %q is not the teleprompter page of one project", got)
	}

	// A message with no project, or a project in another member, opens nothing.
	for _, msg := range []string{
		`{"vukem":"openTeleprompter","id":1}`,
		`{"vukem":"openTeleprompter","id":1,"path":"` + projectOK + `"}`,
		`{"vukem":"openTeleprompter","id":1,"url":"http://127.0.0.1:8123/"}`,
	} {
		if _, ok := ParseBridgeMessage(msg); ok {
			t.Errorf("the bridge accepted %s", msg)
		}
	}
}

func quote(s string) string {
	var b strings.Builder

	b.WriteByte('"')

	for _, r := range s {
		switch r {
		case '"', '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteRune(r)
		}
	}

	b.WriteByte('"')

	return b.String()
}

func TestBridgeScriptDefinesTheTeleprompterMembers(t *testing.T) {
	for _, want := range []string{`openTeleprompter:`, `closeTeleprompter:`, `m.project=project`} {
		if !strings.Contains(BridgeScript, want) {
			t.Errorf("BridgeScript lacks %s", want)
		}
	}
}
