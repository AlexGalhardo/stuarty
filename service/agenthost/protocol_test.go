package agenthost

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/chat"
)

// spy is a Handler that records what the agent sends and answers permission requests with
// the function it was given.
type spy struct {
	mu       sync.Mutex
	updates  []Update
	requests []string
	answer   func(PermissionRequest) (string, bool)
}

func (r *spy) Notify(method string, params json.RawMessage) {
	if method != MethodSessionUpdate {
		return
	}

	if u, ok := ParseUpdate(params); ok {
		r.mu.Lock()
		r.updates = append(r.updates, u)
		r.mu.Unlock()
	}
}

func (r *spy) Request(_ context.Context, method string, params json.RawMessage) (any, *RPCError) {
	r.mu.Lock()
	r.requests = append(r.requests, method)
	r.mu.Unlock()

	if method != MethodRequestPermission {
		return nil, MethodNotFound(method)
	}

	req, err := ParsePermission(params)
	if err != nil {
		return nil, InvalidParams()
	}

	id, cancelled := r.answer(req)

	return PermissionOutcome(id, cancelled), nil
}

func (r *spy) texts() string {
	r.mu.Lock()
	defer r.mu.Unlock()

	var b strings.Builder

	for _, u := range r.updates {
		if u.Kind == chat.KindAgentMessage {
			b.WriteString(u.Text)
		}
	}

	return b.String()
}

// startAgent runs the fake agent in a mode and connects to it.
func startAgent(t *testing.T, mode string, h Handler) *Conn {
	t.Helper()

	cmd := exec.Command(fakeAgent(t), "--mode="+mode)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	conn := NewConn(stdout, stdin, h)

	t.Cleanup(func() {
		conn.Close()
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	return conn
}

func ctx(t *testing.T) context.Context {
	t.Helper()

	c, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)

	return c
}

func TestProtocolEchoTurn(t *testing.T) {
	t.Parallel()

	rec := &spy{}
	conn := startAgent(t, "echo", rec)

	init, err := conn.Initialize(ctx(t), "test")
	if err != nil {
		t.Fatal(err)
	}

	if init.ProtocolVersion != ProtocolVersion || !init.AgentCapabilities.MCP.HTTP {
		t.Fatalf("initialize = %+v", init)
	}

	sid, err := conn.NewSession(ctx(t), t.TempDir(), []MCPServer{
		{
			Type:    "http",
			Name:    "stuarty",
			URL:     "http://127.0.0.1:1/mcp",
			Headers: []Header{{Name: "Authorization", Value: "Bearer x"}},
		},
	})
	if err != nil || sid != "sess_fake" {
		t.Fatalf("NewSession = %q, %v", sid, err)
	}

	stop, err := conn.Prompt(ctx(t), sid, "hello there")
	if err != nil || stop != "end_turn" {
		t.Fatalf("Prompt = %q, %v", stop, err)
	}

	// session/update notifications arrive before the response, on the same stream, so they are
	// all recorded when Prompt returns.
	if got := rec.texts(); got != "echo: hello there" {
		t.Errorf("message text = %q", got)
	}

	var sawCall, sawDone, sawThought bool

	for _, u := range rec.updates {
		switch {
		case u.Kind == chat.KindToolCall && u.Tool.Status == chat.ToolPending && u.Tool.Title == "Reading the project":
			sawCall = true
		case u.Kind == chat.KindToolCall && u.Tool.Status == chat.ToolCompleted && u.Tool.Output == "done" && !u.HasTool["title"]:
			sawDone = true
		case u.Kind == chat.KindAgentThought && u.Text == "thinking":
			sawThought = true
		}
	}

	if !sawCall || !sawDone || !sawThought {
		t.Errorf("updates = %+v", rec.updates)
	}

	for _, u := range rec.updates[:2] {
		if u.MessageID != "m1" {
			t.Errorf("the chunks of one message share an id, got %q", u.MessageID)
		}
	}
}

func TestProtocolPermissionRequestIsAnsweredByThePerson(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		answer func(PermissionRequest) (string, bool)
		want   string
	}{
		{"selected", func(r PermissionRequest) (string, bool) {
			id, c, _ := chat.PickOption(r.Options, chat.AnswerAllow)
			return id, c
		}, "permission: selected once"},
		{"cancelled", func(PermissionRequest) (string, bool) { return "", true }, "permission: cancelled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var seen PermissionRequest

			rec := &spy{answer: func(r PermissionRequest) (string, bool) {
				seen = r
				return tc.answer(r)
			}}

			conn := startAgent(t, "permission", rec)
			if _, err := conn.Initialize(ctx(t), "test"); err != nil {
				t.Fatal(err)
			}

			sid, err := conn.NewSession(ctx(t), t.TempDir(), nil)
			if err != nil {
				t.Fatal(err)
			}

			if _, err := conn.Prompt(ctx(t), sid, "go"); err != nil {
				t.Fatal(err)
			}

			if got := strings.TrimSpace(rec.texts()); got != tc.want {
				t.Errorf("agent saw %q, want %q", got, tc.want)
			}

			if seen.Title != "Delete every clip <b>now</b>" || seen.ToolKind != "delete" || len(seen.Options) != 3 {
				t.Errorf("request = %+v", seen)
			}
		})
	}
}

