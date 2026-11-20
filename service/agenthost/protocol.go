package agenthost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/chat"
)

// Methods of the protocol that this client uses (SDD-017 section 2).
const (
	MethodInitialize        = "initialize"
	MethodSessionNew        = "session/new"
	MethodSessionPrompt     = "session/prompt"
	MethodSessionCancel     = "session/cancel"
	MethodSessionUpdate     = "session/update"
	MethodRequestPermission = "session/request_permission"
)

// InitializeResult is what the agent says about itself; only what the client acts on is read.
type InitializeResult struct {
	ProtocolVersion   int `json:"protocolVersion"`
	AgentCapabilities struct {
		MCP struct {
			HTTP bool `json:"http"`
			SSE  bool `json:"sse"`
		} `json:"mcpCapabilities"`
	} `json:"agentCapabilities"`
	AuthMethods []json.RawMessage `json:"authMethods"`
}

// Initialize opens the protocol. The client offers no capability: the agent uses its own tools
// and Stuarty's MCP tools, and reads no file and runs no terminal through the client.
func (c *Conn) Initialize(ctx context.Context, version string) (*InitializeResult, error) {
	var res InitializeResult

	err := c.Call(ctx, MethodInitialize, map[string]any{
		"protocolVersion":    ProtocolVersion,
		"clientCapabilities": map[string]any{},
		"clientInfo":         map[string]any{"name": "stuarty", "title": "Stuarty", "version": version},
	}, &res)
	if err != nil {
		return nil, err
	}

	return &res, nil
}

// Header is a name and a value of an MCP server entry.
type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// MCPServer is an entry of mcpServers: an HTTP server and the header that authenticates to it.
type MCPServer struct {
	Type    string   `json:"type"`
	Name    string   `json:"name"`
	URL     string   `json:"url"`
	Headers []Header `json:"headers"`
}

// ErrSignInRequired is the agent saying that its own sign-in is missing.
var ErrSignInRequired = errors.New("agenthost: the agent needs the user to sign in to it first")

// NewSession opens a session in the working directory cwd with the given MCP servers.
func (c *Conn) NewSession(ctx context.Context, cwd string, servers []MCPServer) (string, error) {
	if servers == nil {
		servers = []MCPServer{}
	}

	var res struct {
		SessionID string `json:"sessionId"`
	}

	err := c.Call(ctx, MethodSessionNew, map[string]any{"cwd": cwd, "mcpServers": servers}, &res)

	var rpc *RPCError
	if errors.As(err, &rpc) && rpc.Code == -32000 {
		return "", ErrSignInRequired
	}

	if err != nil {
		return "", err
	}

	if res.SessionID == "" || len(res.SessionID) > 256 {
		return "", fmt.Errorf("%w: the agent opened a session with no usable id", ErrProtocol)
	}

	return res.SessionID, nil
}

// Prompt sends one message, as one text block per argument, and waits for the turn to end; it
// returns the stop reason.
func (c *Conn) Prompt(ctx context.Context, sessionID string, blocks ...string) (string, error) {
	var res struct {
		StopReason string `json:"stopReason"`
	}

	prompt := make([]map[string]any, 0, len(blocks))
	for _, b := range blocks {
		prompt = append(prompt, map[string]any{"type": "text", "text": b})
	}

	err := c.Call(ctx, MethodSessionPrompt, map[string]any{"sessionId": sessionID, "prompt": prompt}, &res)
	if err != nil {
		return "", err
	}

	return res.StopReason, nil
}

// Cancel asks the agent to stop the turn in progress.
func (c *Conn) Cancel(sessionID string) error {
	return c.Notify(MethodSessionCancel, map[string]any{"sessionId": sessionID})
}

// Update is a session/update, reduced to what the transcript keeps. Every text is cleaned and
// clipped: it is untrusted (SDD-017 section 8).
type Update struct {
	SessionID string
	// Kind is the chat event kind the update becomes: agent_message, agent_thought, tool_call
	// or plan.
	Kind chat.Kind
	// MessageID groups the chunks of one message; empty when the agent sent none.
	MessageID string
	Text      string
	// Tool is set for tool_call and tool_call_update. Fields the update left out are empty and
	// Has says which were present.
	Tool    *chat.Tool
	HasTool map[string]bool
}

type contentBlock struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	URI      string `json:"uri"`
	Name     string `json:"name"`
	Resource *struct {
		URI  string `json:"uri"`
		Text string `json:"text"`
	} `json:"resource"`
}

// text is what a content block shows as text. A file or an image is named, never fetched and
// never embedded.
func (b contentBlock) text() string {
	switch b.Type {
	case "text":
		return b.Text
	case "image", "audio":
		return "[" + b.Type + "]"
	case "resource_link":
		return "[resource] " + b.Name + " " + b.URI
	case "resource":
		if b.Resource != nil {
			if b.Resource.Text != "" {
				return b.Resource.Text
			}

			return "[resource] " + b.Resource.URI
		}
	}

	return ""
}

type toolContent struct {
	Type    string        `json:"type"`
	Content *contentBlock `json:"content"`
	Path    string        `json:"path"`
}

