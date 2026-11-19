package service

import (
	"context"
	"time"

	"github.com/AlexGalhardo/stuarty/core/chat"
	"github.com/AlexGalhardo/stuarty/service/agenthost"
)

// Codes of the hosted agent chat (SDD-017 section 11).
const (
	CodeAgentNotFound    = "E_AGENT_NOT_FOUND"
	CodeChatNotFound     = "E_CHAT_NOT_FOUND"
	CodeAgentUnavailable = "E_AGENT_UNAVAILABLE"
)

// ChatService is the hosted agent chat (SDD-017). The profiles are written, and a session is
// started, steered, answered and deleted, only by a person in a browser session; every other caller,
// the session token of a hosted agent included, gets E_PRIVILEGED_SESSION. The reads take the read
// scope and the project check of the session.
type ChatService interface {
	AgentProfileList(ctx context.Context, req AgentProfileListRequest) (*AgentProfileListResult, error)
	AgentProfileSet(ctx context.Context, req AgentProfileSetRequest) (*agenthost.Profile, error)
	AgentProfileRemove(ctx context.Context, req AgentProfileRemoveRequest) (*agenthost.Profile, error)
	ChatSessionStart(ctx context.Context, req ChatSessionStartRequest) (*ChatSession, error)
	ChatSend(ctx context.Context, req ChatSendRequest) (*ChatSession, error)
	ChatCancel(ctx context.Context, req ChatCancelRequest) (*ChatSession, error)
	ChatAnswerPermission(ctx context.Context, req ChatAnswerRequest) (*ChatSession, error)
	ChatSessionList(ctx context.Context, req ChatSessionListRequest) (*ChatSessionListResult, error)
	ChatSessionGet(ctx context.Context, req ChatRef) (*ChatSession, error)
	ChatTranscriptGet(ctx context.Context, req ChatTranscriptRequest) (*ChatTranscriptResult, error)
	ChatSessionDelete(ctx context.Context, req ChatRef) (*ChatSession, error)
}

// AgentProfileListRequest has no member.
type AgentProfileListRequest struct{}

// AgentSuggestion is a known agent (core/chat) and whether its program was found on this machine.
type AgentSuggestion struct {
	Name        string   `json:"name"`
	Title       string   `json:"title"`
	Executables []string `json:"executables"`
	Args        []string `json:"args"`
	Install     string   `json:"install"`
	// Found is true when one of the executables is on the search path of the server. FoundPath is
	// the absolute path found, shown to a person who may write a profile and to nobody else.
	Found     bool   `json:"found"`
	FoundPath string `json:"found_path,omitempty"`
}

// AgentProfileListResult is the profiles of the workspace and the suggestions (SDD-017 section 3).
type AgentProfileListResult struct {
	Profiles    []agenthost.Profile `json:"profiles"`
	Suggestions []AgentSuggestion   `json:"suggestions"`
	// CanWrite is true for the caller who may write a profile: a person with the admin scope.
	CanWrite bool `json:"can_write"`
	// Available is false when this process cannot host an agent (a command line client that
	// owns the workspace for one command).
	Available bool `json:"available"`
}

// AgentProfileSetRequest writes a profile: a name, an executable and its arguments.
type AgentProfileSetRequest struct {
	Name    string   `json:"name"`
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
}

// AgentProfileRemoveRequest names the profile.
type AgentProfileRemoveRequest struct {
	Name string `json:"name"`
}

// ChatSession is a chat session as a client sees it (SDD-017 section 9).
type ChatSession struct {
	ID         string         `json:"id"`
	ProjectID  string         `json:"project_id"`
	Profile    string         `json:"profile"`
	State      chat.State     `json:"state"`
	EndReason  chat.EndReason `json:"end_reason,omitempty"`
	Title      string         `json:"title"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	EndedAt    *time.Time     `json:"ended_at,omitempty"`
	EventCount int            `json:"event_count"`
	// Live is true while the agent process of the session runs.
	Live bool `json:"live"`
}

// ChatSessionStartRequest starts a session of a project with a profile.
type ChatSessionStartRequest struct {
	ProjectID string `json:"project_id"`
	Profile   string `json:"profile"`
}

// ChatSendRequest sends the next message of the person to the agent.
type ChatSendRequest struct {
	SessionID string `json:"session_id"`
	Text      string `json:"text"`
}

// ChatCancelRequest cancels the turn in progress; with End it ends the session.
type ChatCancelRequest struct {
	SessionID string `json:"session_id"`
	End       bool   `json:"end,omitempty"`
}

// ChatAnswerRequest answers a permission request of the agent: allow, reject or cancel.
type ChatAnswerRequest struct {
	SessionID string `json:"session_id"`
	RequestID string `json:"request_id"`
	Answer    string `json:"answer"`
}

// ChatRef names a session.
type ChatRef struct {
	SessionID string `json:"session_id"`
}

// ChatSessionListRequest lists the sessions of a project, newest first.
type ChatSessionListRequest struct {
	ProjectID string `json:"project_id"`
}

// ChatSessionListResult is the sessions of a project.
type ChatSessionListResult struct {
	Sessions []ChatSession `json:"sessions"`
}

// Transcript formats.
const (
	ChatFormatJSON     = "json"
	ChatFormatMarkdown = "markdown"
)

// ChatTranscriptRequest reads the events of a session after a number (JSON), or the whole
// transcript as Markdown.
type ChatTranscriptRequest struct {
	SessionID string `json:"session_id"`
	After     int64  `json:"after,omitempty"`
	Limit     int    `json:"limit,omitempty"`
	Format    string `json:"format,omitempty"`
}

// ChatTranscriptResult is a page of events, or the Markdown of the whole transcript. Every text
// in it is untrusted (SDD-007 section 8).
type ChatTranscriptResult struct {
	Session ChatSession  `json:"session"`
	Events  []chat.Event `json:"events,omitempty"`
	// NextAfter is the number to ask for the next page; nil when there is none.
	NextAfter *int64 `json:"next_after,omitempty"`
	Markdown  string `json:"markdown,omitempty"`
}
