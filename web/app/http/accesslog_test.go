package httpapi_test

import (
	"strings"
	"testing"
)

// ADR-0026 item 2, SDD-007 section 10.1 path 3: the request line carries the
// actor and the project, and never the query of /auth/launch, the Authorization
// header or a cookie.
func TestAccessLogCarriesActorAndProjectButNoCredential(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	id := h.newProject("log")

	const (
		code   = "launchcode-canary-0123456789abcdef"
		cookie = "session-canary-fedcba9876543210"
	)

	h.do(req{path: "/auth/launch?code=" + code, noAuth: true, headers: map[string]string{"Cookie": "sid=" + cookie}})
	h.get("/api/v1/projects/" + id)
	h.get("/api/v1/projects/prj_not%0Ainjected/lint")

	logs := h.app.Logs()

	for _, secret := range []string{code, cookie, canary} {
		if strings.Contains(logs, secret) {
			t.Errorf("a credential is in the log:\n%s", logs)
		}
	}

	var project, launch string

	for line := range strings.SplitSeq(logs, "\n") {
		switch {
		case strings.Contains(line, "/api/v1/projects/"+id):
			project = line
		case strings.Contains(line, "/auth/launch"):
			launch = line
		}
	}

	if !strings.Contains(project, "actor=local:") || !strings.Contains(project, "project="+id) {
		t.Errorf("the project request line = %q", project)
	}

	if launch == "" || strings.Contains(launch, "?") {
		t.Errorf("the launch line = %q", launch)
	}

	if strings.Contains(logs, "injected") && strings.Contains(logs, "project=prj_not") {
		t.Errorf("a malformed project id reached the log:\n%s", logs)
	}
}
