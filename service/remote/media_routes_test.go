package remote

import (
	"net/http"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
)

func TestRemoteMediaRequestsKeepPathQueryAndBodySeparate(t *testing.T) {
	t.Parallel()

	ctx := asCLI("cli")
	recording := service.RecordingRef{ID: "rec_a/b"}
	for _, tc := range []struct {
		name, method, path, query, body string
		call                            func(*Client) error
	}{
		{"pause", http.MethodPost, "/recordings/rec_a%2Fb/pause", "", "{}", func(c *Client) error {
			_, err := c.RecordingPause(ctx, recording)
			return err
		}},
		{"resume", http.MethodPost, "/recordings/rec_a%2Fb/resume", "", "{}", func(c *Client) error {
			_, err := c.RecordingResume(ctx, recording)
			return err
		}},
		{"events", http.MethodGet, "/recordings/rec_a%2Fb/events", "from_ms=100&kind=click&limit=5", "", func(c *Client) error {
			_, err := c.RecordingEvents(ctx, service.RecordingEventsRequest{ID: recording.ID, FromMS: 100, Kind: "click", Limit: 5})
			return err
		}},
		{"place", http.MethodPost, "/recordings/rec_a%2Fb/place", "", `{"timeline_id":"prj_b"}`, func(c *Client) error {
			_, err := c.RecordingPlace(ctx, service.RecordingPlaceRequest{ID: recording.ID, TimelineID: "prj_b"})
			return err
		}},
		{"upload chunk", http.MethodPut, "/recordings/rec_a%2Fb/upload/screen/3", "elapsed_ms=123&sha256=abcd", "frame", func(c *Client) error {
			_, err := c.RecordingUploadChunk(ctx, service.RecordingUploadChunkRequest{
				ID: recording.ID, Track: "screen", Index: 3, ElapsedMS: 123, SHA256: "abcd", Body: strings.NewReader("frame"), Size: 5,
			})
			return err
		}},
		{"finish", http.MethodPost, "/recordings/rec_a%2Fb/finish", "", `{"elapsed_ms":500}`, func(c *Client) error {
			_, err := c.RecordingUploadFinish(ctx, service.RecordingUploadFinishRequest{ID: recording.ID, ElapsedMS: 500})
			return err
		}},
		{"import image", http.MethodPost, "/images/import/file", "base_revision=2&dry_run=true&file_name=cover.png&height=640&image_id=img_a&wait_ms=250&width=360", "pixels", func(c *Client) error {
			_, err := c.ImageImportFile(ctx, service.ImageImportFileRequest{
				ImageID: "img_a", FileName: "cover.png", Width: 360, Height: 640, BaseRevision: 2, WaitMS: 250,
				DryRun: true, Body: strings.NewReader("pixels"), Size: 6,
			})
			return err
		}},
		{"export check", http.MethodGet, "/projects/prj_a/export-check", "codec=h264&end=5s&preset=youtube-shorts&start=1s&timeline_id=prj_b", "", func(c *Client) error {
			_, err := c.ExportCheck(ctx, service.ExportCheckRequest{
				ProjectID: "prj_a", TimelineID: "prj_b", Preset: "youtube-shorts", Start: "1s", End: "5s", Codec: "h264",
			})
			return err
		}},
		{"audio measure", http.MethodGet, "/projects/prj_a/audio-measure", "end=5s&item_id=itm_a&start=1s&timeline_id=prj_b", "", func(c *Client) error {
			_, err := c.AudioMeasure(ctx, service.AudioMeasureRequest{
				ProjectID: "prj_a", TimelineID: "prj_b", ItemID: "itm_a", Start: "1s", End: "5s",
			})
			return err
		}},
		{"plan", http.MethodGet, "/projects/prj_a/plan", "from=5&timeline_id=prj_b&until=10&width=320", "", func(c *Client) error {
			_, err := c.PlanWindow(ctx, service.PlanWindowRequest{ProjectID: "prj_a", TimelineID: "prj_b", From: 5, Until: 10, Width: 320})
			return err
		}},
		{"frame", http.MethodGet, "/projects/prj_a/frame", "at=1s&max_tokens=500&timeline_id=prj_b&width=320", "", func(c *Client) error {
			_, err := c.ViewFrame(ctx, service.ViewFrameRequest{ProjectID: "prj_a", TimelineID: "prj_b", At: "1s", MaxTokens: 500, Width: 320})
			return err
		}},
		{"sheet", http.MethodGet, "/projects/prj_a/sheet", "cols=2&count=4&from=1s&max_tokens=500&timeline_id=prj_b&until=5s", "", func(c *Client) error {
			_, err := c.ViewSheet(ctx, service.ViewSheetRequest{
				ProjectID: "prj_a", TimelineID: "prj_b", From: "1s", Until: "5s", Cols: 2, Count: 4, MaxTokens: 500,
			})
			return err
		}},
		{"export presets", http.MethodGet, "/export-presets", "codecs=true", "", func(c *Client) error {
			_, err := c.ExportPresetList(ctx, service.ExportPresetListRequest{Codecs: true})
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var got seen
			c := server(t, http.StatusOK, `{}`, &got)
			if err := tc.call(c); err != nil {
				t.Fatal(err)
			}
			if got.method != tc.method || got.path != "/api/v1"+tc.path || got.rawQuery != tc.query ||
				got.body != tc.body {
				t.Errorf("request = %s %s ?%s %q, want %s %s ?%s %q",
					got.method, got.path, got.rawQuery, got.body, tc.method, tc.path, tc.query, tc.body)
			}
		})
	}
}

