package httpapi_test

import (
	"encoding/json"
	"net/http"
	"regexp"
	"testing"
)

var editorBootstrapJSON = regexp.MustCompile(`(?s)<script type="application/json" id="vv-bootstrap"[^>]*>(.*?)</script>`)

type editorBoot struct {
	Session  *struct{ Privileged *bool } `json:"session"`
	Messages map[string]any              `json:"messages"`
}

func editorBootOf(t *testing.T, res resp) editorBoot {
	t.Helper()

	if res.status != http.StatusOK {
		t.Fatalf("/editor = %d\n%s", res.status, res.body)
	}

	m := editorBootstrapJSON.FindSubmatch(res.body)
	if m == nil {
		t.Fatalf("the editor page has no bootstrap\n%s", res.body)
	}

	var boot editorBoot
	if err := json.Unmarshal(m[1], &boot); err != nil {
		t.Fatalf("the bootstrap is not JSON: %v\n%s", err, m[1])
	}

	return boot
}

// The scene bar under the timeline (epic 85) needs, from the page of the editor, the privilege of the session (the
// value sharedProps gives the other pages) and the catalogue of the bar. The privilege only shows controls: a caller
// that is not a person's browser gets false, and the answer itself is still refused by the service
// (TestChatHostedAgentEditsThroughMCP).
func TestEditorBootstrapCarriesTheSessionAndTheSceneBarCatalogue(t *testing.T) {
	t.Parallel()

	h := pagesHarness(t)
	id := h.newProject("Scene bar")

	browser := editorBootOf(t, h.do(req{path: "/editor/" + id, noAuth: true, headers: map[string]string{"Cookie": cookie(h)}}))
	if browser.Session == nil || browser.Session.Privileged == nil || !*browser.Session.Privileged {
		t.Errorf("the browser of a person: session = %+v, want privileged true", browser.Session)
	}

	for _, key := range []string{
		"ui.shell.scene.allow", "ui.shell.scene.deny", "ui.shell.scene.label",
		"ui.session.person_only.local", "ui.editor.panels.handoff",
	} {
		if s, ok := browser.Messages[key].(string); !ok || s == "" {
			t.Errorf("the catalogue of the editor lacks %q", key)
		}
	}

	for _, prefix := range []string{"ui.chat.", "ui.agent_activity."} {
		found := false

		for key := range browser.Messages {
			if len(key) > len(prefix) && key[:len(prefix)] == prefix {
				found = true

				break
			}
		}

		if !found {
			t.Errorf("the catalogue of the editor has no %s message", prefix)
		}
	}

	// The local token is a caller, not a person's browser: the page draws the bar with its controls disabled.
	token := editorBootOf(t, h.do(req{path: "/editor/" + id}))
	if token.Session == nil || token.Session.Privileged == nil || *token.Session.Privileged {
		t.Errorf("the local token: session = %+v, want privileged false", token.Session)
	}
}
