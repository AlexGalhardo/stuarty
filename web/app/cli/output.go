package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/service"
	"github.com/AlexGalhardo/vukem-video/service/workspace"
)

// Exit codes of SDD-000 section 8: 0 success, 1 a permanent failure, 2 a
// transient one that is safe to retry.
const (
	ExitOK        = 0
	ExitFailure   = 1
	ExitTransient = 2
)

// retryableCodes are the codes SDD-000 section 8 marks retryable. The exit code
// follows the code, so a remote error that lost its flag still exits 2.
var retryableCodes = map[string]bool{
	command.CodeRevisionConflict: true,
	service.CodeWorkspaceLocked:  true,
	service.CodeQueueFull:        true,
	service.CodeRateLimited:      true,
	"E_ASSET_NOT_READY":          true,
}

// output is what a handler returns: the document for --json, the text for a
// person, and an exit code when a successful call still reports a failure
// (lint --strict).
type output struct {
	value any
	text  string
	exit  int
}

// errorDoc is the error document of SDD-003 section 1.4 without the HTTP-only
// members instance and request_id.
type errorDoc struct {
	Type      string         `json:"type"`
	Title     string         `json:"title"`
	Status    int            `json:"status"`
	Detail    string         `json:"detail"`
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Hint      string         `json:"hint,omitempty"`
	Retryable bool           `json:"retryable"`
	OpIndex   *int           `json:"op_index,omitempty"`
	Path      string         `json:"path,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
}

// asServiceError turns any error into the *service.Error the CLI reports. An
// error that is not a known kind is E_INTERNAL without its text, as the
// service does.
func asServiceError(err error) *service.Error {
	if se, ok := errors.AsType[*service.Error](err); ok {
		return se
	}

	if we, ok := errors.AsType[*workspace.Error](err); ok {
		return &service.Error{Code: we.Code, Message: we.Message, Retryable: we.Retryable}
	}

	if ce, ok := errors.AsType[*command.Error](err); ok {
		return &service.Error{
			Code: ce.Code, Message: ce.Message, Hint: ce.Hint, Retryable: ce.Retryable,
			OpIndex: ce.OpIndex, Path: ce.Path, Details: ce.Details,
		}
	}

	return &service.Error{
		Code:    command.CodeInternal,
		Message: "the command failed because of an internal error.",
		Hint:    "Report this as a bug with the command that was run.",
	}
}

// exitCodeOf maps an error to its exit code.
func exitCodeOf(se *service.Error) int {
	if se.Retryable || retryableCodes[se.Code] {
		return ExitTransient
	}

	return ExitFailure
}

func newErrorDoc(se *service.Error) errorDoc {
	return errorDoc{
		Type: "urn:vukem-video:error:" + se.Code, Title: se.Code, Status: se.HTTPStatus(),
		Detail: se.Message, Code: se.Code, Message: se.Message, Hint: se.Hint,
		Retryable: se.Retryable || retryableCodes[se.Code], OpIndex: se.OpIndex, Path: se.Path, Details: se.Details,
	}
}

const errorTextCap = 2000

// errorText is the text of SDD-003 section 3.9 for an error, capped at 2,000
// characters with the details cut first.
func errorText(se *service.Error) string {
	var b strings.Builder

	fmt.Fprintf(&b, "error %s: %s", se.Code, se.Message)

	if se.Hint != "" {
		fmt.Fprintf(&b, "\nhint: %s", se.Hint)
	}

	fmt.Fprintf(&b, "\nretryable: %t", se.Retryable || retryableCodes[se.Code])

	if se.OpIndex != nil {
		fmt.Fprintf(&b, "  op_index: %d", *se.OpIndex)
	}

	if se.Path != "" {
		fmt.Fprintf(&b, "  path: %s", se.Path)
	}

	if len(se.Details) > 0 {
		if raw, err := json.Marshal(se.Details); err == nil {
			room := max(errorTextCap-b.Len()-len("\ndetails: "), 0)
			b.WriteString("\ndetails: ")
			b.WriteString(truncate(string(raw), room))
		}
	}

	return truncate(b.String(), errorTextCap)
}

func truncate(s string, limit int) string {
	r := []rune(s)
	if len(r) <= limit {
		return s
	}

	return string(r[:max(limit-1, 0)]) + "…"
}

// writeJSON prints one JSON document and a newline.
func writeJSON(w io.Writer, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("encode the result: %w", err)
	}

	_, err = fmt.Fprintf(w, "%s\n", raw)

	return err
}

// writeError prints a failure the way the mode says: the error document on
// standard output with --json (standard error stays empty), the text on
// standard error otherwise. It returns the exit code.
func writeError(rt *Runtime, asJSON bool, err error) int {
	se := asServiceError(err)

	if asJSON {
		_ = writeJSON(rt.Stdout, newErrorDoc(se))
	} else {
		fmt.Fprintln(rt.Stderr, errorText(se))
	}

	return exitCodeOf(se)
}

// writeOutput prints a handler's result and returns its exit code.
func writeOutput(rt *Runtime, asJSON bool, out *output) int {
	if asJSON {
		if err := writeJSON(rt.Stdout, out.value); err != nil {
			return writeError(rt, true, err)
		}

		return out.exit
	}

	fmt.Fprintln(rt.Stdout, strings.TrimRight(out.text, "\n"))

	return out.exit
}

// itoa keeps the text renderers short.
func itoa(n int64) string { return strconv.FormatInt(n, 10) }
