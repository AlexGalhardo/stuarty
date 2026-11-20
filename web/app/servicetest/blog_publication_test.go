package servicetest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	corepub "github.com/AlexGalhardo/stuarty/core/publish"
	enginestore "github.com/AlexGalhardo/stuarty/engine/store"
	"github.com/AlexGalhardo/stuarty/service"
	pub "github.com/AlexGalhardo/stuarty/service/publish"
	"github.com/AlexGalhardo/stuarty/service/publish/destinations"
)

// pngBytes is a file a post may carry: the signature of a PNG and some filler, different for each seed.
func blogPNG(seed byte, size int) []byte {
	return append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{seed}, size)...)
}

// seedImage puts an image in the content-addressed store and in the project, as an import and an ingest leave it.
func (c *calendar) seedImage(assetID, name string, data []byte) string {
	c.t.Helper()

	res, err := enginestore.New(string(c.dir)).Put(context.Background(), bytes.NewReader(data), int64(len(data))+1)
	if err != nil {
		c.t.Fatal(err)
	}

	hash := "sha256:" + res.Hash

	err = c.store.Write(context.Background(), func(ctx context.Context, tx service.Tx) error {
		if _, _, uerr := tx.Assets().Upsert(ctx, service.AssetRow{
			Hash:         hash,
			Kind:         "image",
			SizeBytes:    int64(len(data)),
			OriginalName: name,
			IngestState:  service.IngestReady,
			CreatedMs:    1,
			UpdatedMs:    1,
		}); uerr != nil {
			return uerr
		}

		return tx.Assets().PutProjectAsset(ctx, service.ProjectAssetRow{
			ProjectID: c.project, AssetID: assetID, AssetHash: hash, State: service.ProjectAssetActive, Name: name,
			ImportedBy: ui, ImportedMs: 1,
		})
	})
	if err != nil {
		c.t.Fatal(err)
	}

	return res.Hash
}

const (
	blogCover = "ast_0123456789"
	blogOne   = "ast_1111111111"
	blogTwo   = "ast_2222222222"
)

// postBody is a post with a cover and two images.
func postBody(title, slug string) string {
	return "---\ntitle: " + title + "\nslug: " + slug + "\nsummary: A short one.\ntags: video, editing\ncover: " + blogCover +
		"\nlanguage: en\n---\nIntro with *style*.\n\n![the desk](asset:" + blogOne + ")\n\nMore.\n\n![the cut](asset:" +
		blogTwo + ")\n"
}

// blogPost writes a post in the project and gives it its three images; it returns the id of the text.
func (c *calendar) blogPost(title string) string {
	c.t.Helper()

	res, err := c.svc.TextWrite(c.as(token), service.TextWriteRequest{
		ProjectID: c.project, Title: title, TextKind: "post", Text: postBody(title, "how-i-cut-a-video"),
	})
	if err != nil {
		c.t.Fatal(err)
	}

	return res.Item.ID
}

func filesSecret(t *testing.T, settings map[string]string) string {
	t.Helper()

	raw, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}

	return string(raw)
}

func (c *calendar) seedBlogImages() {
	c.t.Helper()

	c.seedImage(blogCover, "Cover Photo.png", blogPNG(1, 300))
	c.seedImage(blogOne, "desk.png", blogPNG(2, 500))
	c.seedImage(blogTwo, "the cut.png", blogPNG(3, 700))
}

// blogConnection makes a connection of a blog destination, as the Connections page does.
func (c *calendar) blogConnection(kind, endpoint, secret string) (string, error) {
	c.t.Helper()

	conn, err := c.svc.ConnectionCreate(c.as(token), service.ConnectionCreateRequest{
		Kind: kind, Label: "My blog", Endpoint: endpoint, Accepts: []string{"text", "image"}, Secret: secret,
	})
	if err != nil {
		return "", err
	}

	return conn.ID, nil
}

func (c *calendar) mustBlogConnection(kind, endpoint, secret string) string {
	c.t.Helper()

	id, err := c.blogConnection(kind, endpoint, secret)
	if err != nil {
		c.t.Fatal(err)
	}

	return id
}

