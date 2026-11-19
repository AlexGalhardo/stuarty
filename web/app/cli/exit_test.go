package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/workspace"
)

// errorCodes is every code of SDD-000 section 8 and SDD-002 A.8, with the exit
// code SDD-003 section 5.1 gives it: 2 for a retryable code, 1 for the rest.
var errorCodes = map[string]int{
	command.CodeUnknownCommand:      1,
	command.CodeValidation:          1,
	command.CodeInvalidTime:         1,
	command.CodeNotFound:            1,
	command.CodeIDExists:            1,
	command.CodeTypeMismatch:        1,
	command.CodeOverlap:             1,
	command.CodeOutOfSource:         1,
	command.CodeInvalidState:        1,
	command.CodeInUse:               1,
	command.CodeNotEmpty:            1,
	command.CodeLimit:               1,
	command.CodeFontUnknown:         1,
	command.CodeSlot:                1,
	command.CodeIdempotencyMismatch: 1,
	command.CodeInternal:            1,
	"E_UNAUTHORIZED":                1,
	"E_FORBIDDEN":                   1,
	"E_PATH_NOT_ALLOWED":            1,
	"E_FILE_NOT_FOUND":              1,
	"E_UNSUPPORTED_MEDIA":           1,
	"E_INGEST_FAILED":               1,
	"E_FFMPEG_MISSING":              1,
	"E_NO_ENCODER":                  1,
	"E_EXPORT_FAILED":               1,
	"E_VERIFY_FAILED":               1,
	"E_TRANSCRIBE_UNAVAILABLE":      1,
	"E_TRANSCRIBE_FAILED":           1,
	"E_LIMIT_EXCEEDED":              1,
	"E_CANCELLED":                   1,
	"E_NOTHING_TO_UNDO":             1,
	"E_NOTHING_TO_REDO":             1,
	"E_LINT_BLOCKING":               1,
	"E_PAYLOAD_TOO_LARGE":           1,
	command.CodeRevisionConflict:    2,
	"E_WORKSPACE_LOCKED":            2,
	"E_QUEUE_FULL":                  2,
	"E_RATE_LIMITED":                2,
	"E_ASSET_NOT_READY":             2,
}

// TestExitCodeOverEveryErrorCode runs one failing command per code and checks
// the exit code with and without the Retryable flag the error arrived with, so
// a remote error that lost its flag still maps by its code.
func TestExitCodeOverEveryErrorCode(t *testing.T) {
	t.Parallel()

	for code, want := range errorCodes {
		for _, flagged := range []bool{false, want == 2} {
			h := newHarness(t)
			h.svc.err = &service.Error{Code: code, Message: "m", Retryable: flagged}

			if got := h.run("project:list", "--json"); got != want {
				t.Errorf("%s (retryable flag %t): exit %d, want %d", code, flagged, got, want)
			}

			doc := oneJSONDocument(t, h.stdout.String())
			if doc["code"] != code || doc["retryable"] != (want == 2) {
				t.Errorf("%s: error document = %v", code, doc)
			}

			if h.stderr.Len() != 0 {
				t.Errorf("%s: stderr must be empty with --json, got %q", code, h.stderr.String())
			}
		}
	}
}

func TestErrorDocumentHasTheProblemMembersAndNoHTTPOnlyOnes(t *testing.T) {
	t.Parallel()

	op := 2
	h := newHarness(t)
	h.svc.err = &service.Error{
		Code: command.CodeOverlap, Message: "overlaps", Hint: "use 3s", OpIndex: &op, Path: "operations[2]",
		Details: map[string]any{"next_free": "3s"},
	}

	h.run("project:list", "--json")

	doc := oneJSONDocument(t, h.stdout.String())
	if doc["type"] != "urn:stuarty:error:E_OVERLAP" || doc["title"] != "E_OVERLAP" ||
		doc["status"] != float64(409) ||
		doc["detail"] != "overlaps" ||
		doc["op_index"] != float64(2) ||
		doc["path"] != "operations[2]" {
		t.Fatalf("doc = %v", doc)
	}

	for _, k := range []string{"instance", "request_id"} {
		if _, has := doc[k]; has {
			t.Errorf("the CLI document must not carry %s", k)
		}
	}
}

func TestErrorsThatAreNotServiceErrorsAreMappedWithoutTheirText(t *testing.T) {
	t.Parallel()

	locked := &workspace.Error{
		Code:      workspace.CodeWorkspaceLocked,
		Message:   "another process owns this workspace",
		Retryable: true,
	}

	h := newHarness(t)
	h.res.err = locked

	if code := h.run("project:list", "--json"); code != ExitTransient {
		t.Fatalf("a locked workspace must exit 2, got %d", code)
	}

	h = newHarness(t)
	h.res.err = errors.New("open C:\\Users\\secret\\db.sqlite: boom")

	if code := h.run("project:list", "--json"); code != ExitFailure {
		t.Fatalf("exit %d", code)
	}

	doc := oneJSONDocument(t, h.stdout.String())
	if doc["code"] != command.CodeInternal || strings.Contains(h.stdout.String(), "secret") ||
		strings.Contains(h.stdout.String(), "boom") {
		t.Fatalf("an internal error must not leak its text: %v", doc)
	}
}
