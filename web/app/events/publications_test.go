package events

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/service"
	wire "github.com/AlexGalhardo/stuarty/service/events"
	pub "github.com/AlexGalhardo/stuarty/service/publish"

	"github.com/AlexGalhardo/vukem/sse"
)

// SDD-020 section 29 and SDD-007: in network mode the stream of a member carries their own publications
// only; the stream of an administrator and of local mode carries every one.
func TestPublicationsStreamOfAMemberCarriesTheirOwnOnly(t *testing.T) {
	t.Parallel()

	e := New(nil, Options{})
	t.Cleanup(e.Close)

	ctx := context.Background()

	subscribe := func(owner string) <-chan sse.Event {
		t.Helper()

		live, unsub, err := e.hub.TrySubscribe(ctx, publicationsTopicOf(owner))
		if err != nil {
			t.Fatal(err)
		}

		t.Cleanup(unsub)

		return live
	}

	all, first, second := subscribe(""), subscribe("usr_first"), subscribe("usr_second")

	p := pub.Publication{ID: "pub_0000000001", ProjectID: "prj_0000000001", Title: "The first one's post"}
	e.Publish(service.Event{Type: wire.PublicationUpdated, ProjectID: p.ProjectID, Publication: &p, Owner: "usr_first"})

	for name, ch := range map[string]<-chan sse.Event{"every publication": all, "its owner": first} {
		select {
		case ev := <-ch:
			if ev.Event != wire.PublicationUpdated || !strings.Contains(ev.Data, p.ID) {
				t.Errorf("the stream of %s got %+v", name, ev)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("the stream of %s got nothing", name)
		}
	}

	select {
	case ev := <-second:
		t.Fatalf("another member's stream got a publication of the first: %+v", ev)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestProjectSubscribersReceiveAssetAndNestedTimelineChanges(t *testing.T) {
	t.Parallel()
	e := New(nil, Options{})
	t.Cleanup(e.Close)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	const project = "prj_0000000001"
	live, unsubscribe, err := e.hub.TrySubscribe(ctx, topic(project))
	if err != nil {
		t.Fatal(err)
	}
	defer unsubscribe()
	for _, tc := range []struct {
		ev   service.Event
		want string
	}{
		{service.Event{Type: wire.AssetUpdated, ProjectID: project, AssetID: "ast_0000000001"}, `"asset_id":"ast_0000000001"`},
		{service.Event{Type: wire.NestedUpdated, ProjectID: project, TimelineID: "prj_0000000002"}, `"timeline_id":"prj_0000000002"`},
	} {
		e.Publish(tc.ev)
		select {
		case got := <-live:
			if got.Event != tc.ev.Type || !strings.Contains(got.Data, tc.want) {
				t.Fatalf("got %+v, want %s with %s", got, tc.ev.Type, tc.want)
			}
		case <-ctx.Done():
			t.Fatalf("no %s event arrived", tc.ev.Type)
		}
	}
}

func TestInvalidStoredJobErrorsAreOmittedFromEvents(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{"invalid json", `{"message":"error without a code"}`} {
		if err := errorOf([]byte(raw)); err != nil {
			t.Fatalf("errorOf(%q) = %+v", raw, err)
		}
	}
}
