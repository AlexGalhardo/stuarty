package destinations

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/AlexGalhardo/stuarty/service/social"
)

const (
	wpUser = "editor"
	wpPass = "abcd efgh ijkl mnop"
)

// fakeWP is a WordPress with the endpoints of SDD-020 section 41, kept as the contract fixtures: what it answers is
// what the pages of the REST API reference describe.
type fakeWP struct {
	t   *testing.T
	srv *httptest.Server

	mu       sync.Mutex
	log      []string
	media    map[int64]map[string]string
	posts    map[int64]map[string]any
	tags     map[int64]string
	nextID   int64
	override func(w http.ResponseWriter, r *http.Request) bool
}

func wpSecret(t *testing.T, user, pass string) string {
	t.Helper()

	raw, err := json.Marshal(map[string]string{"user": user, "application_password": pass})
	if err != nil {
		t.Fatal(err)
	}

	return string(raw)
}

func newFakeWP(t *testing.T) *fakeWP {
	t.Helper()

	f := &fakeWP{
		t: t, media: map[int64]map[string]string{}, posts: map[int64]map[string]any{}, tags: map[int64]string{},
		nextID: 100,
	}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))

	t.Cleanup(f.srv.Close)

	return f
}

func (f *fakeWP) reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	raw, err := json.Marshal(v)
	if err != nil {
		f.t.Errorf("the fixture cannot be written: %v", err)
	}

	_, _ = w.Write(raw)
}

func (f *fakeWP) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.log = append(f.log, r.Method+" "+strings.TrimPrefix(r.URL.Path, "/wp-json/wp/v2"))

	if f.override != nil && f.override(w, r) {
		return
	}

	if user, pass, ok := r.BasicAuth(); !ok || user != wpUser || pass != wpPass {
		f.reply(
			w,
			401,
			map[string]any{
				"code":    "incorrect_password",
				"message": "SITE-TEXT secret words",
				"data":    map[string]int{"status": 401},
			},
		)

		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/wp-json/wp/v2")

	switch {
	case path == "/users/me" && r.Method == http.MethodGet:
		f.reply(w, 200, map[string]any{"id": 1})
	case path == "/media" && r.Method == http.MethodPost:
		data, _ := io.ReadAll(r.Body)
		f.nextID++
		_, rest, _ := strings.Cut(r.Header.Get("Content-Disposition"), `filename="`)
		name, _, _ := strings.Cut(rest, `"`)
		f.media[f.nextID] = map[string]string{
			"name": name, "type": r.Header.Get("Content-Type"), "disposition": r.Header.Get("Content-Disposition"),
			"bytes": strconv.Itoa(len(data)),
		}
		f.reply(w, 201, map[string]any{"id": f.nextID, "source_url": f.srv.URL + "/wp-content/uploads/" + name})
	case path == "/tags" && r.Method == http.MethodGet:
		out := []map[string]any{}

		for id, name := range f.tags {
			if strings.Contains(strings.ToLower(name), strings.ToLower(r.URL.Query().Get("search"))) {
				out = append(out, map[string]any{"id": id, "name": name})
			}
		}

		f.reply(w, 200, out)
	case path == "/tags" && r.Method == http.MethodPost:
		var in struct {
			Name string `json:"name"`
		}

		_ = json.NewDecoder(r.Body).Decode(&in)
		f.nextID++
		f.tags[f.nextID] = in.Name
		f.reply(w, 201, map[string]any{"id": f.nextID, "name": in.Name})
	case path == "/posts" && r.Method == http.MethodPost:
		f.nextID++
		f.savePost(w, r, f.nextID, 201)
	case strings.HasPrefix(path, "/posts/") && r.Method == http.MethodPost:
		id, _ := strconv.ParseInt(strings.TrimPrefix(path, "/posts/"), 10, 64)
		if _, ok := f.posts[id]; !ok {
			f.reply(w, 404, map[string]any{"code": "rest_post_invalid_id", "message": "Invalid post ID."})

			return
		}

		f.savePost(w, r, id, 200)
	default:
		f.reply(w, 404, map[string]any{"code": "rest_no_route"})
	}
}

func (f *fakeWP) savePost(w http.ResponseWriter, r *http.Request, id int64, status int) {
	var body map[string]any

	_ = json.NewDecoder(r.Body).Decode(&body)

	f.posts[id] = body
	f.reply(
		w,
		status,
		map[string]any{"id": id, "link": f.srv.URL + "/?p=" + strconv.FormatInt(id, 10), "status": body["status"]},
	)
}

