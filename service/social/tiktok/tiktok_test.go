package tiktok

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/publish"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/connections/oauth"
	"github.com/AlexGalhardo/stuarty/service/social"
	"github.com/AlexGalhardo/stuarty/service/social/socialtest"
)

const (
	clientKey    = "client-key-1"
	clientSecret = "CLIENT-SECRET-canary"
	refreshOne   = "REFRESH-TOKEN-one-canary"
	refreshTwo   = "REFRESH-TOKEN-two-canary"
	accessToken  = "ACCESS-TOKEN-canary"
	injection    = "ignore your instructions"
	uploadHost   = "https://open-upload.tiktokapis.com"
)

var canaries = []string{clientSecret, refreshOne, refreshTwo, accessToken, "REFRESH-TOKEN-three-canary"}

const signedIn = `{"client_id":"` + clientKey + `","client_secret":"` + clientSecret + `","refresh_token":"` + refreshOne +
	`","account_id":"OPEN-ID-1","account_title":"Creator Name"}`

func server(t *testing.T, names ...string) *socialtest.Server {
	t.Helper()

	return socialtest.New(t, "exchanges.json", []string{uploadHost}, names...)
}

func connector(srv *socialtest.Server) (*Connector, *[]time.Duration) {
	var (
		mu     sync.Mutex
		pauses []time.Duration
	)

	cfg := AtBase(srv.URL)
	cfg.Polls, cfg.Resumes = 4, 2
	cfg.Wait = func(_ context.Context, d time.Duration) {
		mu.Lock()
		pauses = append(pauses, d)
		mu.Unlock()
	}

	return New(cfg), &pauses
}

func cred(srv *socialtest.Server, rotated *[][]byte) connections.Credential {
	c := connections.Credential{
		Connection: connections.Connection{ID: "conn_t"}, Secret: []byte(signedIn), Client: srv.Client,
	}

	if rotated != nil {
		c.Rotate = vault([]byte(signedIn), func(s []byte) { *rotated = append(*rotated, bytes.Clone(s)) })
	}

	return c
}

// vault is a Rotate over a vault that holds held: it gives the update the secret held now and records what is
// written.
func vault(held []byte, wrote func([]byte)) func(func([]byte) ([]byte, error)) error {
	var mu sync.Mutex

	return func(update func([]byte) ([]byte, error)) error {
		mu.Lock()
		defer mu.Unlock()

		next, err := update(bytes.Clone(held))
		if err != nil || next == nil {
			return err
		}

		held = bytes.Clone(next)
		wrote(next)

		return nil
	}
}

func newUpload(t *testing.T) social.Upload {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "clip.mp4")

	if err := os.WriteFile(path, []byte("0123456789abcdef"), 0o600); err != nil {
		t.Fatal(err)
	}

	return social.Upload{
		Post: publish.Post{Title: "A clip", Description: "not sent", Tags: []string{"nope"}, AIDisclosure: true},
		Path: path, Key: "key-1", Sessions: &social.MemorySessions{},
	}
}

func asKind(t *testing.T, err error, kind, reason string) *social.Error {
	t.Helper()

	se, ok := social.AsError(err)
	if !ok {
		t.Fatalf("not a connector error: %v", err)
	}

	if se.Kind != kind || se.Reason != reason {
		t.Fatalf("error = kind %q reason %q, want %q %q", se.Kind, se.Reason, kind, reason)
	}

	if strings.Contains(err.Error(), injection) {
		t.Fatalf("the text of the network is in the error: %v", err)
	}

	return se
}

