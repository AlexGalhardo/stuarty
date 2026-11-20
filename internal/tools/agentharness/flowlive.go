package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AlexGalhardo/stuarty/internal/outbound"
)

// The live services of the first-flow scenario are started by the person who runs the harness, on
// loopback (the steps are in docs/agent-benchmark.md): a Stuarty server with a
// fake AI provider and a fake social network behind it. The harness only reaches them, as a person.

// livePerson signs in the way the desktop app does, with the launcher secret the server was given, and
// decides in the approval queue.
type livePerson struct {
	base, secret, home string
	client             *http.Client //nolint:forbidigo // loopback only: the client of outbound.LocalServer
}

// statusErr is the error of a request that failed or answered a status that is not success.
func statusErr(what string, code int, err error) error {
	if err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}

	return fmt.Errorf("%s: status %d", what, code) //nolint:err113 // a status, not a sentinel
}

func newLivePerson(base, secret, home string) *livePerson {
	jar, _ := cookiejar.New(nil)
	c := outbound.LocalServer(30 * time.Second)
	c.Jar = jar

	return &livePerson{base: strings.TrimRight(base, "/"), secret: secret, home: home, client: c}
}

func (p *livePerson) do(ctx context.Context, method, path, body string, header map[string]string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, method, p.base+path, strings.NewReader(body))
	if err != nil {
		return nil, 0, err
	}

	req.Header.Set("Origin", p.base)

	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	for k, v := range header {
		req.Header.Set(k, v)
	}

	res, err := p.client.Do(req)
	if err != nil {
		return nil, 0, err
	}

	defer func() { _ = res.Body.Close() }()

	data, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))

	return data, res.StatusCode, err
}

func (p *livePerson) signIn(ctx context.Context) error {
	token, err := os.ReadFile(filepath.Join(p.home, "token"))
	if err != nil {
		return fmt.Errorf("read the local token: %w", err)
	}

	data, code, err := p.do(ctx, http.MethodPost, "/api/v1/system/launch", "", map[string]string{
		"Authorization":    "Bearer " + strings.TrimSpace(string(token)),
		"Stuarty-Launcher": p.secret,
	})
	if err != nil || code != http.StatusOK {
		return statusErr("launch a person session", code, err)
	}

	var launch struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(data, &launch); err != nil || !strings.HasPrefix(launch.URL, p.base) {
		return fmt.Errorf("the launch answer is not an address of the server: %s", data)
	}

	_, code, err = p.do(ctx, http.MethodGet, strings.TrimPrefix(launch.URL, p.base), "", nil)
	if err != nil || code >= http.StatusBadRequest {
		return statusErr("redeem the launch code", code, err)
	}

	return nil
}

// ApprovePending implements Person.
func (p *livePerson) ApprovePending(ctx context.Context) (int, error) {
	if err := p.signIn(ctx); err != nil {
		return 0, err
	}

	data, code, err := p.do(ctx, http.MethodGet, "/api/v1/approvals", "", nil)
	if err != nil || code != http.StatusOK {
		return 0, statusErr("list the queue", code, err)
	}

	var queue struct {
		Requests []struct {
			ID string `json:"id"`
		} `json:"requests"`
	}
	if err := json.Unmarshal(data, &queue); err != nil {
		return 0, fmt.Errorf("read the queue: %w", err)
	}

	for _, r := range queue.Requests {
		body := `{"decision":"approve"}`

		if _, code, err := p.do(ctx, http.MethodPost, "/api/v1/approvals/"+r.ID+"/decide", body, nil); err != nil ||
			code != http.StatusOK {
			return 0, statusErr("approve "+r.ID, code, err)
		}
	}

	return len(queue.Requests), nil
}

// liveNetwork reads the uploads of the fake social network.
type liveNetwork struct {
	base   string
	client *http.Client //nolint:forbidigo // loopback only: the client of outbound.LocalServer
}

func newLiveNetwork(base string) *liveNetwork {
	return &liveNetwork{base: strings.TrimRight(base, "/"), client: outbound.LocalServer(15 * time.Second)}
}

// Uploads implements Network.
func (n *liveNetwork) Uploads(ctx context.Context) ([]upload, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, n.base+"/_uploads", http.NoBody)
	if err != nil {
		return nil, err
	}

	res, err := n.client.Do(req)
	if err != nil {
		return nil, err
	}

	defer func() { _ = res.Body.Close() }()

	var out struct {
		Uploads []upload `json:"uploads"`
	}

	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&out); err != nil {
		return nil, err
	}

	return out.Uploads, nil
}
