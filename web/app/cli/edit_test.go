package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/command"
)

func TestEditShorthandBuildsOneOperation(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.svc.revision = 7

	code := h.run("edit", "--project", "prj_a", "item.move", "item_id=itm_a", "start=2s",
		"with_items:=true", `item_ids:=["itm_a","itm_b"]`, "text=a:=b", "--json")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, h.stdout.String())
	}

	req := h.svc.editReq
	if len(req.Operations) != 1 || req.Operations[0].Op != "item.move" {
		t.Fatalf("operations = %+v", req.Operations)
	}

	var args map[string]any
	if err := json.Unmarshal(req.Operations[0].Args, &args); err != nil {
		t.Fatal(err)
	}

	ids, _ := args["item_ids"].([]any)
	if args["start"] != "2s" || args["with_items"] != true || len(ids) != 2 || args["text"] != "a:=b" {
		t.Fatalf("args = %v", args)
	}

	if req.BaseRevision != 7 || h.svc.getReq.ProjectID != "prj_a" {
		t.Fatalf("the base must be read from ProjectGet when absent: %+v", req)
	}

	oneJSONDocument(t, h.stdout.String())
}

func TestEditNumberLookingStringStaysAString(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.run("edit", "--project", "prj_a", "text.set", "text=123", "n:=123")

	var args map[string]json.RawMessage
	if err := json.Unmarshal(h.svc.editReq.Operations[0].Args, &args); err != nil {
		t.Fatal(err)
	}

	if string(args["text"]) != `"123"` || string(args["n"]) != "123" {
		t.Fatalf("args = %s", h.svc.editReq.Operations[0].Args)
	}
}

func TestEditRejectsBadShorthand(t *testing.T) {
	t.Parallel()

	for name, args := range map[string][]string{
		"duplicate key":    {"item.move", "start=1s", "start=2s"},
		"invalid json":     {"item.move", "ids:=[1,"},
		"no equals":        {"item.move", "start"},
		"empty key":        {"item.move", "=1s"},
		"ops and an op":    {"item.move", "--ops", "-"},
		"nothing to apply": {},
	} {
		h := newHarness(t)

		if code := h.run(append([]string{"edit", "--project", "prj_a", "--json"}, args...)...); code != ExitFailure {
			t.Errorf("%s: exit %d", name, code)
			continue
		}

		if doc := oneJSONDocument(t, h.stdout.String()); doc["code"] != command.CodeValidation {
			t.Errorf("%s: doc = %v", name, doc)
		}

		if h.svc.editReq.ProjectID != "" {
			t.Errorf("%s: the service must not be called", name)
		}
	}
}

func TestEditOpsFromStdin(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.rt.Stdin = strings.NewReader(`[{"op":"track.add","args":{"kind":"visual","name":"Video"}},{"op":"track.add"}]`)

	if code := h.run("edit", "--project", "prj_a", "--ops", "-", "--base", "3"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr.String())
	}

	req := h.svc.editReq
	if len(req.Operations) != 2 || req.BaseRevision != 3 || h.svc.getReq.ProjectID != "" {
		t.Fatalf("an explicit --base must not read the project: %+v", req)
	}
}

func TestEditOpsObjectAndFlagOverrides(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	file := filepath.Join(dir, "batch.json")

	body := `{"operations":[{"op":"track.add","args":{"kind":"visual"}}],"base_revision":9,"idempotency_key":"k1","dry_run":false}`
	if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	h := newHarness(t)

	if code := h.run("edit", "--project", "prj_a", "--ops", file); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr.String())
	}

	if r := h.svc.editReq; r.BaseRevision != 9 || r.IdempotencyKey != "k1" || r.DryRun || h.svc.getReq.ProjectID != "" {
		t.Fatalf("object members not used: %+v", r)
	}

	if code := h.run(
		"edit",
		"--project",
		"prj_a",
		"--ops",
		file,
		"--base",
		"4",
		"--key",
		"k2",
		"--dry-run",
	); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr.String())
	}

	if r := h.svc.editReq; r.BaseRevision != 4 || r.IdempotencyKey != "k2" || !r.DryRun {
		t.Fatalf("flags must override the object: %+v", r)
	}

	if !strings.HasPrefix(h.stdout.String(), "dry run, would commit as r5 (base r4) by local:cli: changes=0") {
		t.Fatalf("stdout = %q", h.stdout.String())
	}
}

func TestEditOpsRejectsMalformedBatches(t *testing.T) {
	t.Parallel()

	for name, in := range map[string]string{
		"not json":       "nope",
		"unknown member": `{"operations":[],"surprise":1}`,
		"bad array":      `[{"op":1}]`,
	} {
		h := newHarness(t)
		h.rt.Stdin = strings.NewReader(in)

		if code := h.run("edit", "--project", "prj_a", "--ops", "-", "--json"); code != ExitFailure {
			t.Errorf("%s: exit %d", name, code)
		}
	}
}

// VV-138: --base 0 is a real base on the shorthand form, not an absent one; the
// CLI sends it as given and the service decides (a stale base that touches
// nothing another actor changed is rebased, SDD-002 A.7).
func TestEditShorthandSendsAnExplicitBaseOfZero(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	if code := h.run(
		"edit", "--project", "prj_a", "--base", "0", "marker.add", "at=2s", "label=x", "--json",
	); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stdout.String())
	}

	if r := h.svc.editReq; r.BaseRevision != 0 || h.svc.getReq.ProjectID != "" {
		t.Fatalf("an explicit --base 0 must not be replaced by the current revision: %+v", r)
	}
}
