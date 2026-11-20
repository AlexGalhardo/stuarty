package httpapi_test

import (
	"net/http"
	"strings"
	"testing"
)

// SDD-020 section 21: the calendar listens to a stream, and the stream is an event stream for a person.
func TestPublicationsStreamOpensForACallerWithCredentials(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	c := h.stream("/api/v1/publications/events", nil)
	if c.status != http.StatusOK || !strings.HasPrefix(c.header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("status %d, content type %q", c.status, c.header.Get("Content-Type"))
	}
}

// A stream with no credentials is refused before it becomes a stream.
func TestPublicationsStreamNeedsCredentials(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	if res := h.do(req{path: "/api/v1/publications/events", noAuth: true}); res.status != http.StatusUnauthorized {
		t.Errorf("the stream with no credentials = %d, want 401", res.status)
	}
}
