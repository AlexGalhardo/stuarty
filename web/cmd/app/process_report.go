package main

import (
	"errors"
	"io"
	"slices"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/workspace"
	"github.com/AlexGalhardo/stuarty/web/app/cli"
)

// jsonFlag is the flag every command accepts to print one JSON document (SDD-003 section 5.6).
const jsonFlag = "--json"

// splitJSON takes --json out of the arguments of a command the binary runs itself.
func splitJSON(args []string) (rest []string, asJSON bool) {
	for _, a := range args {
		if a == jsonFlag {
			asJSON = true

			continue
		}

		rest = append(rest, a)
	}

	return slices.Clip(rest), asJSON
}

// invalid is the E_VALIDATION of a bad argument, with the usage line as its hint.
func invalid(message, usage string) *service.Error {
	return &service.Error{Code: command.CodeValidation, Message: message, Hint: usage}
}

// report prints a failure of user:* or workspace:* in the format of the other commands (SDD-003 section 3.9):
// "error E_CODE: message", the hint and "retryable", or with --json the error document. An error of a kind the
// shared format does not know keeps its own text, which here is the one a person needs (a path, a full disk).
func report(out, errw io.Writer, asJSON bool, err error) int {
	if _, ok := errors.AsType[*service.Error](err); !ok {
		_, isWS := errors.AsType[*workspace.Error](err)
		_, isCmd := errors.AsType[*command.Error](err)

		if !isWS && !isCmd {
			err = &service.Error{Code: command.CodeInternal, Message: strings.TrimRight(err.Error(), ".") + "."}
		}
	}

	return cli.ReportError(out, errw, asJSON, err)
}