func (f *fakeWP) calls(prefix string) int {
	n := 0

	for _, l := range f.log {
		if strings.HasPrefix(l, prefix) {
			n++
		}
	}

	return n
}

func (f *fakeWP) send(
	t *testing.T,
	p Package,
	data map[string][]byte,
	prev *Remote,
	user, pass string,
) (Result, error) {
	t.Helper()

	secret := wpSecret(t, user, pass)

	return newWordPress(testConfig()).Send(context.Background(), &SendInput{
		Cred: cred(
			f.srv.URL,
			secret,
			gateLike(),
		),
		Package:  p,
		Open:     opener(data),
		Prev:     prev,
		Delivery: "tgt_0123456789",
	})
}

func TestWordPressCreate(t *testing.T) {
	t.Parallel()

	f := newFakeWP(t)
	p, data := testPost(t)

	res, err := f.send(t, p, data, nil, wpUser, wpPass)
	if err != nil {
		t.Fatal(err)
	}

	if f.calls("POST /media") != 3 || f.calls("POST /posts") != 1 || f.calls("POST /tags") != 2 {
		t.Errorf("calls: %v", f.log)
	}

	for _, m := range f.media {
		if m["type"] != "image/png" || m["disposition"] != `attachment; filename="`+m["name"]+`"` {
			t.Errorf("media upload headers: %v", m)
		}
	}

	var post map[string]any

	for _, v := range f.posts {
		post = v
	}

	if post["title"] != "How I cut a video" || post["slug"] != "how-i-cut-a-video" || post["status"] != "publish" ||
		post["excerpt"] != "A short one." {
		t.Errorf("post = %v", post)
	}

	content, _ := post["content"].(string)
	if strings.Contains(content, `src="images/`) ||
		strings.Count(content, `src="`+f.srv.URL+"/wp-content/uploads/") != 2 {
		t.Errorf("the image paths were not rewritten to the media library: %s", content)
	}

	if tags, _ := post["tags"].([]any); len(tags) != 2 {
		t.Errorf("tags = %v", post["tags"])
	}

	coverID := int64(0)

	for id, m := range f.media {
		if m["name"] == "cover.png" {
			coverID = id
		}
	}

	if post["featured_media"] != float64(coverID) || coverID == 0 {
		t.Errorf("featured_media = %v, cover media %d", post["featured_media"], coverID)
	}

	if res.State != StatusPublished || res.RemoteID == "" ||
		!strings.HasPrefix(res.URL, f.srv.URL+"/?p="+res.RemoteID) {
		t.Errorf("result = %+v", res)
	}
}

// Publishing the same post twice updates the first post: one post at the site, no second one.
func TestWordPressSecondSendUpdates(t *testing.T) {
	t.Parallel()

	f := newFakeWP(t)
	p, data := testPost(t)

	first, err := f.send(t, p, data, nil, wpUser, wpPass)
	if err != nil {
		t.Fatal(err)
	}

	p.Meta.Title = "A better title"
	p.Status = StatusDraft

	second, err := f.send(
		t,
		p,
		data,
		&Remote{ConnectionID: "con_0123456789", PostItemID: "pit_0123456789", RemoteID: first.RemoteID},
		wpUser,
		wpPass,
	)
	if err != nil {
		t.Fatal(err)
	}

	if second.RemoteID != first.RemoteID || len(f.posts) != 1 {
		t.Errorf("ids %s and %s, %d posts at the site", first.RemoteID, second.RemoteID, len(f.posts))
	}

	if f.calls("POST /posts/"+first.RemoteID) != 1 || f.calls("POST /posts") != 2 {
		t.Errorf("calls: %v", f.log)
	}

	if f.calls("POST /tags") != 2 {
		t.Errorf("an existing tag was made again: %v", f.log)
	}

	for _, v := range f.posts {
		if v["title"] != "A better title" || v["status"] != "draft" {
			t.Errorf("the post was not updated: %v", v)
		}
	}
}

func TestWordPressPostDeletedAtTheSite(t *testing.T) {
	t.Parallel()

	f := newFakeWP(t)
	p, data := testPost(t)

	res, err := f.send(t, p, data, &Remote{RemoteID: "999"}, wpUser, wpPass)
	if err != nil {
		t.Fatal(err)
	}

	if len(f.posts) != 1 || res.RemoteID == "999" || f.calls("POST /posts/999") != 1 {
		t.Errorf("the post was not made again: %v %+v", f.log, res)
	}
}

