package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/AlexGalhardo/stuarty/core/command"
)

// Codes the service adds to those of the core (SDD-000 section 8).
const (
	CodeUnauthorized     = "E_UNAUTHORIZED"
	CodeForbidden        = "E_FORBIDDEN"
	CodePayloadTooLarge  = "E_PAYLOAD_TOO_LARGE"
	CodeNothingToUndo    = "E_NOTHING_TO_UNDO"
	CodeNothingToRedo    = "E_NOTHING_TO_REDO"
	CodeQueueFull        = "E_QUEUE_FULL"
	CodeRateLimited      = "E_RATE_LIMITED"
	CodeWorkspaceLocked  = "E_WORKSPACE_LOCKED"
	CodeLintBlocking     = "E_LINT_BLOCKING"
	CodeClientClosed     = "E_CLIENT_CLOSED"
	CodeMethodNotAllowed = "E_METHOD_NOT_ALLOWED"

	// CodeConnectionNotFound, CodeConnectionDestination and CodeVaultLocked are the codes of SDD-014 section 7.
	CodeConnectionNotFound    = "E_CONNECTION_NOT_FOUND"
	CodeConnectionDestination = "E_CONNECTION_DESTINATION"
	CodeVaultLocked           = "E_VAULT_LOCKED"
	// CodePrivilegedSession, CodeApprovalNotFound, CodeCostCeiling and CodeApprovalExpired are the codes of SDD-015 section 8.
	CodePrivilegedSession = "E_PRIVILEGED_SESSION"
	CodeApprovalNotFound  = "E_APPROVAL_NOT_FOUND"
	CodeCostCeiling       = "E_COST_CEILING"
	CodeApprovalExpired   = "E_APPROVAL_EXPIRED"
	// CodeProvider and CodePriceUnknown are the codes of SDD-016 section 8.
	CodeProvider          = "E_PROVIDER"
	CodePriceUnknown      = "E_PRICE_UNKNOWN"
	codeInternalMessage   = "the operation failed because of an internal error."
	codeInternalHint      = "Report this as a bug with the operation that was sent."
	hintRestoreProject    = "restore the project first"
	hintReadProjectAgain  = "read the project again with project_get and plan the batch against its current revision"
	maxOperationsPerBatch = 200
)

