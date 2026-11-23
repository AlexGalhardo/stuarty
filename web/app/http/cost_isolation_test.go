package httpapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/approval"
	"github.com/AlexGalhardo/stuarty/service/remote"
)

// hostDoer sends a request of the CLI client to the test listener under the host the server is configured for.
type hostDoer struct{ c remote.Doer }

func (d hostDoer) Do(r *http.Request) (*http.Response, error) {
	r.Host = netHost

	return d.c.Do(r)
}

// VV-372, over the real surfaces: GET /cost with a session, with a key, and through the client the CLI uses in network
// mode. A member reads the spending and the limits of their own project and nothing of another member's.
func TestSecurity_T06_CostOverHTTPAndCLIIsTheOwners(t *testing.T) {
	const anaMicros, bobMicros = 1234567, 7654321

	h := newNetworkHarness(t, resolved(), true)
	h.user(t, "root@example.com", service.RoleAdmin)
	h.user(t, "ana@example.com", service.RoleMember)
	h.user(t, "bob@example.com", service.RoleMember)
	root, ana, bob := h.session(t, "root@example.com"), h.session(t, "ana@example.com"), h.session(t, "bob@example.com")

	project := func(session, name string) string {
		res := h.send(t, http.MethodPost, "/api/v1/projects", projectJSON(name), session, nil)
		if res.status != http.StatusCreated && res.status != http.StatusOK {
			t.Fatalf("create %s = %d %s", name, res.status, res.body)
		}

		var p struct {
			ProjectID string `json:"project_id"`
		}
		if err := json.Unmarshal(res.body, &p); err != nil {
			t.Fatal(err)
		}

		return p.ProjectID
	}

	anaProject, bobProject := project(ana, "Ana"), project(bob, "Bob")
	day := approval.Day(h.clk.now())

	for id, spend := range map[string]int64{anaProject: anaMicros, bobProject: bobMicros} {
		if _, err := h.st.DB().ExecContext(t.Context(), `INSERT INTO cost_ledger (id, request_id, job_id, connection_id,
 project_id, kind, currency, estimate_micros, actual_micros, state, day, created_ms, settled_ms)
 VALUES (?, 'apr_iso', ?, '', ?, 'text.generate', 'USD', ?, ?, 'settled', ?, 1, 1)`,
			"led_"+id, "job_"+id, id, spend, spend, day); err != nil {
			t.Fatal(err)
		}

		body := fmt.Sprintf(`{"project_id":%q,"currency":"USD","per_day_micros":%d}`, id, spend+1)
		if res := h.send(t, http.MethodPut, "/api/v1/cost/ceilings", body, root, nil); res.status != http.StatusOK {
			t.Fatalf("set ceiling = %d %s", res.status, res.body)
		}
	}

	all := h.send(t, http.MethodGet, "/api/v1/cost", "", root, nil)
	if !strings.Contains(string(all.body), bobProject) || !strings.Contains(string(all.body), strconv.Itoa(anaMicros)) {
		t.Fatalf("the administrator does not read every line, so the fixture proves nothing: %s", all.body)
	}

	check := func(surface, body string) {
		t.Helper()

		if !strings.Contains(body, anaProject) || !strings.Contains(body, strconv.Itoa(anaMicros)) {
			t.Errorf("%s: the member lost their own spending: %s", surface, body)
		}

		for _, leak := range []string{bobProject, strconv.Itoa(bobMicros), strconv.Itoa(bobMicros + 1), `"workspace"`} {
			if strings.Contains(body, leak) {
				t.Errorf("%s: the member reads %s of another member: %s", surface, leak, body)
			}
		}
	}

	res := h.send(t, http.MethodGet, "/api/v1/cost", "", ana, nil)
	if res.status != http.StatusOK {
		t.Fatalf("GET /cost = %d %s", res.status, res.body)
	}

	check("session", string(res.body))

	key := h.makeKey(t, ana, `{"name":"agent","scopes":["read"],"expires_in_days":30}`)

	res = h.asKey(t, key.Secret, http.MethodGet, "/api/v1/cost", "")
	if res.status != http.StatusOK {
		t.Fatalf("GET /cost with a key = %d %s", res.status, res.body)
	}

	check("key", string(res.body))

	summary, err := remote.New(h.srv.URL, key.Secret, hostDoer{h.srv.Client()}).
		CostSummary(t.Context(), service.CostSummaryRequest{})
	if err != nil {
		t.Fatalf("the client of the CLI: %v", err)
	}

	raw, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}

	check("cli client", string(raw))
}
