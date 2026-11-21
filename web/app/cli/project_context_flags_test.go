package cli

import "github.com/AlexGalhardo/stuarty/service/testproject"

// The flags of a valid project:create (epic 56: the description and the goal are required).
const (
	ctxDescription = testproject.Description
	ctxSpecific    = "Publish one short video that explains the idea"
	ctxMeasurable  = "At least one hundred views in the first week"
	ctxAchievable  = "The footage and the script already exist"
	ctxRelevant    = "It brings the first audience to the channel"
	ctxTimeBound   = "Edit on Monday and publish on Tuesday"
	ctxDue         = "2999-12-31"
)