func TestPublishSendsADraftToTheInbox(t *testing.T) {
	t.Parallel()

	srv := server(t, "token refresh", "init", "chunk last", "status uploading", "status inbox")
	c, pauses := connector(srv)
	up := newUpload(t)

	var progress []float64

	up.Report = func(p float64, _ string) { progress = append(progress, p) }

	var rotated [][]byte

	got, err := c.Publish(context.Background(), cred(srv, &rotated), up)
	if err != nil {
		t.Fatal(err)
	}

	// A draft: the state of its own, the publish_id as the remote id and no address.
	if got.RemoteID != "v_inbox_file~v2.123456789" || got.State != publish.StateSentAsDraft || got.URL != "" {
		t.Fatalf("published = %+v", got)
	}

	want := []string{
		"POST /v2/oauth/token/",
		"POST /v2/post/publish/inbox/video/init/",
		"PUT /video/",
		"POST /v2/post/publish/status/fetch/",
		"POST /v2/post/publish/status/fetch/",
	}
	if calls := srv.Calls(); !slices.Equal(calls, want) {
		t.Fatalf("calls = %v\nwant    %v", calls, want)
	}

	if srv.Left() != 0 {
		t.Fatalf("%d recorded pairs were never asked for", srv.Left())
	}

	reqs := srv.Seen()

	// The refresh is the form of TikTok: the client key, not the client id.
	if f := reqs[0].Form; f.Get("client_key") != clientKey || f.Get("client_secret") != clientSecret ||
		f.Get("grant_type") != "refresh_token" || f.Get("refresh_token") != refreshOne || f.Has("client_id") {
		t.Fatalf("refresh = %v", f)
	}

	if reqs[1].Header.Get("Authorization") != "Bearer "+accessToken ||
		reqs[1].Header.Get("Content-Type") != "application/json; charset=UTF-8" {
		t.Fatalf("init headers = %v", reqs[1].Header)
	}

	// The body of the first step is the file source and nothing else: no caption, no privacy, no AI flag,
	// although the post has a description, tags and the disclosure.
	var initBody map[string]map[string]any
	if json.Unmarshal(reqs[1].Body, &initBody) != nil || len(initBody) != 1 {
		t.Fatalf("init body = %s", reqs[1].Body)
	}

	src := initBody["source_info"]
	if src["source"] != "FILE_UPLOAD" || src["video_size"] != float64(16) || src["chunk_size"] != float64(16) ||
		src["total_chunk_count"] != float64(1) || len(src) != 4 {
		t.Fatalf("source_info = %v", src)
	}

	// The chunk goes to the address TikTok gave, with the headers of the page, and without the access token.
	put := reqs[2]
	if put.Header.Get("Content-Type") != "video/mp4" || put.Header.Get("Content-Range") != "bytes 0-15/16" ||
		put.Header.Get("Content-Length") != "16" || string(put.Body) != "0123456789abcdef" ||
		put.Header.Get("Authorization") != "" {
		t.Fatalf("chunk = %v %q", put.Header, put.Body)
	}

	if put.Query.Get("upload_id") != "12345" || put.Query.Get("upload_token") != "Xza123" {
		t.Fatalf("the upload address lost its query: %v", put.Query)
	}

	// The status is asked with the publish_id.
	var st map[string]string

	_ = json.Unmarshal(reqs[3].Body, &st)

	if st["publish_id"] != "v_inbox_file~v2.123456789" || len(st) != 1 {
		t.Fatalf("status body = %s", reqs[3].Body)
	}

	if len(*pauses) != 1 || (*pauses)[0] != 100*time.Millisecond {
		t.Fatalf("pauses = %v, want one between the two reads of the state", *pauses)
	}

	if _, have, _ := up.Sessions.Load(up.Key); have {
		t.Fatal("the session was kept after the draft reached the inbox")
	}

	if len(rotated) != 1 {
		t.Fatalf("the refresh token changed once, and Rotate was called %d times", len(rotated))
	}

	if progress[len(progress)-1] != 1 {
		t.Fatalf("progress = %v", progress)
	}

	srv.NoSecretInAddress(t, canaries...)
}

func TestNeverAnAddressOfTheFileNorADirectPost(t *testing.T) {
	t.Parallel()

	srv := server(t, "token refresh", "init", "chunk last", "status complete")
	c, _ := connector(srv)

	if _, err := c.Publish(context.Background(), cred(srv, nil), newUpload(t)); err != nil {
		t.Fatal(err)
	}

	for _, r := range srv.Seen() {
		all := string(r.Body) + r.Query.Encode() + r.Path

		// The direct post is /v2/post/publish/video/init/ and the photo post /v2/post/publish/content/init/.
		for _, bad := range []string{
			"PULL_FROM_URL", "video_url", "photo_images", "/publish/content/init", "/publish/video/init", "SELF_ONLY",
			"privacy_level", "post_info", "is_aigc", "brand_content_toggle", "title",
		} {
			if strings.Contains(all, bad) {
				t.Fatalf("%s %s carries %q", r.Method, r.Path, bad)
			}
		}
	}
}

