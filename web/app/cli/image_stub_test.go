package cli

import (
	"context"

	"github.com/AlexGalhardo/stuarty/service"
)

// The stub of the purity test answers the image operations too.

func (s stub) ImageCreate(context.Context, service.ImageCreateRequest) (*service.ImageCreateResult, error) {
	return reply[service.ImageCreateResult](s.err)
}

func (s stub) ImageList(context.Context, service.ImageListRequest) (*service.ImageListResult, error) {
	return reply[service.ImageListResult](s.err)
}

func (s stub) ImageDescribe(context.Context, service.ImageDescribeRequest) (*service.ImageDescribeResult, error) {
	return reply[service.ImageDescribeResult](s.err)
}

func (s stub) ImageEdit(context.Context, service.ImageEditRequest) (*service.ImageEditResult, error) {
	return reply[service.ImageEditResult](s.err)
}

func (s stub) ImageUndo(context.Context, service.ImageStepRequest) (*service.ImageStepResult, error) {
	return reply[service.ImageStepResult](s.err)
}

func (s stub) ImageRedo(context.Context, service.ImageStepRequest) (*service.ImageStepResult, error) {
	return reply[service.ImageStepResult](s.err)
}

func (s stub) ImageExport(context.Context, service.ImageExportRequest) (*service.ImageExportResult, error) {
	return reply[service.ImageExportResult](s.err)
}
