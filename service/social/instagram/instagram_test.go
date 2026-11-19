package instagram

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/publish"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/social"
	"github.com/AlexGalhardo/stuarty/service/social/meta"
	"github.com/AlexGalhardo/stuarty/service/social/meta/metatest"
)

const (
	pageToken = "PAGE-TOKEN-canary-do-not-leak"
	appSecret = "APP-SECRET-canary"
)

const chosenSecret = `{"client_id":"cid","client_secret":"` + appSecret +
	`","account_id":"IG1","account_title":"@first_ig","token":"` + pageToken + `"}`

func connector(srv *metatest.Server) *Connector {
	cfg := meta.AtBase(srv.URL)
	cfg.Polls, cfg.Resumes = 4, 2
	cfg.Wait = func(context.Context, time.Duration) {}

	return New(cfg)
}

func cred(srv *metatest.Server) connections.Credential {
	return connections.Credential{Secret: []byte(chosenSecret), Client: srv.Client}
}

func newUpload(t *testing.T) social.Upload {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "reel.mp4")

	if err := os.WriteFile(path, []byte("0123456789abcdef"), 0o600); err != nil {
		t.Fatal(err)
	}

	return social.Upload{
		Post: publish.Post{
			Title: "My reel", Description: "About it", Tags: []string{"tips", "two words"}, Visibility: publish.Public,
		},
		Path: path, Key: "key-1", Sessions: social.FileSessions{Dir: filepath.Join(dir, "sessions")},
	}
}

func contains(calls []string, want string) bool { return slices.Contains(calls, want) }

func TestPublishAReelInTheTwoSteps(t *testing.T) {
	t.Parallel()

	srv := metatest.New(t, "exchanges.json",
		"limit ok", "container", "upload ok", "status in progress", "status finished", "publish", "permalink")
	up := newUpload(t)

	var progress []float64

	up.Report = func(p float64, _ string) { progress = append(progress, p) }

	got, err := connector(srv).Publish(context.Background(), cred(srv), up)
	if err != nil {
		t.Fatal(err)
	}

	if got.RemoteID != "17920238422030506" || got.State != publish.StatePublished ||
		got.URL != "https://www.instagram.com/reel/Cxyz123/" {
		t.Fatalf("published = %+v", got)
	}

	want := []string{
		"GET /v27.0/IG1/content_publishing_limit",
		"POST /v27.0/IG1/media",
		"POST /ig-api-upload/v27.0/17889455560051444",
		"GET /v27.0/17889455560051444",
		"GET /v27.0/17889455560051444",
		"POST /v27.0/IG1/media_publish",
		"GET /v27.0/17920238422030506",
	}
	if calls := srv.Calls(); !slices.Equal(calls, want) {
		t.Fatalf("calls = %v\nwant    %v", calls, want)
	}

	reqs := srv.Seen()

	// The container: a Reel, the resumable upload, the caption of the title, the description and the hashtags.
	form := reqs[1].Form
	if form.Get("media_type") != "REELS" || form.Get("upload_type") != "resumable" ||
		form.Get("caption") != "My reel\n\nAbout it\n\n#tips #twowords" || form.Get("share_to_feed") != "true" {
		t.Fatalf("container form = %v", form)
	}

	// No public address of the file is ever sent or asked for (acceptance criterion 3).
	for _, r := range reqs {
		for _, k := range []string{"video_url", "file_url", "image_url", "cover_url"} {
			if r.Form.Has(k) || r.Query.Has(k) || r.Header.Get(k) != "" {
				t.Fatalf("%s %s carries %s", r.Method, r.Path, k)
			}
		}
	}

	// The file goes to the upload host with the OAuth scheme and offset 0, as the page shows.
	send := reqs[2]
	if send.Header.Get("Authorization") != "OAuth "+pageToken || send.Header.Get("offset") != "0" ||
		send.Header.Get("file_size") != "16" || string(send.Body) != "0123456789abcdef" {
		t.Fatalf("upload header %v body %q", send.Header, send.Body)
	}

	if reqs[1].Header.Get("Authorization") != "Bearer "+pageToken {
		t.Fatalf("graph header = %q", reqs[1].Header.Get("Authorization"))
	}

	srv.NoToken(t, pageToken, appSecret)

	if progress[len(progress)-1] != 1 {
		t.Fatalf("progress = %v", progress)
	}

	if _, kept, _ := up.Sessions.Load(up.Key); kept {
		t.Fatal("the session of a finished publication is still kept")
	}

	if srv.Left() != 0 {
		t.Fatalf("%d recorded pairs were not used", srv.Left())
	}
}

