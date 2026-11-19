package httpapi

import (
	"context"
	"strconv"

	"github.com/AlexGalhardo/stuarty/core/chat"
	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/agenthost"

	"github.com/AlexGalhardo/vukem/web"
)

// ChatStreams is what the event stream of a chat session needs from the host of the hosted agent:
// the events already written and the ones to come (SDD-017 section 8). *agenthost.Host is one.
type ChatStreams interface {
	Subscribe(ctx context.Context, id string, after int64) (*agenthost.Subscription, error)
}

type agentProfileIn struct {
	Name    string   `json:"name"              validate:"required" doc:"The profile name: 1 to 32 characters from a-z, 0-9 and -."`
	Command string   `json:"command,omitempty"                     doc:"The program: an absolute path, or a name that is on the search path of the server. It is resolved when the profile is written."`
	Args    []string `json:"args,omitempty"                        doc:"The arguments, one string each; no shell is involved."`
}

type agentProfileRefIn struct {
	Name string `json:"name" validate:"required" doc:"The profile name."`
}

type chatStartIn struct {
	ID      string `json:"id"      validate:"required" doc:"The project id, prj_ and 10 characters."`
	Profile string `json:"profile" validate:"required" doc:"The name of the agent profile to start."`
}

type chatProjectIn struct {
	ID string `json:"id" validate:"required" doc:"The project id, prj_ and 10 characters."`
}

type chatSessionIn struct {
	ID string `json:"id" validate:"required" doc:"A chat session id, cht_ and 10 characters."`
}

type chatSendIn struct {
	ID   string `json:"id"   validate:"required" doc:"A chat session id, cht_ and 10 characters."`
	Text string `json:"text" validate:"required" doc:"The message to the agent, at most 20000 characters of plain text."`
}

type chatCancelIn struct {
	ID  string `json:"id"            validate:"required" doc:"A chat session id, cht_ and 10 characters."`
	End bool   `json:"end,omitempty"                     doc:"true also ends the session and stops the agent."`
}

type chatAnswerIn struct {
	ID        string `json:"id"         validate:"required" doc:"A chat session id, cht_ and 10 characters."`
	RequestID string `json:"request_id" validate:"required" doc:"A permission request id, cpr_ and 10 characters."`
	Answer    string `json:"answer"     validate:"required" doc:"allow (once), reject (once) or cancel. A standing choice is never offered."`
}

type chatTranscriptIn struct {
	ID     string `json:"id"               validate:"required" doc:"A chat session id, cht_ and 10 characters."`
	Format string `json:"format,omitempty"                     doc:"json (the default: events after a number) or markdown (the whole transcript)."`
	After  int64  `json:"after,omitempty"                      doc:"Only the events with a number above this one (json)."`
	Limit  int    `json:"limit,omitempty"                      doc:"The most events in the page (json), at most 500."`
}

type chatStreamIn struct {
	ID    string `json:"id"              validate:"required" doc:"A chat session id, cht_ and 10 characters."`
	After int64  `json:"after,omitempty"                     doc:"The number of the last event the client holds; Last-Event-ID wins."`
}

func (h *apiHandler) agentProfilesList(c *web.Ctx, _ noIn) (service.AgentProfileListResult, error) {
	res, err := h.Service.AgentProfileList(c.Context(), service.AgentProfileListRequest{})

	return deref(res), err
}

func (h *apiHandler) agentProfilesSet(c *web.Ctx, in agentProfileIn) (agenthost.Profile, error) {
	res, err := h.Service.AgentProfileSet(c.Context(), service.AgentProfileSetRequest(in))

	return deref(res), err
}

func (h *apiHandler) agentProfilesRemove(c *web.Ctx, in agentProfileRefIn) (agenthost.Profile, error) {
	res, err := h.Service.AgentProfileRemove(c.Context(), service.AgentProfileRemoveRequest{Name: in.Name})

	return deref(res), err
}

func (h *apiHandler) chatStart(c *web.Ctx, in chatStartIn) (service.ChatSession, error) {
	res, err := h.Service.ChatSessionStart(c.Context(), service.ChatSessionStartRequest{
		ProjectID: in.ID, Profile: in.Profile,
	})

	return deref(res), err
}