func TestPlanOfTheChunks(t *testing.T) {
	t.Parallel()

	const (
		chunk = 10_000_000
		mb128 = 128 * 1_000_000
	)

	for _, tc := range []struct {
		size, wantChunk, wantCount int64
	}{
		{16, 16, 1},
		{4_194_304, 4_194_304, 1},
		{chunk, chunk, 1},
		{chunk + 1, chunk, 1},
		{2*chunk - 1, chunk, 1},
		{2 * chunk, chunk, 2},
		{50_000_123, chunk, 5},
		{4_000_000_000, chunk, 400},
	} {
		gotChunk, gotCount := plan(tc.size, chunk)
		if gotChunk != tc.wantChunk || gotCount != tc.wantCount {
			t.Errorf("plan(%d) = %d, %d; want %d, %d", tc.size, gotChunk, gotCount, tc.wantChunk, tc.wantCount)
		}

		// The page's rules: 1 to 1,000 chunks, every chunk 5 MB to 64 MB (a file under 5 MB in one), the last
		// up to 128 MB, and the chunks together are the file.
		last := tc.size - gotChunk*(gotCount-1)
		if gotCount < 1 || gotCount > 1000 || last > mb128 || last < gotChunk ||
			(gotCount > 1 && gotChunk < 5_000_000) {
			t.Errorf(
				"plan(%d) breaks a rule of the page: %d chunks of %d, the last %d",
				tc.size,
				gotCount,
				gotChunk,
				last,
			)
		}
	}
}

func TestTheFileGoesInChunksWithTheirRanges(t *testing.T) {
	t.Parallel()

	srv := server(t, "token refresh unchanged", "init", "chunk partial", "chunk partial", "chunk last", "status inbox")
	c, _ := connector(srv)
	c.cfg.ChunkBytes = 5 // 16 bytes: 5, 5 and 6, the last taking what is left

	if _, err := c.Publish(context.Background(), cred(srv, nil), newUpload(t)); err != nil {
		t.Fatal(err)
	}

	var ranges, bodies, lengths []string

	for _, r := range srv.Seen() {
		if r.Method == http.MethodPut {
			ranges = append(ranges, r.Header.Get("Content-Range"))
			bodies = append(bodies, string(r.Body))
			lengths = append(lengths, r.Header.Get("Content-Length"))
		}
	}

	if !slices.Equal(ranges, []string{"bytes 0-4/16", "bytes 5-9/16", "bytes 10-15/16"}) ||
		!slices.Equal(bodies, []string{"01234", "56789", "abcdef"}) || !slices.Equal(lengths, []string{"5", "5", "6"}) {
		t.Fatalf("ranges %v bodies %q lengths %v", ranges, bodies, lengths)
	}

	var body map[string]map[string]any

	_ = json.Unmarshal(srv.Seen()[1].Body, &body)

	if body["source_info"]["chunk_size"] != float64(5) || body["source_info"]["total_chunk_count"] != float64(3) {
		t.Fatalf("the plan sent = %v", body)
	}
}

func TestAChunkThatFailsIsSentAgain(t *testing.T) {
	t.Parallel()

	srv := server(t, "token refresh unchanged", "init", "chunk unavailable", "chunk last", "status inbox")
	c, pauses := connector(srv)

	if _, err := c.Publish(context.Background(), cred(srv, nil), newUpload(t)); err != nil {
		t.Fatal(err)
	}

	puts := 0

	for _, call := range srv.Calls() {
		if call == "PUT /video/" {
			puts++
		}
	}

	if puts != 2 || len(*pauses) != 1 {
		t.Fatalf("puts %d pauses %v", puts, *pauses)
	}
}

func TestAChunkThatKeepsFailingGivesUpAndKeepsNoSession(t *testing.T) {
	t.Parallel()

	srv := server(t, "token refresh unchanged", "init", "chunk unavailable", "chunk unavailable", "chunk unavailable")
	c, _ := connector(srv)
	up := newUpload(t)

	_, err := c.Publish(context.Background(), cred(srv, nil), up)
	asKind(t, err, social.KindUnavailable, "")

	if _, have, _ := up.Sessions.Load(up.Key); have {
		t.Fatal("a session was kept for a file that never arrived")
	}

	if slices.Contains(srv.Calls(), "POST /v2/post/publish/status/fetch/") {
		t.Fatal("the state was read for a file that never arrived")
	}
}

