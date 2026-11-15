package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync/atomic"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/core/timeline"
	"github.com/AlexGalhardo/vukem-video/service"
	"github.com/AlexGalhardo/vukem-video/service/workspace"
	"github.com/AlexGalhardo/vukem-video/web/app/store"
)

// newService is the real Service over a real store in a temporary workspace,
// with identifiers from a counter so that the tests can name them.
func newService(tb testing.TB) service.Service {
	tb.Helper()

	dir, err := workspace.Resolve(func(string) string { return filepath.Join(tb.TempDir(), "ws") })
	if err != nil {
		tb.Fatal(err)
	}

	if err := dir.EnsureLayout(); err != nil {
		tb.Fatal(err)
	}

	st, err := store.Open(context.Background(), dir, store.Options{SchemaVersion: timeline.CurrentSchemaVersion})
	if err != nil {
		tb.Fatal(err)
	}

	tb.Cleanup(func() { _ = st.Close() })

	var n atomic.Int64

	svc, err := service.New(service.Options{
		Store: st, Registry: command.Default(),
		NewIDs: func(prefix string) string { return fmt.Sprintf("%s_%010d", prefix, n.Add(1)) },
	})
	if err != nil {
		tb.Fatal(err)
	}

	return svc
}

// connect runs the server and a client in memory and returns the client session.
func connect(t *testing.T, svc service.Service, o Options, clientName string) *sdk.ClientSession {
	t.Helper()

	srv, err := New(svc, o)
	if err != nil {
		t.Fatal(err)
	}

	ct, st := sdk.NewInMemoryTransports()

	ctx := t.Context()

	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = ss.Close() })

	client := sdk.NewClient(&sdk.Implementation{Name: clientName, Version: "1"}, nil)

	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = cs.Close() })

	return cs
}

// call runs a tool and returns the result.
func call(t *testing.T, cs *sdk.ClientSession, name string, args any) *sdk.CallToolResult {
	t.Helper()

	res, err := cs.CallTool(t.Context(), &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: protocol error: %v", name, err)
	}

	return res
}

// textOf is the text block of a result.
func textOf(t *testing.T, res *sdk.CallToolResult) string {
	t.Helper()

	if len(res.Content) != 1 {
		t.Fatalf("want one content block, got %d", len(res.Content))
	}

	tc, ok := res.Content[0].(*sdk.TextContent)
	if !ok {
		t.Fatalf("content is %T", res.Content[0])
	}

	return tc.Text
}

// structured is the structured content as a map.
func structured(t *testing.T, res *sdk.CallToolResult) map[string]any {
	t.Helper()

	m, ok := res.StructuredContent.(map[string]any)
	if !ok {
		raw := marshal(t, res.StructuredContent)
		t.Fatalf("structured content is not an object: %s", raw)
	}

	return m
}

func ok(t *testing.T, res *sdk.CallToolResult) *sdk.CallToolResult {
	t.Helper()

	if res.IsError {
		t.Fatalf("tool error: %s", textOf(t, res))
	}

	return res
}

// marshal encodes a value for a comparison and fails the test when it cannot.
func marshal(tb testing.TB, v any) []byte {
	tb.Helper()

	raw, err := json.Marshal(v)
	if err != nil {
		tb.Fatal(err)
	}

	return raw
}
