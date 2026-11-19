package export

import (
	"errors"
	"fmt"

	"github.com/AlexGalhardo/stuarty/core/lint"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
)

// Error codes of SDD-000 section 8 that an export raises. E_NO_ENCODER and E_INTERNAL come from
// engine/ffmpeg; a cancelled export returns the error of its context.
const (
	CodeLintBlocking = "E_LINT_BLOCKING"
	CodeExportFailed = "E_EXPORT_FAILED"
	CodeVerifyFailed = "E_VERIFY_FAILED"
)

// Reasons of E_EXPORT_FAILED (details.reason).
const (
	ReasonEncoder   = "encoder_failed"
	ReasonDecoder   = "decoder_failed"
	ReasonRender    = "render_failed"
	ReasonAudio     = "audio_failed"
	ReasonTimeout   = "timeout"
	ReasonStalled   = "stalled"
	ReasonPublish   = "publish_failed"
	ReasonBadOutput = "bad_output_dir"
	ReasonBadFormat = "bad_format"
)

// scrubLimit is how much of a tool's stderr an error keeps.
const scrubLimit = 600

func exportErr(reason, msg string, cause error, stderr string) *ffmpeg.Error {
	details := map[string]any{"reason": reason}
	if stderr != "" {
		details["stderr"] = ffmpeg.Scrub(stderr, scrubLimit)
	}
	return &ffmpeg.Error{
		Code:    CodeExportFailed,
		Reason:  reason,
		Message: msg,
		Details: details,
		Err:     cause,
		Hint:    "Run \"stuarty doctor\" and try the export again; if it fails again, report it with the output of doctor.",
	}
}

// wrapRun turns the error of a child run into the error of the export. A cancelled context passes
// through unchanged so the caller can tell a cancel from a failure.
func wrapRun(err error, reason, msg, stderr string) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ffmpeg.ErrStalled):
		return exportErr(ReasonStalled, "The export stopped making progress and was ended.", err, stderr)
	case errors.Is(err, ffmpeg.ErrDeadline):
		return exportErr(ReasonTimeout, "The export took longer than its deadline and was ended.", err, stderr)
	case errors.Is(err, ffmpeg.ErrOutputTooLarge):
		text := "The exported file passed the size limit and the export was ended."

		return exportErr(ReasonSizeLimit, text, err, stderr)
	}
	if ffErr, ok := errors.AsType[*ffmpeg.Error](err); ok {
		return ffErr
	}
	return exportErr(reason, msg, err, stderr)
}

// lintError is the E_LINT_BLOCKING of an export the lint refuses.
func lintError(findings []lint.Finding) *ffmpeg.Error {
	return &ffmpeg.Error{
		Code:    CodeLintBlocking,
		Reason:  "lint_blocking",
		Message: fmt.Sprintf("The project has %d problem(s) that stop an export.", len(findings)),
		Hint:    "Fix the findings, or run the lint to see them, then export again.",
		Details: map[string]any{"findings": findings},
	}
}

// Check is one failed check of the verification of section 8 step 7.
type Check struct {
	Name     string `json:"name"`
	Expected string `json:"expected"`
	Got      string `json:"got"`
}

func verifyError(checks []Check) *ffmpeg.Error {
	return &ffmpeg.Error{
		Code:    CodeVerifyFailed,
		Reason:  "verify_failed",
		Message: fmt.Sprintf("The exported file failed %d check(s) and was deleted.", len(checks)),
		Hint:    "Export again; if it keeps failing, report it with the output of \"stuarty doctor\".",
		Details: map[string]any{"checks": checks},
	}
}

// ReasonSizeLimit is details.reason of an export whose file passed STUARTY_MAX_EXPORT_BYTES.
const ReasonSizeLimit = "size_limit"

func sizeLimitError(size, limit int64) *ffmpeg.Error {
	e := exportErr(ReasonSizeLimit, "The exported file is larger than the limit and was deleted.", nil, "")
	e.Hint = "Export a shorter project, or raise STUARTY_MAX_EXPORT_BYTES."
	e.Details["limit"] = "export_bytes"
	e.Details["value"] = size
	e.Details["ceiling"] = limit
	e.Details["variable"] = "STUARTY_MAX_EXPORT_BYTES"

	return e
}
