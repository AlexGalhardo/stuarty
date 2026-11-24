package httpapi_test

import (
	"net/http"
	"strings"
	"testing"
)

// VV-396: the binder refuses a control character in any string; for the name of an asset rename it answers with
// the error of the rules, and for every other field it keeps its own answer.
func TestHiddenTextOnlyTheRenameNameTakesTheRulesError(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	hash := "sha256:" + strings.Repeat("0", 64)
	res := h.post("/api/v1/assets/"+hash+"/rename", `{"name":"Beep\u0007cat"}`)
	got := res.json(t)
	details, _ := got["details"].(map[string]any)

	if res.status != http.StatusBadRequest || got["code"] != "E_VALIDATION" || got["path"] != "name" ||
		details["reason"] != "name_invalid" || got["message"] != "the name has a control or invisible character" {
		t.Fatalf("rename = %d %s", res.status, res.body)
	}

	res = h.post("/api/v1/projects", `{"name":"Beep\u0007cat","description":"d","goal":"g"}`)
	got = res.json(t)

	if res.status != http.StatusBadRequest || got["code"] != "E_VALIDATION" || got["details"] != nil ||
		strings.Contains(string(res.body), "name_invalid") {
		t.Fatalf("project create = %d %s", res.status, res.body)
	}
}