func TestProtocolOffersNoClientCapability(t *testing.T) {
	t.Parallel()

	rec := &spy{}
	conn := startAgent(t, "echo", rec)

	// What the agent asks for outside the capabilities is refused with method not found: the
	// spy is the Handler, so call it the way the connection does.
	_, rpc := rec.Request(context.Background(), "fs/read_text_file", nil)
	if rpc == nil || rpc.Code != -32601 {
		t.Errorf("fs/read_text_file = %v", rpc)
	}

	_, rpc = rec.Request(context.Background(), "terminal/create", nil)
	if rpc == nil || rpc.Code != -32601 {
		t.Errorf("terminal/create = %v", rpc)
	}

	// And an unknown method of the agent, over the wire, is answered by the connection.
	var res json.RawMessage

	err := conn.Call(ctx(t), "nonexistent/method", map[string]any{}, &res)

	var rpcErr *RPCError
	if !errors.As(err, &rpcErr) || rpcErr.Code != -32601 {
		t.Errorf("an unknown method = %v", err)
	}
}

func TestProtocolStartFailures(t *testing.T) {
	t.Parallel()

	t.Run("old or unknown version is reported to the caller", func(t *testing.T) {
		t.Parallel()

		conn := startAgent(t, "bad-version", &spy{})

		res, err := conn.Initialize(ctx(t), "test")
		if err != nil || res.ProtocolVersion == ProtocolVersion {
			t.Fatalf("Initialize = %+v, %v", res, err)
		}
	})

	t.Run("an agent without http mcp is reported", func(t *testing.T) {
		t.Parallel()

		conn := startAgent(t, "no-http", &spy{})

		res, err := conn.Initialize(ctx(t), "test")
		if err != nil || res.AgentCapabilities.MCP.HTTP {
			t.Fatalf("Initialize = %+v, %v", res, err)
		}
	})

	t.Run("an agent that needs a sign-in", func(t *testing.T) {
		t.Parallel()

		conn := startAgent(t, "sign-in", &spy{})
		if _, err := conn.Initialize(ctx(t), "test"); err != nil {
			t.Fatal(err)
		}

		if _, err := conn.NewSession(ctx(t), t.TempDir(), nil); !errors.Is(err, ErrSignInRequired) {
			t.Errorf("NewSession = %v", err)
		}
	})

	t.Run("an agent that exits fails the call and closes the connection", func(t *testing.T) {
		t.Parallel()

		conn := startAgent(t, "exit-start", &spy{})

		_, err := conn.Initialize(ctx(t), "test")
		if !errors.Is(err, ErrClosed) {
			t.Fatalf("Initialize = %v", err)
		}

		select {
		case <-conn.Done():
		case <-time.After(5 * time.Second):
			t.Fatal("Done was not closed")
		}
	})

	t.Run("a line that is not JSON ends the connection with a protocol error", func(t *testing.T) {
		t.Parallel()

		conn := startAgent(t, "badline", &spy{})

		if _, err := conn.Initialize(ctx(t), "test"); !errors.Is(err, ErrProtocol) {
			t.Fatalf("Initialize = %v", err)
		}
	})

	t.Run("a line over the limit ends the connection", func(t *testing.T) {
		t.Parallel()

		conn := startAgent(t, "longline", &spy{})

		if _, err := conn.Initialize(ctx(t), "test"); !errors.Is(err, ErrLineTooLong) {
			t.Fatalf("Initialize = %v", err)
		}
	})
}

func TestProtocolWrongAnswersAreProtocolErrors(t *testing.T) {
	t.Parallel()

	// An answer to a request that was never made, a message with no id and no method, and a
	// version other than 2.0 each end the connection.
	for name, line := range map[string]string{
		"unknown reply":  `{"jsonrpc":"2.0","id":77,"result":{}}`,
		"empty message":  `{"jsonrpc":"2.0"}`,
		"wrong version":  `{"jsonrpc":"1.0","method":"x"}`,
		"not an object":  `[1,2,3]`,
		"string reply":   `{"jsonrpc":"2.0","id":"abc","result":{}}`,
		"truncated json": `{"jsonrpc":"2.0",`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			pr, pw := io.Pipe()
			conn := NewConn(pr, io.Discard, &spy{})

			t.Cleanup(func() { conn.Close(); _ = pw.Close() })

			go func() { _, _ = pw.Write([]byte(line + "\n")) }()

			select {
			case <-conn.Done():
			case <-time.After(5 * time.Second):
				t.Fatal("the connection stayed open")
			}

			if err := conn.Err(); !errors.Is(err, ErrProtocol) {
				t.Errorf("Err = %v", err)
			}
		})
	}
}

