package service

import (
	"maps"
	"regexp"
	"slices"
)

var (
	// variableInParens is the " (VUKEM_VIDEO_X)" of a limit message.
	variableInParens = regexp.MustCompile(` \(VUKEM_VIDEO_[A-Z_]+\)`)
	// operatorClause is the "; the person running the server can raise VUKEM_VIDEO_X" of a limit hint.
	operatorClause = regexp.MustCompile(
		`[;,]? the person running the server can \w+ VUKEM_VIDEO_[A-Z_]+(?: or VUKEM_VIDEO_[A-Z_]+)*`,
	)
	anyVariable = regexp.MustCompile(`VUKEM_VIDEO_[A-Z_]+`)
)

// hidesVariables is true for a caller of network mode (it has an owner user)
// who is not an administrator: the name of a configuration variable tells such
// a caller how the server is set up and is of no use to them (SDD-007 section 9.1).
func hidesVariables(c Caller) bool {
	return c.OwnerUserID != "" && !slices.Contains(c.Scopes, ScopeAdmin)
}

// forCaller returns e without the configuration variable names when c may not
// see them. It copies: an error may be shared, such as the one stored in a job.
func forCaller(c Caller, e *Error) *Error {
	if e == nil || !hidesVariables(c) {
		return e
	}

	out := *e

	out.Message = anyVariable.ReplaceAllString(variableInParens.ReplaceAllString(e.Message, ""), "the limit")
	out.Hint = anyVariable.ReplaceAllString(operatorClause.ReplaceAllString(e.Hint, ""), "the limit")

	if _, ok := e.Details["variable"]; ok {
		out.Details = maps.Clone(e.Details)
		delete(out.Details, "variable")
	}

	return &out
}

// callerError is forCaller for an error value of a method that returns error.
func callerError(c Caller, err error) error {
	if e, ok := err.(*Error); ok { //nolint:errorlint // only a direct *Error is rewritten
		return forCaller(c, e)
	}

	return err
}

// EventError is the error of an event. An event reaches every subscriber of the
// project, whoever they are, so it never carries the name of a configuration
// variable (SDD-007 section 9.1): an administrator reads the full error from
// JobGet and AuditList. It copies, like forCaller.
func EventError(e *Error) *Error {
	return forCaller(Caller{Actor: "event", OwnerUserID: "event"}, e)
}