func TestRemoteChatRequestsPreserveSessionAndTranscriptParameters(t *testing.T) {
	t.Parallel()
	var got seen
	c := server(t, http.StatusOK, `{}`, &got)
	ctx := asCLI("cli")
	ref := service.ChatRef{SessionID: "sess_a"}
	check := func(method, path, query, body string, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		if got.method != method || got.path != "/api/v1"+path || got.rawQuery != query || got.body != body {
			t.Fatalf("%s %s: request = %+v", method, path, got)
		}
	}
	_, err := c.AgentProfileSet(
		ctx,
		service.AgentProfileSetRequest{Name: "test-agent", Command: "agent", Args: []string{"serve"}},
	)
	check(http.MethodPut, "/agents/profiles/test-agent", "", `{"command":"agent","args":["serve"]}`, err)
	_, err = c.AgentProfileRemove(ctx, service.AgentProfileRemoveRequest{Name: "test-agent"})
	check(http.MethodDelete, "/agents/profiles/test-agent", "", "", err)
	_, err = c.ChatSessionStart(ctx, service.ChatSessionStartRequest{ProjectID: "prj_a", Profile: "test-agent"})
	check(http.MethodPost, "/projects/prj_a/chat/sessions", "", `{"profile":"test-agent"}`, err)
	_, err = c.ChatSend(ctx, service.ChatSendRequest{SessionID: ref.SessionID, Text: "hello"})
	check(http.MethodPost, "/chat/sessions/sess_a/messages", "", `{"text":"hello"}`, err)
	_, err = c.ChatCancel(ctx, service.ChatCancelRequest{SessionID: ref.SessionID, End: true})
	check(http.MethodPost, "/chat/sessions/sess_a/cancel", "", `{"end":true}`, err)
	_, err = c.ChatAnswerPermission(
		ctx,
		service.ChatAnswerRequest{SessionID: ref.SessionID, RequestID: "req_1", Answer: "reject"},
	)
	check(http.MethodPost, "/chat/sessions/sess_a/permissions/req_1", "", `{"answer":"reject"}`, err)
	_, err = c.ChatSessionList(ctx, service.ChatSessionListRequest{ProjectID: "prj_a"})
	check(http.MethodGet, "/projects/prj_a/chat/sessions", "", "", err)
	_, err = c.ChatSessionGet(ctx, ref)
	check(http.MethodGet, "/chat/sessions/sess_a", "", "", err)
	_, err = c.ChatActivityGet(ctx, ref)
	check(http.MethodGet, "/chat/sessions/sess_a/activity", "", "", err)
	_, err = c.ChatTranscriptGet(
		ctx,
		service.ChatTranscriptRequest{SessionID: ref.SessionID, After: 3, Limit: 5, Format: "markdown"},
	)
	check(http.MethodGet, "/chat/sessions/sess_a/transcript", "after=3&format=markdown&limit=5", "", err)
	_, err = c.ChatSessionDelete(ctx, ref)
	check(http.MethodDelete, "/chat/sessions/sess_a", "", "", err)
	_, err = c.InboxList(ctx, service.InboxListRequest{})
	check(http.MethodGet, "/inbox", "", "", err)
}
