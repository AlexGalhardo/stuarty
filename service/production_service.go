package service

import "context"

// ProductionService is the part of the service that works on the production of a project (SDD-003 section
// 2.3.1, ADR-0039): its items, its brief, its description and its copy. Service embeds it.
type ProductionService interface {
	ProjectDescribe(ctx context.Context, req ProjectDescribeRequest) (*ProjectDescribeResult, error)
	ProjectDuplicate(ctx context.Context, req ProjectDuplicateRequest) (*ProjectCreateResult, error)
	ProjectItemAdd(ctx context.Context, req ProjectItemAddRequest) (*ProjectItemResult, error)
	ProjectItemMove(ctx context.Context, req ProjectItemMoveRequest) (*ProjectItemResult, error)
	ProjectItemRename(ctx context.Context, req ProjectItemRenameRequest) (*ProjectItemResult, error)
	ProjectItemTrash(ctx context.Context, req ProjectItemTrashRequest) (*ProjectItemResult, error)
	ProjectItemRestore(ctx context.Context, req ProjectItemTrashRequest) (*ProjectItemResult, error)
	BriefGet(ctx context.Context, req BriefGetRequest) (*BriefResult, error)
	BriefSet(ctx context.Context, req BriefSetRequest) (*BriefResult, error)
}

var _ ProductionService = (*svc)(nil)
