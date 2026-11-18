package command

import (
	"encoding/json"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

// fetchedTree is the document as a client fetches it: encoding/json of the typed
// document, every default written out.
func fetchedTree(t *testing.T, doc *timeline.Document) map[string]any {
	t.Helper()

	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}

	var out map[string]any
	if err := decodeNumbers(data, &out); err != nil {
		t.Fatal(err)
	}

	return out
}

// patchTree is the mechanical patch of SDD-012 section 5.4 on a fetched tree: it
// copies the after-values in and derives nothing.
func patchTree(t *testing.T, doc map[string]any, diff []Change) {
	t.Helper()

	fields := func(target map[string]any, ch map[string]Pair, id string) {
		for name, p := range ch {
			if !jsonEqual(target[name], p[0]) {
				t.Errorf("%s.%s: the local value %v is not the before-value %v", id, name, target[name], p[0])
			}

			if p[1] == nil {
				delete(target, name)
			} else {
				target[name] = p[1]
			}
		}
	}

	coll := map[string]string{"asset": "assets", "track": "tracks", "item": "items", "font": "fonts"}

	for _, c := range diff {
		switch c.Kind {
		case KindProjectUpdated:
			fields(doc, c.Changes, c.ID)
		case KindSettingsUpdated:
			fields(doc["settings"].(map[string]any), c.Changes, c.ID)
		default:
			scope, verb, _ := splitKind(c.Kind)
			m, _ := doc[coll[scope]].(map[string]any)

			if m == nil {
				m = map[string]any{}
				doc[coll[scope]] = m
			}

			switch verb {
			case "added":
				m[c.ID] = c.Object
			case "removed":
				delete(m, c.ID)
			default:
				fields(m[c.ID].(map[string]any), c.Changes, c.ID)
			}
		}
	}
}

func splitKind(kind string) (scope, verb string, ok bool) {
	for i := range kind {
		if kind[i] == '.' {
			return kind[:i], kind[i+1:], true
		}
	}

	return kind, "", false
}

// checkDiffPatchesFetchedDocument is the contract of SDD-002 section A.5: patching
// the fetched document before the edit with the diff gives the fetched document after it.
func checkDiffPatchesFetchedDocument(t *testing.T, before *timeline.Document, res Result) {
	t.Helper()

	got := fetchedTree(t, before)
	patchTree(t, got, res.Diff)

	if want := fetchedTree(t, res.Doc); !jsonEqual(normalizeEmpty(got), normalizeEmpty(want)) {
		t.Errorf("the diff does not patch the fetched document into the edited one:\n got %v\nwant %v", got, want)
	}
}

// normalizeEmpty makes a null collection and an empty one equal: the fetched
// document of an empty project may use either. The fonts map is optional, so an
// empty one is the same as none.
func normalizeEmpty(m map[string]any) map[string]any {
	for _, k := range collections {
		if v, ok := m[k]; ok && v == nil {
			m[k] = map[string]any{}
		}
	}

	if f, ok := m["fonts"].(map[string]any); ok && len(f) == 0 {
		delete(m, "fonts")
	}

	return m
}
