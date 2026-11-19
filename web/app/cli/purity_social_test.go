package cli

import (
	"context"

	"github.com/AlexGalhardo/stuarty/service"
)

func (s stub) SocialNetworkList(
	context.Context, service.SocialNetworkListRequest,
) (*service.SocialNetworkListResult, error) {
	return reply[service.SocialNetworkListResult](s.err)
}

func (s stub) SocialConnectBegin(
	context.Context, service.SocialConnectBeginRequest,
) (*service.SocialConnectBeginResult, error) {
	return reply[service.SocialConnectBeginResult](s.err)
}

func (s stub) SocialConnectComplete(
	context.Context, service.SocialConnectCompleteRequest,
) (*service.SocialAccount, error) {
	return reply[service.SocialAccount](s.err)
}

func (s stub) SocialAccountList(
	context.Context, service.SocialAccountListRequest,
) (*service.SocialAccountListResult, error) {
	return reply[service.SocialAccountListResult](s.err)
}

func (s stub) PublishValidate(
	context.Context, service.PublishValidateRequest,
) (*service.PublishValidateResult, error) {
	return reply[service.PublishValidateResult](s.err)
}

func (s stub) PublishNow(context.Context, service.PublishNowRequest) (*service.Job, error) {
	return reply[service.Job](s.err)
}

func (s stub) PublishStatus(
	context.Context, service.PublishStatusRequest,
) (*service.PublishStatusResult, error) {
	return reply[service.PublishStatusResult](s.err)
}
