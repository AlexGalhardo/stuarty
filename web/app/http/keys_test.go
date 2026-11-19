package httpapi_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/service"
)

type keyBody struct {
	Key struct {
		KeyID  string   `json:"key_id"`
		Scopes []string `json:"scopes"`
	} `json:"key"`
	Secret string `json:"secret"`
}

// makeKey creates a key from a session and returns its secret and id.
func (h *networkHarness) makeKey(t *testing.T, session, body string) keyBody {
	t.Helper()

	res := h.send(t, http.MethodPost, "/api/v1/keys", body, session, nil)
	if res.status != http.StatusCreated {
		t.Fatalf("KeyCreate(%s) = %d %s", body, res.status, res.body)
	}

	var out keyBody
	if err := json.Unmarshal(res.body, &out); err != nil {
		t.Fatal(err)
	}

	return out
}

func bearer(secret string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + secret}
}

func (h *networkHarness) asKey(t *testing.T, secret, method, path, body string) netResp {
	t.Helper()

	return h.send(t, method, path, body, "", bearer(secret))
}

// A key never creates, lists or revokes keys (SDD-007 section 15 check 12).
func TestSecurity_T33_KeyCannotCreateKeys(t *testing.T) {
	h := newNetworkHarness(t, resolved(), true)
	h.user(t, "ana@example.com", service.RoleMember)
	ana := h.session(t, "ana@example.com")

	key := h.makeKey(t, ana, `{"name":"agent","scopes":["read","edit"]}`)

	for _, c := range []struct{ method, path, body string }{
		{http.MethodPost, "/api/v1/keys", `{"name":"x","scopes":["read"]}`},
		{http.MethodGet, "/api/v1/keys", ""},
		{http.MethodDelete, "/api/v1/keys/" + key.Key.KeyID, ""},
	} {
		res := h.asKey(t, key.Secret, c.method, c.path, c.body)
		if res.status != http.StatusForbidden || res.code() != service.CodeForbidden {
			t.Errorf("a key calling %s %s = %d %s, want 403", c.method, c.path, res.status, res.body)
		}
	}
}

// A key holds a subset of its creator's current scopes: a member cannot mint
// an admin key (SDD-007 section 4.2).
func TestSecurity_T33_KeyCannotExceedOwner(t *testing.T) {
	h := newNetworkHarness(t, resolved(), true)
	h.user(t, "ana@example.com", service.RoleMember)
	ana := h.session(t, "ana@example.com")

	for _, scopes := range []string{`["read","admin"]`, `["admin"]`, `["read","edit","media","render","admin"]`} {
		res := h.send(t, http.MethodPost, "/api/v1/keys", `{"name":"x","scopes":`+scopes+`}`, ana, nil)
		if res.status != http.StatusForbidden || res.code() != service.CodeForbidden {
			t.Errorf("a member asking for %s = %d %s, want 403", scopes, res.status, res.body)
		}
	}

	if res := h.send(
		t, http.MethodPost, "/api/v1/keys", `{"name":"ok","scopes":["read","edit"]}`, ana, nil,
	); res.status != http.StatusCreated {
		t.Errorf("a member asking for read and edit = %d %s, want 201", res.status, res.body)
	}
}

// Effective scopes are the key's intersected with the owner's present role, so
// a demoted owner narrows the key at the next request.
func TestSecurity_T33_DemotedOwnerNarrowsKey(t *testing.T) {
	h := newNetworkHarness(t, resolved(), true)
	admin := h.user(t, "root@example.com", service.RoleAdmin)
	session := h.session(t, "root@example.com")

	key := h.makeKey(t, session, `{"name":"ops","scopes":["read","admin"],"expires_in_days":30}`)

	if res := h.asKey(t, key.Secret, http.MethodGet, "/api/v1/audit", ""); res.status != http.StatusOK {
		t.Fatalf("an admin key reading the audit = %d %s", res.status, res.body)
	}

	if _, err := h.st.DB().
		ExecContext(t.Context(), `UPDATE users SET role = 'member' WHERE id = ?`, admin.ID); err != nil {
		t.Fatal(err)
	}

	if res := h.asKey(t, key.Secret, http.MethodGet, "/api/v1/audit", ""); res.status != http.StatusForbidden {
		t.Errorf("the key of a demoted owner = %d, want 403", res.status)
	}

	if res := h.asKey(t, key.Secret, http.MethodGet, "/api/v1/projects", ""); res.status != http.StatusOK {
		t.Errorf("the scope that is left = %d, want 200", res.status)
	}
}

