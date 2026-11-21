package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
)

// callFunc is the body of a tool: it decodes its arguments, calls one Service
// method and returns the result with its text.
type callFunc func(ctx context.Context, svc service.Service, args json.RawMessage) (*toolOutput, error)

// toolOutput is what a tool returns: the Service result (for structured
// content) and the readable text (for content).
type toolOutput struct {
	value any
	text  string
	// image and mime are the picture of view, the only tool that returns one.
	image []byte
	mime  string
	// isError marks a result that carries data but failed as a whole (media_import with nothing imported).
	isError bool
}

// parseArguments validates the arguments against the inputSchema of the tool
// (SDD-003 section 4.1). A failure is E_VALIDATION with the path of the first
// invalid value and the constraint that failed, never the value (SDD-003
// section 1.2 rule 9); a bad operations[].op goes on to the Service, which answers E_UNKNOWN_COMMAND.
func parseArguments(d *definition, raw json.RawMessage) (json.RawMessage, error) {
	raw = rawOrEmpty(raw)

	inst, err := jsonschema.UnmarshalJSON(strings.NewReader(string(raw)))
	if err != nil {
		return nil, &service.Error{Code: command.CodeValidation, Message: "the arguments are not a JSON object."}
	}

	if err := d.input.Validate(inst); err != nil {
		failure := validationFailure(err)

		// An operation the build does not have is for the Service to refuse: its
		// E_UNKNOWN_COMMAND names the operation by index and lists the known
		// ones, the same text the CLI and HTTP give (SDD-003 section 9 check 12).
		se, ok := failure.(*service.Error) //nolint:errorlint // built above
		if ok && se.Code == command.CodeUnknownCommand {
			return raw, nil
		}

		return nil, failure
	}

	return raw, nil
}

func validationFailure(err error) error {
	ve, ok := err.(*jsonschema.ValidationError) //nolint:errorlint // the validator returns it unwrapped
	if !ok {
		return &service.Error{Code: command.CodeValidation, Message: "the arguments are not valid."}
	}

	leaf := ve
	for len(leaf.Causes) > 0 {
		leaf = leaf.Causes[0]
	}

	path := formatPath(leaf.InstanceLocation)

	// A failed "required" is reported on the parent object; name the missing properties instead.
	if req, isReq := leaf.ErrorKind.(*kind.Required); isReq && len(req.Missing) > 0 {
		loc := slices.Concat(leaf.InstanceLocation, req.Missing[:1])
		path = formatPath(loc)
	}
	// An unknown member is reported on its parent object; name the member.
	// The key is caller-controlled and the text goes to a model: only a plain identifier is echoed (ADR-0033).
	unknown := ""

	if extra, isExtra := leaf.ErrorKind.(*kind.AdditionalProperties); isExtra {
		unknown = "an unknown argument"

		if len(extra.Properties) > 0 && identifier.MatchString(extra.Properties[0]) {
			path = formatPath(slices.Concat(leaf.InstanceLocation, extra.Properties[:1]))
			unknown = ""
		}
	}

	constraint := "schema"

	if kp := leaf.ErrorKind.KeywordPath(); len(kp) > 0 {
		constraint = kp[len(kp)-1]
	}

	code := command.CodeValidation
	if len(leaf.InstanceLocation) == 3 && leaf.InstanceLocation[0] == "operations" && leaf.InstanceLocation[2] == "op" {
		code = command.CodeUnknownCommand
	}

	message := fmt.Sprintf(
		"the argument %q breaks the constraint %q%s.",
		path,
		constraint,
		expectation(leaf.ErrorKind),
	)

	if unknown != "" {
		message = fmt.Sprintf("%s (under %q) breaks the constraint %q%s.", unknown, path, constraint, expectation(leaf.ErrorKind))
	}

	return &service.Error{
		Code:    code,
		Path:    path,
		Message: message,
		Hint:    "check the tool definition; command_help lists the operations",
	}
}

