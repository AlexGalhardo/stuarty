package cli

import (
	"context"

	"github.com/AlexGalhardo/stuarty/service"
)

func (s stub) TextList(context.Context, service.TextListRequest) (*service.TextListResult, error) {
	return reply[service.TextListResult](s.err)
}

func (s stub) TextGet(context.Context, service.TextGetRequest) (*service.TextGetResult, error) {
	return reply[service.TextGetResult](s.err)
}

func (s stub) TextWrite(context.Context, service.TextWriteRequest) (*service.TextWriteResult, error) {
	return reply[service.TextWriteResult](s.err)
}

func (s stub) TextRevisions(context.Context, service.TextRevisionsRequest) (*service.TextRevisionsResult, error) {
	return reply[service.TextRevisionsResult](s.err)
}

func (s stub) TextRevert(context.Context, service.TextRevertRequest) (*service.TextWriteResult, error) {
	return reply[service.TextWriteResult](s.err)
}

func (s stub) TextDraft(context.Context, service.TextDraftRequest) (*service.Job, error) {
	return reply[service.Job](s.err)
}

func (s stub) CaptionValidate(
	context.Context,
	service.CaptionValidateRequest,
) (*service.CaptionValidateResult, error) {
	return reply[service.CaptionValidateResult](s.err)
}

func (s stub) ScriptEstimate(context.Context, service.ScriptEstimateRequest) (*service.ScriptEstimateResult, error) {
	return reply[service.ScriptEstimateResult](s.err)
}