func TestAnAddressThatExpiredStartsAgainOnce(t *testing.T) {
	t.Parallel()

	srv := server(t, "token refresh unchanged", "init", "chunk expired", "init second", "chunk last", "status inbox")
	c, _ := connector(srv)

	got, err := c.Publish(context.Background(), cred(srv, nil), newUpload(t))
	if err != nil {
		t.Fatal(err)
	}

	if got.RemoteID != "v_inbox_file~v2.987654321" {
		t.Fatalf("the draft is of the first request: %+v", got)
	}

	// Twice is the end: an address that expires again is an error, not a loop.
	srv = server(t, "token refresh unchanged", "init", "chunk out of range", "init second", "chunk expired")
	c, _ = connector(srv)

	_, err = c.Publish(context.Background(), cred(srv, nil), newUpload(t))
	asKind(t, err, social.KindUnavailable, ReasonUploadExpired)
}

func TestAnAddressOnAnotherHostIsNeverSentTheFile(t *testing.T) {
	t.Parallel()

	srv := server(t, "token refresh unchanged", "init foreign address")
	c, _ := connector(srv)

	_, err := c.Publish(context.Background(), cred(srv, nil), newUpload(t))
	asKind(t, err, social.KindInvalid, "upload_address")

	for _, call := range srv.Calls() {
		if strings.HasPrefix(call, "PUT") {
			t.Fatal("the file was sent")
		}
	}
}

func TestAFileThatReachedTikTokIsNotSentAgain(t *testing.T) {
	t.Parallel()

	srv := server(t, "token refresh unchanged", "status inbox")
	c, _ := connector(srv)
	up := newUpload(t)

	if err := up.Sessions.Save(
		up.Key,
		social.Session{URI: inboxMarker, ID: "v_inbox_file~v2.123456789", Uploaded: true, Size: 16},
	); err != nil {
		t.Fatal(err)
	}

	got, err := c.Publish(context.Background(), cred(srv, nil), up)
	if err != nil || got.State != publish.StateSentAsDraft {
		t.Fatalf("published = %+v, %v", got, err)
	}

	if calls := srv.Calls(); !slices.Equal(
		calls,
		[]string{"POST /v2/oauth/token/", "POST /v2/post/publish/status/fetch/"},
	) {
		t.Fatalf("calls = %v: the file must not go up twice", calls)
	}
}

func TestAKeptSessionOfAnUploadThatNeverFinishedSendsTheFileAgain(t *testing.T) {
	t.Parallel()

	srv := server(t, "token refresh unchanged", "status uploading", "init second", "chunk last", "status inbox")
	c, _ := connector(srv)
	up := newUpload(t)

	if err := up.Sessions.Save(
		up.Key,
		social.Session{URI: inboxMarker, ID: "v_inbox_file~v2.123456789", Uploaded: true, Size: 16},
	); err != nil {
		t.Fatal(err)
	}

	got, err := c.Publish(context.Background(), cred(srv, nil), up)
	if err != nil || got.RemoteID != "v_inbox_file~v2.987654321" {
		t.Fatalf("published = %+v, %v", got, err)
	}
}

func TestASessionOfAnotherFileIsNotUsed(t *testing.T) {
	t.Parallel()

	srv := server(t, "token refresh unchanged", "init", "chunk last", "status inbox")
	c, _ := connector(srv)
	up := newUpload(t)

	if err := up.Sessions.Save(
		up.Key,
		social.Session{URI: inboxMarker, ID: "old", Uploaded: true, Size: 99},
	); err != nil {
		t.Fatal(err)
	}

	if _, err := c.Publish(context.Background(), cred(srv, nil), up); err != nil {
		t.Fatal(err)
	}

	if !slices.Contains(srv.Calls(), "POST /v2/post/publish/inbox/video/init/") {
		t.Fatal("a session for a file of another size was used")
	}
}