func TestKeyWithReadScopeReadsAndCannotWrite(t *testing.T) {
	h := newNetworkHarness(t, resolved(), true)
	h.user(t, "ana@example.com", service.RoleMember)
	ana := h.session(t, "ana@example.com")

	key := h.makeKey(t, ana, `{"name":"reader","scopes":["read"]}`)

	if res := h.asKey(t, key.Secret, http.MethodGet, "/api/v1/projects", ""); res.status != http.StatusOK {
		t.Errorf("GET /projects = %d %s", res.status, res.body)
	}

	res := h.asKey(t, key.Secret, http.MethodPost, "/api/v1/projects", `{"name":"x"}`)
	if res.status != http.StatusForbidden || res.code() != service.CodeForbidden {
		t.Errorf("POST /projects = %d %s, want E_FORBIDDEN", res.status, res.body)
	}

	// The actor of a key is the key.
	if !strings.HasPrefix(key.Key.KeyID, "key_") || len(key.Key.KeyID) != 14 {
		t.Errorf("key id = %q", key.Key.KeyID)
	}
}

// The secret is shown once; the listing has neither it nor its hash, and the
// stored column is its SHA-256 (SDD-007 section 4.2).
func TestKeySecretShownOnce(t *testing.T) {
	h := newNetworkHarness(t, resolved(), true)
	h.user(t, "ana@example.com", service.RoleMember)
	ana := h.session(t, "ana@example.com")

	created := h.send(t, http.MethodPost, "/api/v1/keys", `{"name":"agent","scopes":["read"]}`, ana, nil)
	if created.status != http.StatusCreated || created.header.Get("Cache-Control") != "no-store" {
		t.Fatalf("create = %d, Cache-Control %q", created.status, created.header.Get("Cache-Control"))
	}

	var body keyBody
	_ = json.Unmarshal(created.body, &body)

	if !regexp.MustCompile(`^vvk_[A-Za-z0-9-]+\.[A-Za-z0-9]+$`).MatchString(body.Secret) {
		t.Fatalf("secret = %q, want vvk_<token id>.<secret>", body.Secret)
	}

	_, secret, _ := strings.Cut(body.Secret, ".")
	sum := sha256.Sum256([]byte(secret))
	hashed := hex.EncodeToString(sum[:])

	list := h.send(t, http.MethodGet, "/api/v1/keys", "", ana, nil)
	if list.status != http.StatusOK || strings.Contains(string(list.body), secret) ||
		strings.Contains(string(list.body), hashed) || strings.Contains(string(list.body), `"secret"`) {
		t.Errorf("the listing shows the secret or its hash: %d %s", list.status, list.body)
	}

	if !strings.Contains(string(list.body), body.Key.KeyID) {
		t.Errorf("the listing lacks the key: %s", list.body)
	}

	var stored string
	if err := h.st.DB().QueryRowContext(t.Context(), `SELECT hash FROM access_tokens`).Scan(&stored); err != nil {
		t.Fatal(err)
	}

	if len(stored) != 64 || stored != hashed {
		t.Errorf("stored hash = %q, want the SHA-256 of the secret", stored)
	}
}

