package meta

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/social"
)

// MaxBody is the most of an answer of Meta that is read.
const MaxBody = 1 << 20

// Call makes one request to the Graph API through the gated client and decodes the JSON answer into out
// (which may be nil, or a *json.RawMessage to keep the body). A GET sends params in the address, a POST sends
// them as a form. The token goes in the Authorization header and nowhere else, so no error text of the HTTP
// client, which holds the address, can hold a token. A refusal of Meta is a *social.Error.
func (b *Base) Call(
	ctx context.Context, cl connections.Doer, method, path string, params url.Values, token string, out any,
) error {
	target := b.cfg.graph(path)

	var body io.Reader = http.NoBody

	if method == http.MethodGet && len(params) > 0 {
		target += "?" + params.Encode()
	}

	if method == http.MethodPost {
		body = strings.NewReader(params.Encode())
	}

	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return err
	}

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	req.Header.Set("Accept", "application/json")

	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	res, err := cl.Do(req)
	if err != nil {
		return TransportError(b.kind.Network, err)
	}

	defer func() { _ = res.Body.Close() }()

	data, _ := io.ReadAll(io.LimitReader(res.Body, MaxBody))

	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
		return Classify(b.kind.Network, res.StatusCode, data)
	}

	if out == nil {
		return nil
	}

	if raw, ok := out.(*json.RawMessage); ok {
		*raw = append((*raw)[:0], data...)

		return nil
	}

	if json.Unmarshal(data, out) != nil {
		return social.NewError(social.KindUnavailable, b.kind.Network, "bad_answer", res.StatusCode, nil)
	}

	return nil
}
