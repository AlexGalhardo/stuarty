package owner

import (
	"errors"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/service"
	"github.com/AlexGalhardo/vukem-video/service/workspace"
	"github.com/AlexGalhardo/vukem-video/web/app/store"
)

// StartupError turns the start-up refusals that a person must read into the
// *service.Error that every surface prints (SDD-005 section 7 step 2). A
// command, the stdio MCP server and serve then say the same thing, and the text
// is fixed: the raw error of a loose token names the file. Any other error is
// returned as it is.
func StartupError(err error) error {
	if newer, ok := errors.AsType[*store.NewerWorkspaceError](err); ok {
		return &service.Error{
			Code:    command.CodeInvalidState,
			Message: newer.Error(),
			Hint:    "Nothing was written. Install a version of vukem-video that is at least as new as the one that wrote this workspace.",
		}
	}

	if errors.Is(err, workspace.ErrUnusablePath) {
		return &service.Error{
			Code:    command.CodeInvalidState,
			Message: "the workspace path holds a character that breaks the database URL: ?, # or %.",
			Hint:    "Choose a workspace folder whose path has none of them, with " + workspace.EnvHome + ".",
		}
	}

	if errors.Is(err, workspace.ErrTokenPermissions) {
		return &service.Error{
			Code:    command.CodeInvalidState,
			Message: "the token file of the workspace is too loose: only its owner may read it.",
			Hint: "Restrict the file token in the workspace folder to its owner, or delete it: " +
				"the next start writes a new token.",
		}
	}

	return err
}