func TestKeyRevocationAndExpiry(t *testing.T) {
	h := newNetworkHarness(t, resolved(), true)
	h.user(t, "ana@example.com", service.RoleMember)
	ana := h.session(t, "ana@example.com")

	revoked := h.makeKey(t, ana, `{"name":"a","scopes":["read"]}`)
	expiring := h.makeKey(t, ana, `{"name":"b","scopes":["read"],"expires_in_days":1}`)

	for _, k := range []keyBody{revoked, expiring} {
		if res := h.asKey(t, k.Secret, http.MethodGet, "/api/v1/projects", ""); res.status != http.StatusOK {
			t.Fatalf("a fresh key = %d", res.status)
		}
	}

	if res := h.send(
		t,
		http.MethodDelete,
		"/api/v1/keys/"+revoked.Key.KeyID,
		"",
		ana,
		nil,
	); res.status != http.StatusOK {
		t.Fatalf("revoke = %d %s", res.status, res.body)
	}

	res := h.asKey(t, revoked.Secret, http.MethodGet, "/api/v1/projects", "")
	if res.status != http.StatusUnauthorized || res.code() != service.CodeUnauthorized {
		t.Errorf("a revoked key = %d %s, want E_UNAUTHORIZED", res.status, res.body)
	}

	// The row stays, marked revoked.
	if list := h.send(
		t,
		http.MethodGet,
		"/api/v1/keys",
		"",
		ana,
		nil,
	); !strings.Contains(
		string(list.body),
		`"revoked_at":"`,
	) {
		t.Errorf("the revoked key left the listing: %s", list.body)
	}

	h.clk.advance(24*time.Hour + time.Minute)
	// Sessions idle out at 8 hours; the expiry is checked with the key alone.
	if res := h.asKey(
		t,
		expiring.Secret,
		http.MethodGet,
		"/api/v1/projects",
		"",
	); res.status != http.StatusUnauthorized {
		t.Errorf("an expired key = %d, want 401", res.status)
	}

	for _, bad := range []string{"vvk_nope.nope", "vvk_", "x", expiring.Secret + "x"} {
		if res := h.asKey(t, bad, http.MethodGet, "/api/v1/projects", ""); res.status != http.StatusUnauthorized {
			t.Errorf("the credential %q = %d, want 401", bad, res.status)
		}
	}
}

func TestKeyValidation(t *testing.T) {
	h := newNetworkHarness(t, resolved(), true)
	h.user(t, "root@example.com", service.RoleAdmin)
	root := h.session(t, "root@example.com")

	project := h.send(t, http.MethodPost, "/api/v1/projects", `{"name":"p"}`, root, nil)
	if project.status != http.StatusCreated {
		t.Fatalf("project = %d %s", project.status, project.body)
	}

	var p struct {
		ProjectID string `json:"project_id"`
	}

	_ = json.Unmarshal(project.body, &p)

	for _, c := range []struct {
		name, body string
		want       int
	}{
		{"admin for 31 days", `{"name":"a","scopes":["admin"],"expires_in_days":31}`, http.StatusBadRequest},
		{"admin for 30 days", `{"name":"a","scopes":["admin"],"expires_in_days":30}`, http.StatusCreated},
		{"admin default is capped", `{"name":"a","scopes":["read","admin"]}`, http.StatusCreated},
		{"366 days", `{"name":"a","scopes":["read"],"expires_in_days":366}`, http.StatusBadRequest},
		{"negative days", `{"name":"a","scopes":["read"],"expires_in_days":-1}`, http.StatusBadRequest},
		{"no scope", `{"name":"a","scopes":[]}`, http.StatusBadRequest},
		{"unknown scope", `{"name":"a","scopes":["root"]}`, http.StatusBadRequest},
		{"61 characters", `{"name":"` + strings.Repeat("n", 61) + `","scopes":["read"]}`, http.StatusBadRequest},
		{"60 characters", `{"name":"` + strings.Repeat("n", 60) + `","scopes":["read"]}`, http.StatusCreated},
		{"admin with a project", `{"name":"a","scopes":["admin"],"project_id":"` + p.ProjectID + `"}`, http.StatusBadRequest},
		{"a project of mine", `{"name":"a","scopes":["read"],"project_id":"` + p.ProjectID + `"}`, http.StatusCreated},
		{"a project that is not", `{"name":"a","scopes":["read"],"project_id":"prj_0000000000"}`, http.StatusNotFound},
	} {
		if res := h.send(t, http.MethodPost, "/api/v1/keys", c.body, root, nil); res.status != c.want {
			t.Errorf("%s = %d %s, want %d", c.name, res.status, res.body, c.want)
		}

		// 10 creations an hour: stay under it.
		h.clk.advance(7 * time.Minute)
		root = h.session(t, "root@example.com")
	}
}

