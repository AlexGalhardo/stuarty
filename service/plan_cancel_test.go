package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/plan"
)

// VV-216: a client that gave up while a window was planned is E_CLIENT_CLOSED, not an internal error.
func TestPlanCancelledByTheClientIsClientClosed(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	s := &svc{}

	err := s.registerBitmaps(ctx, &ProjectRow{}, nil, nil, plan.Options{}, &plan.WindowPlan{}, 0, 60)

	se, ok := err.(*Error) //nolint:errorlint // asError returns the concrete type
	if !ok || se.Code != CodeClientClosed || se.HTTPStatus() != StatusClientClosed {
		t.Fatalf("err = %v, want E_CLIENT_CLOSED", err)
	}

	if se.HTTPStatus() == http.StatusInternalServerError {
		t.Error("a client that gave up must not answer 500")
	}
}
