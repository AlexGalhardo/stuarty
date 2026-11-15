package lint

import (
	"encoding/json"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

type snapshotRule struct {
	Code     string   `json:"code"`
	Severity string   `json:"severity"`
	Phase    string   `json:"phase"`
	Depth    []string `json:"depth"`
	Blocking bool     `json:"blocking"`
	Key      string   `json:"key"`
}

// The committed lint-rules.json is the rule table of Catalog: adding or changing a
// rule without running task codegen fails here. Every code of SDD-003 section 3.6 is in
// the file exactly once.
func TestLintRulesSnapshot(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("../schema/lint-rules.json")
	if err != nil {
		t.Fatal(err)
	}

	var got []snapshotRule
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}

	catalog := Catalog()
	if len(got) != len(catalog) {
		t.Fatalf("lint-rules.json has %d rules, the table has %d (run task codegen)", len(got), len(catalog))
	}

	for i, r := range catalog {
		want := snapshotRule{
			Code: r.Code, Severity: r.Severity, Phase: r.Phase,
			Depth: strings.Split(strings.ReplaceAll(r.Run, " ", ""), ","), Blocking: Blocking(r.Code),
			Key: "lint.rule." + r.Code + ".title",
		}
		if got[i].Code != want.Code || got[i].Severity != want.Severity || got[i].Phase != want.Phase ||
			got[i].Blocking != want.Blocking || got[i].Key != want.Key || !slices.Equal(got[i].Depth, want.Depth) {
			t.Errorf("rule %s is stale in lint-rules.json: %+v, want %+v", r.Code, got[i], want)
		}
	}

	sdd, err := os.ReadFile("../../docs/sdds/003-agent-interfaces.md")
	if err != nil {
		t.Fatal(err)
	}

	inFile := map[string]int{}
	for _, r := range got {
		inFile[r.Code]++
	}

	rows := regexp.MustCompile(`(?m)^\| .(L_[A-Z_]+). \|`).FindAllStringSubmatch(string(sdd), -1)
	if len(rows) != len(got) {
		t.Errorf("SDD-003 section 3.6 lists %d codes, the file has %d", len(rows), len(got))
	}

	for _, m := range rows {
		if inFile[m[1]] != 1 {
			t.Errorf("%s is in lint-rules.json %d times, want once", m[1], inFile[m[1]])
		}
	}
}