// publishPost plans, schedules, approves and sends a post to a connection, and returns the publication and its target.
func (c *calendar) publishPost(postID, connID, visibility string) (*service.Publication, string) {
	c.t.Helper()

	p, err := c.svc.PublicationCreate(c.as(token), service.PublicationCreateRequest{
		ProjectID: c.project, Title: "How I cut a video", Post: postID, Targets: []pub.TargetInput{{
			ConnectionID: connID, At: c.at(time.Hour), Zone: "UTC", Fields: pub.Fields{Visibility: visibility},
		}},
	})
	if err != nil {
		c.t.Fatal(err)
	}

	id := p.Targets[0].ID
	c.schedule(p, id)
	c.approve(p, id)
	c.clock.advance(time.Hour)

	if n := c.tick(); n != 1 {
		c.t.Fatalf("started %d sends, want 1", n)
	}

	return c.get(p), id
}

// A post with a cover and two images goes to a folder of files after a person approves it, and appears on the
// calendar of the project (criteria 1 and 6 of epic 23).
func TestBlogFilesPublishThroughApproval(t *testing.T) {
	c := newCalendar(t)
	c.seedBlogImages()

	postID := c.blogPost("How I cut a video")
	folder := t.TempDir()
	secret := filesSecret(t, map[string]string{"folder": folder, "template": "hugo"})
	conn := c.mustBlogConnection(destinations.KindFiles, "http://localhost", secret)

	p, err := c.svc.PublicationCreate(c.as(token), service.PublicationCreateRequest{
		ProjectID: c.project, Title: "How I cut a video", Post: postID, Targets: []pub.TargetInput{{
			ConnectionID: conn, At: c.at(time.Hour), Zone: "UTC", Fields: pub.Fields{Visibility: "public"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	id := p.Targets[0].ID
	if p.Post != postID || p.Export != "" || p.Targets[0].Network != destinations.KindFiles {
		t.Fatalf("publication = %+v", p)
	}

	c.schedule(p, id)

	// It waits on a person, and says what it will write.
	pending, err := c.svc.ApprovalList(c.as(token), service.ApprovalListRequest{})
	if err != nil || len(pending.Requests) != 1 {
		t.Fatalf("pending = %+v, %v", pending, err)
	}

	for _, want := range []string{"Blog: files", "My blog", "How I cut a video", "public", "how-i-cut-a-video/index.md"} {
		if !strings.Contains(pending.Requests[0].Summary, want) {
			t.Errorf("the summary lacks %q: %s", want, pending.Requests[0].Summary)
		}
	}

	c.clock.advance(2 * time.Hour)

	if n := c.tick(); n != 0 {
		t.Fatalf("a post was sent before the approval: %d", n)
	}

	if entries, _ := os.ReadDir(folder); len(entries) != 0 {
		t.Fatalf("something was written before the approval: %v", entries)
	}

	c.clock.advance(-2 * time.Hour)
	c.approve(p, id)
	c.clock.advance(time.Hour)

	if n := c.tick(); n != 1 {
		t.Fatalf("started %d sends, want 1", n)
	}

	got := c.get(p).Target(id)
	if got.State != corepub.TargetPublished || got.RemoteID != "how-i-cut-a-video" {
		t.Fatalf("target = %+v", got)
	}

	dir := filepath.Join(folder, "how-i-cut-a-video")

	index, rerr := os.ReadFile(filepath.Join(dir, "index.md"))
	if rerr != nil {
		t.Fatal(rerr)
	}

	for _, want := range []string{
		`title: "How I cut a video"`, "draft: false", `image: "images/cover.png"`,
		"![the desk](images/1-desk.png)", "![the cut](images/2-the-cut.png)", "Intro with *style*.",
	} {
		if !strings.Contains(string(index), want) {
			t.Errorf("index.md lacks %q:\n%s", want, index)
		}
	}

	for _, im := range []struct {
		name string
		size int
	}{{"cover.png", 300}, {"1-desk.png", 500}, {"2-the-cut.png", 700}} {
		data, ierr := os.ReadFile(filepath.Join(dir, "images", im.name))
		if ierr != nil || len(data) != im.size+8 {
			t.Errorf("images/%s: %d bytes, %v", im.name, len(data), ierr)
		}
	}

	// The calendar lists it at its time.
	list, err := c.svc.PublicationList(c.as(token), service.PublicationListRequest{
		ProjectID: c.project, From: c.clock.now().Add(-time.Hour).UTC().Format(time.RFC3339),
		To: c.clock.now().Add(3 * time.Hour).UTC().Format(time.RFC3339),
	})
	if err != nil || len(list.Publications) != 1 || list.Publications[0].Post != postID {
		t.Errorf("calendar = %+v, %v", list, err)
	}
}

// fakeWP is a WordPress with the endpoints of SDD-020 section 41: one post per id, media by name.
type fakeWP struct {
	srv *httptest.Server

	mu     sync.Mutex
	posts  map[int64]map[string]any
	nextID int64
	media  int
	calls  []string
}

func newFakeWP(t *testing.T) *fakeWP {
	t.Helper()

	f := &fakeWP{posts: map[int64]map[string]any{}, nextID: 100}

	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()

		path := strings.TrimPrefix(r.URL.Path, "/wp-json/wp/v2")
		f.calls = append(f.calls, r.Method+" "+path)

		if u, p, ok := r.BasicAuth(); !ok || u != "editor" || p != "abcd efgh ijkl" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"code":"incorrect_password"}`))

			return
		}

		reply := func(status int, v any) {
			raw, _ := json.Marshal(v)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = w.Write(raw)
		}

		switch {
		case path == "/media":
			_, _ = io.Copy(io.Discard, r.Body)
			f.nextID++
			f.media++
			reply(
				201,
				map[string]any{
					"id":         f.nextID,
					"source_url": f.srv.URL + "/uploads/" + strconv.FormatInt(f.nextID, 10) + ".png",
				},
			)
		case path == "/tags" && r.Method == http.MethodGet:
			reply(200, []any{})
		case path == "/tags":
			f.nextID++
			reply(201, map[string]any{"id": f.nextID})
		case path == "/posts" || strings.HasPrefix(path, "/posts/"):
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)

			id := f.nextID + 1
			if path != "/posts" {
				id, _ = strconv.ParseInt(strings.TrimPrefix(path, "/posts/"), 10, 64)
				if _, ok := f.posts[id]; !ok {
					reply(404, map[string]any{"code": "rest_post_invalid_id"})

					return
				}
			} else {
				f.nextID = id
			}

			f.posts[id] = body
			reply(200, map[string]any{"id": id, "link": f.srv.URL + "/?p=" + strconv.FormatInt(id, 10)})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	t.Cleanup(f.srv.Close)

	return f
}

// Publishing the same post to WordPress twice updates the first post instead of creating a second (criterion 2).
func TestBlogWordPressSecondPublishUpdates(t *testing.T) {
	c := newCalendar(t)
	c.seedBlogImages()

	postID := c.blogPost("How I cut a video")
	wp := newFakeWP(t)
	conn := c.mustBlogConnection(destinations.KindWordPress, wp.srv.URL,
		`{"user":"editor","application_password":"abcd efgh ijkl"}`)

	first, id := c.publishPost(postID, conn, "public")
	if got := first.Target(id); got.State != corepub.TargetPublished || got.RemoteID != "101" && got.RemoteID == "" {
		t.Fatalf("first target = %+v", got)
	}

	firstRemote := first.Target(id).RemoteID

	// The person edits the post and publishes it again, as a new publication to the same connection.
	if _, err := c.svc.TextWrite(c.as(token), service.TextWriteRequest{
		ProjectID: c.project, ItemID: postID, Text: strings.Replace(postBody("How I cut a video", "how-i-cut-a-video"),
			"More.", "More, and a second paragraph.", 1),
	}); err != nil {
		t.Fatal(err)
	}

	second, id2 := c.publishPost(postID, conn, "public")
	if got := second.Target(id2); got.State != corepub.TargetPublished || got.RemoteID != firstRemote {
		t.Fatalf("second target = %+v, the first remote id was %s", got, firstRemote)
	}

	wp.mu.Lock()
	defer wp.mu.Unlock()

	if len(wp.posts) != 1 {
		t.Fatalf("%d posts at WordPress, want 1: %v", len(wp.posts), wp.calls)
	}

	for _, body := range wp.posts {
		if !strings.Contains(body["content"].(string), "a second paragraph") || body["status"] != "publish" {
			t.Errorf("the post was not updated: %v", body)
		}
	}

	if wp.media != 6 {
		t.Errorf("%d media uploads, want 3 for each of the two sends", wp.media)
	}
}

// A webhook delivery carries a valid HMAC signature and a timestamp.
func TestBlogWebhookDelivery(t *testing.T) {
	c := newCalendar(t)
	c.seedBlogImages()

	var (
		mu   sync.Mutex
		hits []struct {
			header http.Header
			body   []byte
		}
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)

		mu.Lock()
		hits = append(hits, struct {
			header http.Header
			body   []byte
		}{r.Header.Clone(), body})
		mu.Unlock()

		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	const secret = "a-long-signing-secret-of-32-bytes"

	conn := c.mustBlogConnection(destinations.KindWebhookLocal, srv.URL+"/hook",
		`{"signing_secret":"`+secret+`","media_base_url":"https://stuarty.example"}`)

	p, id := c.publishPost(c.blogPost("How I cut a video"), conn, "public")
	if got := p.Target(id); got.State != corepub.TargetPublished {
		t.Fatalf("target = %+v", got)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(hits) != 1 {
		t.Fatalf("%d deliveries, want 1", len(hits))
	}

	h := hits[0]
	if err := destinations.Verify(
		secret,
		h.header.Get(destinations.HeaderTimestamp),
		h.header.Get(destinations.HeaderDelivery),
		h.header.Get(destinations.HeaderSignature),
		h.body,
		time.Now(),
	); err != nil {
		t.Errorf("the signature does not verify: %v", err)
	}

	if h.header.Get(destinations.HeaderDelivery) != id || !strings.Contains(string(h.body), `"post.published"`) ||
		!strings.Contains(string(h.body), "https://stuarty.example/api/v1/posts/how-i-cut-a-video/media/cover.png") {
		t.Errorf("headers %v, body %s", h.header, h.body)
	}
}

// A webhook to a private address is refused unless local delivery was explicitly allowed (criterion 4).
func TestBlogWebhookPrivateAddressNeedsTheLocalKind(t *testing.T) {
	c := newCalendar(t)

	const secret = `{"signing_secret":"a-long-signing-secret-of-32-bytes"}`

	for _, endpoint := range []string{
		"http://127.0.0.1:9/hook", "https://localhost/hook", "https://10.0.0.5/hook", "https://192.168.1.2/hook",
		"https://[::1]/hook", "https://169.254.169.254/latest",
	} {
		_, err := c.blogConnection(destinations.KindWebhook, endpoint, secret)

		var e *service.Error
		if !errors.As(err, &e) || e.Code != command.CodeValidation || e.Path != "endpoint" {
			t.Errorf("a public webhook to %s: %v, want E_VALIDATION on the endpoint", endpoint, err)
		}
	}

	if _, err := c.blogConnection(destinations.KindWebhook, "https://hooks.example.com/stuarty", secret); err != nil {
		t.Errorf("a public webhook was refused: %v", err)
	}

	for _, endpoint := range []string{"http://127.0.0.1:9/hook", "https://10.0.0.5/hook"} {
		if _, err := c.blogConnection(destinations.KindWebhookLocal, endpoint, secret); err != nil {
			t.Errorf("a local webhook to %s was refused: %v", endpoint, err)
		}
	}
}

// The read API returns only posts published to it; the same slug cannot be taken by another post.
func TestBlogAPIPublishesOnlyWhatWasApproved(t *testing.T) {
	c := newCalendar(t)
	c.seedBlogImages()

	conn := c.mustBlogConnection(destinations.KindAPI, "http://localhost", `{}`)
	reader := service.WithCaller(
		context.Background(),
		service.Caller{Actor: "key:k", Scopes: []string{service.ScopePostsRead}},
	)

	empty, err := c.svc.PublishedPostList(reader, service.PublishedPostListRequest{})
	if err != nil || len(empty.Posts) != 0 {
		t.Fatalf("a fresh server lists %v, %v", empty, err)
	}

	// A planned post that nobody approved is not served.
	postID := c.blogPost("How I cut a video")

	p, err := c.svc.PublicationCreate(c.as(token), service.PublicationCreateRequest{
		ProjectID: c.project, Title: "t", Post: postID, Targets: []pub.TargetInput{{
			ConnectionID: conn, At: c.at(time.Hour), Zone: "UTC", Fields: pub.Fields{Visibility: "public"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	c.schedule(p, p.Targets[0].ID)

	if got, _ := c.svc.PublishedPostList(reader, service.PublishedPostListRequest{}); len(got.Posts) != 0 {
		t.Fatalf("a post waiting for approval is served: %v", got.Posts)
	}

	c.approve(p, p.Targets[0].ID)
	c.clock.advance(time.Hour)

	if n := c.tick(); n != 1 {
		t.Fatalf("started %d sends", n)
	}

	list, err := c.svc.PublishedPostList(reader, service.PublishedPostListRequest{})
	if err != nil || len(list.Posts) != 1 || list.Posts[0].Slug != "how-i-cut-a-video" ||
		list.Posts[0].Cover != "images/cover.png" {
		t.Fatalf("list = %+v, %v", list, err)
	}

	one, err := c.svc.PublishedPostGet(reader, service.PublishedPostGetRequest{Slug: "how-i-cut-a-video"})
	if err != nil || !strings.Contains(one.HTML, `<img src="images/1-desk.png" alt="the desk">`) ||
		len(one.Images) != 3 {
		t.Fatalf("get = %+v, %v", one, err)
	}

	media, err := c.svc.PublishedPostMedia(
		reader,
		service.PublishedPostMediaRequest{Slug: "how-i-cut-a-video", Name: "cover.png"},
	)
	if err != nil || media.ContentType != "image/png" || media.Bytes != 308 || strings.HasPrefix(media.Path, "..") {
		t.Fatalf("media = %+v, %v", media, err)
	}

	if data, rerr := os.ReadFile(
		filepath.Join(string(c.dir), filepath.FromSlash(media.Path)),
	); rerr != nil ||
		len(data) != 308 {
		t.Errorf("the path does not lead to the image: %v", rerr)
	}

	// Another post with the same slug is refused by the destination, with an actionable error.
	other := c.blogPost("Another post")
	p2, err := c.svc.PublicationCreate(c.as(token), service.PublicationCreateRequest{
		ProjectID: c.project, Title: "t2", Post: other, Targets: []pub.TargetInput{{
			ConnectionID: conn, At: c.at(time.Hour), Zone: "UTC", Fields: pub.Fields{Visibility: "public"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	c.schedule(p2, p2.Targets[0].ID)
	c.approve(p2, p2.Targets[0].ID)
	c.clock.advance(time.Hour)
	c.tick()

	failed := c.get(p2).Target(p2.Targets[0].ID)
	if failed.State != corepub.TargetFailed || failed.Error == nil || failed.Error.Kind != "invalid" {
		t.Errorf("the second post with the slug: %+v", failed)
	}

	// The read API holds the scope: a caller without it, and a project key, are refused.
	for _, who := range []service.Caller{
		{Actor: "key:k", Scopes: []string{service.ScopeRead}}, {Actor: "key:k", Scopes: []string{service.ScopeEdit, service.ScopeAdmin}},
	} {
		if _, err := c.svc.PublishedPostList(
			service.WithCaller(context.Background(), who),
			service.PublishedPostListRequest{},
		); err == nil {
			t.Errorf("a caller with %v read the posts", who.Scopes)
		}
	}
}

// A draft is not served, and a draft that is approved and later made public replaces it.
func TestBlogAPIDraftIsNotServed(t *testing.T) {
	c := newCalendar(t)
	c.seedBlogImages()

	conn := c.mustBlogConnection(destinations.KindAPI, "http://localhost", `{}`)
	reader := service.WithCaller(
		context.Background(),
		service.Caller{Actor: "key:k", Scopes: []string{service.ScopePostsRead}},
	)
	postID := c.blogPost("How I cut a video")

	c.publishPost(postID, conn, "private")

	if got, _ := c.svc.PublishedPostList(reader, service.PublishedPostListRequest{}); len(got.Posts) != 0 {
		t.Errorf("a draft is listed: %v", got.Posts)
	}

	if _, err := c.svc.PublishedPostGet(
		reader,
		service.PublishedPostGetRequest{Slug: "how-i-cut-a-video"},
	); err == nil {
		t.Error("a draft is served")
	}

	c.publishPost(postID, conn, "public")

	if got, _ := c.svc.PublishedPostList(reader, service.PublishedPostListRequest{}); len(got.Posts) != 1 {
		t.Errorf("the published post is not listed: %v", got.Posts)
	}
}

// What a person approved is what is sent: an image that changed after the approval is not.
func TestBlogImageChangedAfterApprovalIsNotSent(t *testing.T) {
	c := newCalendar(t)
	hash := c.seedImage(blogCover, "Cover Photo.png", blogPNG(1, 300))
	c.seedImage(blogOne, "desk.png", blogPNG(2, 500))
	c.seedImage(blogTwo, "the cut.png", blogPNG(3, 700))

	folder := t.TempDir()
	secret := filesSecret(t, map[string]string{"folder": folder})
	conn := c.mustBlogConnection(destinations.KindFiles, "http://localhost", secret)
	postID := c.blogPost("How I cut a video")

	p, err := c.svc.PublicationCreate(c.as(token), service.PublicationCreateRequest{
		ProjectID: c.project, Title: "t", Post: postID, Targets: []pub.TargetInput{{
			ConnectionID: conn, At: c.at(time.Hour), Zone: "UTC", Fields: pub.Fields{Visibility: "public"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	id := p.Targets[0].ID
	c.schedule(p, id)
	c.approve(p, id)

	// The file of the cover is replaced in the store by one of the same size.
	abs, err := enginestore.New(string(c.dir)).Path(hash)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.Chmod(abs, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(abs, blogPNG(9, 300), 0o600); err != nil {
		t.Fatal(err)
	}

	c.clock.advance(time.Hour)
	c.tick()

	got := c.get(p).Target(id)
	if got.State != corepub.TargetFailed || got.Error == nil || got.Error.Code != command.CodeInvalidState {
		t.Fatalf("target = %s, error %+v", got.State, got.Error)
	}

	if entries, _ := os.ReadDir(folder); len(entries) != 0 {
		t.Errorf("a post with a changed image was written: %v", entries)
	}
}

// A post goes only to a blog destination and a video only to a social network; a post with a problem is refused with
// every issue.
func TestBlogRefusals(t *testing.T) {
	c := newCalendar(t)
	c.seedBlogImages()

	folder := t.TempDir()
	secret := filesSecret(t, map[string]string{"folder": folder})
	blog := c.mustBlogConnection(destinations.KindFiles, "http://localhost", secret)
	postID := c.blogPost("How I cut a video")

	var e *service.Error

	// A post to a social network.
	_, err := c.svc.PublicationCreate(c.as(token), service.PublicationCreateRequest{
		ProjectID: c.project, Title: "t", Post: postID, Targets: []pub.TargetInput{{ConnectionID: c.connID}},
	})
	if !errors.As(err, &e) || e.Code != command.CodeValidation {
		t.Errorf("a post to a social network: %v", err)
	}

	// A video to a blog destination.
	_, err = c.svc.PublicationCreate(c.as(token), service.PublicationCreateRequest{
		ProjectID: c.project, Title: "t", Export: c.exp, Targets: []pub.TargetInput{{ConnectionID: blog}},
	})
	if !errors.As(err, &e) || e.Code != command.CodeValidation {
		t.Errorf("a video to a blog destination: %v", err)
	}

	// Both, and neither.
	for _, in := range []service.PublicationCreateRequest{
		{ProjectID: c.project, Title: "t", Export: c.exp, Post: postID},
		{ProjectID: c.project, Title: "t"},
	} {
		if _, err := c.svc.PublicationCreate(c.as(token), in); err == nil {
			t.Errorf("%+v was accepted", in)
		}
	}

	// A text that is not a post.
	copyText, err := c.svc.TextWrite(
		c.as(token),
		service.TextWriteRequest{ProjectID: c.project, Title: "c", Text: "plain"},
	)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := c.svc.PublicationCreate(c.as(token), service.PublicationCreateRequest{
		ProjectID: c.project, Title: "t", Post: copyText.Item.ID, Targets: []pub.TargetInput{{ConnectionID: blog}},
	}); err == nil {
		t.Error("a copy was taken for a post")
	}

	// A post with problems is planned (a draft may be unfinished) and refused when it asks for approval, with every
	// issue at once.
	bad, err := c.svc.TextWrite(c.as(token), service.TextWriteRequest{
		ProjectID: c.project, Title: "bad", TextKind: "post",
		Text: "---\ntitle: Bad\nslug: Bad Slug\ncover: ast_9999999999\n---\n![x](https://tracker.example/p.png)",
	})
	if err != nil {
		t.Fatal(err)
	}

	p, err := c.svc.PublicationCreate(c.as(token), service.PublicationCreateRequest{
		ProjectID: c.project,
		Title:     "t",
		Post:      bad.Item.ID,
		Targets:   []pub.TargetInput{{ConnectionID: blog, At: c.at(time.Hour)}},
	})
	if err != nil {
		t.Fatalf("an unfinished post could not be planned: %v", err)
	}

	_, err = c.svc.PublicationSchedule(c.as(token), service.PublicationScheduleRequest{ID: p.ID})
	if !errors.As(err, &e) || e.Code != command.CodeValidation || e.Details["issues"] == nil {
		t.Fatalf("scheduling a bad post: %v", err)
	}

	issues := fmt.Sprint(e.Details["issues"])
	for _, want := range []string{"slug", "cover", "body.images"} {
		if !strings.Contains(issues, want) {
			t.Errorf("the issues lack %s: %s", want, issues)
		}
	}

	_ = sha256.Sum256
	_ = hex.EncodeToString
}
