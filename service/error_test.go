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

// One row per code of SDD-000 section 8: the status the table gives it and the
// exit code of the CLI. A code that is not in the table is a 500.
func TestHTTPStatusTable(t *testing.T) {
	t.Parallel()

	rows := []struct {
		code   string
		status int
	}{
		{"E_VALIDATION", http.StatusBadRequest},
		{"E_INVALID_TIME", http.StatusBadRequest},
		{"E_UNKNOWN_COMMAND", http.StatusBadRequest},
		{"E_UNAUTHORIZED", http.StatusUnauthorized},
		{"E_FORBIDDEN", http.StatusForbidden},
		{"E_PATH_NOT_ALLOWED", http.StatusForbidden},
		{"E_NOT_FOUND", http.StatusNotFound},
		{"E_FILE_NOT_FOUND", http.StatusNotFound},
		{"E_REVISION_CONFLICT", http.StatusConflict},
		{"E_IDEMPOTENCY_MISMATCH", http.StatusConflict},
		{"E_OVERLAP", http.StatusConflict},
		{"E_INVALID_STATE", http.StatusConflict},
		{"E_IN_USE", http.StatusConflict},
		{"E_NOT_EMPTY", http.StatusConflict},
		{"E_ID_EXISTS", http.StatusConflict},
		{"E_ASSET_NOT_READY", http.StatusConflict},
		{"E_NOTHING_TO_UNDO", http.StatusConflict},
		{"E_NOTHING_TO_REDO", http.StatusConflict},
		{"E_PAYLOAD_TOO_LARGE", http.StatusRequestEntityTooLarge},
		{"E_METHOD_NOT_ALLOWED", http.StatusMethodNotAllowed},
		{"E_LIMIT", http.StatusUnprocessableEntity},
		{"E_LIMIT_EXCEEDED", http.StatusUnprocessableEntity},
		{"E_UNSUPPORTED_MEDIA", http.StatusUnprocessableEntity},
		{"E_TYPE_MISMATCH", http.StatusUnprocessableEntity},
		{"E_OUT_OF_SOURCE", http.StatusUnprocessableEntity},
		{"E_FONT_UNKNOWN", http.StatusUnprocessableEntity},
		{"E_SLOT", http.StatusUnprocessableEntity},
		{"E_LINT_BLOCKING", http.StatusUnprocessableEntity},
		{"E_QUEUE_FULL", http.StatusTooManyRequests},
		{"E_RATE_LIMITED", http.StatusTooManyRequests},
		{"E_CLIENT_CLOSED", 499},
		{"E_WORKSPACE_LOCKED", http.StatusServiceUnavailable},
		{"E_TRANSCRIBE_UNAVAILABLE", http.StatusServiceUnavailable},
		{"E_FFMPEG_MISSING", http.StatusServiceUnavailable},
		{"E_NO_ENCODER", http.StatusServiceUnavailable},
		{"E_INTERNAL", http.StatusInternalServerError},
		// Job codes travel in a 200 job object; raised by a call itself they are 500.
		{"E_INGEST_FAILED", http.StatusInternalServerError},
		{"E_EXPORT_FAILED", http.StatusInternalServerError},
		{"E_VERIFY_FAILED", http.StatusInternalServerError},
		{"E_TRANSCRIBE_FAILED", http.StatusInternalServerError},
		{"E_CANCELLED", http.StatusInternalServerError},
		{"E_SOMETHING_NEW", http.StatusInternalServerError},
	}

	for _, row := range rows {
		t.Run(row.code, func(t *testing.T) {
			t.Parallel()

			if got := (&Error{Code: row.code}).HTTPStatus(); got != row.status {
				t.Errorf("%s = %d, want %d", row.code, got, row.status)
			}
		})
	}
}