// Error is the error of every service method (SDD-003 section 2.2).
type Error struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Hint      string         `json:"hint,omitempty"`
	Retryable bool           `json:"retryable"`
	OpIndex   *int           `json:"op_index,omitempty"`
	Path      string         `json:"path,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// StatusClientClosed is the status for a request its caller gave up on (the
// nginx convention); it is below 500, so it is not logged as a server fault.
const StatusClientClosed = 499

// HTTPStatus is the status of SDD-000 section 8; an unknown code is 500.
func (e *Error) HTTPStatus() int {
	switch e.Code {
	case command.CodeValidation, command.CodeInvalidTime, command.CodeUnknownCommand:
		return http.StatusBadRequest
	case CodeUnauthorized:
		return http.StatusUnauthorized
	case CodeForbidden, "E_PATH_NOT_ALLOWED", CodeConnectionDestination, CodePrivilegedSession:
		return http.StatusForbidden
	case command.CodeNotFound, "E_FILE_NOT_FOUND", CodeConnectionNotFound, CodeApprovalNotFound, CodeAgentNotFound,
		CodeChatNotFound:
		return http.StatusNotFound
	case command.CodeRevisionConflict, command.CodeIdempotencyMismatch, command.CodeOverlap, command.CodeInvalidState,
		command.CodeInUse, command.CodeNotEmpty, command.CodeIDExists, "E_ASSET_NOT_READY", CodeNothingToUndo, CodeNothingToRedo,
		CodeApprovalExpired:
		return http.StatusConflict
	case CodeMethodNotAllowed:
		return http.StatusMethodNotAllowed
	case CodePayloadTooLarge:
		return http.StatusRequestEntityTooLarge
	case command.CodeLimit,
		"E_LIMIT_EXCEEDED",
		"E_UNSUPPORTED_MEDIA",
		command.CodeTypeMismatch,
		command.CodeOutOfSource,
		command.CodeFontUnknown,
		command.CodeSlot,
		CodeLintBlocking,
		CodeCostCeiling,
		CodePriceUnknown:
		return http.StatusUnprocessableEntity
	case CodeProvider:
		return http.StatusBadGateway
	case CodeQueueFull, CodeRateLimited:
		return http.StatusTooManyRequests
	case CodeClientClosed:
		return StatusClientClosed
	case CodeWorkspaceLocked, "E_TRANSCRIBE_UNAVAILABLE", "E_FFMPEG_MISSING", "E_NO_ENCODER", CodeVaultLocked,
		CodeAgentUnavailable:
		return http.StatusServiceUnavailable
	}

	return http.StatusInternalServerError
}

// ExitCode is 2 when a retry can succeed and 1 otherwise.
func (e *Error) ExitCode() int {
	if e.Retryable {
		return 2
	}

	return 1
}

// newError builds an error whose retryable flag follows the code. String
// arguments are shortened the same way as command.Errorf, so a validation
// message cannot send back a value the caller submitted (SDD-003 section 1.2
// rule 9).
func newError(code, format string, args ...any) *Error {
	retryable := code == command.CodeRevisionConflict || code == CodeQueueFull || code == CodeRateLimited ||
		code == CodeWorkspaceLocked || code == "E_ASSET_NOT_READY"

	return &Error{
		Code: code, Message: command.Errorf(code, format, args...).Message, Retryable: retryable,
	}
}

func (e *Error) withHint(format string, args ...any) *Error {
	e.Hint = fmt.Sprintf(format, args...)
	return e
}

func (e *Error) withPath(path string) *Error {
	e.Path = path
	return e
}

func (e *Error) withDetail(key string, value any) *Error {
	if e.Details == nil {
		e.Details = map[string]any{}
	}

	e.Details[key] = value

	return e
}

// withLimit adds the three details every limit error carries (SDD-007 section 9.1): the name of the
// limit, what was seen and the ceiling.
func (e *Error) withLimit(name string, value, ceiling any) *Error {
	return e.withDetail("limit", name).withDetail("value", value).withDetail("ceiling", ceiling)
}

// queueVariables names the variable that sets the ceiling of a job kind, for the workspace and for
// one actor.
var queueVariables = map[string][2]string{
	JobExport:     {"STUARTY_MAX_EXPORTS", "STUARTY_MAX_EXPORTS_PER_KEY"},
	JobIngest:     {"STUARTY_MAX_INGESTS", "STUARTY_MAX_INGESTS"},
	JobTranscribe: {"STUARTY_MAX_TRANSCRIBES", "STUARTY_MAX_TRANSCRIBES_PER_KEY"},
}

// queueFullError is E_QUEUE_FULL with the jobs that hold the slots and the limit that was reached.
func queueFullError(full *QueueFullError) *Error {
	e := newError(CodeQueueFull, "a concurrency limit is reached.").
		withHint("wait for a running job to finish with job_get, or stop one with job_cancel, then try again").
		withDetail("running", full.Running)

	if full.Kind == "" {
		return e
	}

	name, index := full.Kind+"s", 0
	if full.PerActor {
		name, index = name+"_per_key", 1
	}

	_ = e.withLimit(name, len(full.Running), full.Ceiling)

	if v, ok := queueVariables[full.Kind]; ok {
		_ = e.withDetail("variable", v[index])
	}

	return e
}

func internalError() *Error {
	return newError(command.CodeInternal, "%s", codeInternalMessage).withHint("%s", codeInternalHint)
}

// fromCommand turns an error of the core into a service error.
func fromCommand(err error) *Error {
	ce := command.AsError(err)

	return &Error{
		Code: ce.Code, Message: ce.Message, Hint: ce.Hint, Retryable: ce.Retryable,
		OpIndex: ce.OpIndex, Path: ce.Path, Details: ce.Details,
	}
}

// asError returns err as a *Error. A sentinel of the store maps to its code; a
// context error is reported as such (the caller gave up), and anything else is
// E_INTERNAL without its text, which may hold paths or SQL.
func asError(err error) error {
	if err == nil {
		return nil
	}

	if se, ok := errors.AsType[*Error](err); ok {
		return se
	}

	if ce, ok := errors.AsType[*command.Error](err); ok {
		return fromCommand(ce)
	}

	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return newError(CodeClientClosed, "the caller closed the request before it finished.")
	case errors.Is(err, ErrNotFound):
		return newError(command.CodeNotFound, "the project or object does not exist.")
	case errors.Is(err, ErrExists):
		return newError(command.CodeIDExists, "an object with that identifier already exists.")
	case errors.Is(err, ErrStale):
		return internalError()
	}

	if full, ok := errors.AsType[*QueueFullError](err); ok {
		return queueFullError(full)
	}

	return internalError()
}
