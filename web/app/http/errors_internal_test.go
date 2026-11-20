package httpapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"

	"github.com/AlexGalhardo/vukem/errs"
	"github.com/AlexGalhardo/vukem/web"
)

func TestClientClosedIsNotAServerStatus(t *testing.T) {
	t.Parallel()

	he := httpError(&service.Error{Code: service.CodeClientClosed, Message: "closed"})
	if he.Status != service.StatusClientClosed || errs.Status(he) >= 500 {
		t.Errorf("status = %d, want 499 (vukem logs 5xx at ERROR)", he.Status)
	}
}

// VV-216: an error a handler returns after the client gave up is not a server
// fault, whatever code path raised it.
func TestClientGoneIsMappedTo499(t *testing.T) {
	t.Parallel()

	s := &web.Services{}

	gone, cancel := context.WithCancel(context.Background())
	cancel()

	live := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	closed := httptest.NewRequest(http.MethodGet, "/", http.NoBody).WithContext(gone)

	cases := []struct {
		name string
		r    *http.Request
		err  error
		want int
	}{
		{"cancelled request, raw context error", closed, fmt.Errorf("read frame: %w", context.Canceled), 499},
		{"cancelled request, deadline of the request", closed, context.DeadlineExceeded, 499},
		{"live request, a context error is a real fault", live, context.Canceled, 500},
		{"cancelled request, another error stays an error", closed, errors.New("disk failure"), 500},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c := web.NewCtx(httptest.NewRecorder(), tc.r, s)

			got := http.StatusInternalServerError

			if mapped, ok := clientGone(c, tc.err); ok {
				got = errs.Status(mapped)
			}

			if got != tc.want {
				t.Errorf("status = %d, want %d", got, tc.want)
			}
		})
	}
}

// VV-216: the mapper is installed where vukem renders the errors of a handler,
// so a raw context error of a request the client closed is answered and logged
// as 499, below the error level, and a service error keeps its own status.
func TestInstalledErrorsAnswerAClosedRequestWith499(t *testing.T) {
	t.Parallel()

	var logged bytes.Buffer

	s := &web.Services{Log: slog.New(slog.NewTextHandler(&logged, &slog.HandlerOptions{Level: slog.LevelDebug}))}
	installErrors(s)

	gone, cancel := context.WithCancel(context.Background())
	cancel()

	cases := []struct {
		name string
		err  error
		want int
	}{
		{"raw context error", fmt.Errorf("read plan: %w", context.Canceled), service.StatusClientClosed},
		{
			"service error wins",
			&service.Error{Code: service.CodeRateLimited, Message: "slow down"},
			http.StatusTooManyRequests,
		},
	}

	for _, tc := range cases {
		rec := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/x", http.NoBody).WithContext(gone)
		r.Header.Set("Accept", "application/json")

		web.DefaultErrorHandler(web.NewCtx(rec, r, s), tc.err)

		if rec.Code != tc.want {
			t.Errorf("%s: status = %d, want %d", tc.name, rec.Code, tc.want)
		}
	}

	if strings.Contains(logged.String(), "level=ERROR") {
		t.Errorf("a closed request was logged at error level:\n%s", logged.String())
	}
}
