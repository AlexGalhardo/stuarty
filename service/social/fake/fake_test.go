package fake

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/publish"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/social"
)

func TestConnectorReportsProcessingAndChecksItsEndpoint(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ping", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	mux.HandleFunc("POST /videos", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"id":"video-1","state":"processing"}`)
	})
	mux.HandleFunc("GET /videos/video-1", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"state":"published"}`)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	connector := New(server.URL)
	credential := connections.Credential{
		Client: server.Client(),
		Secret: []byte(`{"refresh_token":"fake-refresh-token","account_id":"acct-1","account_title":"Test account"}`),
	}
	ctx := t.Context()
	if _, err := connector.Account(ctx, connections.Credential{}); err == nil {
		t.Fatal("Account accepted a missing sign-in")
	}
	if account, err := connector.Account(ctx, credential); err != nil || account.ID != "acct-1" {
		t.Fatalf("Account = %+v, %v", account, err)
	}
	if _, err := connector.Begin("client", ""); err == nil {
		t.Fatal("Begin accepted a missing secret")
	}
	if probe, err := connector.Test(
		ctx,
		credential,
	); err != nil || probe.OK ||
		probe.Status != http.StatusServiceUnavailable {
		t.Fatalf("Test = %+v, %v", probe, err)
	}
	post := publish.Post{Title: "Test video", AIDisclosure: true}
	if result, err := connector.Publish(ctx, credential, social.Upload{Post: post}); err != nil ||
		result.State != publish.StateProcessing || result.RemoteID != "video-1" {
		t.Fatalf("Publish = %+v, %v", result, err)
	}
	if state, err := connector.Status(ctx, credential, "video-1"); err != nil || state.State != publish.StatePublished {
		t.Fatalf("Status = %+v, %v", state, err)
	}
	if !slices.Equal(connector.Uploaded(), []string{post.Title}) || !slices.Equal(connector.Disclosed(), []bool{true}) {
		t.Fatal("the recording of the upload does not match the sent post")
	}
}
