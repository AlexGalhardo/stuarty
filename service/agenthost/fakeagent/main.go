// Command fakeagent is the agent the tests of the hosted chat run in place of a real one
// (SDD-017 section 13). It speaks the Agent Client Protocol on its standard streams and
// misbehaves on request. It is built by the tests and by the end-to-end suite; it is never part
// of the product.
//
//	fakeagent --mode=<mode> [--pidfile=<path>]
//
// The modes: echo (answers with the text it got), permission (asks before a tool call and
// reports the answer), mcp (the prompt "mcp <tool> <json>" calls that tool of the MCP server it
// was given, with its header, and reports the result), markup (answers with hostile markup),
// mcp-info (reports the params of session/new), env (reports the names of its environment), spawn (starts a child that sleeps and writes its
// pid to --pidfile), flood (writes thousands of chunks), hang-start (never answers initialize),
// hang-turn (never ends a turn and ignores session/cancel), deaf (ignores the close of its
// input), exit-start (exits before answering initialize), exit-turn (exits during a turn),
// badline (writes a line that is not JSON), longline (writes a line over the limit),
// no-http, bad-version and sign-in (the failures of the start).
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type msg struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   any             `json:"error,omitempty"`
}

var (
	out    = bufio.NewWriter(os.Stdout)
	outMu  sync.Mutex
	mode   string
	waiter = map[string]chan json.RawMessage{}
	waitMu sync.Mutex
	nextID = 1000
	mcp    []struct {
		URL     string `json:"url"`
		Headers []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"headers"`
	}
)

func send(m msg) {
	m.JSONRPC = "2.0"

	raw, err := json.Marshal(m)
	if err != nil {
		return
	}

	outMu.Lock()
	defer outMu.Unlock()

	_, _ = out.Write(append(raw, '\n'))
	_ = out.Flush()
}

func update(u map[string]any) {
	raw, err := json.Marshal(map[string]any{"sessionId": "sess_fake", "update": u})
	if err != nil {
		return
	}

	send(msg{Method: "session/update", Params: raw})
}

func chunk(text string) {
	update(map[string]any{
		"sessionUpdate": "agent_message_chunk", "messageId": "m1",
		"content": map[string]any{"type": "text", "text": text},
	})
}

func respond(id json.RawMessage, result any) { send(msg{ID: id, Result: result}) }

func main() {
	pidfile := flag.String("pidfile", "", "write the pid of the child of spawn mode here")
	flag.StringVar(&mode, "mode", "echo", "behaviour")
	flag.Parse()

	if mode == "sleep" {
		time.Sleep(10 * time.Minute)
		return
	}

	if mode == "hang-start" {
		time.Sleep(10 * time.Minute)
		return
	}

	sc := bufio.NewReaderSize(os.Stdin, 1<<20)

	for {
		line, err := sc.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			var m msg
			if json.Unmarshal(line, &m) == nil {
				handle(m, *pidfile)
			}
		}

		if err != nil {
			if err == io.EOF && (mode == "deaf" || mode == "hang-turn") {
				time.Sleep(10 * time.Minute)
			}

			return
		}
	}
}

func handle(m msg, pidfile string) {
	if m.Method == "" && len(m.ID) > 0 { // a response to a request of ours
		waitMu.Lock()
		ch := waiter[string(m.ID)]
		waitMu.Unlock()

		if ch != nil {
			raw, err := json.Marshal(m.Result)
			if err != nil {
				return
			}

			ch <- raw
		}

		return
	}

	switch m.Method {
	case "initialize":
		switch mode {
		case "exit-start":
			os.Exit(3)
		case "badline":
			fmt.Fprintln(os.Stdout, "this is not json")
			return
		case "longline":
			_, _ = os.Stdout.WriteString(strings.Repeat("a", 2<<20) + "\n")
			return
		}

		version, hasHTTP := 1, mode != "no-http"
		if mode == "bad-version" {
			version = 99
		}

		respond(m.ID, map[string]any{
			"protocolVersion": version,
			"agentCapabilities": map[string]any{
				"mcpCapabilities": map[string]any{"http": hasHTTP, "sse": false},
			},
			"authMethods": []any{},
			"agentInfo":   map[string]any{"name": "fakeagent", "version": "0"},
		})
	case "session/new":
		if mode == "sign-in" {
			send(msg{ID: m.ID, Error: map[string]any{"code": -32000, "message": "authentication required"}})
			return
		}

		var p struct {
			MCP []struct {
				URL     string `json:"url"`
				Headers []struct {
					Name  string `json:"name"`
					Value string `json:"value"`
				} `json:"headers"`
			} `json:"mcpServers"`
		}

		_ = json.Unmarshal(m.Params, &p)
		mcp = p.MCP
		newParams = string(m.Params)

		respond(m.ID, map[string]any{"sessionId": "sess_fake"})
	case "session/prompt":
		go prompt(m, pidfile)
	case "session/cancel":
		if mode != "hang-turn" {
			select {
			case cancelled <- struct{}{}:
			default:
			}
		}
	default:
		if len(m.ID) > 0 {
			send(msg{ID: m.ID, Error: map[string]any{"code": -32601, "message": "method not found"}})
		}
	}
}

var (
	cancelled = make(chan struct{}, 4)
	// newParams are the params of session/new, as the agent got them.
	newParams string
)

func promptText(m msg) string {
	var p struct {
		Prompt []struct {
			Text string `json:"text"`
		} `json:"prompt"`
	}

	_ = json.Unmarshal(m.Params, &p)

	var parts []string
	for _, b := range p.Prompt {
		parts = append(parts, b.Text)
	}

	return strings.Join(parts, "\n")
}

func prompt(m msg, pidfile string) {
	const sid = "sess_fake"

	text := promptText(m)
	end := func(reason string) { respond(m.ID, map[string]any{"stopReason": reason}) }

	switch mode {
	case "echo":
		chunk("echo: ")
		chunk(text)
		update(map[string]any{
			"sessionUpdate": "tool_call", "toolCallId": "call_1", "title": "Reading the project", "kind": "read",
			"status": "pending",
		})
		update(map[string]any{
			"sessionUpdate": "tool_call_update",
			"toolCallId":    "call_1",
			"status":        "completed",
			"content": []any{
				map[string]any{"type": "content", "content": map[string]any{"type": "text", "text": "done"}},
			},
		})
		update(map[string]any{
			"sessionUpdate": "agent_thought_chunk", "content": map[string]any{"type": "text", "text": "thinking"},
		})
		end("end_turn")
	case "markup":
		chunk("<img src=x onerror=alert(1)> **bold** [a](javascript:alert(1)) \u202e```\n# not a heading")
		end("end_turn")
	case "permission":
		answer := askPermission(sid)
		chunk("permission: " + answer)
		end("end_turn")
	case "mcp":
		chunk(callMCP(text))
		end("end_turn")
	case "mcp-info":
		chunk(newParams)
		end("end_turn")
	case "env":
		var names []string
		for _, kv := range os.Environ() {
			names = append(names, strings.SplitN(kv, "=", 2)[0])
		}

		sort.Strings(names)
		chunk("env: " + strings.Join(names, ","))
		end("end_turn")
	case "spawn":
		c := exec.CommandContext(context.Background(), os.Args[0], "--mode=sleep") //nolint:gosec // its own copy
		if err := c.Start(); err == nil && pidfile != "" {
			_ = os.WriteFile(pidfile, []byte(strconv.Itoa(c.Process.Pid)), 0o600) //nolint:gosec // a path the test gave
		}

		chunk("spawned")
		end("end_turn")
	case "flood":
		for i := range 20000 {
			update(map[string]any{
				"sessionUpdate": "tool_call", "toolCallId": "c" + strconv.Itoa(i), "title": "t", "status": "pending",
			})
		}

		end("end_turn")
	case "hang-turn":
		chunk("working")
		select {} // never ends, ignores session/cancel
	case "deaf":
		chunk("ok")
		end("end_turn")
	case "exit-turn":
		chunk("about to die")
		os.Exit(4)
	case "cancellable":
		chunk("working")
		<-cancelled
		end("cancelled")
	default:
		end("end_turn")
	}
}