func TestKeyLimitsPerUser(t *testing.T) {
	h := newNetworkHarness(t, resolved(), true)
	h.user(t, "ana@example.com", service.RoleMember)
	ana := h.session(t, "ana@example.com")

	for i := range 10 {
		h.makeKey(t, ana, `{"name":"k`+string(rune('a'+i))+`","scopes":["read"]}`)
	}

	res := h.send(t, http.MethodPost, "/api/v1/keys", `{"name":"eleventh","scopes":["read"]}`, ana, nil)
	if res.status != http.StatusTooManyRequests || res.code() != service.CodeRateLimited ||
		res.header.Get("Retry-After") == "" {
		t.Errorf(
			"the 11th creation in an hour = %d %s, Retry-After %q",
			res.status,
			res.body,
			res.header.Get("Retry-After"),
		)
	}

	// 50 unrevoked keys at most: four more hours of ten.
	for range 4 {
		h.clk.advance(61 * time.Minute)
		ana = h.session(t, "ana@example.com")

		for range 10 {
			h.makeKey(t, ana, `{"name":"k","scopes":["read"]}`)
		}
	}

	h.clk.advance(61 * time.Minute)
	ana = h.session(t, "ana@example.com")

	res = h.send(t, http.MethodPost, "/api/v1/keys", `{"name":"fifty-first","scopes":["read"]}`, ana, nil)
	if res.status != http.StatusUnprocessableEntity || res.code() != "E_LIMIT_EXCEEDED" {
		t.Errorf("the 51st key = %d %s, want E_LIMIT_EXCEEDED", res.status, res.body)
	}
}

// A user revokes own keys only; an admin revokes any (SDD-007 section 4.2).
func TestKeyRevocationByOwnerOrAdmin(t *testing.T) {
	h := newNetworkHarness(t, resolved(), true)
	h.user(t, "root@example.com", service.RoleAdmin)
	h.user(t, "ana@example.com", service.RoleMember)
	h.user(t, "ben@example.com", service.RoleMember)

	root := h.session(t, "root@example.com")
	ana := h.session(t, "ana@example.com")
	ben := h.session(t, "ben@example.com")

	key := h.makeKey(t, ana, `{"name":"a","scopes":["read"]}`)

	if res := h.send(
		t,
		http.MethodDelete,
		"/api/v1/keys/"+key.Key.KeyID,
		"",
		ben,
		nil,
	); res.status != http.StatusNotFound {
		t.Errorf("another user revoking = %d, want 404", res.status)
	}

	if res := h.send(
		t,
		http.MethodGet,
		"/api/v1/keys",
		"",
		ben,
		nil,
	); strings.Contains(
		string(res.body),
		key.Key.KeyID,
	) {
		t.Error("a user lists another user's key")
	}

	if res := h.send(t, http.MethodDelete, "/api/v1/keys/"+key.Key.KeyID, "", root, nil); res.status != http.StatusOK {
		t.Errorf("an admin revoking = %d %s, want 200", res.status, res.body)
	}

	if res := h.asKey(t, key.Secret, http.MethodGet, "/api/v1/projects", ""); res.status != http.StatusUnauthorized {
		t.Errorf("the revoked key = %d, want 401", res.status)
	}
}

// A password change revokes the keys of the user.
func TestPasswordChangeRevokesKeys(t *testing.T) {
	h := newNetworkHarness(t, resolved(), true)
	h.user(t, "ana@example.com", service.RoleMember)
	ana := h.session(t, "ana@example.com")
	key := h.makeKey(t, ana, `{"name":"a","scopes":["read"]}`)

	if _, err := h.auth.ChangePassword(t.Context(), "ana@example.com", "a brand new password"); err != nil {
		t.Fatal(err)
	}

	if res := h.asKey(t, key.Secret, http.MethodGet, "/api/v1/projects", ""); res.status != http.StatusUnauthorized {
		t.Errorf("the key after a password change = %d, want 401", res.status)
	}
}

