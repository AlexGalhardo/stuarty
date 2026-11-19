package httpapi_test

import (
	"net/http"
	"testing"
)

// SDD-007 T-48 and T-49 over HTTP, from the browser side: a session whose launch code was asked for
// with the token alone (`stuarty open`, an agent) or with a wrong launcher secret works as the editor
// but cannot decide, write a rule or set a ceiling; the desktop app's session, whose launch proves the
// launcher secret, can.
func TestSecurity_T48_BrowserSessionWithoutTheLauncherSecretCannotDecide(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.pendingRequest("apr_0000000001", "Post the launch video")
	h.pendingRequest("apr_0000000002", "Post the second video")

	writes := func(id string) []req {
		return []req{
			{method: http.MethodPost, path: "/api/v1/approvals/" + id + "/decide", body: `{"decision":"approve"}`},
			{method: http.MethodPost, path: "/api/v1/approval-rules", body: `{"kind":"publish.post"}`},
			{method: http.MethodPut, path: "/api/v1/cost/ceilings", body: `{"currency":"USD","per_day_micros":1}`},
		}
	}

	for name, secret := range map[string]string{"no secret": "", "wrong secret": launcherSecret + "x"} {
		headers := map[string]string{
			"Cookie": h.sessionWith(t, secret), "Sec-Fetch-Site": "same-origin", "Origin": h.srv.URL,
		}

		if res := h.do(req{path: "/api/v1/approvals/apr_0000000001", noAuth: true, headers: headers}); res.status !=
			http.StatusOK {
			t.Errorf("%s: reading a request = %d %s, want 200", name, res.status, res.body)
		}

		for _, r := range writes("apr_0000000001") {
			r.noAuth, r.headers = true, headers
			if res := h.do(r); res.status != http.StatusForbidden || res.json(t)["code"] != "E_PRIVILEGED_SESSION" {
				t.Errorf("%s: %s %s = %d %s, want 403 E_PRIVILEGED_SESSION", name, r.method, r.path, res.status, res.body)
			}
		}
	}

	person := map[string]string{"Cookie": h.session(t), "Sec-Fetch-Site": "same-origin", "Origin": h.srv.URL}

	for _, r := range writes("apr_0000000002") {
		r.noAuth, r.headers = true, person
		if res := h.do(r); res.status != http.StatusOK {
			t.Errorf("the desktop session: %s %s = %d %s, want 200", r.method, r.path, res.status, res.body)
		}
	}

	if got := h.get("/api/v1/approvals/apr_0000000001").json(t)["state"]; got != "pending" {
		t.Errorf("the request a session without the secret tried to approve is %v, want pending", got)
	}

	if got := h.get("/api/v1/approvals/apr_0000000002").json(t)["state"]; got != "approved" {
		t.Errorf("the request the desktop session approved is %v, want approved", got)
	}
}
