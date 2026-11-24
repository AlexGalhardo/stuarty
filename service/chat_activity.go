package service

import (
	"context"
	"time"

	"github.com/AlexGalhardo/stuarty/core/chat"
	core "github.com/AlexGalhardo/stuarty/core/library"
	"github.com/AlexGalhardo/stuarty/service/library"
)

// ChatActivityResult is what a session is doing: the skill it read and the tool it called last
// (SDD-017 section 8.1). It is the result of chat.Reduce, told from the events of the session and
// nothing else; the titles of the tools and the names of the skills are text from an agent or from
// the library, and the notice says so.
type ChatActivityResult struct {
	SessionID       string             `json:"session_id"`
	State           chat.State         `json:"state"`
	Skill           chat.SkillSlot     `json:"skill"`
	SkillsLoaded    []chat.LoadedSkill `json:"skills_loaded"`
	Tool            *chat.ToolActivity `json:"tool"`
	UpdatedAt       time.Time          `json:"updated_at"`
	UntrustedNotice string             `json:"untrusted_notice"`
}

// ChatActivityGet returns the activity of one session. It is a read with the project check of the
// session's project: a session of another project, or of one the caller may not see, is as missing
// as an id that does not exist.
func (s *svc) ChatActivityGet(ctx context.Context, req ChatRef) (*ChatActivityResult, error) {
	c, row, err := s.chatSession(ctx, "ChatActivityGet", req.SessionID)
	if err != nil {
		return nil, err
	}

	events, err := s.activityEvents(ctx, row.ID)
	if err != nil {
		return nil, err
	}

	a := chat.Reduce(row.ID, row.State, events, s.reviewedSkillExists(ctx, c))

	if a.UpdatedAt.IsZero() {
		a.UpdatedAt = row.UpdatedAt
	}

	return &ChatActivityResult{
		SessionID: a.SessionID, State: a.State, Skill: a.Skill, SkillsLoaded: a.SkillsLoaded, Tool: a.Tool,
		UpdatedAt: a.UpdatedAt, UntrustedNotice: UntrustedNotice,
	}, nil
}

// activityEvents are the events the activity is told from. A live session keeps them in memory; an
// ended one is read from its transcript.
func (s *svc) activityEvents(ctx context.Context, id string) ([]chat.Event, error) {
	if s.agents != nil {
		if events, ok := s.agents.ActivityEvents(id); ok {
			return events, nil
		}
	}

	var all []chat.Event

	for after := int64(0); ; {
		page, err := s.store.Chat().EventList(ctx, id, after, chat.MaxTranscriptPage)
		if err != nil {
			return nil, asError(err)
		}

		all = append(all, page...)

		if len(page) < chat.MaxTranscriptPage {
			return all, nil
		}

		after = page[len(page)-1].Seq
	}
}

// reviewedSkillExists says whether the library holds a skill that a person reviewed, the one fact of
// the library that tells "available" from "none". A server with no library has none.
func (s *svc) reviewedSkillExists(ctx context.Context, c Caller) bool {
	if s.lib == nil {
		return false
	}

	rows, err := s.lib.List(ctx, core.KindSkill, library.Filter{ProjectID: c.ProjectID})

	return err == nil && len(rows) > 0
}