// expectation says what the failed constraint accepts: the values of an enum and the JSON type. It
// never repeats the value that was sent (SDD-003 section 1.2 rule 9).
func expectation(k jsonschema.ErrorKind) string {
	switch k := k.(type) {
	case *kind.AdditionalProperties:
		return ": it is not an argument of this tool"
	case *kind.Required:
		return ": the argument is required (missing: " + strings.Join(k.Missing, ", ") + ")"
	case *kind.Enum:
		values := make([]string, len(k.Want))

		for i, v := range k.Want {
			values[i] = fmt.Sprint(v)
		}

		return ": use one of " + strings.Join(values, ", ")
	case *kind.Type:
		want := strings.Join(k.Want, " or ")

		if slices.Contains(k.Want, "array") {
			return ": expected an array, such as [\"a\",\"b\"], not a comma-separated string"
		}

		return ": expected " + want
	case *kind.Maximum:
		return ": use at most " + k.Want.RatString()
	case *kind.Minimum:
		return ": use at least " + k.Want.RatString()
	case *kind.ExclusiveMaximum:
		return ": use less than " + k.Want.RatString()
	case *kind.ExclusiveMinimum:
		return ": use more than " + k.Want.RatString()
	case *kind.MaxLength:
		return fmt.Sprintf(": use at most %d characters", k.Want)
	case *kind.MinLength:
		return fmt.Sprintf(": use at least %d characters", k.Want)
	case *kind.MaxItems:
		return fmt.Sprintf(": use at most %d items", k.Want)
	case *kind.MinItems:
		return fmt.Sprintf(": use at least %d items", k.Want)
	case *kind.Pattern:
		return ": use a value that matches " + k.Want
	case *kind.Const:
		return fmt.Sprintf(": use %v", k.Want)
	}

	return ""
}

// identifier is the only shape of a caller-chosen key that an error may repeat.
var identifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)

// formatPath writes an instance location as operations[2].op. A segment that is a caller-chosen key and not a plain
// identifier is written as "?", so a path never carries a control character or a forged line.
func formatPath(loc []string) string {
	var b strings.Builder

	for i, p := range loc {
		if isIndex(p) {
			b.WriteString("[" + p + "]")

			continue
		}

		if i > 0 {
			b.WriteByte('.')
		}

		if !identifier.MatchString(p) {
			p = "?"
		}

		b.WriteString(p)
	}

	return b.String()
}

func isIndex(s string) bool {
	if s == "" {
		return false
	}

	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}

	return true
}

// successResult is the CallToolResult of SDD-003 section 1.3: the text as the
// content, and the result struct as structured content without its text and
// without the bulk members.
func successResult(out *toolOutput, bulk map[string]bool) (*sdk.CallToolResult, error) {
	raw, err := json.Marshal(out.value)
	if err != nil {
		return errorResult(err), nil
	}

	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return errorResult(err), nil
	}

	delete(doc, "text")

	for k := range bulk {
		delete(doc, k)
	}

	content := []sdk.Content{&sdk.TextContent{Text: out.text}}
	if len(out.image) > 0 {
		content = append(content, &sdk.ImageContent{Data: out.image, MIMEType: out.mime})
	}

	return &sdk.CallToolResult{Content: content, StructuredContent: doc, IsError: out.isError}, nil
}

// Caps of SDD-003 section 1.3: an error block is at most 6,000 characters, 2,000
// of text and 4,000 of envelope, whose details.diff is cut first.
const (
	envelopeCap = 4000
)

// errorResult is an execution error: isError true, no structured content, one
// text block holding the text of section 3.9 without its details line, a
// newline, and the envelope of section 1.4 as one line of JSON.
func errorResult(err error) *sdk.CallToolResult {
	se := asServiceError(err)
	text := se.Text(false) + "\n" + envelopeLine(se)

	return &sdk.CallToolResult{IsError: true, Content: []sdk.Content{&sdk.TextContent{Text: text}}}
}

func envelopeLine(se *service.Error) string {
	doc := se.Document()

	line := mustJSON(doc)
	if len([]rune(line)) <= envelopeCap {
		return line
	}

	if doc.Details != nil {
		delete(doc.Details, "diff")
		delete(doc.Details, "diff_truncated")
	}

	line = mustJSON(doc)
	if len([]rune(line)) <= envelopeCap {
		return line
	}

	doc.Details = nil

	return mustJSON(doc)
}

func mustJSON(v any) string {
	var b strings.Builder

	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)

	if err := enc.Encode(v); err != nil {
		return "{}"
	}

	return strings.TrimRight(b.String(), "\n")
}

// bulkFields lists the json names of the members the Service marks bulk:"true"
// (the whole document of project_get), which structured content leaves out.
func (a *adapter) bulkFields() map[string]bool {
	out := map[string]bool{}

	for _, v := range []any{service.ProjectGetResult{}, service.ViewResult{}} {
		t := reflect.TypeOf(v)
		for f := range t.Fields() {
			if f.Tag.Get("bulk") == "true" {
				out[strings.Split(f.Tag.Get("json"), ",")[0]] = true
			}
		}
	}

	return out
}