// ParseUpdate reads the params of a session/update. ok is false for an update the transcript
// does not keep (a mode change, the list of commands, a variant of a later protocol).
func ParseUpdate(params json.RawMessage) (Update, bool) {
	var p struct {
		SessionID string `json:"sessionId"`
		Update    struct {
			Kind       string          `json:"sessionUpdate"`
			MessageID  string          `json:"messageId"`
			Content    json.RawMessage `json:"content"`
			ToolCallID string          `json:"toolCallId"`
			Title      *string         `json:"title"`
			ToolKind   *string         `json:"kind"`
			Status     *string         `json:"status"`
			Entries    []struct {
				Content  string `json:"content"`
				Priority string `json:"priority"`
				Status   string `json:"status"`
			} `json:"entries"`
		} `json:"update"`
	}

	if json.Unmarshal(params, &p) != nil || p.SessionID == "" {
		return Update{}, false
	}

	u := Update{SessionID: p.SessionID, MessageID: clean(p.Update.MessageID, 128)}

	switch p.Update.Kind {
	case "agent_message_chunk", "agent_thought_chunk":
		u.Kind = chat.KindAgentMessage
		if p.Update.Kind == "agent_thought_chunk" {
			u.Kind = chat.KindAgentThought
		}

		var b contentBlock
		if json.Unmarshal(p.Update.Content, &b) != nil {
			return Update{}, false
		}

		u.Text = chat.CleanText(b.text())

		return u, true
	case "plan":
		u.Kind = chat.KindPlan

		var lines []string
		for _, e := range p.Update.Entries {
			lines = append(lines, fmt.Sprintf("[%s] %s", clean(e.Status, 20), clean(e.Content, 500)))
		}

		u.Text = chat.CleanText(strings.Join(lines, "\n"))

		return u, true
	case "tool_call", "tool_call_update":
		u.Kind = chat.KindToolCall
		u.Tool = &chat.Tool{CallID: clean(p.Update.ToolCallID, 128)}
		u.HasTool = map[string]bool{}

		if u.Tool.CallID == "" {
			return Update{}, false
		}

		if p.Update.Title != nil {
			u.Tool.Title, u.HasTool["title"] = clean(*p.Update.Title, 500), true
		}

		if p.Update.ToolKind != nil {
			u.Tool.Kind, u.HasTool["kind"] = clean(*p.Update.ToolKind, 32), true
		}

		u.Tool.Status = chat.ToolPending

		if p.Update.Status != nil {
			u.Tool.Status, u.HasTool["status"] = toolStatus(*p.Update.Status), true
		}

		if out, ok := toolOutput(p.Update.Content); ok {
			u.Tool.Output, u.HasTool["output"] = out, true
		}

		return u, true
	}

	return Update{}, false
}

func toolStatus(s string) chat.ToolStatus {
	switch chat.ToolStatus(s) {
	case chat.ToolPending, chat.ToolInProgress, chat.ToolCompleted, chat.ToolFailed:
		return chat.ToolStatus(s)
	}

	return chat.ToolPending
}

// toolOutput joins the text of the content of a tool call.
func toolOutput(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", false
	}

	var items []toolContent
	if json.Unmarshal(raw, &items) != nil {
		return "", false
	}

	var parts []string

	for _, it := range items {
		switch it.Type {
		case "content":
			if it.Content != nil {
				parts = append(parts, it.Content.text())
			}
		case "diff":
			parts = append(parts, "[diff] "+it.Path)
		case "terminal":
			parts = append(parts, "[terminal]")
		}
	}

	out, _ := chat.Clip(chat.CleanText(strings.Join(parts, "\n")), chat.MaxToolOutput)

	return out, true
}

func clean(s string, limit int) string {
	out, _ := chat.Clip(chat.CleanText(s), limit)

	return out
}

// PermissionRequest is a session/request_permission.
type PermissionRequest struct {
	SessionID string
	CallID    string
	Title     string
	ToolKind  string
	Options   []chat.Option
}

// ParsePermission reads the params of a session/request_permission.
func ParsePermission(params json.RawMessage) (PermissionRequest, error) {
	var p struct {
		SessionID string `json:"sessionId"`
		ToolCall  struct {
			ID    string `json:"toolCallId"`
			Title string `json:"title"`
			Kind  string `json:"kind"`
		} `json:"toolCall"`
		Options []struct {
			ID   string `json:"optionId"`
			Name string `json:"name"`
			Kind string `json:"kind"`
		} `json:"options"`
	}

	if err := json.Unmarshal(params, &p); err != nil || p.SessionID == "" {
		return PermissionRequest{}, errors.New("the permission request is not valid")
	}

	req := PermissionRequest{
		SessionID: p.SessionID, CallID: clean(p.ToolCall.ID, 128), Title: clean(p.ToolCall.Title, 500),
		ToolKind: clean(p.ToolCall.Kind, 32),
	}

	for _, o := range p.Options {
		req.Options = append(req.Options, chat.Option{ID: o.ID, Name: o.Name, Kind: chat.OptionKind(o.Kind)})
	}

	req.Options = chat.CleanOptions(req.Options)

	return req, nil
}

// PermissionOutcome is the result sent for a permission request: a selected option, or
// cancelled.
func PermissionOutcome(optionID string, cancelled bool) map[string]any {
	if cancelled {
		return map[string]any{"outcome": map[string]any{"outcome": "cancelled"}}
	}

	return map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": optionID}}
}

// MethodNotFound is the error for a request the client offered no capability for.
func MethodNotFound(method string) *RPCError {
	return &RPCError{Code: codeMethodNotFound, Message: "method not found: " + clean(method, 64)}
}

// InvalidParams is the error for a request whose params are not valid.
func InvalidParams() *RPCError { return &RPCError{Code: codeInvalidParams, Message: "invalid params"} }