func TestProtocolCloseFailsCallsInFlight(t *testing.T) {
	t.Parallel()

	pr, pw := io.Pipe()
	conn := NewConn(pr, io.Discard, &spy{})

	t.Cleanup(func() { _ = pw.Close() })

	errs := make(chan error, 1)

	go func() { errs <- conn.Call(context.Background(), "x", map[string]any{}, nil) }()

	// Whether the call was registered before the close or not, it fails with ErrClosed.
	conn.Close()

	select {
	case err := <-errs:
		if !errors.Is(err, ErrClosed) {
			t.Errorf("Call = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the call did not end")
	}

	if err := conn.Notify("x", map[string]any{}); err == nil {
		t.Error("Notify on a closed connection succeeded")
	}
}

func TestProtocolCallEndsWithItsContext(t *testing.T) {
	t.Parallel()

	pr, pw := io.Pipe()
	conn := NewConn(pr, io.Discard, &spy{})

	t.Cleanup(func() { conn.Close(); _ = pw.Close() })

	c, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	if err := conn.Call(c, "x", map[string]any{}, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Call = %v", err)
	}
}

func TestParseUpdate(t *testing.T) {
	t.Parallel()

	wrap := func(update string) json.RawMessage {
		return json.RawMessage(`{"sessionId":"s","update":` + update + `}`)
	}

	t.Run("text is cleaned", func(t *testing.T) {
		t.Parallel()

		u, ok := ParseUpdate(
			wrap(`{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"a\u0000b\u202ec"}}`),
		)
		if !ok || u.Kind != chat.KindAgentMessage || u.Text != "abc" {
			t.Errorf("ParseUpdate = %+v, %v", u, ok)
		}
	})

	t.Run("an image is a name, not a payload", func(t *testing.T) {
		t.Parallel()

		u, ok := ParseUpdate(wrap(`{"sessionUpdate":"agent_message_chunk","content":{"type":"image","data":"AAAA"}}`))
		if !ok || u.Text != "[image]" {
			t.Errorf("ParseUpdate = %+v, %v", u, ok)
		}
	})

	t.Run("plan", func(t *testing.T) {
		t.Parallel()

		u, ok := ParseUpdate(
			wrap(`{"sessionUpdate":"plan","entries":[{"content":"cut","priority":"high","status":"pending"}]}`),
		)
		if !ok || u.Kind != chat.KindPlan || u.Text != "[pending] cut" {
			t.Errorf("ParseUpdate = %+v, %v", u, ok)
		}
	})

	t.Run("tool output is joined and bounded", func(t *testing.T) {
		t.Parallel()

		big := strings.Repeat("x", chat.MaxToolOutput*2)
		u, ok := ParseUpdate(wrap(
			`{"sessionUpdate":"tool_call_update","toolCallId":"c","status":"failed","content":[` +
				`{"type":"content","content":{"type":"text","text":"` + big + `"}},{"type":"diff","path":"a.txt","newText":"y"}]}`))

		if !ok || u.Tool.Status != chat.ToolFailed || len(u.Tool.Output) > chat.MaxToolOutput {
			t.Errorf("ParseUpdate = %+v, %v", u, ok)
		}
	})

	t.Run("an unknown status is pending, an unknown update is ignored", func(t *testing.T) {
		t.Parallel()

		u, ok := ParseUpdate(wrap(`{"sessionUpdate":"tool_call","toolCallId":"c","status":"exploding"}`))
		if !ok || u.Tool.Status != chat.ToolPending {
			t.Errorf("ParseUpdate = %+v, %v", u, ok)
		}

		for _, raw := range []string{
			`{"sessionUpdate":"current_mode_update","currentModeId":"x"}`,
			`{"sessionUpdate":"available_commands_update"}`,
			`{"sessionUpdate":"tool_call"}`,
			`{"sessionUpdate":"agent_message_chunk","content":7}`,
			`{}`,
		} {
			if _, ok := ParseUpdate(wrap(raw)); ok {
				t.Errorf("%s was kept", raw)
			}
		}

		if _, ok := ParseUpdate(json.RawMessage(`{"update":{}}`)); ok {
			t.Error("an update with no session was kept")
		}
	})
}

func TestParsePermission(t *testing.T) {
	t.Parallel()

	req, err := ParsePermission(
		json.RawMessage(`{"sessionId":"s","toolCall":{"toolCallId":"c","title":"T\u0000","kind":"edit"},` +
			`"options":[{"optionId":"o","name":"Once","kind":"allow_once"},{"optionId":"x","name":"?","kind":"allow_forever"}]}`),
	)
	if err != nil {
		t.Fatal(err)
	}

	if req.Title != "T" || len(req.Options) != 1 || req.Options[0].Kind != chat.AllowOnce {
		t.Errorf("req = %+v", req)
	}

	if _, err := ParsePermission(json.RawMessage(`{"toolCall":{}}`)); err == nil {
		t.Error("a request with no session was accepted")
	}

	if got := PermissionOutcome("o", false)["outcome"].(map[string]any); got["outcome"] != "selected" ||
		got["optionId"] != "o" {
		t.Errorf("outcome = %v", got)
	}

	if got := PermissionOutcome("", true)["outcome"].(map[string]any); got["outcome"] != "cancelled" {
		t.Errorf("outcome = %v", got)
	}
}
