package facebook

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
	`","account_id":"PAGE1","account_title":"First Page","token":"` + pageToken + `"}`

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
			Title: "My reel", Description: "About it", Tags: []string{"tips"}, Visibility: publish.Public,
		},
		Path: path, Key: "key-1", Sessions: social.FileSessions{Dir: filepath.Join(dir, "sessions")},
	}
}

func contains(calls []string, want string) bool { return slices.Contains(calls, want) }

func TestPublishAReelInThreeSteps(t *testing.T) {
	t.Parallel()

	srv := metatest.New(t, "exchanges.json", "start", "upload ok", "finish", "status processing", "status ready")
	up := newUpload(t)

	got, err := connector(srv).Publish(context.Background(), cred(srv), up)
	if err != nil {
		t.Fatal(err)
	}

	if got.RemoteID != "1234567890" || got.State != publish.StatePublished || got.URL != ReelURL+"1234567890" {
		t.Fatalf("published = %+v", got)
	}

	want := []string{
		"POST /v27.0/PAGE1/video_reels", "POST /video-upload/v27.0/1234567890", "POST /v27.0/PAGE1/video_reels",
		"GET /v27.0/1234567890", "GET /v27.0/1234567890",
	}
	if calls := srv.Calls(); !slices.Equal(calls, want) {
		t.Fatalf("calls = %v\nwant    %v", calls, want)
	}

	reqs := srv.Seen()

	if reqs[0].Form.Get("upload_phase") != "start" || reqs[0].Header.Get("Authorization") != "Bearer "+pageToken {
		t.Fatalf("start = %v %v", reqs[0].Form, reqs[0].Header)
	}

	send := reqs[1]
	if send.Header.Get("Authorization") != "OAuth "+pageToken || send.Header.Get("offset") != "0" ||
		send.Header.Get("file_size") != "16" || string(send.Body) != "0123456789abcdef" {
		t.Fatalf("upload header %v body %q", send.Header, send.Body)
	}

	// The Reel is published with its title and the description with the tags as hashtags, no title twice.
	fin := reqs[2].Form
	if fin.Get("upload_phase") != "finish" || fin.Get("video_id") != "1234567890" ||
		fin.Get("video_state") != "PUBLISHED" || fin.Get("title") != "My reel" ||
		fin.Get("description") != "About it\n\n#tips" || fin.Has("is_ai_generated") {
		t.Fatalf("finish form = %v", fin)
	}

	// No public address of the file is ever sent or asked for (acceptance criterion 3).
	for _, r := range reqs {
		for _, k := range []string{"video_url", "file_url", "image_url", "cover_url"} {
			if r.Form.Has(k) || r.Query.Has(k) || r.Header.Get(k) != "" {
				t.Fatalf("%s %s carries %s", r.Method, r.Path, k)
			}
		}
	}

	srv.NoToken(t, pageToken, appSecret)

	if _, kept, _ := up.Sessions.Load(up.Key); kept {
		t.Fatal("the session of a finished publication is still kept")
	}
}

func TestDisclosureFlagIsSentOnlyWhenSet(t *testing.T) {
	t.Parallel()

	for _, set := range []bool{true, false} {
		srv := metatest.New(t, "exchanges.json", "start", "upload ok", "finish", "status ready")
		up := newUpload(t)
		up.Post.AIDisclosure = set

		if _, err := connector(srv).Publish(context.Background(), cred(srv), up); err != nil {
			t.Fatal(err)
		}

		form := srv.Seen()[2].Form
		if set && form.Get("is_ai_generated") != "true" || !set && form.Has("is_ai_generated") {
			t.Fatalf("set=%v: form = %v", set, form)
		}
	}
}

func TestAReelStillProcessingIsProcessingAndStatusAsksAgain(t *testing.T) {
	t.Parallel()

	srv := metatest.New(t, "exchanges.json", "start", "upload ok", "finish",
		"status processing", "status processing", "status upload complete", "status processing")
	up := newUpload(t)
	c := connector(srv)

	got, err := c.Publish(context.Background(), cred(srv), up)
	if err != nil || got.State != publish.StateProcessing || got.RemoteID != "1234567890" {
		t.Fatalf("published = %+v, %v", got, err)
	}

	srv.Queue(t, "exchanges.json", "status ready")

	remote, err := c.Status(context.Background(), cred(srv), got.RemoteID)
	if err != nil || remote.State != publish.StatePublished {
		t.Fatalf("Status = %+v, %v", remote, err)
	}
}