func (h *apiHandler) chatList(c *web.Ctx, in chatProjectIn) (service.ChatSessionListResult, error) {
	res, err := h.Service.ChatSessionList(c.Context(), service.ChatSessionListRequest{ProjectID: in.ID})

	return deref(res), err
}

func (h *apiHandler) chatGet(c *web.Ctx, in chatSessionIn) (service.ChatSession, error) {
	res, err := h.Service.ChatSessionGet(c.Context(), service.ChatRef{SessionID: in.ID})

	return deref(res), err
}

func (h *apiHandler) chatTranscript(c *web.Ctx, in chatTranscriptIn) (service.ChatTranscriptResult, error) {
	res, err := h.Service.ChatTranscriptGet(c.Context(), service.ChatTranscriptRequest{
		SessionID: in.ID, After: in.After, Limit: in.Limit, Format: in.Format,
	})

	return deref(res), err
}

func (h *apiHandler) chatSend(c *web.Ctx, in chatSendIn) (service.ChatSession, error) {
	res, err := h.Service.ChatSend(c.Context(), service.ChatSendRequest{SessionID: in.ID, Text: in.Text})

	return deref(res), err
}

func (h *apiHandler) chatCancel(c *web.Ctx, in chatCancelIn) (service.ChatSession, error) {
	res, err := h.Service.ChatCancel(c.Context(), service.ChatCancelRequest{SessionID: in.ID, End: in.End})

	return deref(res), err
}

func (h *apiHandler) chatAnswer(c *web.Ctx, in chatAnswerIn) (service.ChatSession, error) {
	res, err := h.Service.ChatAnswerPermission(c.Context(), service.ChatAnswerRequest{
		SessionID: in.ID, RequestID: in.RequestID, Answer: in.Answer,
	})

	return deref(res), err
}

func (h *apiHandler) chatDelete(c *web.Ctx, in chatSessionIn) (service.ChatSession, error) {
	res, err := h.Service.ChatSessionDelete(c.Context(), service.ChatRef{SessionID: in.ID})

	return deref(res), err
}

// chatStream is GET /chat/sessions/{id}/events (SDD-017 section 8): the events of a session after the
// one the client holds, then the live ones for a session that runs. The Service decides who may see
// the session; everything that can fail is checked before the response becomes a stream.
func (h *apiHandler) chatStream(c *web.Ctx, in chatStreamIn) (noIn, error) {
	if h.Events == nil {
		return noIn{}, &service.Error{
			Code:    command.CodeInternal,
			Message: "the event stream is not available in this build.",
		}
	}

	check := func() (*service.ChatSession, error) {
		return h.Service.ChatSessionGet(c.Context(), service.ChatRef{SessionID: in.ID})
	}

	session, err := check()
	if err != nil {
		return noIn{}, err
	}

	// The id of a frame is the number of its event, and an event that grows is sent in pieces under
	// one id, so a client that reconnects may hold only the start of the last one it saw: it is
	// sent that event again, whole, and the client replaces it.
	after := in.After
	if header := c.Request().Header.Get("Last-Event-ID"); header != "" {
		if n, perr := strconv.ParseInt(header, 10, 64); perr == nil {
			after = n
		}
	}

	after = max(after-1, 0)

	authorized := func() bool { _, err := check(); return err == nil }

	if h.Chat != nil && session.Live {
		if sub, serr := h.Chat.Subscribe(c.Context(), in.ID, after); serr == nil {
			defer sub.Close()

			return noIn{}, h.Events.ServeChat(c, in.ID, sub.Snapshot, sub.Events, authorized)
		}
		// The session ended between the two calls: it is read from the store.
	}

	var replay []chat.Event

	for {
		page, terr := h.Service.ChatTranscriptGet(c.Context(), service.ChatTranscriptRequest{
			SessionID: in.ID, After: after, Limit: chat.MaxTranscriptPage,
		})
		if terr != nil {
			return noIn{}, terr
		}

		replay = append(replay, page.Events...)

		if page.NextAfter == nil {
			break
		}

		after = *page.NextAfter
	}

	return noIn{}, h.Events.ServeChat(c, in.ID, replay, nil, authorized)
}
