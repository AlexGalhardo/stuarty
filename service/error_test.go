package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
)

// A browser that aborts a truth frame, or a deadline that passes, is the
// caller's doing: it must not look like a server failure (status 499, not 500).
func TestContextEndingIsAClientClosedOutcome(t *testing.T) {
	t.Parallel()

	for name, cause := range map[string]error{
		"canceled":         context.Canceled,
		"deadline":         context.DeadlineExceeded,
		"wrapped canceled": fmt.Errorf("frame: %w", context.Canceled),
	} {
		for from, got := range map[string]error{"asError": asError(cause), "viewError": viewError(cause)} {
			se, ok := errors.AsType[*Error](got)
			if !ok || se.Code != CodeClientClosed {
				t.Fatalf("%s/%s: got %v, want %s", name, from, got, CodeClientClosed)
			}

			if se.HTTPStatus() != StatusClientClosed || se.HTTPStatus() >= http.StatusInternalServerError {
				t.Errorf("%s/%s: status = %d", name, from, se.HTTPStatus())
			}

			if se.Retryable {
				t.Errorf("%s/%s: a closed request is not retryable by itself", name, from)
			}
		}
	}

	if se, _ := errors.AsType[*Error](asError(errors.New("boom"))); se == nil || se.Code != "E_INTERNAL" {
		t.Errorf("an unknown error must stay E_INTERNAL, got %v", se)
	}
}
