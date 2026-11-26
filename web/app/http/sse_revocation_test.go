package httpapi_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/internal/testfs"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/agenthost"
	"github.com/AlexGalhardo/stuarty/service/config"
	"github.com/AlexGalhardo/stuarty/service/workspace"
	"github.com/AlexGalhardo/stuarty/web/app/auth"
	"github.com/AlexGalhardo/stuarty/web/app/events"
	httpapi "github.com/AlexGalhardo/stuarty/web/app/http"
	"github.com/AlexGalhardo/stuarty/web/app/store"
	"github.com/AlexGalhardo/stuarty/web/bootstrap"

	vukem "github.com/AlexGalhardo/vukem"
	"github.com/AlexGalhardo/vukem/auth/hash"
	"github.com/AlexGalhardo/vukem/ratelimit"
	"github.com/AlexGalhardo/vukem/vukemtest"
)

func TestSecurity_SSERevocationAndDisabledAgentToken(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	dir, err := workspace.Resolve(func(string) string { return filepath.Join(testfs.TempDir(t), "ws") })
	if err != nil {
		t.Fatal(err)
	}
	if err := dir.EnsureLayout(); err != nil {
		t.Fatal(err)
	}

	st, err := store.OpenTemplate(ctx, dir, store.Options{SchemaVersion: timeline.CurrentSchemaVersion})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	res := config.Resolved{
		Mode: config.ModeNetwork, Addr: "127.0.0.1:0", AppURL: netOrigin,
		TrustedProxies: []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")},
	}
	netw, err := httpapi.NewNetwork(res, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}

	clk := &clock{t: time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)}
	netAuth, err := auth.NewNetwork(st.DB(), auth.NetworkOptions{Now: clk.now, Hasher: hash.Fake()})
	if err != nil {
		t.Fatal(err)
	}

	var revokedCalls atomic.Int32
	netAuth.OnUserRevoked = func(string) { revokedCalls.Add(1) }

	hub := events.New(st, events.Options{Revalidate: 20 * time.Millisecond, Heartbeat: 50 * time.Millisecond})
	t.Cleanup(hub.Close)

	svc, err := service.New(service.Options{Store: st, Publish: hub.Publish})
	if err != nil {
		t.Fatal(err)
	}

	agentTokens := agenthost.NewTokens(clk.now)
	loose := ratelimit.NewMemory(1_000_000, time.Minute)
	deps := &httpapi.Deps{
		Service: svc, Registry: command.Default(), Workspace: dir,
		Network: netw, Auth: netAuth, Events: hub, AgentTokens: agentTokens,
		Sessions: auth.NewSessions(clk.now), Launch: auth.NewLaunch(clk.now),
		LoginLimits: &httpapi.LoginLimits{PerEmailAndAddress: loose, PerEmail: loose, PerAddress: loose},
		Now:         clk.now,
	}

	app := vukemtest.New(t, func(opts ...vukem.Option) (*vukem.App, error) {
		return bootstrap.NewWith(deps, opts...)
	})

	srv := httptest.NewTLSServer(app.Handler())
	t.Cleanup(srv.Close)

	adminUser, err := netAuth.Users.Create(ctx, "admin@example.com", goodPass, service.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	bobUser, err := netAuth.Users.Create(ctx, "bob@example.com", goodPass, service.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Revoking an API key closes an open SSE stream on the next Revalidate tick.
	bobKey, _, err := netAuth.Keys().CreateKey(ctx, service.NewKey{
		UserID: bobUser.ID, Name: "sse-key", Scopes: service.RoleScopes(service.RoleAdmin),
		MaxActive: 10, MaxRecent: 10,
	})
	if err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/v1/approvals/events", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = netHost
	req.Header.Set("Origin", netOrigin)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("Authorization", "Bearer "+bobKey)

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("SSE open status = %d, want 200: %s", resp.StatusCode, body)
	}

	closed := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		close(closed)
	}()

	if err := netAuth.RevokeKeysOf(ctx, bobUser.ID); err != nil {
		t.Fatal(err)
	}

	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("SSE stream stayed open after API key revocation")
	}

	// 2. Disabling a user rejects their live hosted agent stc_ token immediately.
	tok, revoke, err := agentTokens.Issue(agenthost.Grant{
		SessionID: "cht_0000000001", ProjectID: "prj_0000000001", Owner: bobUser.ID, Scopes: agenthost.GrantScopes(),
	}, clk.now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	defer revoke()

	disabled := true
	if _, err := netAuth.Users.Update(ctx, bobUser.ID, nil, &disabled); err != nil {
		t.Fatal(err)
	}

	agentReq, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/v1/projects", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	agentReq.Host = netHost
	agentReq.Header.Set("Origin", netOrigin)
	agentReq.Header.Set("X-Forwarded-Proto", "https")
	agentReq.Header.Set("Authorization", "Bearer "+tok)

	agentResp, err := srv.Client().Do(agentReq)
	if err != nil {
		t.Fatal(err)
	}
	_ = agentResp.Body.Close()
	if agentResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("disabled user agent token status = %d, want 401", agentResp.StatusCode)
	}

	// 3. ChangePassword invokes OnUserRevoked hook.
	if _, err := netAuth.ChangePassword(ctx, adminUser.Email, "a brand new strong pass"); err != nil {
		t.Fatal(err)
	}
	if got := revokedCalls.Load(); got != 1 {
		t.Fatalf("OnUserRevoked calls after ChangePassword = %d, want 1", got)
	}
}
