package service

import (
	"context"

	"github.com/AlexGalhardo/stuarty/core/command"
)

// TextDraft asks a model for a draft of a text (SDD-019 section 6).
func (s *svc) TextDraft(_ context.Context, _ TextDraftRequest) (*Job, error) {
	return nil, newError(command.CodeInternal, "TextDraft is not available yet.")
}