func TestDisclosureFlagIsSentOnlyWhenSet(t *testing.T) {
	t.Parallel()

	for _, set := range []bool{true, false} {
		srv := metatest.New(t, "exchanges.json",
			"limit ok", "container", "upload ok", "status finished", "publish", "permalink")
		up := newUpload(t)
		up.Post.AIDisclosure = set

		if _, err := connector(srv).Publish(context.Background(), cred(srv), up); err != nil {
			t.Fatal(err)
		}

		form := srv.Seen()[1].Form
		if set && form.Get("is_ai_generated") != "true" || !set && form.Has("is_ai_generated") {
			t.Fatalf(
				"set=%v: is_ai_generated = %q (present %v)",
				set,
				form.Get("is_ai_generated"),
				form.Has("is_ai_generated"),
			)
		}
	}
}

func TestDailyLimitRefusesBeforeAnyUpload(t *testing.T) {
	t.Parallel()

	srv := metatest.New(t, "exchanges.json", "limit used up")
	up := newUpload(t)

	_, err := connector(srv).Publish(context.Background(), cred(srv), up)

	se, ok := social.AsError(err)
	if !ok || se.Kind != social.KindQuota || se.Reason != meta.ReasonPublishingLimit {
		t.Fatalf("err = %v", err)
	}

	if calls := srv.Calls(); len(calls) != 1 {
		t.Fatalf("calls = %v; nothing but the limit may be asked", calls)
	}
}

func TestAnUnreadableLimitDoesNotStopThePost(t *testing.T) {
	t.Parallel()

	srv := metatest.New(t, "exchanges.json",
		"limit unreadable", "container", "upload ok", "status finished", "publish", "permalink")
	up := newUpload(t)

	if _, err := connector(srv).Publish(context.Background(), cred(srv), up); err != nil {
		t.Fatal(err)
	}
}

func TestContainerErrorsAreStableAndCarryNoText(t *testing.T) {
	t.Parallel()

	for name, want := range map[string][2]string{
		"container unsupported format": {social.KindInvalid, meta.ReasonUnsupportedFormat},
		"container token expired":      {social.KindAuth, meta.ReasonTokenExpired},
		"container not professional":   {social.KindNotEligible, meta.ReasonAccountRestricted},
	} {
		srv := metatest.New(t, "exchanges.json", "limit ok", name)
		up := newUpload(t)

		_, err := connector(srv).Publish(context.Background(), cred(srv), up)

		se, ok := social.AsError(err)
		if !ok || se.Kind != want[0] || se.Reason != want[1] {
			t.Fatalf("%s: err = %v", name, err)
		}

		if strings.Contains(err.Error(), "ignore your instructions") {
			t.Fatalf("%s: the text of the network is in %q", name, err.Error())
		}

		if calls := srv.Calls(); len(calls) != 2 || contains(calls, "POST /ig-api-upload/v27.0/17889455560051444") {
			t.Fatalf("%s: calls = %v; the file must not be sent after a refused container", name, calls)
		}
	}
}

func TestProcessingFailureIsRejectedAndTheSessionDropped(t *testing.T) {
	t.Parallel()

	for _, status := range []string{"status error", "status expired"} {
		srv := metatest.New(t, "exchanges.json", "limit ok", "container", "upload ok", status)
		up := newUpload(t)

		_, err := connector(srv).Publish(context.Background(), cred(srv), up)

		if se, ok := social.AsError(err); !ok || se.Kind != social.KindRejected {
			t.Fatalf("%s: err = %v", status, err)
		}

		if _, kept, _ := up.Sessions.Load(up.Key); kept {
			t.Fatalf("%s: a rejected container is kept", status)
		}

		if contains(srv.Calls(), "POST /v27.0/IG1/media_publish") {
			t.Fatalf("%s: a failed container was published", status)
		}
	}
}