func askPermission(sid string) string {
	nextID++
	id := json.RawMessage(strconv.Itoa(nextID))
	ch := make(chan json.RawMessage, 1)

	waitMu.Lock()
	waiter[string(id)] = ch
	waitMu.Unlock()

	params, err := json.Marshal(map[string]any{
		"sessionId": sid,
		"toolCall":  map[string]any{"toolCallId": "call_p", "title": "Delete every clip <b>now</b>", "kind": "delete"},
		"options": []any{
			map[string]any{"optionId": "always", "name": "Always allow", "kind": "allow_always"},
			map[string]any{"optionId": "once", "name": "Allow once", "kind": "allow_once"},
			map[string]any{"optionId": "no", "name": "Reject", "kind": "reject_once"},
		},
	})
	if err != nil {
		return "marshal"
	}

	send(msg{ID: id, Method: "session/request_permission", Params: params})

	select {
	case raw := <-ch:
		var r struct {
			Outcome struct {
				Outcome  string `json:"outcome"`
				OptionID string `json:"optionId"`
			} `json:"outcome"`
		}

		_ = json.Unmarshal(raw, &r)

		return r.Outcome.Outcome + " " + r.Outcome.OptionID
	case <-time.After(time.Minute):
		return "timeout"
	}
}

// callMCP runs "mcp <tool> <json arguments>" against the first MCP server it was given.
func callMCP(text string) string {
	// The person's message is the last line: the sentence Stuarty puts ahead of the first message
	// comes before it.
	lines := strings.Split(strings.TrimSpace(text), "\n")
	fields := strings.SplitN(strings.TrimSpace(lines[len(lines)-1]), " ", 3)

	if len(fields) < 2 || fields[0] != "mcp" || len(mcp) == 0 {
		return "mcp: nothing to call"
	}

	args := "{}"
	if len(fields) == 3 {
		args = fields[2]
	}

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":` + strconv.Quote(fields[1]) +
		`,"arguments":` + args + `}}`

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, mcp[0].URL, strings.NewReader(body))
	if err != nil {
		return "mcp: " + err.Error()
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	for _, h := range mcp[0].Headers {
		req.Header.Set(h.Name, h.Value)
	}

	client := &http.Client{Timeout: 30 * time.Second} //nolint:forbidigo // loopback only

	resp, err := client.Do(req)
	if err != nil {
		return "mcp: " + err.Error()
	}

	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	return "mcp " + strconv.Itoa(resp.StatusCode) + ": " + string(raw)
}
