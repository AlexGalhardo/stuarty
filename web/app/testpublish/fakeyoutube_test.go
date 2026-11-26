package testpublish

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFakeYouTubeResumesAfterTheSenderLosesTheConnection(t *testing.T) {
	t.Parallel()
	network := NewFakeYouTube(t)
	const session = "/upload/session/0"
	open := httptest.NewRequest(http.MethodPost, "/upload/youtube/v3/videos", http.NoBody)
	open.Header.Set("X-Upload-Content-Length", "6")
	created := httptest.NewRecorder()
	network.serve(created, open)
	if created.Code != http.StatusOK || created.Header().Get("Location") != network.Srv.URL+session {
		t.Fatalf("upload session response = %d, %v", created.Code, created.Header())
	}
	put := func(ctx context.Context, path, contentRange, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequestWithContext(ctx, http.MethodPut, path, strings.NewReader(body))
		r.Header.Set("Content-Range", contentRange)
		w := httptest.NewRecorder()
		network.serve(w, r)
		return w
	}
	ctx := t.Context()
	if res := put(ctx, "/upload/session/9", "bytes */6", ""); res.Code != http.StatusNotFound {
		t.Fatalf("unknown upload = %d", res.Code)
	}
	if res := put(
		ctx,
		session,
		"bytes */6",
		"",
	); res.Code != http.StatusPermanentRedirect ||
		res.Header().Get("Range") != "" {
		t.Fatalf("initial offset = %d, %v", res.Code, res.Header())
	}
	if res := put(ctx, session, "bytes 2-5/6", "cdef"); res.Code != http.StatusBadRequest {
		t.Fatalf("upload starting at the wrong offset = %d", res.Code)
	}
	stalled := network.StallAt(3)
	interrupted, cancel := context.WithCancel(ctx)
	cancel()
	put(interrupted, session, "bytes 0-5/6", "abcdef")
	select {
	case <-stalled:
	default:
		t.Fatal("the fake network did not stall at the requested offset")
	}
	if res := put(
		ctx,
		session,
		"bytes */6",
		"",
	); res.Code != http.StatusPermanentRedirect ||
		res.Header().Get("Range") != "bytes=0-2" {
		t.Fatalf("saved offset = %d, %v", res.Code, res.Header())
	}
	finished := put(ctx, session, "bytes 3-5/6", "def")
	report := put(ctx, session, "bytes */6", "")
	if finished.Code != http.StatusOK || finished.Body.String() != report.Body.String() ||
		network.Received(0) != 6 || network.Finished() != 1 {
		t.Fatalf("completed upload = %d, %q, received %d bytes", finished.Code, report.Body, network.Received(0))
	}
}