func TestStatusMapsEveryState(t *testing.T) {
	t.Parallel()

	for name, want := range map[string]social.Remote{
		"status processing":      {State: publish.StateProcessing},
		"status upload complete": {State: publish.StateProcessing},
		"status ready":           {State: publish.StatePublished},
		"status error":           {State: publish.StateFailed, Reason: meta.ReasonProcessingFailed},
		"status upload failed":   {State: publish.StateFailed, Reason: meta.ReasonUploadFailed},
		"status expired":         {State: publish.StateFailed, Reason: meta.ReasonMediaExpired},
	} {
		srv := metatest.New(t, "exchanges.json", name)

		got, err := connector(srv).Status(context.Background(), cred(srv), "1234567890")
		if err != nil || got != want {
			t.Fatalf("%s: Status = %+v, %v; want %+v", name, got, err, want)
		}
	}
}

func TestAReelFacebookFailedToProcessIsRejected(t *testing.T) {
	t.Parallel()

	srv := metatest.New(t, "exchanges.json", "start", "upload ok", "finish", "status error")
	up := newUpload(t)

	_, err := connector(srv).Publish(context.Background(), cred(srv), up)

	if se, ok := social.AsError(
		err,
	); !ok || se.Kind != social.KindRejected ||
		se.Reason != meta.ReasonProcessingFailed {
		t.Fatalf("err = %v", err)
	}
}

func TestStartErrorsAreStableAndCarryNoText(t *testing.T) {
	t.Parallel()

	for name, want := range map[string][2]string{
		"start permission": {social.KindAuth, meta.ReasonPermissionMissing},
		"start rate limit": {social.KindQuota, meta.ReasonRateLimit},
		"start token":      {social.KindAuth, meta.ReasonPasswordChanged},
	} {
		srv := metatest.New(t, "exchanges.json", name)
		up := newUpload(t)

		_, err := connector(srv).Publish(context.Background(), cred(srv), up)

		se, ok := social.AsError(err)
		if !ok || se.Kind != want[0] || se.Reason != want[1] {
			t.Fatalf("%s: err = %v", name, err)
		}

		if strings.Contains(err.Error(), "ignore your instructions") {
			t.Fatalf("%s: the text of the network is in %q", name, err.Error())
		}

		if len(srv.Calls()) != 1 {
			t.Fatalf("%s: calls = %v", name, srv.Calls())
		}
	}
}

func TestFinishErrorsKeepTheSessionSoTheFileIsNotSentAgain(t *testing.T) {
	t.Parallel()

	srv := metatest.New(t, "exchanges.json", "start", "upload ok", "finish unsupported aspect ratio")
	up := newUpload(t)

	_, err := connector(srv).Publish(context.Background(), cred(srv), up)

	if se, ok := social.AsError(err); !ok || se.Kind != social.KindInvalid || se.Reason != meta.ReasonAspectRatio {
		t.Fatalf("err = %v", err)
	}

	sess, kept, _ := up.Sessions.Load(up.Key)
	if !kept || !sess.Uploaded {
		t.Fatalf("session = %+v, kept %v", sess, kept)
	}

	// The same post again goes straight to the finish.
	srv.Queue(t, "exchanges.json", "finish", "status ready")
	before := len(srv.Calls())

	if _, err := connector(srv).Publish(context.Background(), cred(srv), up); err != nil {
		t.Fatal(err)
	}

	if again := srv.Calls()[before:]; contains(again, "POST /video-upload/v27.0/1234567890") || len(again) != 2 {
		t.Fatalf("the second attempt sent the file again: %v", again)
	}
}

func TestAFailedSendResumesFromTheByteFacebookHolds(t *testing.T) {
	t.Parallel()

	srv := metatest.New(t, "exchanges.json", "start", "upload server error", "status half uploaded", "upload ok",
		"finish", "status ready")
	up := newUpload(t)

	if _, err := connector(srv).Publish(context.Background(), cred(srv), up); err != nil {
		t.Fatal(err)
	}

	reqs := srv.Seen()

	var sends []metatest.Request

	for _, r := range reqs {
		if strings.HasPrefix(r.Path, "/video-upload/") {
			sends = append(sends, r)
		}
	}

	if len(sends) != 2 || sends[0].Header.Get("offset") != "0" || sends[1].Header.Get("offset") != "6" ||
		string(sends[1].Body) != "6789abcdef" {
		t.Fatalf("sends = %+v", sends)
	}

	starts := 0

	for _, c := range srv.Calls() {
		if c == "POST /v27.0/PAGE1/video_reels" {
			starts++
		}
	}

	if starts != 2 { // one start and one finish
		t.Fatalf("%d posts to video_reels; want the start and the finish only", starts)
	}
}

