package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AlexGalhardo/vukem/ratelimit"

	"github.com/AlexGalhardo/vukem-video/service"
	"github.com/AlexGalhardo/vukem-video/web/app/http/middleware"
)

type broken struct{}

func (broken) Allow(context.Context, string) (ratelimit.Result, error) {
	return ratelimit.Result{}, errors.New("the store is down")
}

// A limiter whose store fails denies the request.
func TestLimiterFailsClosed(t *testing.T) {
	t.Parallel()

	r := httptest.NewRequest(http.MethodGet, "/", http.NoBody)

	e := middleware.Allow(r, broken{}, "k")
	if e == nil || e.Code != service.CodeRateLimited || !e.Retryable {
		t.Errorf("Allow over a failing store = %+v, want E_RATE_LIMITED", e)
	}

	f := middleware.NewFailedAuth(broken{}, nil)
	if f.Record(r, "ip:1") == nil {
		t.Error("a failing store let a failed authentication through")
	}

	if f.Blocked("ip:1") == nil {
		t.Error("the address was not remembered as refused")
	}
}

// P4-SRV-003: a refusal names the limit, what was counted and the ceiling (SDD-007 section 9.1).
func TestDeniedRequestsCarryTheLimitDetails(t *testing.T) {
	t.Parallel()

	r := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	l := ratelimit.NewMemory(2, time.Minute)

	for range 2 {
		if e := middleware.Allow(r, l, "k"); e != nil {
			t.Fatalf("inside the budget: %v", e)
		}
	}

	check := func(what string, e *service.Error, limit string) {
		t.Helper()

		if e == nil || e.Details["limit"] != limit || e.Details["value"] != 2 || e.Details["ceiling"] != 2 ||
			e.Details["retry_after"] == nil {
			t.Errorf("%s = %+v, want limit %s, value 2, ceiling 2 and retry_after", what, e, limit)
		}
	}

	check("a request over the budget", middleware.Allow(r, l, "k"), "requests")

	f := middleware.NewFailedAuth(ratelimit.NewMemory(2, time.Minute), nil)
	for range 2 {
		if f.Record(r, "ip:1") != nil {
			t.Fatal("a failure inside the budget was refused")
		}
	}

	check("the third failed authentication", f.Record(r, "ip:1"), "failed_authentications")
	check("the address that was blocked", f.Blocked("ip:1"), "failed_authentications")
}

func TestFailedAuthRefusesUntilTheLimiterLetsGo(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	f := middleware.NewFailedAuth(ratelimit.NewMemory(2, time.Minute), func() time.Time { return now })
	r := httptest.NewRequest(http.MethodGet, "/", http.NoBody)

	for range 2 {
		if f.Record(r, "ip:1") != nil {
			t.Fatal("a failure inside the budget was refused")
		}
	}

	if e := f.Record(r, "ip:1"); e == nil || e.Details["retry_after"] == nil {
		t.Fatalf("the third failure = %+v, want a refusal with retry_after", e)
	}

	if f.Blocked("ip:1") == nil {
		t.Error("the address is not blocked")
	}

	if f.Blocked("ip:2") != nil {
		t.Error("another address is blocked")
	}

	now = now.Add(2 * time.Minute)

	if f.Blocked("ip:1") != nil {
		t.Error("the block outlived the window")
	}
}
