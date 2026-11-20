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

func (s stub) ImageImport(context.Context, service.ImageImportRequest) (*service.ImageImportResult, error) {
	return reply[service.ImageImportResult](s.err)
}

func (s stub) ImageRevert(context.Context, service.ImageRevertRequest) (*service.ImageRevertResult, error) {
	return reply[service.ImageRevertResult](s.err)
}

func (s stub) ImageDiff(context.Context, service.ImageDiffRequest) (*service.ImageDiffResult, error) {
	return reply[service.ImageDiffResult](s.err)
}

func (s stub) ImageCheckpointCreate(
	context.Context,
	service.ImageCheckpointCreateRequest,
) (*service.Checkpoint, error) {
	return reply[service.Checkpoint](s.err)
}

func (s stub) ImageCheckpointList(
	context.Context,
	service.ImageCheckpointListRequest,
) (*service.ImageCheckpointListResult, error) {
	return reply[service.ImageCheckpointListResult](s.err)
}

func (s stub) ImageCheckpointRename(
	context.Context,
	service.ImageCheckpointRenameRequest,
) (*service.Checkpoint, error) {
	return reply[service.Checkpoint](s.err)
}

func (s stub) ImageCheckpointRemove(
	context.Context,
	service.ImageCheckpointRemoveRequest,
) (*service.Checkpoint, error) {
	return reply[service.Checkpoint](s.err)
}
