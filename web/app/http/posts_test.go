package httpapi_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/writing"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/publish/destinations"
)

type postsList struct {
	Posts []struct {
		Slug  string `json:"slug"`
		Title string `json:"title"`
	} `json:"posts"`
	NextCursor string `json:"next_cursor"`
}

func (l postsList) slugs() string {
	var s []string
	for _, p := range l.Posts {
		s = append(s, p.Slug)
	}

	return strings.Join(s, ",")
}

// publishRow puts a post in the table the read API reads, as the send of a blog_api target does.
func (h *networkHarness) publishRow(t *testing.T, slug, owner, status string, at time.Time) {
	t.Helper()

	err := h.st.Blog().PutPublished(t.Context(), destinations.PublishedPost{
		Slug: slug, PostItemID: "pit_" + slug, ProjectID: "prj_0000000001", ConnectionID: "con_0000000001",
		OwnerUserID: owner, Meta: writing.PostMeta{Title: "Post " + slug, Tags: []string{"a"}, Language: "en"},
		Status: status, Markdown: "Body of " + slug, HTML: "<p>Body of " + slug + "</p>\n",
		Images: []destinations.Image{{
			Name: "cover.png", Role: "cover", AssetID: "ast_0123456789", Hash: strings.Repeat("a", 64),
			ContentType: "image/png", Bytes: 10, SHA256: strings.Repeat("a", 64),
		}},
		PublishedAt: at, UpdatedAt: at,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func decodeList(t *testing.T, res netResp) postsList {
	t.Helper()

	var l postsList
	if err := json.Unmarshal(res.body, &l); err != nil {
		t.Fatalf("%s: %v", res.body, err)
	}

	return l
}

// The read API returns only the posts published to it, and only the ones of the owner of the key.
func TestPostsAPIServesOnlyPublishedPosts(t *testing.T) {
	h := newNetworkHarness(t, resolved(), true)
	ana := h.user(t, "ana@example.com", service.RoleAdmin)
	session := h.session(t, "ana@example.com")
	key := h.makeKey(t, session, `{"name":"site","scopes":["posts:read"]}`)

	at := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	h.publishRow(t, "first", ana.ID, destinations.StatusPublished, at)
	h.publishRow(t, "second", ana.ID, destinations.StatusPublished, at.Add(time.Hour))
	h.publishRow(t, "a-draft", ana.ID, destinations.StatusDraft, at.Add(2*time.Hour))
	h.publishRow(t, "someone-elses", "usr_other", destinations.StatusPublished, at.Add(3*time.Hour))

	list := h.asKey(t, key.Secret, http.MethodGet, "/api/v1/posts", "")
	if list.status != http.StatusOK {
		t.Fatalf("list = %d %s", list.status, list.body)
	}

	if got := decodeList(t, list).slugs(); got != "second,first" {
		t.Errorf("the list is %q, want the two published posts of the owner, newest first", got)
	}

	// The list is a page: the cursor continues where it stopped.
	page := decodeList(t, h.asKey(t, key.Secret, http.MethodGet, "/api/v1/posts?limit=1", ""))
	if page.slugs() != "second" || page.NextCursor == "" {
		t.Fatalf("page 1 = %q, cursor %q", page.slugs(), page.NextCursor)
	}

	next := decodeList(t, h.asKey(t, key.Secret, http.MethodGet, "/api/v1/posts?limit=1&cursor="+page.NextCursor, ""))
	if next.slugs() != "first" || next.NextCursor != "" {
		t.Errorf("page 2 = %q, cursor %q", next.slugs(), next.NextCursor)
	}

	if res := h.asKey(
		t,
		key.Secret,
		http.MethodGet,
		"/api/v1/posts?limit=999",
		"",
	); res.status != http.StatusBadRequest {
		t.Errorf("a page of 999 = %d, want a refusal", res.status)
	}

	if res := h.asKey(
		t,
		key.Secret,
		http.MethodGet,
		"/api/v1/posts?cursor=nonsense",
		"",
	); res.status != http.StatusBadRequest {
		t.Errorf("a made-up cursor = %d, want a refusal", res.status)
	}

	// One post, with its body.
	one := h.asKey(t, key.Secret, http.MethodGet, "/api/v1/posts/first", "")
	if one.status != http.StatusOK || !strings.Contains(string(one.body), `"markdown":"Body of first"`) ||
		!strings.Contains(string(one.body), "/api/v1/posts/first/media/cover.png") {
		t.Errorf("get = %d %s", one.status, one.body)
	}

	// Everything else is the same not found: a draft, another user's post, a slug nobody published, an image the
	// post does not have.
	for _, path := range []string{
		"/api/v1/posts/a-draft", "/api/v1/posts/someone-elses", "/api/v1/posts/nothing",
		"/api/v1/posts/first/media/other.png", "/api/v1/posts/a-draft/media/cover.png",
		"/api/v1/posts/someone-elses/media/cover.png",
	} {
		if res := h.asKey(t, key.Secret, http.MethodGet, path, ""); res.status != http.StatusNotFound {
			t.Errorf("GET %s = %d %s, want 404", path, res.status, res.body)
		}
	}
}

// ETag and If-None-Match: the same page answers 304 with no body, and a change of the content changes the tag.
func TestPostsAPIETag(t *testing.T) {
	h := newNetworkHarness(t, resolved(), true)
	ana := h.user(t, "ana@example.com", service.RoleAdmin)
	key := h.makeKey(t, h.session(t, "ana@example.com"), `{"name":"site","scopes":["posts:read"]}`)

	at := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	h.publishRow(t, "first", ana.ID, destinations.StatusPublished, at)

	for _, path := range []string{"/api/v1/posts", "/api/v1/posts/first"} {
		res := h.asKey(t, key.Secret, http.MethodGet, path, "")
		tag := res.header.Get("ETag")

		if res.status != http.StatusOK || !strings.HasPrefix(tag, `"`) {
			t.Fatalf("GET %s = %d, ETag %q", path, res.status, tag)
		}

		again := h.send(t, http.MethodGet, path, "", "", map[string]string{
			"Authorization": "Bearer " + key.Secret, "If-None-Match": tag,
		})
		if again.status != http.StatusNotModified || len(again.body) != 0 {
			t.Errorf("GET %s with its ETag = %d, %d bytes", path, again.status, len(again.body))
		}

		stale := h.send(t, http.MethodGet, path, "", "", map[string]string{
			"Authorization": "Bearer " + key.Secret, "If-None-Match": `"not-it"`,
		})
		if stale.status != http.StatusOK {
			t.Errorf("GET %s with another ETag = %d", path, stale.status)
		}
	}

	// The content changes: the tag does.
	before := h.asKey(t, key.Secret, http.MethodGet, "/api/v1/posts/first", "").header.Get("ETag")

	h.publishRow(t, "first", ana.ID, destinations.StatusPublished, at.Add(time.Hour))

	h.st.Blog()
	if after := h.asKey(t, key.Secret, http.MethodGet, "/api/v1/posts/first", "").header.Get("ETag"); after == before {
		t.Error("the ETag did not change with the post")
	}
}

// T-95: a key made for the read API reaches its three routes and nothing else, and a key with another scope does not
// reach them.
func TestSecurity_T95_PostsKeyReachesOnlyPosts(t *testing.T) {
	h := newNetworkHarness(t, resolved(), true)
	ana := h.user(t, "ana@example.com", service.RoleAdmin)
	h.user(t, "bia@example.com", service.RoleMember)
	session := h.session(t, "ana@example.com")
	key := h.makeKey(t, session, `{"name":"site","scopes":["posts:read"]}`)
	reader := h.makeKey(t, session, `{"name":"reader","scopes":["read"]}`)

	// A member may make the key too: the posts are theirs.
	if res := h.send(t, http.MethodPost, "/api/v1/keys", `{"name":"site","scopes":["posts:read"]}`,
		h.session(t, "bia@example.com"), nil); res.status != http.StatusCreated {
		t.Errorf("a member's posts:read key = %d %s", res.status, res.body)
	}

	h.publishRow(t, "first", ana.ID, destinations.StatusPublished, time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC))

	for _, c := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/v1/projects", ""},
		{http.MethodGet, "/api/v1/projects/prj_0000000001", ""},
		{http.MethodGet, "/api/v1/publications", ""},
		{http.MethodPost, "/api/v1/publications", `{"project_id":"prj_0000000001","title":"x","post":"pit_a"}`},
		{http.MethodGet, "/api/v1/connections", ""},
		{http.MethodGet, "/api/v1/keys", ""},
		{http.MethodPost, "/api/v1/keys", `{"name":"x","scopes":["posts:read"]}`},
		{http.MethodGet, "/api/v1/approvals", ""},
		{http.MethodGet, "/api/v1/publications/events", ""},
		{http.MethodGet, "/api/v1/commands", ""},
		{http.MethodGet, "/api/v1/posts/../projects", ""},
		{http.MethodGet, "/api/v1/posts/%2e%2e/projects", ""},
		{http.MethodGet, "/api/v1/postsx", ""},
		{http.MethodPost, "/api/v1/posts", `{}`},
		{http.MethodDelete, "/api/v1/posts/first", ""},
		{http.MethodGet, "/", ""},
		{http.MethodGet, "/publish", ""},
		{http.MethodGet, "/mcp", ""},
	} {
		res := h.asKey(t, key.Secret, c.method, c.path, c.body)
		if res.status == http.StatusOK || res.status == http.StatusCreated || res.status == http.StatusAccepted {
			t.Errorf("the posts key on %s %s = %d %s: it must be refused", c.method, c.path, res.status, res.body)
		}
	}

	// A key with another scope is refused on the posts routes: read is not posts:read.
	for _, path := range []string{"/api/v1/posts", "/api/v1/posts/first", "/api/v1/posts/first/media/cover.png"} {
		res := h.asKey(t, reader.Secret, http.MethodGet, path, "")
		if res.status != http.StatusForbidden || res.code() != service.CodeForbidden {
			t.Errorf("a read key on %s = %d %s, want 403", path, res.status, res.body)
		}
	}

	// Its own routes still answer.
	if res := h.asKey(t, key.Secret, http.MethodGet, "/api/v1/posts", ""); res.status != http.StatusOK {
		t.Errorf("the posts key on its own list = %d %s", res.status, res.body)
	}

	// A key refuses to hold a scope its creator does not hold, as every key does.
	if res := h.send(t, http.MethodPost, "/api/v1/keys", `{"name":"x","scopes":["posts:read","admin"]}`,
		h.session(t, "bia@example.com"), nil); res.status != http.StatusForbidden {
		t.Errorf("a member's admin key = %d, want 403", res.status)
	}
}