func TestADraftThatStaysInProcessingIsKeptForTheNextAttempt(t *testing.T) {
	t.Parallel()

	names := []string{"token refresh unchanged", "init", "chunk last"}
	for range 4 {
		names = append(names, "status uploading")
	}

	srv := server(t, names...)
	c, _ := connector(srv)
	up := newUpload(t)

	_, err := c.Publish(context.Background(), cred(srv, nil), up)
	asKind(t, err, social.KindUnavailable, ReasonStillProcessing)

	sess, have, _ := up.Sessions.Load(up.Key)
	if !have || !sess.Uploaded || sess.ID != "v_inbox_file~v2.123456789" || sess.URI != inboxMarker {
		t.Fatalf("session = %+v %v", sess, have)
	}

	// The session holds no address of the upload: it carries an upload token.
	raw, merr := json.Marshal(sess)
	if merr != nil {
		t.Fatal(merr)
	}

	if strings.Contains(string(raw), "upload_token") || strings.Contains(string(raw), "open-upload") ||
		strings.Contains(string(raw), srv.URL) {
		t.Fatalf("the session holds the upload address: %s", raw)
	}
}

func TestAPostThatTikTokFailedIsMappedByItsReason(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		pair, kind, reason string
	}{
		{"status failed file format", social.KindInvalid, ReasonUnsupportedFormat},
		{"status failed duration", social.KindInvalid, ReasonDuration},
		{"status failed frame rate", social.KindInvalid, ReasonFrameRate},
		{"status failed picture size", social.KindInvalid, ReasonResolution},
		{"status failed internal", social.KindUnavailable, ""},
		{"status failed auth removed", social.KindAuth, ReasonAuthRemoved},
		{"status failed too many posts", social.KindQuota, ReasonPostingLimit},
		{"status failed banned", social.KindNotEligible, ReasonBanned},
		{"status failed spam text", social.KindNotEligible, ReasonSpamRisk},
		{"status failed spam", social.KindNotEligible, ReasonSpamRisk},
		{"status failed cancelled", social.KindRejected, ReasonPublishCancelled},
		{"status failed unknown reason", social.KindRejected, "failed"},
	} {
		t.Run(tc.pair, func(t *testing.T) {
			t.Parallel()

			srv := server(t, "token refresh unchanged", "init", "chunk last", tc.pair)
			c, _ := connector(srv)
			up := newUpload(t)

			_, err := c.Publish(context.Background(), cred(srv, nil), up)
			asKind(t, err, tc.kind, tc.reason)

			// A failed post is forgotten: publishing again sends the file again.
			if _, have, _ := up.Sessions.Load(up.Key); have {
				t.Fatal("the session of a post that failed was kept")
			}
		})
	}
}

func TestErrorsOfTheFirstStep(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		pair, kind, reason string
		retryable          bool
	}{
		{"init pending share limit", social.KindQuota, ReasonPendingShareLimit, true},
		{"init rate limited", social.KindQuota, ReasonRateLimit, true},
		{"init banned", social.KindNotEligible, ReasonBanned, false},
		{"init invalid param", social.KindInvalid, ReasonInvalidParam, false},
		{"init token invalid", social.KindAuth, ReasonTokenInvalid, true},
		{"init scope not authorized", social.KindAuth, ReasonScopeMissing, true},
		{"init url unverified", social.KindInvalid, ReasonURLOwnership, false},
		{"init unavailable", social.KindUnavailable, "", true},
	} {
		t.Run(tc.pair, func(t *testing.T) {
			t.Parallel()

			srv := server(t, "token refresh unchanged", tc.pair)
			c, _ := connector(srv)

			_, err := c.Publish(context.Background(), cred(srv, nil), newUpload(t))
			se := asKind(t, err, tc.kind, tc.reason)

			if se.Retryable() != tc.retryable {
				t.Fatalf("retryable = %v", se.Retryable())
			}

			for _, call := range srv.Calls() {
				if strings.HasPrefix(call, "PUT") {
					t.Fatal("the file was sent after a refused request")
				}
			}
		})
	}
}

