package cli

import (
	"context"

	"github.com/AlexGalhardo/stuarty/service"
)

func (s stub) PublicationCreate(context.Context, service.PublicationCreateRequest) (*service.Publication, error) {
	return reply[service.Publication](s.err)
}

func (s stub) PublicationUpdate(context.Context, service.PublicationUpdateRequest) (*service.Publication, error) {
	return reply[service.Publication](s.err)
}

func (s stub) PublicationGet(context.Context, service.PublicationGetRequest) (*service.Publication, error) {
	return reply[service.Publication](s.err)
}

func (s stub) PublicationList(
	context.Context, service.PublicationListRequest,
) (*service.PublicationListResult, error) {
	return reply[service.PublicationListResult](s.err)
}

func (s stub) PublicationSchedule(
	context.Context, service.PublicationScheduleRequest,
) (*service.Publication, error) {
	return reply[service.Publication](s.err)
}

func (s stub) PublicationSendNow(context.Context, service.PublicationSendNowRequest) (*service.Publication, error) {
	return reply[service.Publication](s.err)
}

func (s stub) PublicationRetry(context.Context, service.PublicationRetryRequest) (*service.Publication, error) {
	return reply[service.Publication](s.err)
}

func (s stub) PublicationCancel(context.Context, service.PublicationCancelRequest) (*service.Publication, error) {
	return reply[service.Publication](s.err)
}
