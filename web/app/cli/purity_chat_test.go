package cli

import (
	"context"

	"github.com/AlexGalhardo/stuarty/service"
)

func (s stub) AgentProfileList(
	context.Context,
	service.AgentProfileListRequest,
) (*service.AgentProfileListResult, error) {
	return reply[service.AgentProfileListResult](s.err)
}

func (s stub) ChatSessionList(context.Context, service.ChatSessionListRequest) (*service.ChatSessionListResult, error) {
	return reply[service.ChatSessionListResult](s.err)
}

func (s stub) ChatSessionGet(context.Context, service.ChatRef) (*service.ChatSession, error) {
	return reply[service.ChatSession](s.err)
}

func (s stub) ChatActivityGet(context.Context, service.ChatRef) (*service.ChatActivityResult, error) {
	return reply[service.ChatActivityResult](s.err)
}

func (s stub) InboxList(context.Context, service.InboxListRequest) (*service.InboxListResult, error) {
	return reply[service.InboxListResult](s.err)
}

func (s stub) ChatTranscriptGet(context.Context, service.ChatTranscriptRequest) (*service.ChatTranscriptResult, error) {
	return reply[service.ChatTranscriptResult](s.err)
}
