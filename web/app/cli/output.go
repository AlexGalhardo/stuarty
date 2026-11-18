package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// output is what a handler returns: the document for --json, the text for a
// person, and an exit code when a successful call still reports a failure
// (lint --strict).
type output struct {
	value any
	text  string
	exit  int
}

// asServiceError turns any error into the *service.Error the CLI reports. An
// error that is not a known kind is E_INTERNAL without its text, as the
// service does.
func asServiceError(err error) *service.Error {
	if se, ok := errors.AsType[*service.Error](err); ok {
		return se
	}

	if we, ok := errors.AsType[*workspace.Error](err); ok {
		out := &service.Error{Code: we.Code, Message: we.Message, Retryable: we.Retryable}
		if errors.Is(err, workspace.ErrLocked) {
			out.Hint = "another process owns this workspace: close the desktop app or serve and run the command again, " +
				"or wait if it is a short command"
		}

		return out
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

// forCLI adapts a shared error to this surface without changing the original: a
// missing file names the path that was looked for. The hint names the commands of the CLI.
func forCLI(se *service.Error, resolved []string) *service.Error {
	out := *se.ForSurface(service.SurfaceCLI)
	if se.Code == "E_FILE_NOT_FOUND" && len(resolved) > 0 {
		out.Message = strings.TrimRight(se.Message, ".") + ": " + resolved[len(resolved)-1] + "."
	}

	return &out
}

// exitCodeOf maps an error to its exit code.
func exitCodeOf(se *service.Error) int {
	if se.IsRetryable() {
		return ExitTransient
	}

	return ExitFailure
}

// writeJSON prints one JSON document and a newline.
func writeJSON(w io.Writer, v any) error {
	var buf bytes.Buffer

	enc := json.NewEncoder(&buf)
	// "<" and ">" stay as they are: the texts carry <untrusted_media_text> blocks and an
	// escaped form costs an agent tokens (SDD-003 section 5.6 shows them unescaped).
	enc.SetEscapeHTML(false)

	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encode the result: %w", err)
	}

	_, err := w.Write(buf.Bytes())

	return err
}

// writeError prints a failure the way the mode says: the error document on
// standard output with --json (standard error stays empty), the text on
// standard error otherwise. It returns the exit code.
func writeError(rt *Runtime, asJSON bool, err error) int {
	se := forCLI(asServiceError(err), rt.resolved)

	if asJSON {
		_ = writeJSON(rt.Stdout, se.Document())
	} else {
		fmt.Fprintln(rt.Stderr, se.Text(true))
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
