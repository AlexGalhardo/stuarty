package mcp

import (
	"io"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// A request followed at once by the end of the input still gets its response
// (VV-205).
func TestRunStdioAnswersTheRequestsReadBeforeEOF(t *testing.T) {
	const reqs = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}` + "\n" +
		`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}` + "\n"

	pr, pw := io.Pipe()
	done := make(chan string, 1)

	go func() {
		b, _ := io.ReadAll(pr)
		done <- string(b)
	}()

	in := io.NopCloser(strings.NewReader(reqs))

	err := runDraining(t.Context(), newService(t), Options{Session: "s1"}, &sdk.IOTransport{Reader: in, Writer: pw})
	_ = pw.Close()

	if err != nil {
		t.Logf("run ended with %v", err)
	}

	select {
	case out := <-done:
		if !strings.Contains(out, `"id":1`) || !strings.Contains(out, `"id":2`) {
			t.Fatalf("responses missing from the output: %s", out)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out")
	}
}