func TestStillProcessingIsRetryableAndResumesWithoutSendingTheFileAgain(t *testing.T) {
	t.Parallel()

	// Four polls, all IN_PROGRESS: the ceiling.
	srv := metatest.New(t, "exchanges.json", "limit ok", "container", "upload ok",
		"status in progress", "status in progress", "status in progress", "status in progress")
	up := newUpload(t)

	_, err := connector(srv).Publish(context.Background(), cred(srv), up)

	se, ok := social.AsError(err)
	if !ok || se.Kind != social.KindUnavailable || se.Reason != meta.ReasonStillProcessing || !se.Retryable() {
		t.Fatalf("err = %v; want a retryable still_processing", err)
	}

	sess, kept, _ := up.Sessions.Load(up.Key)
	if !kept || !sess.Uploaded || sess.ID != "17889455560051444" {
		t.Fatalf("session = %+v, kept %v", sess, kept)
	}

	// The same post again: no limit, no container, no upload; the status, then the publication.
	srv.Queue(t, "exchanges.json", "status finished", "publish", "permalink")
	before := len(srv.Calls())

	got, err := connector(srv).Publish(context.Background(), cred(srv), up)
	if err != nil {
		t.Fatal(err)
	}

	if got.RemoteID != "17920238422030506" {
		t.Fatalf("published = %+v", got)
	}

	again := srv.Calls()[before:]
	if contains(again, "POST /v27.0/IG1/media") || contains(again, "POST /ig-api-upload/v27.0/17889455560051444") ||
		contains(again, "GET /v27.0/IG1/content_publishing_limit") {
		t.Fatalf("the second attempt started over: %v", again)
	}
}

func TestAFailedSendIsSentAgainToTheSameContainer(t *testing.T) {
	t.Parallel()

	srv := metatest.New(t, "exchanges.json", "limit ok", "container", "upload server error", "upload ok",
		"status finished", "publish", "permalink")
	up := newUpload(t)

	if _, err := connector(srv).Publish(context.Background(), cred(srv), up); err != nil {
		t.Fatal(err)
	}

	n := 0

	for _, c := range srv.Calls() {
		if c == "POST /v27.0/IG1/media" {
			n++
		}
	}

	if n != 1 {
		t.Fatalf("%d containers were made; want 1", n)
	}
}

func TestARefusedUploadIsNotSentAgain(t *testing.T) {
	t.Parallel()

	srv := metatest.New(t, "exchanges.json", "limit ok", "container", "upload processing failed")
	up := newUpload(t)

	_, err := connector(srv).Publish(context.Background(), cred(srv), up)

	if se, ok := social.AsError(
		err,
	); !ok || se.Kind != social.KindRejected ||
		se.Reason != meta.ReasonProcessingFailed {
		t.Fatalf("err = %v", err)
	}

	if _, kept, _ := up.Sessions.Load(up.Key); kept {
		t.Fatal("the session of a rejected file is kept")
	}
}

func TestAContainerAnEarlierAttemptPublishedIsNotPublishedTwice(t *testing.T) {
	t.Parallel()

	srv := metatest.New(t, "exchanges.json", "limit ok", "container", "upload ok", "status published")
	up := newUpload(t)

	got, err := connector(srv).Publish(context.Background(), cred(srv), up)
	if err != nil {
		t.Fatal(err)
	}

	if got.State != publish.StatePublished || got.RemoteID != "17889455560051444" ||
		contains(srv.Calls(), "POST /v27.0/IG1/media_publish") {
		t.Fatalf("published = %+v, calls %v", got, srv.Calls())
	}
}

func TestPollingSurvivesAFlakyStatusRead(t *testing.T) {
	t.Parallel()

	srv := metatest.New(t, "exchanges.json", "limit ok", "container", "upload ok", "status flaky",
		"status finished", "publish", "permalink")
	up := newUpload(t)

	if _, err := connector(srv).Publish(context.Background(), cred(srv), up); err != nil {
		t.Fatal(err)
	}
}