// A key limited to a project reaches that project only.
func TestKeyLimitedToAProject(t *testing.T) {
	h := newNetworkHarness(t, resolved(), true)
	h.user(t, "ana@example.com", service.RoleMember)
	ana := h.session(t, "ana@example.com")

	ids := make([]string, 2)

	for i := range ids {
		res := h.send(t, http.MethodPost, "/api/v1/projects", `{"name":"p"}`, ana, nil)

		var p struct {
			ProjectID string `json:"project_id"`
		}

		_ = json.Unmarshal(res.body, &p)
		ids[i] = p.ProjectID
	}

	key := h.makeKey(t, ana, `{"name":"a","scopes":["read","edit"],"project_id":"`+ids[0]+`"}`)

	if res := h.asKey(t, key.Secret, http.MethodGet, "/api/v1/projects/"+ids[0], ""); res.status != http.StatusOK {
		t.Errorf("its project = %d %s", res.status, res.body)
	}

	if res := h.asKey(
		t,
		key.Secret,
		http.MethodGet,
		"/api/v1/projects/"+ids[1],
		"",
	); res.status != http.StatusForbidden {
		t.Errorf("another project = %d, want 403", res.status)
	}
}

// VV-246: a key is limited to a project, never to the id of one of its timelines: that id is refused with
// the answer for an id that names nothing, and another user's project is not found either.
func TestSecurity_T06_KeyIsNotLimitedToATimelineID(t *testing.T) {
	h := newNetworkHarness(t, resolved(), true)
	h.user(t, "ana@example.com", service.RoleMember)
	h.user(t, "bob@example.com", service.RoleMember)
	ana := h.session(t, "ana@example.com")
	bob := h.session(t, "bob@example.com")

	res := h.send(t, http.MethodPost, "/api/v1/projects", `{"name":"p"}`, ana, nil)

	var p struct {
		ProjectID string `json:"project_id"`
	}

	_ = json.Unmarshal(res.body, &p)

	res = h.send(
		t, http.MethodPost, "/api/v1/projects/"+p.ProjectID+"/items",
		`{"kind":"timeline","title":"Wide","preset":"16:9"}`, ana, nil,
	)

	var added struct {
		Item struct {
			TimelineID string `json:"timeline_id"`
		} `json:"item"`
	}

	if err := json.Unmarshal(res.body, &added); err != nil || added.Item.TimelineID == "" {
		t.Fatalf("the second timeline = %d %s", res.status, res.body)
	}

	missing := h.send(
		t, http.MethodPost, "/api/v1/keys", `{"name":"a","scopes":["read"],"project_id":"prj_0000000000"}`, ana, nil,
	)

	for name, c := range map[string]struct{ session, id string }{
		"a timeline of mine":          {ana, added.Item.TimelineID},
		"the project of another user": {bob, p.ProjectID},
		"a timeline of another user":  {bob, added.Item.TimelineID},
	} {
		got := h.send(
			t,
			http.MethodPost,
			"/api/v1/keys",
			`{"name":"a","scopes":["read"],"project_id":"`+c.id+`"}`,
			c.session,
			nil,
		)
		if got.status != missing.status || got.status == http.StatusCreated {
			t.Errorf(
				"%s = %d %s, want the refusal of an id that names nothing (%d)",
				name,
				got.status,
				got.body,
				missing.status,
			)
		}
	}
}

