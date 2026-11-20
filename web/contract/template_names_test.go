package contract

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestTemplateNamesResolveOrFailClearly (VV-299): a scenario picks a template by its name, read from the results
// that listed it; a name no result listed, a name two templates share and the former positional form fail with a
// message that says why, instead of sending the placeholder to the Service.
func TestTemplateNamesResolveOrFailClearly(t *testing.T) {
	byName := map[string]string{}
	learnTemplateNames(json.RawMessage(`{"templates": [
		{"template_id": "tpl_aaaaaaaaaa", "name": "Quote card"},
		{"template_id": "tpl_bbbbbbbbbb", "name": "Twin"},
		{"template_id": "tpl_cccccccccc", "name": "Twin"}]}`), byName)
	learnTemplateNames(json.RawMessage(`{"template": {"template_id": "tpl_aaaaaaaaaa", "name": "Quote card"}}`), byName)
	learnTemplateNames(json.RawMessage(`{"template": {"template_id": "tpl_dddddddddd", "name": "Twin"}}`), byName)

	got, err := resolveTemplateNames(`{"template_id": "$template:name=Quote card", "name": "Quote card"}`, byName)
	if err != nil || got != `{"template_id": "tpl_aaaaaaaaaa", "name": "Quote card"}` {
		t.Fatalf("a known name: %s, %v", got, err)
	}

	for args, want := range map[string]string{
		`{"template_id": "$template:name=Spring sale"}`: "no earlier result listed a template of that name",
		`{"template_id": "$template:name=Twin"}`:        "two templates have that name",
		`{"template_id": "$template:0"}`:                "no earlier result listed a template of that name",
	} {
		if _, err := resolveTemplateNames(args, byName); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v, want an error saying %q", args, err, want)
		}
	}
}
