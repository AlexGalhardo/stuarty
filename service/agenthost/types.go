package agenthost

import (
	"context"
	"errors"
	"time"

	"github.com/AlexGalhardo/stuarty/core/chat"
)

// Profile is an agent a person may host: a name, an executable and its arguments (SDD-017
// section 3). Command is an absolute path.
type Profile struct {
	Name      string    `json:"name"`
	Command   string    `json:"command"`
	Args      []string  `json:"args"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Session is a chat session as the store keeps it (SDD-017 section 9).
type Session struct {
	ID          string         `json:"id"`
	ProjectID   string         `json:"project_id"`
	Profile     string         `json:"profile"`
	OwnerUserID string         `json:"-"`
	StartedBy   string         `json:"started_by"`
	State       chat.State     `json:"state"`
	EndReason   chat.EndReason `json:"end_reason,omitempty"`
	Title       string         `json:"title"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	EndedAt     *time.Time     `json:"ended_at,omitempty"`
	EventCount  int            `json:"event_count"`
}

// Store is where profiles, sessions and transcripts live. The SQLite implementation is in
// web/app/store; the host knows only this interface.
type Store interface {
	ProfileList(ctx context.Context) ([]Profile, error)
	ProfileGet(ctx context.Context, name string) (*Profile, error)
	ProfileSet(ctx context.Context, p Profile) error
	ProfileRemove(ctx context.Context, name string) error

	SessionCreate(ctx context.Context, s Session) error
	// SessionUpdate writes the mutable members: state, end reason, title, update time, end
	// time and event count.
	SessionUpdate(ctx context.Context, s Session) error
	SessionGet(ctx context.Context, id string) (*Session, error)
	// SessionList is the sessions of a project, newest first.
	SessionList(ctx context.Context, projectID string) ([]Session, error)
	// SessionDelete removes a session and its events.
	SessionDelete(ctx context.Context, id string) error
	// SessionsEndLive marks every session that is not ended as ended with the reason
	// shutdown (a server that died); it returns how many.
	SessionsEndLive(ctx context.Context, at time.Time) (int, error)
	// SessionsPrune deletes the oldest ended sessions of a project beyond keep.
	SessionsPrune(ctx context.Context, projectID string, keep int) error

	// EventUpsert writes one event of a session, replacing the one with the same seq.
	EventUpsert(ctx context.Context, sessionID string, e chat.Event) error
	// EventList is the events with seq above after, oldest first, at most limit.
	EventList(ctx context.Context, sessionID string, after int64, limit int) ([]chat.Event, error)
}

// ErrNotFound is what a Store answers for a name or an id that does not exist.
var ErrNotFound = errors.New("agenthost: not found")

// Grant is what a session token stands for (SDD-017 section 6.1).
type Grant struct {
	SessionID string
	ProjectID string
	// Owner is the user who started the session in network mode; empty in local mode.
	Owner string
	// Scopes are read, edit, media and render: never admin.
	Scopes []string
}

// GrantScopes are the scopes of every session token: never admin.
func GrantScopes() []string { return []string{"read", "edit", "media", "render"} }

// TokenIssuer hands out the credential an agent uses to reach Stuarty's MCP server. The secret
// is for the protocol message to the child and nowhere else.
type TokenIssuer interface {
	// Issue returns a secret bound to the grant, and a function that revokes it.
	Issue(g Grant, expires time.Time) (secret string, revoke func(), err error)
}

// StartError is why a session could not be started; Reason is the details.reason of
// E_AGENT_UNAVAILABLE (SDD-017 section 11).
type StartError struct {
	Reason string
	Detail string
}

func (e *StartError) Error() string { return "agenthost: the agent could not be started: " + e.Reason }

// The reasons of a StartError.
const (
	ReasonNotFound        = "not_found"
	ReasonStartFailed     = "start_failed"
	ReasonExited          = "exited"
	ReasonTimeout         = "timeout"
	ReasonProtocol        = "protocol"
	ReasonProtocolVersion = "protocol_version"
	ReasonNoHTTPMCP       = "no_http_mcp"
	ReasonSignIn          = "sign_in_required"
	ReasonLimit           = "limit"
)

// The refusals of a live session, for the service to turn into E_INVALID_STATE and
// E_CHAT_NOT_FOUND.
var (
	ErrNoSession   = errors.New("agenthost: no live session")
	ErrNoRequest   = errors.New("agenthost: no such permission request")
	ErrAnswered    = errors.New("agenthost: the permission request was answered")
	ErrHostClosing = errors.New("agenthost: the host is shutting down")
)
