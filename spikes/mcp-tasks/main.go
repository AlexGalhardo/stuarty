// Spike for P1-AGT-002: does the pinned MCP Go SDK support the tasks extension
// (io.modelcontextprotocol/tasks)? Disposable: nothing here is product code and
// no product package imports it. Run: go run ./spikes/mcp-tasks
//
// It sends the requests of the extension, tasks/get and tasks/cancel, to a
// server of the SDK over the in-memory transport and prints what comes back,
// then looks for a tasks type and an extension capability in the public API.
package main

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	srv := mcp.NewServer(&mcp.Implementation{Name: "spike", Version: "0"}, nil)

	ct, st := mcp.NewInMemoryTransports()

	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		return err
	}

	defer func() { _ = ss.Close() }()

	conn, err := ct.Connect(ctx)
	if err != nil {
		return err
	}

	defer func() { _ = conn.Close() }()

	send := func(id int, method, params string) error {
		msg, err := jsonrpc.DecodeMessage([]byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":%q,"params":%s}`, id, method, params)))
		if err != nil {
			return err
		}

		if err := conn.Write(ctx, msg); err != nil {
			return err
		}

		reply, err := conn.Read(ctx)
		if err != nil {
			return err
		}

		raw, _ := jsonrpc.EncodeMessage(reply)
		fmt.Printf("%s -> %s\n", method, raw)

		return nil
	}

	init := `{"protocolVersion":"2025-06-18","capabilities":{"experimental":{"io.modelcontextprotocol/tasks":{}}},"clientInfo":{"name":"spike","version":"0"}}`
	if err := send(1, "initialize", init); err != nil {
		return err
	}

	if err := send(2, "tasks/get", `{"taskId":"job_0000000001"}`); err != nil {
		return err
	}

	if err := send(3, "tasks/cancel", `{"taskId":"job_0000000001"}`); err != nil {
		return err
	}

	// The public API: a type or a capability with "task" in its name.
	for _, v := range []any{mcp.ServerCapabilities{}, mcp.ClientCapabilities{}, mcp.CallToolParams{}, mcp.CallToolResult{}} {
		t := reflect.TypeOf(v)
		for i := range t.NumField() {
			if strings.Contains(strings.ToLower(t.Field(i).Name), "task") {
				fmt.Printf("%s.%s exists\n", t.Name(), t.Field(i).Name)
			}
		}
	}

	fmt.Println("done")

	return nil
}
