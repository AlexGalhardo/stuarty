// Package wire is the part of an adapter that is the same for every provider: one JSON request through
// the gated client of a connection, a bounded read of the answer, and the mapping of a failure to a
// providers.Error built from the status and a short code, never from the body (SDD-016 section 9).
// It dials nothing itself: the client is the one of connections.Credential.
package wire

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/AlexGalhardo/stuarty/internal/outbound"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/providers"
)

// Limits on what is read of an answer.
const (
	// MaxAnswer is the most read of a successful answer.
	MaxAnswer = 4 << 20
	// MaxErrorBody is the most read of a failing answer, to take its short code before it is dropped.
	MaxErrorBody = 8 << 10
	// maxRetryAfter caps the wait a provider may ask for.
	maxRetryAfter = 24 * 60 * 60
)

// Call is one request of an adapter.
type Call struct {
	Method string
	// Path is added to the endpoint of the connection, which is the base URL of the provider.
	Path   string
	Header map[string]string
	// Body is encoded as JSON; nil sends none.
	Body any
	// MaxAnswer is the most read of a successful answer of JSON; zero is the package MaxAnswer. An answer
	// that holds images as base64 needs more than that.
	MaxAnswer int64
}

// URL joins the endpoint of the connection and a path.
func URL(c connections.Credential, path string) string {
	return strings.TrimRight(c.Connection.Endpoint, "/") + path
}

// JSON makes the request through the client of the credential and decodes a 2xx answer into out. A
// failure is a *providers.Error, or the error of the gate (outbound.ErrNoConnection and friends) or of
// the context, returned as they are.
func JSON(ctx context.Context, c connections.Credential, call Call, out any) error {
	res, err := send(ctx, c, &call)
	if err != nil {
		return err
	}

	defer func() { _ = res.Body.Close() }()

	limit := call.MaxAnswer
	if limit <= 0 {
		limit = MaxAnswer
	}

	raw, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return &providers.Error{Kind: providers.KindUnavailable, Status: res.StatusCode}
	}

	if int64(len(raw)) > limit || json.Unmarshal(raw, out) != nil {
		return &providers.Error{Kind: providers.KindUnreadable, Status: res.StatusCode}
	}

	return nil
}

// transportError keeps what the layers above must see as it is and turns the rest into
// "unavailable": the text of a transport error is not ours to forward.
func transportError(ctx context.Context, err error) error {
	switch {
	case errors.Is(err, outbound.ErrNoConnection),
		errors.Is(err, outbound.ErrDestination),
		errors.Is(err, outbound.ErrNotPublic):
		return err
	case errors.Is(ctx.Err(), context.Canceled):
		return ctx.Err()
	case errors.Is(err, context.DeadlineExceeded), isTimeout(err):
		// The client of a connection gives up after its timeout, and a parent context may have a deadline of its
		// own: either way the request did not finish in time (SDD-016 section 6.2).
		return &providers.Error{Kind: providers.KindTimeout}
	case ctx.Err() != nil:
		return ctx.Err()
	}

	return &providers.Error{Kind: providers.KindUnavailable}
}

func isTimeout(err error) bool {
	ne, ok := errors.AsType[net.Error](err)

	return ok && ne.Timeout()
}

// refusedCodes are the short error codes with which a provider says it declined the prompt under its content
// policy (SDD-016 section 6.2): a 400 with one of them is providers.KindRefused, not a malformed request.
var refusedCodes = map[string]bool{
	"content_policy_violation": true,
	"moderation_blocked":       true,
	"content_filter":           true,
	"safety_violation":         true,
}

// StatusError maps a failing status to a providers.Error. body is the start of the answer: only its
// short error code is taken, and only when it matches the safe pattern. retryAfter is the header.
func StatusError(status int, retryAfter string, body []byte) *providers.Error {
	e := &providers.Error{Status: status, Code: errorCode(body)}

	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		e.Kind = providers.KindAuth
	case status == http.StatusTooManyRequests:
		e.Kind = providers.KindRateLimited

		if n, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && n > 0 {
			e.RetryAfter = min(n, maxRetryAfter)
		}
	case status >= 500:
		e.Kind = providers.KindUnavailable
	case status == http.StatusBadRequest && refusedCodes[e.Code]:
		e.Kind = providers.KindRefused
	case status >= 400:
		e.Kind = providers.KindBadRequest
	default:
		e.Kind = providers.KindUnreadable
	}

	return e
}

// errorCode reads error.code, else error.type, of the body of a failure, as both OpenAI and Anthropic
// write them, and returns it only when it is a safe short code.
func errorCode(body []byte) string {
	var v struct {
		Error struct {
			Code any    `json:"code"`
			Type string `json:"type"`
		} `json:"error"`
	}

	if json.Unmarshal(body, &v) != nil {
		return ""
	}

	if s, ok := v.Error.Code.(string); ok {
		if c := providers.SafeCode(s); c != "" {
			return c
		}
	}

	return providers.SafeCode(v.Error.Type)
}

// safeType is the error.type of a failing body, for a code that echoed the credential: the type, when it
// is a safe token that does not echo it either, else nothing.
func safeType(body []byte, c connections.Credential) string {
	var v struct {
		Error struct {
			Type string `json:"type"`
		} `json:"error"`
	}

	if json.Unmarshal(body, &v) != nil || c.Echoes(v.Error.Type) {
		return ""
	}

	return providers.SafeCode(v.Error.Type)
}
