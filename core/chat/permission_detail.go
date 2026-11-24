package chat

import (
	"encoding/json"
	"strings"
)

// The limits of SDD-017 section 7.1.
const (
	// MaxDetailValue is the longest value kept from the input of a call, in bytes.
	MaxDetailValue = 128
	// MaxDetailOperations is how many operation names of an edit are kept.
	MaxDetailOperations = 8
	// MaxDetailInput is the largest input that is read at all, in bytes; a larger one reports nothing.
	MaxDetailInput = 256 << 10
)

// The members of the input of a call that are kept, in the order they are tried. Nothing else is
// read: an allowlist is the only way a secret in the input cannot reach a person's screen or a
// transcript.
var (
	projectKeys = []string{"project_id", "timeline_id", "image_id"}
	itemKeys    = []string{"item_id", "asset_id", "name"}
)

// ReadDetail keeps the allowlisted members of the raw input of a call for which a permission is
// asked (SDD-017 section 7.1). It is nil unless title names a tool of Stuarty's own MCP server, and
// nil when the input holds none of the members. A member that is not a plain string is dropped
// without being read; the arguments of an operation are never read; every value is cleaned like all
// agent text and clipped.
func ReadDetail(title string, input json.RawMessage) *PermissionDetail {
	tool, ok := StuartyToolName(title)
	if !ok || len(input) == 0 || len(input) > MaxDetailInput {
		return nil
	}

	var members map[string]json.RawMessage
	if json.Unmarshal(input, &members) != nil {
		return nil
	}

	d := PermissionDetail{
		Project: firstString(members, projectKeys),
		Item:    firstString(members, itemKeys),
	}

	if tool == "edit" {
		d.Operations = editOperations(members["operations"])
	} else if action := detailString(members["action"]); action != "" {
		d.Operations = []string{action}
	}

	if d.Project == "" && d.Item == "" && len(d.Operations) == 0 {
		return nil
	}

	return &d
}

func firstString(members map[string]json.RawMessage, keys []string) string {
	for _, k := range keys {
		if v := detailString(members[k]); v != "" {
			return v
		}
	}

	return ""
}

// detailString reads a JSON string: anything else (a number, an object, an array, null, nothing) is
// the empty string. The text is a single line.
func detailString(raw json.RawMessage) string {
	var s string
	if len(raw) == 0 || json.Unmarshal(raw, &s) != nil {
		return ""
	}

	line := strings.Join(strings.Fields(CleanText(s)), " ")
	out, _ := Clip(line, MaxDetailValue)

	return out
}

// editOperations is the name, the op member, of each of the first MaxDetailOperations operations of
// an edit. The args of an operation are not decoded.
func editOperations(raw json.RawMessage) []string {
	var ops []map[string]json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &ops) != nil {
		return nil
	}

	var names []string

	for _, o := range ops {
		if name := detailString(o["op"]); name != "" {
			names = append(names, name)
		}

		if len(names) == MaxDetailOperations {
			break
		}
	}

	return names
}