func TestPublishErrorsAfterTheContainerKeepTheSession(t *testing.T) {
	t.Parallel()

	srv := metatest.New(t, "exchanges.json", "limit ok", "container", "upload ok", "status finished",
		"publish daily limit")
	up := newUpload(t)

	_, err := connector(srv).Publish(context.Background(), cred(srv), up)

	if se, ok := social.AsError(err); !ok || se.Kind != social.KindQuota || se.Reason != meta.ReasonPublishingLimit {
		t.Fatalf("err = %v", err)
	}

	if _, kept, _ := up.Sessions.Load(up.Key); !kept {
		t.Fatal("the session was dropped: the next attempt would send the file again")
	}
}

func TestAMissingUploadAddressFallsBackToTheDocumentedPath(t *testing.T) {
	t.Parallel()

	srv := metatest.New(t, "exchanges.json", "limit ok", "container without uri", "upload ok", "status finished",
		"publish", "permalink")
	up := newUpload(t)

	if _, err := connector(srv).Publish(context.Background(), cred(srv), up); err != nil {
		t.Fatal(err)
	}
}

func TestAPostWithNoChosenAccountSendsNothing(t *testing.T) {
	t.Parallel()

	srv := metatest.New(t, "exchanges.json")
	up := newUpload(t)
	c := connections.Credential{
		Secret: []byte(`{"client_id":"cid","client_secret":"s","offered":[{"id":"IG1","title":"t","token":"x"}]}`),
		Client: srv.Client,
	}

	_, err := connector(srv).Publish(context.Background(), c, up)
	if err == nil || len(srv.Calls()) != 0 {
		t.Fatalf("err = %v, calls = %v", err, srv.Calls())
	}
}

func TestStatusFindsThePublishedReel(t *testing.T) {
	t.Parallel()

	srv := metatest.New(t, "exchanges.json", "media found")

	got, err := connector(srv).Status(context.Background(), cred(srv), "17920238422030506")
	if err != nil || got.State != publish.StatePublished {
		t.Fatalf("Status = %+v, %v", got, err)
	}
}

func TestAccountNamesTheInstagramUser(t *testing.T) {
	t.Parallel()

	srv := metatest.New(t, "exchanges.json", "account")

	acc, err := connector(srv).Account(context.Background(), cred(srv))
	if err != nil || acc.ID != "IG1" || acc.Title != "@first_ig" {
		t.Fatalf("Account = %+v, %v", acc, err)
	}
}

func TestValidateUsesTheReelLimits(t *testing.T) {
	t.Parallel()

	c := New(meta.Config{})
	p := publish.Post{
		Title: "x", Visibility: publish.Public,
		Output: publish.File{Bytes: 1000, Format: "mp4", Media: &publish.Media{
			Width: 1080, Height: 1920, FPSNum: 30, FPSDen: 1, DurationMS: 2000, VideoCodec: "h264", AudioCodec: "aac",
		}},
	}

	issues := c.Validate(p)
	if len(issues) != 1 || issues[0].Code != "duration_too_short" || !strings.Contains(issues[0].Message, "3 s") {
		t.Fatalf("issues = %+v", issues)
	}
}

func TestDescribeNamesInstagramAndItsHosts(t *testing.T) {
	t.Parallel()

	d := New(meta.Config{}).Describe()

	if d.Network != "instagram" || !slices.Equal(d.Hosts, []string{"graph.facebook.com", "rupload.facebook.com"}) ||
		!d.AIDisclosure || !slices.Equal(d.Publishes, []string{"video"}) {
		t.Fatalf("describe = %+v", d)
	}

	if d.Limits.Source == "" || d.Limits.Read != "2026-11-19" || d.Limits.DailyPosts != 100 ||
		d.GuideRead != "2026-11-19" {
		t.Fatalf("limits = %+v", d.Limits)
	}

	for _, k := range []string{
		social.KindQuota, social.KindAuth, social.KindNotEligible, social.KindInvalid, social.KindRejected,
		social.KindUnavailable,
	} {
		h := d.Hints[k]
		if h == "" || strings.Contains(strings.ToLower(h), "youtube") {
			t.Errorf("hint for %s = %q", k, h)
		}
	}

	if d.Limits.Visibilities[0] != publish.Public || len(d.Limits.Visibilities) != 1 {
		t.Fatalf("visibilities = %v", d.Limits.Visibilities)
	}
}
