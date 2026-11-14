package command

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

// ConflictInfo names the first object of a batch that another actor changed in the
// meantime. Kind is the kind of the intervening change; the service adds the
// revision and the actor it took the change from (SDD-006 §7.3).
type ConflictInfo struct {
	Collection string `json:"collection"`
	ID         string `json:"id"`
	Kind       string `json:"kind"`
}

// Conflicts implements SDD-002 §A.7 step 2. It reports the first object that ops
// refer to by identifier in their arguments, in the order of the operations and of
// the arguments, that any of the intervening changes added, updated or removed.
// The unit is the object, not the property (ADR-0006).
func Conflicts(ops []Operation, intervening []Change) *ConflictInfo {
	changed := map[string]Change{}
	for _, ch := range intervening {
		coll, _, _ := strings.Cut(ch.Kind, ".")
		collection := map[string]string{"asset": "assets", "track": "tracks", "item": "items"}[coll]
		if collection == "" {
			continue
		}
		key := collection + " " + ch.ID
		if _, seen := changed[key]; !seen {
			changed[key] = ch
		}
	}
	for _, op := range ops {
		for _, id := range referencedIDs(op.Args) {
			collection := collectionOfPrefix(id)
			if ch, ok := changed[collection+" "+id]; ok {
				return &ConflictInfo{Collection: collection, ID: id, Kind: ch.Kind}
			}
		}
	}
	return nil
}

// referencedIDs lists the identifiers of objects found anywhere in an
// arguments value, in document order, without repeats.
func referencedIDs(args json.RawMessage) []string {
	if len(bytes.TrimSpace(args)) == 0 {
		return nil
	}
	var tree any
	if err := decodeNumbers(args, &tree); err != nil {
		return nil
	}
	var ids []string
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case string:
			if collectionOfPrefix(x) != "" && !slices.Contains(ids, x) && validObjectID(x) {
				ids = append(ids, x)
			}
		case []any:
			for _, e := range x {
				walk(e)
			}
		case map[string]any:
			for _, k := range sortedMapKeys(x) {
				walk(x[k])
			}
		}
	}
	walk(tree)
	return ids
}

func validObjectID(s string) bool {
	prefix, _, ok := strings.Cut(s, "_")
	return ok && timeline.ValidID(prefix, s)
}
