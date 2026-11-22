package remote

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/agenthost"
)

// The hosted agent chat (SDD-017). A command line client reads the sessions and the profiles of a
// running server; the operations that act are for a person in the browser, and a client that
// sends them gets E_PRIVILEGED_SESSION from the server.

// AgentProfileList implements service.Service.
func (c *Client) AgentProfileList(
	ctx context.Context, _ service.AgentProfileListRequest,
) (*service.AgentProfileListResult, error) {
	return get[service.AgentProfileListResult](ctx, c, "/agents/profiles", nil)
}

// AgentProfileSet implements service.Service.
func (c *Client) AgentProfileSet(
	ctx context.Context, req service.AgentProfileSetRequest,
) (*agenthost.Profile, error) {
	body := struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
	}{req.Command, req.Args}

	out := new(agenthost.Profile)
	if err := c.do(ctx, http.MethodPut, "/agents/profiles/"+segment(req.Name), nil, body, out); err != nil {
		return nil, err
	}

	return out, nil
}

// AgentProfileRemove implements service.Service.
func (c *Client) AgentProfileRemove(
	ctx context.Context, req service.AgentProfileRemoveRequest,
) (*agenthost.Profile, error) {
	out := new(agenthost.Profile)
	if err := c.do(ctx, http.MethodDelete, "/agents/profiles/"+segment(req.Name), nil, nil, out); err != nil {
		return nil, err
	}

	return out, nil
}

// ChatSessionStart implements service.Service.
func (c *Client) ChatSessionStart(
	ctx context.Context,
	req service.ChatSessionStartRequest,
) (*service.ChatSession, error) {
	body := struct {
		Profile string `json:"profile"`
	}{req.Profile}

	return post[service.ChatSession](ctx, c, "/projects/"+segment(req.ProjectID)+"/chat/sessions", body)
}

// ChatSend implements service.Service.
func (c *Client) ChatSend(ctx context.Context, req service.ChatSendRequest) (*service.ChatSession, error) {
	body := struct {
		Text string `json:"text"`
	}{req.Text}

	return post[service.ChatSession](ctx, c, "/chat/sessions/"+segment(req.SessionID)+"/messages", body)
}

// ChatCancel implements service.Service.
func (c *Client) ChatCancel(ctx context.Context, req service.ChatCancelRequest) (*service.ChatSession, error) {
	body := struct {
		End bool `json:"end"`
	}{req.End}

	return post[service.ChatSession](ctx, c, "/chat/sessions/"+segment(req.SessionID)+"/cancel", body)
}

// ChatAnswerPermission implements service.Service.
func (c *Client) ChatAnswerPermission(
	ctx context.Context, req service.ChatAnswerRequest,
) (*service.ChatSession, error) {
	body := struct {
		Answer string `json:"answer"`
	}{req.Answer}

	return post[service.ChatSession](ctx, c,
		"/chat/sessions/"+segment(req.SessionID)+"/permissions/"+segment(req.RequestID), body)
}

// ChatSessionList implements service.Service.
func (c *Client) ChatSessionList(
	ctx context.Context, req service.ChatSessionListRequest,
) (*service.ChatSessionListResult, error) {
	return get[service.ChatSessionListResult](ctx, c, "/projects/"+segment(req.ProjectID)+"/chat/sessions", nil)
}

// ChatSessionGet implements service.Service.
func (c *Client) ChatSessionGet(ctx context.Context, req service.ChatRef) (*service.ChatSession, error) {
	return get[service.ChatSession](ctx, c, "/chat/sessions/"+segment(req.SessionID), nil)
}

// ChatTranscriptGet implements service.Service.
func (c *Client) ChatTranscriptGet(
	ctx context.Context, req service.ChatTranscriptRequest,
) (*service.ChatTranscriptResult, error) {
	q := url.Values{}
	setIf(q, "format", req.Format)

	if req.After > 0 {
		q.Set("after", strconv.FormatInt(req.After, 10))
	}

	if req.Limit > 0 {
		q.Set("limit", strconv.Itoa(req.Limit))
	}

	return get[service.ChatTranscriptResult](ctx, c, "/chat/sessions/"+segment(req.SessionID)+"/transcript", q)
}

// ChatSessionDelete implements service.Service.
func (c *Client) ChatSessionDelete(ctx context.Context, req service.ChatRef) (*service.ChatSession, error) {
	out := new(service.ChatSession)
	if err := c.do(ctx, http.MethodDelete, "/chat/sessions/"+segment(req.SessionID), nil, nil, out); err != nil {
		return nil, err
	}

	return out, nil
}