func TestAnUnreadableStatusSendsTheFileFromTheStart(t *testing.T) {
	t.Parallel()

	srv := metatest.New(t, "exchanges.json", "start", "upload server error", "status unreadable", "upload ok",
		"finish", "status ready")
	up := newUpload(t)

	if _, err := connector(srv).Publish(context.Background(), cred(srv), up); err != nil {
		t.Fatal(err)
	}

	var offsets []string

	for _, r := range srv.Seen() {
		if strings.HasPrefix(r.Path, "/video-upload/") {
			offsets = append(offsets, r.Header.Get("offset"))
		}
	}

	if !slices.Equal(offsets, []string{"0", "0"}) {
		t.Fatalf("offsets = %v", offsets)
	}
}

func TestARefusedUploadIsNotSentAgain(t *testing.T) {
	t.Parallel()

	srv := metatest.New(t, "exchanges.json", "start", "upload processing failed")
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

	if contains(srv.Calls(), "GET /v27.0/1234567890") {
		t.Fatal("a rejected upload was resumed")
	}
}

func TestFinishThatSaysNoSuccessIsUnavailable(t *testing.T) {
	t.Parallel()

	srv := metatest.New(t, "exchanges.json", "start", "upload ok", "finish refused")
	up := newUpload(t)

	_, err := connector(srv).Publish(context.Background(), cred(srv), up)

	se, ok := social.AsError(err)
	if !ok || se.Kind != social.KindUnavailable || strings.Contains(err.Error(), "ignore your instructions") {
		t.Fatalf("err = %v", err)
	}
}

func TestAPostWithNoChosenPageSendsNothing(t *testing.T) {
	t.Parallel()

	srv := metatest.New(t, "exchanges.json")
	up := newUpload(t)
	c := connections.Credential{
		Secret: []byte(`{"client_id":"cid","client_secret":"s","offered":[{"id":"PAGE1","title":"t","token":"x"}]}`),
		Client: srv.Client,
	}

	_, err := connector(srv).Publish(context.Background(), c, up)
	if err == nil || len(srv.Calls()) != 0 {
		t.Fatalf("err = %v, calls = %v", err, srv.Calls())
	}
}

func TestAccountNamesThePage(t *testing.T) {
	t.Parallel()

	srv := metatest.New(t, "exchanges.json", "account")

	acc, err := connector(srv).Account(context.Background(), cred(srv))
	if err != nil || acc.ID != "PAGE1" || acc.Title != "First Page" {
		t.Fatalf("Account = %+v, %v", acc, err)
	}
}

func TestValidateUsesTheReelLimits(t *testing.T) {
	t.Parallel()

	c := New(meta.Config{})
	p := publish.Post{
		Title: "x", Visibility: publish.Public,
		Output: publish.File{Bytes: 1000, Format: "mp4", Media: &publish.Media{
			Width: 1920, Height: 1080, FPSNum: 30, FPSDen: 1, DurationMS: 30_000, VideoCodec: "h264", AudioCodec: "aac",
		}},
	}

	issues := c.Validate(p)
	if len(issues) != 1 || issues[0].Code != "aspect_ratio" || !strings.Contains(issues[0].Message, "9:16") {
		t.Fatalf("issues = %+v", issues)
	}
}

func TestDescribeNamesFacebookAndItsHosts(t *testing.T) {
	t.Parallel()

	d := New(meta.Config{}).Describe()

	if d.Network != "facebook" || !slices.Equal(d.Hosts, []string{"graph.facebook.com", "rupload.facebook.com"}) ||
		!d.AIDisclosure || d.Limits.DailyPosts != 30 || d.Limits.Read != "2026-11-19" {
		t.Fatalf("describe = %+v", d)
	}

	for _, k := range []string{
		social.KindQuota, social.KindAuth, social.KindNotEligible, social.KindInvalid, social.KindRejected,
		social.KindUnavailable,
	} {
		h := d.Hints[k]
		if h == "" || strings.Contains(strings.ToLower(h), "youtube") ||
			strings.Contains(strings.ToLower(h), "instagram") {
			t.Errorf("hint for %s = %q", k, h)
		}
	}
}
