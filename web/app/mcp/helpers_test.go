package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync/atomic"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/workspace"
	"github.com/AlexGalhardo/stuarty/web/app/store"
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
func connect(tb testing.TB, svc service.Service, o Options, clientName string) *sdk.ClientSession {
	tb.Helper()

	if o.Session == "" {
		o.Session = "s1"
	}

	srv, err := New(svc, o)
	if err != nil {
		tb.Fatal(err)
	}

	ct, st := sdk.NewInMemoryTransports()

	ctx := tb.Context()

	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		tb.Fatal(err)
	}

	tb.Cleanup(func() { _ = ss.Close() })

	client := sdk.NewClient(&sdk.Implementation{Name: clientName, Version: "1"}, nil)

	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		tb.Fatal(err)
	}

	tb.Cleanup(func() { _ = cs.Close() })

	return cs
}

// call runs a tool and returns the result.
func call(tb testing.TB, cs *sdk.ClientSession, name string, args any) *sdk.CallToolResult {
	tb.Helper()

	res, err := cs.CallTool(tb.Context(), &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		tb.Fatalf("%s: protocol error: %v", name, err)
	}

	return res
}

// textOf is the text block of a result.
func textOf(tb testing.TB, res *sdk.CallToolResult) string {
	tb.Helper()

	if len(res.Content) != 1 {
		tb.Fatalf("want one content block, got %d", len(res.Content))
	}

	tc, ok := res.Content[0].(*sdk.TextContent)
	if !ok {
		tb.Fatalf("content is %T", res.Content[0])
	}

	return tc.Text
}

// structured is the structured content as a map.
func structured(tb testing.TB, res *sdk.CallToolResult) map[string]any {
	tb.Helper()

	m, ok := res.StructuredContent.(map[string]any)
	if !ok {
		raw := marshal(tb, res.StructuredContent)
		tb.Fatalf("structured content is not an object: %s", raw)
	}

	return m
}

func ok(tb testing.TB, res *sdk.CallToolResult) *sdk.CallToolResult {
	tb.Helper()

	if res.IsError {
		tb.Fatalf("tool error: %s", textOf(tb, res))
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
