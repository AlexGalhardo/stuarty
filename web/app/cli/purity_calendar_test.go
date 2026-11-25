package cli

import (
	"context"

	"github.com/AlexGalhardo/stuarty/service"
)

func (s stub) PublicationDuplicate(context.Context, service.PublicationDuplicateRequest) (*service.Publication, error) {
	return reply[service.Publication](s.err)
}

func (s stub) CalendarPlan(context.Context, service.CalendarPlanRequest) (*service.CalendarPlanResult, error) {
	return reply[service.CalendarPlanResult](s.err)
}

func (s stub) CalendarDaySet(context.Context, service.CalendarDaySetRequest) (*service.CalendarDay, error) {
	return reply[service.CalendarDay](s.err)
}

func (s stub) CalendarQuietSet(context.Context, service.CalendarQuietSetRequest) (*service.CalendarDay, error) {
	return reply[service.CalendarDay](s.err)
}

func (s stub) CalendarSlotSave(context.Context, service.CalendarSlotSaveRequest) (*service.CalendarSlot, error) {
	return reply[service.CalendarSlot](s.err)
}

func (s stub) CalendarSlotDelete(
	context.Context, service.CalendarSlotDeleteRequest,
) (*service.CalendarSlotDeleteResult, error) {
	return reply[service.CalendarSlotDeleteResult](s.err)
}

func (s stub) CalendarExport(context.Context, service.CalendarExportRequest) (*service.CalendarExportResult, error) {
	return reply[service.CalendarExportResult](s.err)
}
