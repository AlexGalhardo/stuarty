package httpapi

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strconv"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"

	"github.com/AlexGalhardo/vukem/errs"
	"github.com/AlexGalhardo/vukem/web"
)

// ProblemTypePrefix starts the type of every problem: urn:stuarty:error:<code>.
const ProblemTypePrefix = "urn:stuarty:error:"

// CodeMethodNotAllowed is the code of a 405 (OPTIONS included). SDD-000 section 8
// lists no code for it, so it falls under "everything else", status 405.
const CodeMethodNotAllowed = service.CodeMethodNotAllowed

// installErrors makes every error of the app the envelope of SDD-003 section
// 1.4: application/problem+json with the members of RFC 9457 and, next to
// them, code, message, hint, retryable, op_index, path and details.
func installErrors(s *web.Services) {
	web.MapError(s, func(_ *web.Ctx, e *service.Error) error { return httpError(e.ForSurface(service.SurfaceHTTP)) })
	s.ErrorMappers = append(s.ErrorMappers, clientGone)
	s.ProblemDecorator = decorate
}

// clientGone maps a context error to E_CLIENT_CLOSED (499) when the request
// itself has ended, so a caller that gave up is not logged as a 5xx. vukem
// renders any error it does not know as 500 (VF-068). A context error on a
// request that is still live stays a real fault.
func clientGone(c *web.Ctx, err error) (error, bool) { //nolint:revive // the signature of web.Services.ErrorMappers

	if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		return nil, false
	}

	if c.Context().Err() == nil {
		return nil, false
	}

	return httpError((&service.Error{
		Code: service.CodeClientClosed, Message: "the caller closed the request before it finished.",
	}).ForSurface(service.SurfaceHTTP)), true
}

func httpError(e *service.Error) *errs.HTTPError {
	he := errs.New(e.HTTPStatus(), e.Message).WithExtensionsOn5xx().
		WithExtension("code", e.Code).
		WithExtension("message", e.Message).
		WithExtension("retryable", e.Retryable)

	if e.Hint != "" {
		he = he.WithExtension("hint", e.Hint)
	}

	if e.OpIndex != nil {
		he = he.WithExtension("op_index", *e.OpIndex)
	}

	if e.Path != "" {
		he = he.WithExtension("path", e.Path)
	}

	if len(e.Details) > 0 {
		he = he.WithExtension("details", e.Details)
	}

	if after, ok := e.Details["retry_after"].(int); ok {
		he = he.WithHeader("Retry-After", strconv.Itoa(after))
	}

	return he
}

// decorate gives every problem, including the ones vukem writes itself (404,
// 405, a body that is too large, an invalid input), the same members.
func decorate(_ *web.Ctx, p *web.Problem) {
	code, _ := p.Extensions["code"].(string)

	if code == "" {
		code = codeOf(p)

		if p.Extensions == nil {
			p.Extensions = map[string]any{}
		}

		message := p.Detail
		if message == "" {
			message = p.Title
		}

		if len(p.Issues) > 0 {
			withoutCallerKeys(p)

			message = "the request is not valid: " + p.Issues[0].Message
			p.Extensions["path"] = p.Issues[0].Path
		}

		if p.Status >= http.StatusInternalServerError {
			message = "the operation failed because of an internal error."
		}

		p.Extensions["code"], p.Extensions["message"], p.Extensions["retryable"] = code, message, false
		p.Detail = message
	}

	if code == command.CodeValidation && p.Status == http.StatusUnprocessableEntity {
		p.Status = http.StatusBadRequest
	}

	p.Type, p.Title = ProblemTypePrefix+code, code

	if msg, ok := p.Extensions["message"].(string); ok {
		p.Detail = msg
	}
}

// issueWithKey is the text of a validation issue whose path holds a key that an error may not repeat.
const issueWithKey = "a value of the request is not valid."

// withoutCallerKeys takes the caller's keys out of the issues vukem wrote: for a member of a map, its issue path is the
// key and its message repeats it (SDD-003 section 1.2 rule 9).
func withoutCallerKeys(p *web.Problem) {
	for i := range p.Issues {
		if path := p.Issues[i].Path; path != "" && !command.IsSafeKey(path) {
			p.Issues[i].Path, p.Issues[i].Message = "?", issueWithKey
		}
	}

	if len(p.Errors) == 0 {
		return
	}

	clean := make(map[string][]string, len(p.Errors))

	for path, messages := range p.Errors {
		if path == "" || command.IsSafeKey(path) {
			clean[path] = messages
		} else if !slices.Contains(clean["?"], issueWithKey) {
			clean["?"] = append(clean["?"], issueWithKey)
		}
	}

	p.Errors = clean
}

// codeOf is the code of a problem that no service method produced.
func codeOf(p *web.Problem) string {
	switch p.Status {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return command.CodeValidation
	case http.StatusUnauthorized:
		return service.CodeUnauthorized
	case http.StatusForbidden:
		return service.CodeForbidden
	case http.StatusNotFound:
		return command.CodeNotFound
	case http.StatusMethodNotAllowed:
		return CodeMethodNotAllowed
	case http.StatusRequestEntityTooLarge, http.StatusRequestHeaderFieldsTooLarge:
		return service.CodePayloadTooLarge
	case http.StatusTooManyRequests:
		return service.CodeRateLimited
	}

	if p.Status >= http.StatusInternalServerError {
		return command.CodeInternal
	}

	return command.CodeValidation
}
