// Package wire is the part of an adapter that is the same for every provider: one JSON request through
// the gated client of a connection, a bounded read of the answer, and the mapping of a failure to a
// providers.Error built from the status and a short code, never from the body (SDD-016 section 9).
// It dials nothing itself: the client is the one of connections.Credential.
package wire

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
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
}

// URL joins the endpoint of the connection and a path.
func URL(c connections.Credential, path string) string {
	return strings.TrimRight(c.Connection.Endpoint, "/") + path
}

// JSON makes the request through the client of the credential and decodes a 2xx answer into out. A
// failure is a *providers.Error, or the error of the gate (outbound.ErrNoConnection and friends) or of
// the context, returned as they are.
func JSON(ctx context.Context, c connections.Credential, call Call, out any) error {
	var body io.Reader = http.NoBody

	if call.Body != nil {
		raw, err := json.Marshal(call.Body)
		if err != nil {
			return err
		}

		body = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, call.Method, URL(c, call.Path), body)
	if err != nil {
		return err
	}

	req.Header.Set("Accept", "application/json")

	if call.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	for k, v := range call.Header {
		req.Header.Set(k, v)
	}

	res, err := c.Client.Do(req)
	if err != nil {
		return transportError(ctx, err)
	}

	defer func() { _ = res.Body.Close() }()

	if res.StatusCode < 200 || res.StatusCode > 299 {
		raw, _ := io.ReadAll(io.LimitReader(res.Body, MaxErrorBody))

		e := StatusError(res.StatusCode, res.Header.Get("Retry-After"), raw)
		// A code is shown only when it cannot carry text, and a server that repeats the key it was sent as its
		// code has not sent a code (T-55).
		if e.Code != "" && c.Echoes(e.Code) {
			e.Code = safeType(raw, c)
		}

		return e
	}

	raw, err := io.ReadAll(io.LimitReader(res.Body, MaxAnswer+1))
	if err != nil {
		return &providers.Error{Kind: providers.KindUnavailable, Status: res.StatusCode}
	}

	if len(raw) > MaxAnswer || json.Unmarshal(raw, out) != nil {
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
	case ctx.Err() != nil:
		return ctx.Err()
	}

	return &providers.Error{Kind: providers.KindUnavailable}
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