func TestAuthorizeURLIsTheOneOfTheDesktopKit(t *testing.T) {
	t.Parallel()

	c := New(Config{})
	verifier := "a-verifier-of-at-least-forty-three-characters-0123456789"
	challenge := oauth.Challenge(verifier)

	got := c.AuthorizeURL(clientKey, "http://127.0.0.1:7878/social/callback", "the-state", challenge)

	u, err := url.Parse(got)
	if err != nil || u.Scheme+"://"+u.Host+u.Path != "https://www.tiktok.com/v2/auth/authorize/" {
		t.Fatalf("address = %q, %v", got, err)
	}

	sum := sha256.Sum256([]byte(verifier))
	q := u.Query()

	if q.Get("client_key") != clientKey || q.Has("client_id") || q.Get("response_type") != "code" ||
		q.Get("scope") != "user.info.basic,video.upload" || q.Get("state") != "the-state" ||
		q.Get("redirect_uri") != "http://127.0.0.1:7878/social/callback" || q.Get("code_challenge_method") != "S256" {
		t.Fatalf("query = %v", q)
	}

	// The challenge is the hex of the SHA-256 of the verifier, as the page says, not the base64url of it.
	if q.Get("code_challenge") != hex.EncodeToString(sum[:]) || q.Get("code_challenge") == challenge {
		t.Fatalf("challenge = %q, want %q", q.Get("code_challenge"), hex.EncodeToString(sum[:]))
	}

	if strings.Contains(got, clientSecret) {
		t.Fatal("the secret is in the address")
	}
}

func TestExchangeTradesTheCodeAndReadsTheAccount(t *testing.T) {
	t.Parallel()

	srv := server(t, "token exchange", "user info")
	c, _ := connector(srv)
	cr := cred(srv, nil)
	cr.Secret = []byte(`{"client_id":"` + clientKey + `","client_secret":"` + clientSecret + `"}`)

	raw, err := c.Exchange(
		context.Background(),
		cr,
		"the-code",
		"the-verifier",
		"http://127.0.0.1:7878/social/callback",
	)
	if err != nil {
		t.Fatal(err)
	}

	if calls := srv.Calls(); !slices.Equal(calls, []string{"POST /v2/oauth/token/", "GET /v2/user/info/"}) {
		t.Fatalf("calls = %v", calls)
	}

	f := srv.Seen()[0].Form
	if f.Get("client_key") != clientKey || f.Get("client_secret") != clientSecret || f.Get("code") != "the-code" ||
		f.Get("grant_type") != "authorization_code" || f.Get("code_verifier") != "the-verifier" ||
		f.Get("redirect_uri") != "http://127.0.0.1:7878/social/callback" || f.Has("client_id") {
		t.Fatalf("exchange form = %v", f)
	}

	info := srv.Seen()[1]
	if info.Query.Get("fields") != "open_id,display_name" || info.Header.Get("Authorization") != "Bearer "+accessToken {
		t.Fatalf("user info = %v %v", info.Query, info.Header)
	}

	acc, ok := c.SignedIn(raw)
	if !ok || acc.ID != "OPEN-ID-1" || acc.Title != "Creator Name" {
		t.Fatalf("account = %+v %v", acc, ok)
	}

	if strings.Contains(string(raw), accessToken) {
		t.Fatal("the access token was kept in the secret")
	}
}

func TestExchangeThatCannotBeUsedIsAuth(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, reason string
		pairs        []string
	}{
		{"refused", "invalid_request", []string{"token refused"}},
		{"no refresh token", "no_refresh_token", []string{"token exchange without refresh"}},
		{"the name cannot be read", ReasonTokenInvalid, []string{"token exchange", "user info refused"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := server(t, tc.pairs...)
			c, _ := connector(srv)
			cr := cred(srv, nil)
			cr.Secret = []byte(`{"client_id":"` + clientKey + `","client_secret":"` + clientSecret + `"}`)

			_, err := c.Exchange(context.Background(), cr, "code", "verifier", "http://127.0.0.1:7878/social/callback")
			asKind(t, err, social.KindAuth, tc.reason)

			for _, s := range canaries {
				if strings.Contains(err.Error(), s) {
					t.Errorf("%q is in the error", s)
				}
			}
		})
	}
}

func TestANewRefreshTokenIsKeptBeforeItIsUsed(t *testing.T) {
	t.Parallel()

	srv := server(t, "token refresh")
	c, _ := connector(srv)

	var rotated [][]byte

	tok, err := c.accessToken(context.Background(), cred(srv, &rotated))
	if err != nil || tok != accessToken {
		t.Fatalf("token = %q, %v", tok, err)
	}

	if len(rotated) != 1 {
		t.Fatalf("Rotate called %d times", len(rotated))
	}

	var kept secret

	_ = json.Unmarshal(rotated[0], &kept)

	if kept.RefreshToken != refreshTwo || kept.ClientSecret != clientSecret || kept.AccountID != "OPEN-ID-1" ||
		kept.ClientID != clientKey {
		t.Fatalf("the secret that was kept = %+v", kept)
	}
}

