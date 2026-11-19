package wire

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"

	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/providers"
)

// Bytes makes the request as JSON does and returns the body of a 2xx answer, which is not JSON (the audio of
// a speech). The body is read to limit bytes and no further: an answer that is longer is unreadable.
func Bytes(ctx context.Context, c connections.Credential, call Call, limit int64) ([]byte, error) {
	res, err := send(ctx, c, &call)
	if err != nil {
		return nil, err
	}

	defer func() { _ = res.Body.Close() }()

	return readBounded(res, limit)
}

// Download fetches a URL a provider named in its answer, with the gated client of the connection: a host the
// connection does not list is refused by the gate before a socket is made. The request carries no header of
// the adapter, so the key is never sent to a host that is not the API (SDD-016 section 6.1).
func Download(ctx context.Context, c connections.Credential, rawURL string, limit int64) ([]byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" || u.Scheme != "https" && u.Scheme != "http" {
		return nil, &providers.Error{Kind: providers.KindUnreadable, Status: http.StatusOK}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), http.NoBody)
	if err != nil {
		return nil, &providers.Error{Kind: providers.KindUnreadable, Status: http.StatusOK}
	}

	res, err := c.Client.Do(req)
	if err != nil {
		return nil, transportError(ctx, err)
	}

	defer func() { _ = res.Body.Close() }()

	if res.StatusCode < 200 || res.StatusCode > 299 {
		return nil, StatusError(res.StatusCode, res.Header.Get("Retry-After"), nil)
	}

	return readBounded(res, limit)
}

func readBounded(res *http.Response, limit int64) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return nil, &providers.Error{Kind: providers.KindUnavailable, Status: res.StatusCode}
	}

	if int64(len(raw)) > limit {
		return nil, &providers.Error{Kind: providers.KindUnreadable, Status: res.StatusCode}
	}

	return raw, nil
}

// send builds the request of a call, sends it through the client of the credential and returns the answer when
// it is 2xx; any other status is the *providers.Error of StatusError, and the body is closed.
func send(ctx context.Context, c connections.Credential, call *Call) (*http.Response, error) {
	var body io.Reader = http.NoBody

	if call.Body != nil {
		raw, err := json.Marshal(call.Body)
		if err != nil {
			return nil, err
		}

		body = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, call.Method, URL(c, call.Path), body)
	if err != nil {
		return nil, err
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
		return nil, transportError(ctx, err)
	}

	if res.StatusCode >= 200 && res.StatusCode <= 299 {
		return res, nil
	}

	defer func() { _ = res.Body.Close() }()

	raw, _ := io.ReadAll(io.LimitReader(res.Body, MaxErrorBody))

	e := StatusError(res.StatusCode, res.Header.Get("Retry-After"), raw)
	// A code is shown only when it cannot carry text, and a server that repeats the key it was sent as its
	// code has not sent a code (T-55).
	if e.Code != "" && c.Echoes(e.Code) {
		e.Code = safeType(raw, c)
	}

	return nil, e
}