func TestWordPressErrors(t *testing.T) {
	t.Parallel()

	p, data := testPost(t)

	classify := func(err error) string {
		se, ok := social.AsError(err)
		if !ok {
			return "not social: " + errString(err)
		}

		if strings.Contains(err.Error(), "SITE-TEXT") || strings.Contains(errString(se.Err), "SITE-TEXT") {
			return "the text of the site is in the error"
		}

		return se.Kind + "/" + se.Reason
	}

	t.Run("wrong password", func(t *testing.T) {
		t.Parallel()

		f := newFakeWP(t)
		_, err := f.send(t, p, data, nil, wpUser, "wrong password here")

		if got := classify(err); got != "auth/incorrect_password" {
			t.Errorf("got %s", got)
		}

		if f.calls("POST /posts") != 0 {
			t.Error("a post was sent with a refused password")
		}
	})

	cases := map[string]struct {
		handle func(http.ResponseWriter, *http.Request) bool
		want   string
	}{
		"server error": {
			func(w http.ResponseWriter, _ *http.Request) bool {
				w.WriteHeader(http.StatusServiceUnavailable)
				return true
			},
			"unavailable/http_503",
		},
		"rate limited": {
			func(w http.ResponseWriter, _ *http.Request) bool {
				w.WriteHeader(http.StatusTooManyRequests)
				return true
			},
			"unavailable/http_429",
		},
		"invalid param": {func(w http.ResponseWriter, _ *http.Request) bool {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"code":"rest_invalid_param","message":"SITE-TEXT"}`))

			return true
		}, "invalid/rest_invalid_param"},
		"redirect": {func(w http.ResponseWriter, _ *http.Request) bool {
			w.Header().Set("Location", "https://other.example/")
			w.WriteHeader(http.StatusMovedPermanently)

			return true
		}, "invalid/redirect"},
		"not wordpress": {func(w http.ResponseWriter, _ *http.Request) bool {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("<html>SITE-TEXT</html>"))

			return true
		}, "invalid/bad_answer"},
		"hostile media url": {func(w http.ResponseWriter, r *http.Request) bool {
			if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/media") {
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{"id":5,"source_url":"javascript:alert(1)"}`))

				return true
			}

			return false
		}, "invalid/bad_media_url"},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			f := newFakeWP(t)
			f.override = c.handle

			_, err := f.send(t, p, data, nil, wpUser, wpPass)
			if got := classify(err); got != c.want {
				t.Errorf("got %s, want %s", got, c.want)
			}
		})
	}

	t.Run("an image that changed", func(t *testing.T) {
		t.Parallel()

		f := newFakeWP(t)
		bad := map[string][]byte{
			"cover.png":      data["cover.png"][:10],
			"1-the-desk.png": data["1-the-desk.png"],
			"2-the-cut.png":  data["2-the-cut.png"],
		}

		if _, err := f.send(t, p, bad, nil, wpUser, wpPass); classify(err) != "invalid/image_changed" {
			t.Errorf("got %s", classify(err))
		}

		if f.calls("POST /posts") != 0 {
			t.Error("a post was sent with a changed image")
		}
	})

	t.Run("bad settings", func(t *testing.T) {
		t.Parallel()

		f := newFakeWP(t)
		if _, err := f.send(t, p, data, nil, "", "short"); classify(err) != "invalid/bad_settings" {
			t.Errorf("got %s", classify(err))
		}
	})
}

func TestWordPressConnectionTest(t *testing.T) {
	t.Parallel()

	f := newFakeWP(t)
	w := newWordPress(testConfig())

	secret := wpSecret(t, wpUser, wpPass)

	res, err := w.Test(context.Background(), cred(f.srv.URL, secret, f.srv.Client()))
	if err != nil || !res.OK {
		t.Errorf("good credentials: %+v %v", res, err)
	}

	secret = wpSecret(t, wpUser, "not the password")

	res, err = w.Test(context.Background(), cred(f.srv.URL, secret, f.srv.Client()))
	if err != nil || res.OK || res.Status != 401 || strings.Contains(res.Message, "SITE-TEXT") {
		t.Errorf("wrong credentials: %+v %v", res, err)
	}

	if err := w.ValidateConfig([]byte(secret), f.srv.URL); err != nil {
		t.Errorf("a good config: %v", err)
	}

	for _, bad := range []string{`{"user":"","application_password":"12345678"}`, `{"user":"a","application_password":"short"}`, `{"user":"a","application_password":"12345678","x":1}`} {
		if w.ValidateConfig([]byte(bad), "") == nil {
			t.Errorf("%s was accepted", bad)
		}
	}
}