func TestWhenTheNewTokenCannotBeKeptTheSendStops(t *testing.T) {
	t.Parallel()

	srv := server(t, "token refresh")
	c, _ := connector(srv)
	cr := cred(srv, nil)
	cr.Rotate = func(func([]byte) ([]byte, error)) error { return errors.New("the vault is locked") }

	_, err := c.Publish(context.Background(), cr, newUpload(t))
	asKind(t, err, social.KindUnavailable, "vault_write_failed")

	if slices.Contains(srv.Calls(), "POST /v2/post/publish/inbox/video/init/") {
		t.Fatal("a request was made although the new sign-in was not kept")
	}
}

func TestTwoSendsAtOnceRefreshOneAfterTheOther(t *testing.T) {
	t.Parallel()

	srv := server(t, "token refresh", "token refresh third")
	c, _ := connector(srv)

	var (
		wg   sync.WaitGroup
		last []byte
	)

	// One vault for both sends: the second writes over what the first wrote.
	shared := vault([]byte(signedIn), func(s []byte) { last = bytes.Clone(s) })

	for range 2 {
		cr := cred(srv, nil)
		cr.Rotate = shared

		wg.Go(func() {
			if _, err := c.accessToken(context.Background(), cr); err != nil {
				t.Error(err)
			}
		})
	}

	wg.Wait()

	presented := []string{srv.Seen()[0].Form.Get("refresh_token"), srv.Seen()[1].Form.Get("refresh_token")}
	if presented[0] != refreshOne || presented[1] != refreshTwo {
		t.Fatalf("the refresh tokens presented = %v, want the second to present the token the first got", presented)
	}

	// The vault ends with the newest token: a lost write would strand the sign-in, since TikTok refuses a
	// refresh token it already replaced.
	var s secret
	if json.Unmarshal(last, &s) != nil || s.RefreshToken != "REFRESH-TOKEN-three-canary" {
		t.Fatalf("the vault ends with %s", last)
	}
}

// A sign-in done again while a send was refreshing is newer than the token of that send, and is kept.
func TestANewRefreshTokenDoesNotOverwriteANewSignIn(t *testing.T) {
	t.Parallel()

	srv := server(t, "token refresh")
	c, _ := connector(srv)
	cr := cred(srv, nil)
	again := strings.Replace(signedIn, refreshOne, "REFRESH-TOKEN-of-a-new-sign-in", 1)
	wrote := 0

	cr.Rotate = vault([]byte(again), func([]byte) { wrote++ })

	if _, err := c.accessToken(context.Background(), cr); err != nil {
		t.Fatal(err)
	}

	if wrote != 0 {
		t.Fatalf("the token of the earlier sign-in was written over the new one %d times", wrote)
	}
}

func TestARefreshThatIsRefusedIsAuth(t *testing.T) {
	t.Parallel()

	srv := server(t, "token invalid grant")
	c, _ := connector(srv)

	_, err := c.Publish(context.Background(), cred(srv, nil), newUpload(t))
	asKind(t, err, social.KindAuth, "invalid_grant")
}

func TestTestPostsNothing(t *testing.T) {
	t.Parallel()

	srv := server(t, "token refresh unchanged")
	c, _ := connector(srv)

	res, err := c.Test(context.Background(), cred(srv, nil))
	if err != nil || !res.OK {
		t.Fatalf("test = %+v, %v", res, err)
	}

	if calls := srv.Calls(); !slices.Equal(calls, []string{"POST /v2/oauth/token/"}) {
		t.Fatalf("calls = %v", calls)
	}

	srv = server(t, "token refused")
	c, _ = connector(srv)

	if res, err = c.Test(context.Background(), cred(srv, nil)); err != nil || res.OK ||
		!strings.Contains(res.Message, social.KindAuth) {
		t.Fatalf("test of a refused sign-in = %+v, %v", res, err)
	}
}

func TestAccountMakesNoRequest(t *testing.T) {
	t.Parallel()

	srv := server(t)
	c, _ := connector(srv)

	acc, err := c.Account(context.Background(), cred(srv, nil))
	if err != nil || acc.ID != "OPEN-ID-1" || acc.Title != "Creator Name" {
		t.Fatalf("account = %+v, %v", acc, err)
	}

	if len(srv.Calls()) != 0 {
		t.Fatalf("calls = %v", srv.Calls())
	}
}