// VV-246 in network mode: a key stored before the fix with the id of a timeline as its project reaches none of
// the production that holds the timeline, and a key of project A asking for a timeline of another user's
// project gets the answer of an id that names nothing.
func TestSecurity_T06_NetworkKeyDoesNotCrossProductionsByTimeline(t *testing.T) {
	h := newNetworkHarness(t, resolved(), true)
	h.user(t, "ana@example.com", service.RoleMember)
	h.user(t, "bob@example.com", service.RoleMember)
	ana := h.session(t, "ana@example.com")
	bob := h.session(t, "bob@example.com")

	project := func(session string) string {
		res := h.send(t, http.MethodPost, "/api/v1/projects", `{"name":"p"}`, session, nil)

		var p struct {
			ProjectID string `json:"project_id"`
		}

		if err := json.Unmarshal(res.body, &p); err != nil || p.ProjectID == "" {
			t.Fatalf("ProjectCreate = %d %s", res.status, res.body)
		}

		return p.ProjectID
	}

	timeline := func(session, projectID string) string {
		res := h.send(
			t, http.MethodPost, "/api/v1/projects/"+projectID+"/items",
			`{"kind":"timeline","title":"Wide","preset":"16:9"}`, session, nil,
		)

		var added struct {
			Item struct {
				TimelineID string `json:"timeline_id"`
			} `json:"item"`
		}

		if err := json.Unmarshal(res.body, &added); err != nil || added.Item.TimelineID == "" {
			t.Fatalf("the second timeline = %d %s", res.status, res.body)
		}

		return added.Item.TimelineID
	}

	mine := project(ana)
	myTimeline := timeline(ana, mine)
	theirs := project(bob)
	theirTimeline := timeline(bob, theirs)

	// What KeyCreate used to accept: a key limited to the id of a timeline, written as the store keeps it.
	stale := h.makeKey(t, ana, `{"name":"stale","scopes":["read","edit"],"project_id":"`+mine+`"}`)
	if _, err := h.st.DB().ExecContext(
		t.Context(), `UPDATE api_keys SET project_id = ? WHERE id = ?`, myTimeline, stale.Key.KeyID,
	); err != nil {
		t.Fatal(err)
	}

	limited := h.makeKey(t, ana, `{"name":"a","scopes":["read","edit"],"project_id":"`+mine+`"}`)

	calls := []struct{ method, path, body string }{
		{http.MethodGet, "", ""},
		{http.MethodGet, "/describe", ""},
		{http.MethodGet, "/brief", ""},
		{http.MethodPut, "/brief", `{"text":"taken"}`},
		{http.MethodPost, "/items", `{"kind":"text","title":"t","text":"x"}`},
		{http.MethodPost, "/trash", `{}`},
	}

	for _, c := range calls {
		if res := h.asKey(
			t, stale.Secret, c.method, "/api/v1/projects/"+myTimeline+c.path, c.body,
		); res.status != http.StatusForbidden {
			t.Errorf("the stale key, %s %s = %d %s, want 403", c.method, c.path, res.status, res.body)
		}

		nothing := h.asKey(t, limited.Secret, c.method, "/api/v1/projects/prj_0000000000"+c.path, c.body)
		got := h.asKey(t, limited.Secret, c.method, "/api/v1/projects/"+theirTimeline+c.path, c.body)

		if got.status != http.StatusForbidden || got.status != nothing.status {
			t.Errorf(
				"the key of A, %s %s on a timeline of another user = %d %s, want %d as for an id of nothing",
				c.method, c.path, got.status, got.body, nothing.status,
			)
		}
	}

	var state string
	if err := h.st.DB().QueryRowContext(
		t.Context(), `SELECT state FROM projects WHERE id = ?`, mine,
	).Scan(&state); err != nil || state != "active" {
		t.Errorf("the project of the stale key is %q (%v), want it active", state, err)
	}

	if res := h.send(t, http.MethodGet, "/api/v1/projects/"+theirs+"/brief", "", bob, nil); res.status != http.StatusOK ||
		strings.Contains(string(res.body), "taken") {
		t.Errorf("the brief of the other user = %d %s", res.status, res.body)
	}
}

// Concurrent creations by one user create at most the limit; the rest get the
// limit error (the count and the insert are one step).
func TestKeyCreationLimitHoldsUnderConcurrency(t *testing.T) {
	h := newNetworkHarness(t, resolved(), true)
	h.user(t, "ana@example.com", service.RoleMember)
	ana := h.session(t, "ana@example.com")

	const attempts = 40

	statuses := make([]int, attempts)

	var wg sync.WaitGroup

	for i := range attempts {
		wg.Go(func() {
			statuses[i] = h.send(t, http.MethodPost, "/api/v1/keys", `{"name":"k","scopes":["read"]}`, ana, nil).status
		})
	}

	wg.Wait()

	created, limited := 0, 0

	for _, s := range statuses {
		switch s {
		case http.StatusCreated:
			created++
		case http.StatusTooManyRequests:
			limited++
		}
	}

	if created != 10 || limited != attempts-10 {
		t.Errorf(
			"created %d, limited %d of %d concurrent creations; want 10 and %d",
			created,
			limited,
			attempts,
			attempts-10,
		)
	}
}
