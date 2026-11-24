package cli

import (
	"context"

	"github.com/AlexGalhardo/stuarty/service"
)

func (s stub) RecordingSources(
	context.Context, service.RecordingSourcesRequest,
) (*service.RecordingSourcesResult, error) {
	return reply[service.RecordingSourcesResult](s.err)
}

func (s stub) RecordingStart(context.Context, service.RecordingStartRequest) (*service.Recording, error) {
	return reply[service.Recording](s.err)
}

func (s stub) RecordingStop(context.Context, service.RecordingRef) (*service.Recording, error) {
	return reply[service.Recording](s.err)
}

func (s stub) RecordingCancel(context.Context, service.RecordingRef) (*service.Recording, error) {
	return reply[service.Recording](s.err)
}

func (s stub) RecordingGet(context.Context, service.RecordingRef) (*service.Recording, error) {
	return reply[service.Recording](s.err)
}

func (s stub) RecordingList(
	context.Context, service.RecordingListRequest,
) (*service.RecordingListResult, error) {
	return reply[service.RecordingListResult](s.err)
}

func (s stub) RecordingConfirm(context.Context, service.RecordingRef) (*service.Recording, error) {
	return reply[service.Recording](s.err)
}

func (s stub) RecordingDismiss(context.Context, service.RecordingRef) (*service.Recording, error) {
	return reply[service.Recording](s.err)
}

func (s stub) RecordingPlace(context.Context, service.RecordingPlaceRequest) (*service.EditResult, error) {
	return reply[service.EditResult](s.err)
}