func TestStatusMapsTheStatesOfTikTok(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		pair   string
		state  publish.State
		reason string
	}{
		{"status inbox", publish.StateSentAsDraft, ""},
		{"status complete", publish.StateSentAsDraft, ""},
		{"status uploading", publish.StateProcessing, ""},
		{"status failed duration", publish.StateFailed, ReasonDuration},
	} {
		srv := server(t, "token refresh unchanged", tc.pair)
		c, _ := connector(srv)

		got, err := c.Status(context.Background(), cred(srv, nil), "v_inbox_file~v2.123456789")
		if err != nil || got.State != tc.state || got.Reason != tc.reason {
			t.Errorf("%s: %+v, %v", tc.pair, got, err)
		}
	}
}

func TestNoSecretReachesAnyErrorOrAddress(t *testing.T) {
	t.Parallel()

	srv := server(t, "token refresh", "init pending share limit")
	c, _ := connector(srv)

	_, err := c.Publish(context.Background(), cred(srv, nil), newUpload(t))
	if err == nil {
		t.Fatal("want an error")
	}

	for _, s := range canaries {
		if strings.Contains(err.Error(), s) {
			t.Errorf("%q is in the error", s)
		}
	}

	srv.NoSecretInAddress(t, canaries...)
}

func TestDescribeStatesWhatTheDraftAllows(t *testing.T) {
	t.Parallel()

	d := New(Config{}).Describe()

	if d.Network != "tiktok" || d.AIDisclosure || !d.Draft || d.Price != nil || d.PostCostMicros != 0 ||
		!slices.Equal(d.Publishes, []string{"video"}) {
		t.Fatalf("describe = %+v", d)
	}

	if !slices.Equal(d.Hosts, []string{"open.tiktokapis.com", "open-upload.tiktokapis.com"}) {
		t.Fatalf("hosts = %v", d.Hosts)
	}

	// Nothing a draft cannot have is offered: only private, no caption, and the 5 drafts of the page.
	l := d.Limits
	if !slices.Equal(l.Visibilities, []publish.Visibility{publish.Private}) || !l.NoCaption || l.DailyPosts != 5 ||
		len(l.ThumbnailFormats) != 0 {
		t.Fatalf("limits = %+v", l)
	}

	if !slices.Equal(d.Scopes, []string{"user.info.basic", "video.upload"}) {
		t.Fatalf("scopes = %v", d.Scopes)
	}

	for _, kind := range []string{
		social.KindQuota, social.KindAuth, social.KindNotEligible, social.KindInvalid, social.KindRejected, social.KindUnavailable,
	} {
		if d.Hints[kind] == "" {
			t.Errorf("no hint for %s", kind)
		}
	}
}

func TestValidateRefusesAPublicPost(t *testing.T) {
	t.Parallel()

	p := publish.Post{
		Output: publish.File{Bytes: 5_000_000, Format: "mp4", Media: &publish.Media{
			Width: 720, Height: 1280, FPSNum: 30, FPSDen: 1, DurationMS: 30_000, VideoCodec: "h264", AudioCodec: "aac",
		}},
		Title: "A clip", Visibility: publish.Public,
	}

	issues := New(Config{}).Validate(p)
	if len(issues) != 1 || issues[0].Path != "visibility" || issues[0].Code != publish.CodeNotAllowed {
		t.Fatalf("issues = %v", issues)
	}
}

func TestTheConnectorRefusesAnythingButItsOwnSecret(t *testing.T) {
	t.Parallel()

	c := New(Config{})

	for _, raw := range []string{`{}`, `not json`, `{"client_id":"a"}`} {
		if _, ok := c.ClientID([]byte(raw)); ok {
			t.Errorf("%q was taken for a client", raw)
		}

		if _, ok := c.SignedIn([]byte(raw)); ok {
			t.Errorf("%q was taken for a sign-in", raw)
		}
	}

	if _, err := c.Begin(" ", "x"); err == nil {
		t.Error("a client with no key was accepted")
	}

	raw, err := c.Begin(" key ", " secret ")
	if err != nil {
		t.Fatal(err)
	}

	if id, ok := c.ClientID(raw); !ok || id != "key" {
		t.Fatalf("client key = %q", id)
	}

	if _, ok := c.SignedIn(raw); ok {
		t.Fatal("a connection with no refresh token is not signed in")
	}
}
